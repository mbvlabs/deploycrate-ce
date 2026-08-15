package services

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"maps"
	"slices"
	"strings"
	"time"

	containerclient "deploycrate-ce/clients/container"
	postgresqlclient "deploycrate-ce/clients/postgresql"
	"deploycrate-ce/config"
	"deploycrate-ce/internal/storage"
	"deploycrate-ce/internal/validation"
	"deploycrate-ce/models"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgconn"
)

const resourceCredentialPurpose = "resource-credential/v1"

var ErrSystemResourceImmutable = errors.New("DeployCrate system Resources cannot be modified")

type ResourceManagement struct {
	db        storage.Pool
	config    config.Config
	container ResourceContainerService
	postgres  postgresqlclient.Client
	secrets   *EnvironmentSecrets
}

type ResourceContainerService interface {
	Run(context.Context, uuid.UUID, models.ServerCapability, containerclient.RunSpec) error
	Inspect(
		context.Context,
		uuid.UUID,
		models.ServerCapability,
		string,
		string,
	) (containerclient.State, error)
	Logs(context.Context, uuid.UUID, models.ServerCapability, string, string, int) (string, error)
	Start(context.Context, uuid.UUID, models.ServerCapability, string, string) error
	Stop(context.Context, uuid.UUID, models.ServerCapability, string, string) error
	Restart(context.Context, uuid.UUID, models.ServerCapability, string, string) error
	Remove(context.Context, uuid.UUID, models.ServerCapability, string, string) error
	RemoveVolume(context.Context, uuid.UUID, models.ServerCapability, string) error
}

func NewResourceManagement(
	db storage.Pool,
	cfg config.Config,
	secrets *EnvironmentSecrets,
	container ResourceContainerService,
) *ResourceManagement {
	return &ResourceManagement{
		db:        db,
		config:    cfg,
		container: container,
		postgres:  postgresqlclient.New(),
		secrets:   secrets,
	}
}

type ResourceInput struct {
	Name          string
	Slug          string
	ResourceType  models.ResourceTypeEnum
	Configuration json.RawMessage
}

type CreateResourceInput struct {
	Resource         ResourceInput
	Endpoint         *ResourceEndpointInput
	Credential       *ResourceCredentialInput
	Installation     *ResourceInstallationInput
	Volume           *ResourceVolumeInput
	Mount            *ResourceMountInput
	HealthCheck      *ResourceHealthCheckInput
	PrivateNetworkID *uuid.UUID
}

type ResourceEndpointInput struct {
	Name             string
	Role             string
	Address          string
	Port             int32
	Protocol         string
	TLSMode          string
	Settings         json.RawMessage
	PrivateNetworkID *uuid.UUID
}

type ResourceCredentialInput struct {
	Name         string
	Username     string
	Metadata     json.RawMessage
	SecretValues map[string]string
}

type CreateResourceDatabaseInput struct {
	Database     models.ResourceDatabaseDefinition
	CredentialID *uuid.UUID
	Credential   *ResourceCredentialInput
}

func resourceCredentialMetadataPurpose(metadata json.RawMessage) string {
	var value struct {
		Purpose string `json:"purpose"`
	}
	_ = json.Unmarshal(metadata, &value)
	return strings.ToLower(strings.TrimSpace(value.Purpose))
}

func resourceCredentialMetadataDatabase(metadata json.RawMessage) string {
	var value struct {
		Database string `json:"database"`
	}
	_ = json.Unmarshal(metadata, &value)
	return strings.TrimSpace(value.Database)
}

func resourceCredentialMetadataForDatabase(
	metadata json.RawMessage,
	database string,
) (json.RawMessage, error) {
	values := make(map[string]any)
	if len(metadata) > 0 && json.Unmarshal(metadata, &values) != nil {
		return nil, domainError(
			"credential.metadata",
			"invalid",
			"credential metadata must be valid JSON",
		)
	}
	if values == nil {
		values = make(map[string]any)
	}
	values["purpose"] = "application"
	values["database"] = strings.TrimSpace(database)
	return json.Marshal(values)
}

func resourceCredentialMetadataEnvironmentID(metadata json.RawMessage) uuid.UUID {
	var value struct {
		EnvironmentID string `json:"environment_id"`
	}
	_ = json.Unmarshal(metadata, &value)
	environmentID, _ := uuid.Parse(strings.TrimSpace(value.EnvironmentID))
	return environmentID
}

