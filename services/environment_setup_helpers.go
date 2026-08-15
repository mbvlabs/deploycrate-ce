package services

import (
	"context"
	"database/sql"
	"deploycrate-ce/internal/storage"
	"deploycrate-ce/internal/validation"
	"deploycrate-ce/models"
	"encoding/json"
	"errors"
	"fmt"
	"maps"
	"net/url"
	"path"
	"sort"
	"strings"

	"github.com/google/uuid"
)

type preparedEnvironmentConfiguration struct {
	input     EnvironmentSetupInput
	processes []models.EnvironmentProcessInput
	goTargets []models.GoProcessTarget
	serverIDs []uuid.UUID
}

func prepareEnvironmentConfiguration(
	input EnvironmentSetupInput,
	runtime models.BuildpackRuntime,
) (preparedEnvironmentConfiguration, error) {
	input.HealthPath = strings.TrimSpace(input.HealthPath)
	processes := normalizedProcessFormation(input.Processes, input.ContainerPort, input.HealthPath)
	if runtime == models.BuildpackRuntimeGo {
		processes = deriveGoProcessCommands(processes)
	}
	input.Processes = processes
	if err := validateEnvironmentSetupInput(input, runtime); err != nil {
		return preparedEnvironmentConfiguration{}, err
	}
	return preparedEnvironmentConfiguration{
		input:     input,
		processes: processes,
		goTargets: goProcessTargetsFromProcesses(processes),
		serverIDs: normalizedEnvironmentServerIDs(input),
	}, nil
}

func environmentResourceSecretKeys(
	resources []preparedSetupResource,
) (map[string]struct{}, error) {
	keys := map[string]struct{}{"PORT": {}}
	for _, resource := range resources {
		for _, secret := range resource.secrets {
			if _, exists := keys[secret.Key]; exists {
				return nil, errors.Join(
					models.ErrDomainValidation,
					validation.ValidationErrors{{
						Field:   "resources",
						Code:    "duplicate",
						Message: "Resource-managed Environment secret keys must be unique",
					}},
				)
			}
			keys[secret.Key] = struct{}{}
		}
	}
	return keys, nil
}

func createEnvironmentStateRevision(
	ctx context.Context,
	executor storage.Executor,
	environmentID, changeID uuid.UUID,
	runtime models.BuildpackRuntime,
	goTargets []models.GoProcessTarget,
	processes []models.EnvironmentProcessInput,
	domain models.EnvironmentDomainEntity,
	resources []models.EnvironmentResourceState,
	secrets []models.EnvironmentSecretDescriptor,
) (models.EnvironmentStateRevisionEntity, error) {
	state := models.EnvironmentDesiredState{
		SchemaVersion: models.EnvironmentStateSchemaVersion,
		Runtime: models.EnvironmentRuntimeState{
			Runtime:       string(runtime),
			BPGOTargets:   goTargets,
			RestartPolicy: "unless-stopped",
		},
		Processes: processStatesFromInputs(processes),
		Domain: models.EnvironmentDomainState{
			ID:       domain.ID,
			Hostname: domain.Hostname,
			Primary:  true,
		},
		Resources: resources,
		Secrets:   secrets,
	}
	canonicalState, err := models.CanonicalEnvironmentDesiredState(state)
	if err != nil {
		return models.EnvironmentStateRevisionEntity{}, errors.Join(models.ErrDomainValidation, err)
	}
	revision, err := models.EnvironmentStateRevision.Create(
		ctx,
		executor,
		models.CreateEnvironmentStateRevisionData{
			State:         canonicalState,
			EnvironmentID: environmentID,
			ChangeID:      changeID,
		},
	)
	if err != nil {
		return models.EnvironmentStateRevisionEntity{}, err
	}
	if _, err := models.ChangeStateRevision.Create(
		ctx,
		executor,
		models.CreateChangeStateRevisionData{
			Role:                       "result",
			ChangeID:                   changeID,
			EnvironmentStateRevisionID: revision.ID,
		},
	); err != nil {
		return models.EnvironmentStateRevisionEntity{}, err
	}
	return revision, nil
}