func resourceHasDatabase(resource models.ResourceEntity, name string) bool {
	for _, database := range resource.Databases() {
		if strings.EqualFold(strings.TrimSpace(database.Name), strings.TrimSpace(name)) {
			return true
		}
	}
	return false
}

type ResourceInstallationInput struct {
	ImageReference       string
	ImageDigest          string
	ContainerName        string
	RestartPolicy        string
	Configuration        json.RawMessage
	PortMappings         *[]models.ResourceInstallationPortMapping
	ServerID             uuid.UUID
	RegistryCredentialID *uuid.UUID
}

type ResourceVolumeInput struct {
	Name          string
	Driver        string
	Configuration json.RawMessage
	ServerID      uuid.UUID
}

type ResourceMountInput struct {
	MountPath              string
	ReadOnly               bool
	ResourceVolumeID       uuid.UUID
	ResourceInstallationID uuid.UUID
}

type ResourceHealthCheckInput struct {
	Name                 string
	Kind                 string
	Configuration        json.RawMessage
	IntervalSeconds      int32
	TimeoutSeconds       int32
	FailureThreshold     int32
	SuccessThreshold     int32
	Enabled              bool
	ResourceEndpointID   *uuid.UUID
	ResourceCredentialID *uuid.UUID
}

func defaultResourceHealthCheckInput(
	resource models.ResourceEntity,
	endpoint models.ResourceEndpointEntity,
	credential *models.ResourceCredentialEntity,
) *ResourceHealthCheckInput {
	if resource.Engine() != "postgresql" && resource.Engine() != "clickhouse" {
		return nil
	}
	name := "PostgreSQL readiness"
	if resource.Engine() == "clickhouse" {
		name = "ClickHouse readiness"
	}
	input := &ResourceHealthCheckInput{
		Name: name, Kind: resource.Engine(), Configuration: json.RawMessage(`{}`),
		IntervalSeconds: 15, TimeoutSeconds: 3, FailureThreshold: 3, SuccessThreshold: 1,
		Enabled: true, ResourceEndpointID: &endpoint.ID,
	}
	if credential != nil {
		input.ResourceCredentialID = &credential.ID
	}
	return input
}

func (service *ResourceManagement) List(
	ctx context.Context,
	filters models.ResourceListFilters,
) ([]models.ResourceListItem, error) {
	return models.Resource.ListCatalog(ctx, service.db.Executor(), filters)
}

func (service *ResourceManagement) Options(
	ctx context.Context,
) (models.ResourceFormOptions, error) {
	engines := make([]models.ResourceEngineDefinition, 0)
	for _, definition := range models.ResourceEngineCatalog() {
		if definition.Engine != "mysql" && definition.Engine != "registry" &&
			definition.Engine != "opentelemetry" {
			engines = append(engines, definition)
		}
	}
	options, err := models.Resource.FormOptions(ctx, service.db.Executor())
	if err != nil {
		return models.ResourceFormOptions{}, err
	}
	options.Engines = engines
	options.ResourceTypes = []models.ResourceTypeEnum{
		models.ResourceTypeDatabase,
		models.ResourceTypeCache,
		models.ResourceTypeService,
	}
	return options, nil
}

func (service *ResourceManagement) OptionsForEngine(
	ctx context.Context,
	engine string,
) (models.ResourceFormOptions, error) {
	options, err := service.Options(ctx)
	if err != nil {
		return models.ResourceFormOptions{}, err
	}
	engine = strings.ToLower(strings.TrimSpace(engine))
	if slices.ContainsFunc(options.Engines, func(definition models.ResourceEngineDefinition) bool {
		return definition.Engine == engine
	}) {
		return options, nil
	}
	if definition, ok := models.FindResourceEngine(engine); ok {
		options.Engines = append(options.Engines, definition)
	}
	return options, nil
}