func validateEnvironmentSetupInput(
	input EnvironmentSetupInput,
	runtime models.BuildpackRuntime,
) error {
	builder := validation.NewBuilder()
	if _, err := models.ValidateEnvironmentProcessFormation(
		normalizedProcessFormation(input.Processes, input.ContainerPort, input.HealthPath),
	); err != nil {
		return err
	}
	goTargets := goProcessTargetsFromProcesses(input.Processes)
	if runtime != models.BuildpackRuntimeGo && len(goTargets) > 0 {
		builder.Add("processes", "target", "Go targets require the Go runtime")
		return builder.Err()
	}
	if err := models.ValidateGoProcessTargets(goTargets); err != nil {
		builder.Add("processes", "target", err.Error())
	}
	if len(goTargets) > 0 {
		webHasTarget := false
		for _, process := range input.Processes {
			if process.Kind == models.EnvironmentProcessWeb && process.Target != nil {
				webHasTarget = true
				break
			}
		}
		if !webHasTarget {
			builder.Add(
				"processes",
				"target",
				"web must declare a Go target when any other process does",
			)
		}
	}
	if len(input.Resources) > 32 || len(input.Secrets) > 128 {
		builder.Add("resources", "max_items", "Environment setup exceeds the supported item count")
	}
	return builder.Err()
}

func normalizedProcessFormation(
	configured []models.EnvironmentProcessInput,
	containerPort int32,
	healthPath string,
) []models.EnvironmentProcessInput {
	if len(configured) == 0 {
		return []models.EnvironmentProcessInput{
			{
				Name:          models.EnvironmentProcessWeb,
				Kind:          models.EnvironmentProcessWeb,
				Arguments:     []string{},
				Replicas:      1,
				ContainerPort: &containerPort,
				HealthPath:    strings.TrimSpace(healthPath),
			},
		}
	}
	normalized, err := models.ValidateEnvironmentProcessFormation(configured)
	if err != nil {
		return configured
	}
	return normalized
}

const goTargetExecutableDirectory = "/layers/paketo-buildpacks_go-build/targets/bin"

func deriveGoProcessCommands(
	processes []models.EnvironmentProcessInput,
) []models.EnvironmentProcessInput {
	for index := range processes {
		process := &processes[index]
		if process.Target == nil {
			continue
		}
		switch process.Kind {
		case models.EnvironmentProcessWorker, models.EnvironmentProcessRelease:
			command := goTargetExecutableDirectory + "/" + path.Base(*process.Target)
			process.Command = &command
		}
	}
	return processes
}

func goProcessTargetsFromProcesses(
	processes []models.EnvironmentProcessInput,
) []models.GoProcessTarget {
	var web *models.GoProcessTarget
	workers := make([]models.GoProcessTarget, 0, len(processes))
	var release *models.GoProcessTarget
	for _, process := range processes {
		if process.Target == nil {
			continue
		}
		target := models.GoProcessTarget{Process: process.Name, Target: *process.Target}
		switch process.Kind {
		case models.EnvironmentProcessWeb:
			web = &target
		case models.EnvironmentProcessWorker:
			workers = append(workers, target)
		case models.EnvironmentProcessRelease:
			release = &target
		}
	}
	targets := make([]models.GoProcessTarget, 0, len(processes))
	if web != nil {
		targets = append(targets, *web)
	}
	targets = append(targets, workers...)
	if release != nil {
		targets = append(targets, *release)
	}
	return targets
}

func processStatesFromInputs(
	inputs []models.EnvironmentProcessInput,
) []models.EnvironmentProcessState {
	states := make([]models.EnvironmentProcessState, 0, len(inputs))
	for _, input := range inputs {
		state := models.EnvironmentProcessState{
			Name:       input.Name,
			Kind:       input.Kind,
			Command:    input.Command,
			Arguments:  input.Arguments,
			Replicas:   input.Replicas,
			HealthPath: input.HealthPath,
		}
		if input.ContainerPort != nil {
			state.ContainerPort = *input.ContainerPort
		}
		if input.TimeoutSeconds != nil {
			state.TimeoutSeconds = *input.TimeoutSeconds
		}
		states = append(states, state)
	}
	return states
}