func (service *ResourceManagement) Details(
	ctx context.Context,
	resourceID uuid.UUID,
) (models.ResourceDetails, error) {
	resource, err := service.loadResource(ctx, service.db.Executor(), resourceID, false)
	if err != nil {
		return models.ResourceDetails{}, err
	}
	if resource.SystemManaged {
		return models.ResourceDetails{}, models.ErrNotFound
	}
	detail, err := models.Resource.DetailCatalog(ctx, service.db.Executor(), resource)
	if err != nil {
		return models.ResourceDetails{}, err
	}
	detail.Databases = resource.Databases()
	for index := range detail.Connections {
		configuration, configurationErr := parseEnvironmentResourceConfiguration(
			detail.Connections[index].Configuration,
		)
		if configurationErr != nil {
			return models.ResourceDetails{}, configurationErr
		}
		detail.Connections[index].EnvironmentKeyOverrides = maps.Clone(
			configuration.EnvironmentKeyOverrides,
		)
		detail.Connections[index].EnvironmentKeys = connectionEnvironmentKeys(
			resource,
			configuration,
		)
	}
	for index := range detail.Installations {
		service.observeInstallation(ctx, &detail.Installations[index])
	}
	return detail, nil
}

func (service *ResourceManagement) CreateResource(
	ctx context.Context,
	input CreateResourceInput,
) (models.ResourceEntity, error) {
	var requestedConfiguration models.ResourceConfiguration
	_ = json.Unmarshal(input.Resource.Configuration, &requestedConfiguration)
	if strings.EqualFold(strings.TrimSpace(requestedConfiguration.Engine), "opentelemetry") {
		return models.ResourceEntity{}, domainError(
			"resource.configuration.engine",
			"system_managed",
			"OpenTelemetry is managed by the DeployCrate telemetry Resource",
		)
	}
	if input.Installation == nil {
		return models.ResourceEntity{}, domainError(
			"installation",
			"required",
			"Resources require a Docker installation",
		)
	}
	if input.Resource.ResourceType == models.ResourceTypeDatabase && input.Credential == nil {
		return models.ResourceEntity{}, domainError(
			"credential",
			"administrator",
			"database Resources require an administrator credential",
		)
	}
	if input.Resource.ResourceType == models.ResourceTypeDatabase &&
		resourceCredentialMetadataPurpose(input.Credential.Metadata) != "administrator" {
		return models.ResourceEntity{}, domainError(
			"credential.metadata.purpose",
			"administrator",
			"the initial database credential must be the administrator",
		)
	}
	tx, err := service.db.BeginTx(ctx, nil)
	if err != nil {
		return models.ResourceEntity{}, err
	}
	defer tx.Rollback()
	resource, err := models.Resource.Create(ctx, tx, models.CreateResourceData{
		Name:                  input.Resource.Name,
		Slug:                  input.Resource.Slug,
		ResourceType:          input.Resource.ResourceType,
		Configuration:         normalizedJSON(input.Resource.Configuration),
		SystemManaged:         false,
		EnvironmentAttachable: true,
	})
	if err != nil {
		return models.ResourceEntity{}, mapResourceConflict(err)
	}
	var installation *models.ResourceInstallationEntity
	if input.Installation != nil {
		created, createErr := service.createInstallation(ctx, tx, resource, *input.Installation)
		if createErr != nil {
			return models.ResourceEntity{}, prefixResourceValidation(createErr, "installation")
		}
		installation = &created
	}
	var endpoint *models.ResourceEndpointEntity
	if installation != nil {
		created, createErr := service.createManagedPrimaryEndpoint(ctx, tx, resource, *installation)
		if createErr != nil {
			return models.ResourceEntity{}, prefixResourceValidation(createErr, "endpoint")
		}
		endpoint = &created
	}
	if input.PrivateNetworkID != nil {
		if _, err := createManagedResourcePrivateEndpoint(
			ctx,
			tx,
			resource.ID,
			*input.PrivateNetworkID,
		); err != nil {
			return models.ResourceEntity{}, err
		}
	}
	var volume *models.ResourceVolumeEntity
	if input.Volume != nil {
		created, createErr := service.createVolume(ctx, tx, resource, *input.Volume)
		if createErr != nil {
			return models.ResourceEntity{}, prefixResourceValidation(createErr, "volume")
		}
		volume = &created
	}
	if input.Mount != nil {
		if installation == nil || volume == nil {
			return models.ResourceEntity{}, domainError(
				"mount",
				"topology",
				"an initial mount requires both an installation and volume",
			)
		}
		mountInput := *input.Mount
		mountInput.ResourceInstallationID = installation.ID
		mountInput.ResourceVolumeID = volume.ID
		if _, err := service.createMount(ctx, tx, resource, mountInput); err != nil {
			return models.ResourceEntity{}, prefixResourceValidation(err, "mount")
		}
	}
	if input.Endpoint != nil {
		endpointInput := *input.Endpoint
		created, createErr := service.createEndpoint(ctx, tx, resource, endpointInput)
		if createErr != nil {
			return models.ResourceEntity{}, prefixResourceValidation(createErr, "endpoint")
		}
		endpoint = &created
	}
	var credential *models.ResourceCredentialEntity
	if input.Credential != nil {
		credentialInput := *input.Credential
		created, createErr := service.createCredential(ctx, tx, resource, credentialInput)
		if createErr != nil {
			return models.ResourceEntity{}, prefixResourceValidation(createErr, "credential")
		}
		credential = &created
	}
	healthInput := input.HealthCheck
	if healthInput == nil && installation != nil && endpoint != nil {
		healthInput = defaultResourceHealthCheckInput(resource, *endpoint, credential)
	}
	if healthInput != nil {
		candidate := *healthInput
		if endpoint != nil {
			candidate.ResourceEndpointID = &endpoint.ID
		}
		if credential != nil {
			candidate.ResourceCredentialID = &credential.ID
		}
		if _, err := service.createHealthCheck(ctx, tx, resource, candidate); err != nil {
			return models.ResourceEntity{}, prefixResourceValidation(err, "healthCheck")
		}
	}
	if err := tx.Commit(); err != nil {
		return models.ResourceEntity{}, mapResourceConflict(err)
	}
	return resource, nil
}