func processInputsFromState(
	states []models.EnvironmentProcessState,
) []models.EnvironmentProcessInput {
	inputs := make([]models.EnvironmentProcessInput, 0, len(states))
	for _, state := range states {
		input := models.EnvironmentProcessInput{
			Name:       state.Name,
			Kind:       state.Kind,
			Command:    state.Command,
			Arguments:  state.Arguments,
			Replicas:   state.Replicas,
			HealthPath: state.HealthPath,
		}
		if state.Kind == models.EnvironmentProcessWeb {
			port := state.ContainerPort
			input.ContainerPort = &port
		}
		if state.Kind == models.EnvironmentProcessRelease {
			timeout := state.TimeoutSeconds
			input.TimeoutSeconds = &timeout
		}
		inputs = append(inputs, input)
	}
	return inputs
}

func (service *EnvironmentSetup) loadSource(
	ctx context.Context,
	applicationID, environmentID uuid.UUID,
) (environmentSetupSource, models.GitHubRepositoryEntity, models.GitHubInstallationEntity, error) {
	source, err := models.EnvironmentSource.SetupSource(
		ctx, service.db.Executor(), applicationID, environmentID,
	)
	if err != nil {
		return source, models.GitHubRepositoryEntity{}, models.GitHubInstallationEntity{}, err
	}
	source.SetupComplete, err = models.Environment.SetupComplete(
		ctx,
		service.db.Executor(),
		environmentID,
	)
	if err != nil {
		return source, models.GitHubRepositoryEntity{}, models.GitHubInstallationEntity{}, err
	}
	if source.Kind == "image" {
		return source, models.GitHubRepositoryEntity{}, models.GitHubInstallationEntity{}, nil
	}
	repository, err := models.GitHubRepository.Find(ctx, service.db.Executor(), source.RepositoryID)
	if err != nil {
		return source, repository, models.GitHubInstallationEntity{}, err
	}
	installation, err := models.GitHubInstallation.Find(
		ctx,
		service.db.Executor(),
		source.InstallationID,
	)
	return source, repository, installation, err
}

func normalizedEnvironmentServerIDs(input EnvironmentSetupInput) []uuid.UUID {
	serverIDs := make([]uuid.UUID, 0, len(input.ServerIDs)+1)
	seen := make(map[uuid.UUID]struct{}, len(input.ServerIDs)+1)
	for _, serverID := range input.ServerIDs {
		if serverID == uuid.Nil {
			continue
		}
		if _, exists := seen[serverID]; exists {
			continue
		}
		seen[serverID] = struct{}{}
		serverIDs = append(serverIDs, serverID)
	}
	if input.ServerID != uuid.Nil {
		if _, exists := seen[input.ServerID]; !exists {
			serverIDs = append(serverIDs, input.ServerID)
		}
	}
	return serverIDs
}

func (service *EnvironmentSetup) runtimePlacements(
	ctx context.Context,
	serverIDs []uuid.UUID,
) ([]preparedRuntimePlacement, uuid.UUID, error) {
	if len(serverIDs) == 0 {
		return nil, uuid.Nil, errors.Join(
			models.ErrDomainValidation,
			validation.ValidationErrors{
				{
					Field:   "serverIds",
					Code:    "required",
					Message: "select at least one runtime Server target",
				},
			},
		)
	}
	placements := make([]preparedRuntimePlacement, 0, len(serverIDs))
	var networkID uuid.UUID
	for _, serverID := range serverIDs {
		resolvedServerID, resolvedNetworkID, network, err := service.runtimePlacement(ctx, serverID)
		if err != nil {
			return nil, uuid.Nil, err
		}
		if networkID != uuid.Nil && networkID != resolvedNetworkID {
			return nil, uuid.Nil, errors.Join(
				models.ErrDomainValidation,
				errors.New(
					"selected runtime Server targets do not share the DeployCrate private network",
				),
			)
		}
		networkID = resolvedNetworkID
		placements = append(
			placements,
			preparedRuntimePlacement{serverID: resolvedServerID, network: network},
		)
	}
	return placements, networkID, nil
}