func (service *ResourceManagement) UpdateResource(
	ctx context.Context,
	resourceID uuid.UUID,
	input ResourceInput,
) (models.ResourceEntity, error) {
	tx, err := service.db.BeginTx(ctx, nil)
	if err != nil {
		return models.ResourceEntity{}, err
	}
	defer tx.Rollback()
	if err := service.requireNoActiveRestore(ctx, tx, resourceID, nil); err != nil {
		return models.ResourceEntity{}, err
	}
	resource, err := service.loadResource(ctx, tx, resourceID, true)
	if err != nil {
		return models.ResourceEntity{}, err
	}
	if err := service.validateResourceTransition(ctx, tx, resource, input); err != nil {
		return models.ResourceEntity{}, err
	}
	updated, err := models.Resource.Update(ctx, tx, models.UpdateResourceData{
		ID:                    resource.ID,
		Name:                  input.Name,
		Slug:                  input.Slug,
		ResourceType:          input.ResourceType,
		Configuration:         normalizedJSON(input.Configuration),
		SystemManaged:         resource.SystemManaged,
		EnvironmentAttachable: resource.EnvironmentAttachable,
		ArchivedAt:            resource.ArchivedAt,
	})
	if err != nil {
		return models.ResourceEntity{}, mapResourceConflict(err)
	}
	connections, err := models.EnvironmentResource.ActiveForResourceID(ctx, tx, resource.ID)
	if err != nil {
		return models.ResourceEntity{}, err
	}
	if err := service.reconcileEnvironmentResourceConnections(
		ctx,
		tx,
		updated,
		connections,
	); err != nil {
		return models.ResourceEntity{}, err
	}
	if err := tx.Commit(); err != nil {
		return models.ResourceEntity{}, mapResourceConflict(err)
	}
	return updated, nil
}

func (service *ResourceManagement) ArchiveResource(
	ctx context.Context,
	resourceID uuid.UUID,
) (err error) {
	tx, err := service.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if err := service.requireNoActiveRestore(ctx, tx, resourceID, nil); err != nil {
		return err
	}
	resource, err := service.loadResource(ctx, tx, resourceID, true)
	if err != nil {
		return err
	}
	dependencies, err := models.Resource.ArchiveDependencies(ctx, tx, resourceID)
	if err != nil {
		return err
	}
	if dependencies.BindingCount > 0 {
		return domainError(
			"resource",
			"dependency",
			"archive active Environment bindings before archiving this Resource",
		)
	}
	if dependencies.PrivateAccessCount > 0 {
		return domainError(
			"resource",
			"private_access",
			"revoke active WireGuard device grants before archiving this Resource",
		)
	}
	installations := dependencies.Installations
	volumes := dependencies.Volumes
	capability := managedResourceCapability(resource.Engine())
	existing := make([]models.ResourceInstallationEntity, 0, len(installations))
	running := make([]models.ResourceInstallationEntity, 0, len(installations))
	for _, installation := range installations {
		state, inspectErr := service.container.Inspect(
			ctx,
			installation.ServerID,
			capability,
			installation.ID.String(),
			installation.ContainerName,
		)
		if inspectErr != nil {
			return inspectErr
		}
		if !state.Exists {
			continue
		}
		existing = append(existing, installation)
		if !state.Running {
			continue
		}
		if stopErr := service.container.Stop(
			ctx,
			installation.ServerID,
			capability,
			installation.ID.String(),
			installation.ContainerName,
		); stopErr != nil {
			for index := len(running) - 1; index >= 0; index-- {
				stopped := running[index]
				stopErr = errors.Join(
					stopErr,
					service.container.Start(
						context.WithoutCancel(ctx),
						stopped.ServerID,
						capability,
						stopped.ID.String(),
						stopped.ContainerName,
					),
				)
			}
			return stopErr
		}
		running = append(running, installation)
	}
	restoreContainers := len(running) > 0
	defer func() {
		if !restoreContainers {
			return
		}
		for index := len(running) - 1; index >= 0; index-- {
			installation := running[index]
			err = errors.Join(
				err,
				service.container.Start(
					context.WithoutCancel(ctx),
					installation.ServerID,
					capability,
					installation.ID.String(),
					installation.ContainerName,
				),
			)
		}
	}()

	restoreContainers = false
	for _, installation := range existing {
		if err := service.container.Remove(
			ctx,
			installation.ServerID,
			capability,
			installation.ID.String(),
			installation.ContainerName,
		); err != nil {
			return fmt.Errorf("remove Resource container %q: %w", installation.ContainerName, err)
		}
	}
	for _, volume := range volumes {
		if err := service.container.RemoveVolume(
			ctx,
			volume.ServerID,
			capability,
			volume.Name,
		); err != nil {
			return fmt.Errorf("remove Resource volume %q: %w", volume.Name, err)
		}
	}

	now := time.Now().UTC()
	if err := models.Resource.ArchiveCascade(ctx, tx, resourceID, now); err != nil {
		return err
	}
	if err := tx.Commit(); err != nil {
		return err
	}
	return nil
}

func (service *ResourceManagement) loadResource(
	ctx context.Context,
	db storage.Executor,
	resourceID uuid.UUID,
	lock bool,
) (models.ResourceEntity, error) {
	return service.loadResourceWithSystemPolicy(ctx, db, resourceID, lock, false)
}

func (service *ResourceManagement) loadResourceWithSystemPolicy(
	ctx context.Context,
	db storage.Executor,
	resourceID uuid.UUID,
	lock, allowSystem bool,
) (models.ResourceEntity, error) {
	resource, err := models.Resource.FindActive(ctx, db, resourceID, lock)
	if err != nil {
		return models.ResourceEntity{}, err
	}
	if lock && resource.SystemManaged && !allowSystem {
		return models.ResourceEntity{}, ErrSystemResourceImmutable
	}
	return resource, nil
}

func (service *ResourceManagement) validateResourceTransition(
	ctx context.Context,
	db storage.Executor,
	current models.ResourceEntity,
	input ResourceInput,
) error {
	if current.ResourceType != input.ResourceType ||
		current.Engine() != resourceEngine(input.Configuration) {
		kindDependencies, err := models.Resource.HasKindDependencies(ctx, db, current.ID)
		if err != nil {
			return err
		}
		if kindDependencies {
			return domainError(
				"resourceType",
				"topology",
				"archive active endpoints, credentials, and health checks before changing Resource type or engine",
			)
		}
	}
	return nil
}

func resourceEngine(configuration json.RawMessage) string {
	var value struct {
		Engine string `json:"engine"`
	}
	_ = json.Unmarshal(configuration, &value)
	return strings.ToLower(strings.TrimSpace(value.Engine))
}

func domainError(field, code, message string) error {
	return errors.Join(
		models.ErrDomainValidation,
		validation.ValidationErrors{{Field: field, Code: code, Message: message}},
	)
}

func prefixResourceValidation(err error, prefix string) error {
	validationErrors, ok := validation.As(err)
	if !ok {
		return err
	}
	return errors.Join(
		models.ErrDomainValidation,
		validation.WithFieldPrefix(validationErrors, prefix),
	)
}