func (service *EnvironmentSetup) runtimePlacement(
	ctx context.Context,
	serverID uuid.UUID,
) (uuid.UUID, uuid.UUID, models.ServerNetworkEntity, error) {
	server, err := models.Server.Find(ctx, service.db.Executor(), serverID)
	if err != nil || server.ArchivedAt.Valid || !server.IsConfigured ||
		(server.Kind != "self_hosted" && server.Kind != "worker") {
		return uuid.Nil, uuid.Nil, models.ServerNetworkEntity{}, errors.Join(
			models.ErrDomainValidation,
			validation.ValidationErrors{
				{
					Field:   "serverIds",
					Code:    "unavailable",
					Message: "selected runtime Server is unavailable",
				},
			},
		)
	}
	capabilities, err := models.ParseServerCapabilities(server.Capabilities)
	if err != nil || !capabilities.Runtime {
		return uuid.Nil, uuid.Nil, models.ServerNetworkEntity{}, errors.Join(
			models.ErrDomainValidation,
			validation.ValidationErrors{
				{
					Field:   "serverIds",
					Code:    "capability",
					Message: "selected Server does not support application runtimes",
				},
			},
		)
	}
	row, err := models.Application.SystemRuntimePlacement(ctx, service.db.Executor())
	if err != nil {
		return uuid.Nil, uuid.Nil, models.ServerNetworkEntity{}, err
	}
	network, err := models.ServerNetwork.ActiveForServerNetwork(
		ctx, service.db.Executor(), serverID, row.NetworkID,
	)
	if err != nil {
		return uuid.Nil, uuid.Nil, models.ServerNetworkEntity{}, errors.Join(
			models.ErrDomainValidation,
			validation.ValidationErrors{
				{
					Field:   "serverIds",
					Code:    "network",
					Message: "selected Server is not attached to the DeployCrate private network",
				},
			},
		)
	}
	return serverID, row.NetworkID, network, nil
}

func (service *EnvironmentSetup) prepareResources(
	ctx context.Context,
	environmentID uuid.UUID,
	serverIDs []uuid.UUID,
	networkID uuid.UUID,
	inputs []EnvironmentSetupResourceInput,
) ([]preparedSetupResource, error) {
	prepared := make([]preparedSetupResource, 0, len(inputs))
	connectionConfigurations := make(map[uuid.UUID]environmentResourceConfiguration)
	if environmentID != uuid.Nil {
		connections, err := models.EnvironmentResource.ActiveForEnvironment(
			ctx, service.db.Executor(), environmentID,
		)
		if err != nil {
			return nil, err
		}
		for _, connection := range connections {
			configuration, err := parseEnvironmentResourceConfiguration(connection.Configuration)
			if err != nil {
				return nil, err
			}
			connectionConfigurations[connection.ResourceID] = configuration
		}
	}
	aliases := make(map[string]struct{}, len(inputs))
	resources := make(map[uuid.UUID]struct{}, len(inputs))
	selectedServers := make(map[uuid.UUID]struct{}, len(serverIDs))
	for _, serverID := range serverIDs {
		selectedServers[serverID] = struct{}{}
	}
	for index, input := range inputs {
		if _, exists := resources[input.ResourceID]; exists {
			return nil, errors.Join(
				models.ErrDomainValidation,
				validation.ValidationErrors{
					{
						Field:   fmt.Sprintf("resources.%d.resourceId", index),
						Code:    "duplicate",
						Message: "Resource is already selected",
					},
				},
			)
		}
		resource, err := models.Resource.Find(ctx, service.db.Executor(), input.ResourceID)
		_, supported := models.FindResourceEngine(resource.Engine())
		if err != nil || resource.ArchivedAt.Valid || !supported {
			return nil, errors.Join(
				models.ErrDomainValidation,
				errors.New("selected Resource is unavailable"),
			)
		}
		if !resource.EnvironmentAttachable {
			return nil, errors.Join(
				models.ErrDomainValidation,
				validation.ValidationErrors{
					{
						Field:   fmt.Sprintf("resources.%d.resourceId", index),
						Code:    "not_attachable",
						Message: "selected Resource cannot be attached to an Environment",
					},
				},
			)
		}
		input.Alias = strings.ToUpper(strings.TrimSpace(input.Alias))
		input.CredentialProjection = strings.ToLower(strings.TrimSpace(input.CredentialProjection))
		if input.Alias == "" {
			input.Alias = strings.ToUpper(resource.Engine())
		}
		if input.CredentialProjection != resourceCredentialProjectionConnectionURL &&
			input.CredentialProjection != resourceCredentialProjectionIndividualParts {
			return nil, errors.Join(
				models.ErrDomainValidation,
				validation.ValidationErrors{
					{
						Field:   fmt.Sprintf("resources.%d.credentialProjection", index),
						Code:    "unsupported",
						Message: "Choose Connection URL or Individual parts",
					},
				},
			)
		}
		if input.CredentialProjection == resourceCredentialProjectionConnectionURL &&
			resource.Engine() != "postgresql" {
			return nil, errors.Join(
				models.ErrDomainValidation,
				validation.ValidationErrors{
					{
						Field:   fmt.Sprintf("resources.%d.credentialProjection", index),
						Code:    "unsupported",
						Message: "Connection URL is not supported for this Resource",
					},
				},
			)
		}
		if _, exists := aliases[input.Alias]; exists {
			return nil, errors.Join(
				models.ErrDomainValidation,
				validation.ValidationErrors{
					{
						Field:   fmt.Sprintf("resources.%d.alias", index),
						Code:    "duplicate",
						Message: "Resource alias is already selected",
					},
				},
			)
		}
		endpoint, err := models.ResourceEndpoint.Find(ctx, service.db.Executor(), input.EndpointID)
		if err != nil || endpoint.ArchivedAt.Valid || endpoint.ResourceID != resource.ID ||
			(endpoint.PrivateNetworkID != nil && *endpoint.PrivateNetworkID != networkID) {
			return nil, errors.Join(
				models.ErrDomainValidation,
				errors.New("selected Resource endpoint is unavailable from the Environment target"),
			)
		}
		installation, installationErr := models.ResourceInstallation.FindActiveForResource(
			ctx, service.db.Executor(), resource.ID,
		)
		if installationErr == nil {
			if _, selected := selectedServers[installation.ServerID]; !selected {
				return nil, errors.Join(
					models.ErrDomainValidation,
					validation.ValidationErrors{
						{
							Field:   fmt.Sprintf("resources.%d.resourceId", index),
							Code:    "placement",
							Message: "selected managed Resource is not installed on a selected runtime Server target",
						},
					},
				)
			}
		} else if !errors.Is(installationErr, sql.ErrNoRows) {
			return nil, installationErr
		}
		if input.CredentialID == nil && resource.Engine() == "postgresql" {
			return nil, errors.Join(
				models.ErrDomainValidation,
				errors.New("PostgreSQL application credential is required"),
			)
		}
		connectionID := uuid.New()
		var credential *models.ResourceCredentialEntity
		credentialValues := make(map[string]string)
		if input.CredentialID != nil {
			selectedCredential, findErr := models.ResourceCredential.Find(
				ctx,
				service.db.Executor(),
				*input.CredentialID,
			)
			if findErr != nil || selectedCredential.ArchivedAt.Valid ||
				selectedCredential.ResourceID != resource.ID {
				return nil, errors.Join(
					models.ErrDomainValidation,
					errors.New("selected Resource credential is unavailable"),
				)
			}
			if resourceCredentialMetadataPurpose(selectedCredential.Metadata) != "application" {
				return nil, errors.Join(
					models.ErrDomainValidation,
					errors.New(
						"Resource administrator credentials cannot be injected into an Environment",
					),
				)
			}
			if resource.Engine() == "opentelemetry" &&
				resourceCredentialMetadataEnvironmentID(
					selectedCredential.Metadata,
				) != environmentID {
				return nil, errors.Join(
					models.ErrDomainValidation,
					errors.New("selected OpenTelemetry credential belongs to another Environment"),
				)
			}
			input.Database = resourceCredentialMetadataDatabase(selectedCredential.Metadata)
			if input.Database != "" && !resourceHasDatabase(resource, input.Database) {
				return nil, errors.Join(
					models.ErrDomainValidation,
					errors.New("selected Resource Database is unavailable"),
				)
			}
			credentialValues, err = service.resources.credentialSecretValues(selectedCredential)
			if err != nil {
				return nil, err
			}
			credential = &selectedCredential
		}
		overrides := make(map[string]string)
		effectiveKeys := resource.EnvironmentKeys()
		if configuration, exists := connectionConfigurations[resource.ID]; exists {
			overrides = maps.Clone(configuration.EnvironmentKeyOverrides)
			effectiveKeys = connectionEnvironmentKeys(resource, configuration)
		}
		identityToken := credentialValues["token"]
		var credentialInput *ResourceCredentialInput
		if resource.Engine() == "opentelemetry" {
			if identityToken == "" {
				existingCredential, findCredentialErr := models.ResourceCredential.ActiveApplicationForEnvironment(
					ctx, service.db.Executor(), resource.ID, environmentID,
				)
				if findCredentialErr == nil {
					credentialValues, err = service.resources.credentialSecretValues(
						existingCredential,
					)
					if err != nil {
						return nil, err
					}
					identityToken = credentialValues["token"]
					credential = &existingCredential
					input.CredentialID = &existingCredential.ID
				} else if !errors.Is(findCredentialErr, sql.ErrNoRows) {
					return nil, findCredentialErr
				}
			}
			if identityToken == "" {
				identityToken, err = service.telemetry.EnvironmentToken(environmentID)
				if err != nil {
					return nil, err
				}
				metadata, metadataErr := json.Marshal(map[string]string{
					"purpose": "application", "environment_id": environmentID.String(),
				})
				if metadataErr != nil {
					return nil, metadataErr
				}
				credentialInput = &ResourceCredentialInput{
					Name: "Environment " + environmentID.String(), Metadata: metadata,
					SecretValues: map[string]string{"token": identityToken},
				}
			}
		}
		values, environmentKeys, projectionErr := resourceProjectionValues(
			resource,
			endpoint,
			credential,
			credentialValues,
			input.CredentialProjection,
			effectiveKeys,
			identityToken,
		)
		if projectionErr != nil {
			return nil, projectionErr
		}
		secrets := make([]PreparedEnvironmentSecret, 0, len(values))
		for key, value := range values {
			secret, prepareErr := service.secrets.Prepare(
				environmentID,
				key,
				value,
				models.EnvironmentSecretSourceResource,
				connectionID,
			)
			if prepareErr != nil {
				return nil, prepareErr
			}
			secrets = append(secrets, secret)
		}
		prepared = append(prepared, preparedSetupResource{
			input:                   input,
			connectionID:            connectionID,
			resource:                resource,
			endpoint:                endpoint,
			credential:              credential,
			credentialInput:         credentialInput,
			environmentKeys:         environmentKeys,
			environmentKeyOverrides: overrides,
			secrets:                 secrets,
		})
		aliases[input.Alias] = struct{}{}
		resources[input.ResourceID] = struct{}{}
	}
	return prepared, nil
}