func mapResourceConflict(err error) error {
	var pgErr *pgconn.PgError
	if !errors.As(err, &pgErr) || pgErr.Code != "23505" {
		return err
	}
	field := "resource"
	message := "an active record already uses this value"
	switch pgErr.ConstraintName {
	case "resources_active_owner_name":
		field, message = "name", "an active Resource with this name already exists"
	case "resource_endpoints_active_resource_name",
		"resource_credentials_active_resource_name",
		"resource_volumes_active_resource_name",
		"resource_health_checks_active_installation_name":
		field = "name"
	case "resource_installations_active_server_container_name":
		field = "containerName"
	case "resource_volume_mounts_active_installation_path":
		field = "mountPath"
	}
	return domainError(field, "unique", message)
}

func normalizedJSON(value json.RawMessage) json.RawMessage {
	if len(value) == 0 {
		return json.RawMessage(`{}`)
	}
	return value
}

func resourceInstallationConfiguration(input ResourceInstallationInput) (json.RawMessage, error) {
	configuration := normalizedJSON(input.Configuration)
	if input.PortMappings == nil {
		return configuration, nil
	}

	values := make(map[string]any)
	if err := json.Unmarshal(configuration, &values); err != nil {
		return nil, domainError("configuration", "invalid", "configuration must be a JSON object")
	}
	if values == nil {
		values = make(map[string]any)
	}

	mappings := make([]models.ResourceInstallationPortMapping, len(*input.PortMappings))
	seenHostPorts := make(map[string]struct{}, len(mappings))
	for index, mapping := range *input.PortMappings {
		mapping.Protocol = strings.ToLower(strings.TrimSpace(mapping.Protocol))
		if mapping.Protocol == "" {
			mapping.Protocol = "tcp"
		}
		if mapping.HostPort < 1 || mapping.HostPort > 65535 {
			return nil, domainError(
				fmt.Sprintf("portMappings.%d.hostPort", index),
				"range",
				"host port must be between 1 and 65535",
			)
		}
		if mapping.ContainerPort < 1 || mapping.ContainerPort > 65535 {
			return nil, domainError(
				fmt.Sprintf("portMappings.%d.containerPort", index),
				"range",
				"container port must be between 1 and 65535",
			)
		}
		if mapping.Protocol != "tcp" && mapping.Protocol != "udp" {
			return nil, domainError(
				fmt.Sprintf("portMappings.%d.protocol", index),
				"unsupported",
				"port mapping protocol must be TCP or UDP",
			)
		}
		hostKey := fmt.Sprintf("%d/%s", mapping.HostPort, mapping.Protocol)
		if _, exists := seenHostPorts[hostKey]; exists {
			return nil, domainError(
				fmt.Sprintf("portMappings.%d.hostPort", index),
				"duplicate",
				"host port is mapped more than once",
			)
		}
		seenHostPorts[hostKey] = struct{}{}
		mappings[index] = mapping
	}

	if len(mappings) == 0 {
		delete(values, "portMappings")
	} else {
		values["portMappings"] = mappings
	}
	encoded, err := json.Marshal(values)
	if err != nil {
		return nil, err
	}
	return encoded, nil
}

func primaryPortMapping(
	configuration json.RawMessage,
) (models.ResourceInstallationPortMapping, error) {
	var decoded struct {
		PortMappings []models.ResourceInstallationPortMapping `json:"portMappings"`
	}
	if err := json.Unmarshal(configuration, &decoded); err != nil {
		return models.ResourceInstallationPortMapping{}, domainError(
			"configuration",
			"invalid",
			"installation configuration must be valid JSON",
		)
	}
	if len(decoded.PortMappings) != 1 {
		return models.ResourceInstallationPortMapping{}, domainError(
			"portMappings",
			"topology",
			"managed Resources require exactly one Docker port mapping",
		)
	}
	return decoded.PortMappings[0], nil
}

func managedPrimaryPortMapping(
	kind string,
	configuration json.RawMessage,
) (models.ResourceInstallationPortMapping, error) {
	definition, ok := models.FindResourceEngine(kind)
	if !ok {
		return models.ResourceInstallationPortMapping{}, domainError(
			"kind",
			"unsupported",
			"resource kind is not supported",
		)
	}
	mapping, err := primaryPortMapping(configuration)
	if err != nil {
		return models.ResourceInstallationPortMapping{}, err
	}
	if mapping.ContainerPort != definition.DefaultPort {
		return models.ResourceInstallationPortMapping{}, domainError(
			"portMappings.0.containerPort",
			"default",
			fmt.Sprintf(
				"%s Docker installations must use container port %d",
				definition.Label,
				definition.DefaultPort,
			),
		)
	}
	return mapping, nil
}