func preparedCredentialSource(resource preparedSetupResource) string {
	if resource.resource.Engine() == "opentelemetry" {
		return "platform"
	}
	return ""
}

func resourceProjectionValues(
	resource models.ResourceEntity,
	endpoint models.ResourceEndpointEntity,
	credential *models.ResourceCredentialEntity,
	credentialValues map[string]string,
	projection string,
	resourceKeys map[string]string,
	identityToken string,
) (map[string]string, map[string]string, error) {
	definition, supported := models.FindResourceEngine(resource.Engine())
	if !supported {
		return nil, nil, errors.New("Resource engine is unsupported")
	}
	projectedKeys := make(map[string]string)
	valueFor := func(logicalName, value string, values map[string]string) error {
		key := resourceKeys[logicalName]
		if key == "" {
			return errors.New("Resource Environment key mapping is incomplete")
		}
		projectedKeys[logicalName] = key
		if value != "" {
			values[key] = value
		}
		return nil
	}
	values := make(map[string]string)
	if resource.Engine() == "opentelemetry" {
		settings := endpoint.ParsedSettings()
		if projection != resourceCredentialProjectionIndividualParts ||
			!endpoint.IsEnvironmentEndpoint() ||
			settings.Transport != models.ResourceEndpointTransportOTLPHTTP ||
			settings.Authentication != models.ResourceEndpointAuthSignedIdentity ||
			strings.TrimSpace(identityToken) == "" {
			return nil, nil, errors.New(
				"selected OpenTelemetry Resource endpoint cannot be projected into an Environment",
			)
		}
		for logicalName, value := range map[string]string{
			"endpoint": endpoint.URL(), "protocol": settings.Transport,
			"headers": "Authorization=Bearer " + identityToken,
		} {
			if err := valueFor(logicalName, value, values); err != nil {
				return nil, nil, err
			}
		}
		return values, projectedKeys, nil
	}
	database := ""
	if credential != nil {
		database = resourceCredentialMetadataDatabase(credential.Metadata)
	}
	if projection == resourceCredentialProjectionConnectionURL {
		password := credentialValues["password"]
		if resource.Engine() != "postgresql" || credential == nil || password == "" ||
			!credential.Username.Valid ||
			database == "" {
			return nil, nil, errors.New("selected PostgreSQL application credential is incomplete")
		}
		uri := &url.URL{
			Scheme: "postgresql",
			Host:   fmt.Sprintf("%s:%d", endpoint.Address, endpoint.Port),
			Path:   "/" + database,
			User:   url.UserPassword(credential.Username.String, password),
		}
		query := uri.Query()
		query.Set("sslmode", endpoint.TLSMode)
		uri.RawQuery = query.Encode()
		if err := valueFor("url", uri.String(), values); err != nil {
			return nil, nil, err
		}
		return values, projectedKeys, nil
	}
	if projection != resourceCredentialProjectionIndividualParts {
		return nil, nil, errors.New("Resource credential projection is unsupported")
	}
	for logicalName, value := range map[string]string{
		"host": endpoint.Address, "port": fmt.Sprint(endpoint.Port),
		"protocol": endpoint.Protocol, "tls_mode": endpoint.TLSMode,
	} {
		if err := valueFor(logicalName, value, values); err != nil {
			return nil, nil, err
		}
	}
	if definition.ResourceType == models.ResourceTypeDatabase && database != "" {
		if err := valueFor("database", database, values); err != nil {
			return nil, nil, err
		}
	}
	if credential != nil {
		username := ""
		if credential.Username.Valid {
			username = credential.Username.String
		}
		if err := valueFor("username", username, values); err != nil {
			return nil, nil, err
		}
		for _, field := range definition.CredentialFields {
			if err := valueFor(field.Name, credentialValues[field.Name], values); err != nil {
				return nil, nil, err
			}
		}
	}
	return values, projectedKeys, nil
}

func lockPreparedSetupResources(
	ctx context.Context,
	db storage.Executor,
	prepared []preparedSetupResource,
) error {
	ordered := append([]preparedSetupResource(nil), prepared...)
	sort.Slice(ordered, func(left, right int) bool {
		return ordered[left].resource.ID.String() < ordered[right].resource.ID.String()
	})
	for _, item := range ordered {
		resource, err := models.Resource.LockActive(ctx, db, item.resource.ID)
		if err != nil {
			return errors.New("selected Resource changed while the Environment was being prepared")
		}
		if !resource.UpdatedAt.Equal(item.resource.UpdatedAt) {
			return errors.Join(
				models.ErrDomainValidation,
				validation.ValidationErrors{
					{
						Field:   "resources",
						Code:    "stale",
						Message: "a selected Resource changed; review the attachment and try again",
					},
				},
			)
		}
		endpoint, err := models.ResourceEndpoint.Find(ctx, db, item.endpoint.ID)
		if err != nil || !endpoint.UpdatedAt.Equal(item.endpoint.UpdatedAt) ||
			endpoint.ArchivedAt.Valid {
			return errors.Join(
				models.ErrDomainValidation,
				validation.ValidationErrors{
					{
						Field:   "resources",
						Code:    "stale",
						Message: "a selected Resource endpoint changed; review the attachment and try again",
					},
				},
			)
		}
		if item.credential != nil {
			credential, err := models.ResourceCredential.Find(ctx, db, item.credential.ID)
			if err != nil || !credential.UpdatedAt.Equal(item.credential.UpdatedAt) ||
				credential.ArchivedAt.Valid {
				return errors.Join(
					models.ErrDomainValidation,
					validation.ValidationErrors{
						{
							Field:   "resources",
							Code:    "stale",
							Message: "a selected Resource credential changed; review the attachment and try again",
						},
					},
				)
			}
		}
	}
	return nil
}
