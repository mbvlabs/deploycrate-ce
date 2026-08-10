package services

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"maps"
	"slices"
	"strings"
	"time"

	containerclient "deploycrate-ce/clients/container"
	postgresqlclient "deploycrate-ce/clients/postgresql"
	"deploycrate-ce/config"
	"deploycrate-ce/internal/secretcrypto"
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

func (service *ResourceManagement) createManagedPrimaryEndpoint(
	ctx context.Context,
	db storage.Executor,
	resource models.ResourceEntity,
	installation models.ResourceInstallationEntity,
) (models.ResourceEndpointEntity, error) {
	definition, ok := models.FindResourceEngine(resource.Engine())
	if !ok {
		return models.ResourceEndpointEntity{}, domainError(
			"kind",
			"unsupported",
			"resource kind is not supported",
		)
	}
	mapping, err := managedPrimaryPortMapping(resource.Engine(), installation.Configuration)
	if err != nil {
		return models.ResourceEndpointEntity{}, err
	}
	active, err := models.ResourceEndpoint.ActivePrimaryPublicCount(ctx, db, resource.ID)
	if err != nil {
		return models.ResourceEndpointEntity{}, err
	}
	if active != 0 {
		return models.ResourceEndpointEntity{}, domainError(
			"endpoint",
			"primary",
			"managed Resource already has a primary origin endpoint",
		)
	}
	originAddress, err := models.ServerOriginAddress(ctx, db, installation.ServerID)
	if err != nil {
		return models.ResourceEndpointEntity{}, err
	}
	return models.ResourceEndpoint.Create(ctx, db, models.CreateResourceEndpointData{
		Name: "Primary service", Role: "primary", Address: originAddress, Port: mapping.HostPort,
		Protocol: definition.DefaultProtocol, TlsMode: definition.DefaultTLSMode,
		Settings:   json.RawMessage(`{}`),
		ResourceID: resource.ID,
	})
}

func (service *ResourceManagement) syncManagedEndpoints(
	ctx context.Context,
	db storage.Executor,
	resource models.ResourceEntity,
	installation models.ResourceInstallationEntity,
) error {
	definition, ok := models.FindResourceEngine(resource.Engine())
	if !ok {
		return domainError("kind", "unsupported", "resource kind is not supported")
	}
	mapping, err := managedPrimaryPortMapping(resource.Engine(), installation.Configuration)
	if err != nil {
		return err
	}
	endpoints, err := models.ResourceEndpoint.ActiveForResource(ctx, db, resource.ID)
	if err != nil {
		return err
	}
	origins := 0
	privateEndpoints := 0
	for _, endpoint := range endpoints {
		address, err := models.ServerOriginAddress(ctx, db, installation.ServerID)
		if err != nil {
			return err
		}
		if endpoint.PrivateNetworkID == nil {
			if endpoint.Role != "primary" {
				return domainError(
					"endpoint",
					"primary",
					"managed Resource origin endpoint must use the primary role",
				)
			}
			origins++
		} else {
			privateEndpoints++
			attachmentAddress, err := models.ServerNetwork.WireGuardAddress(
				ctx, db, installation.ServerID, *endpoint.PrivateNetworkID,
			)
			if errors.Is(err, sql.ErrNoRows) || strings.TrimSpace(attachmentAddress) == "" {
				return domainError(
					"serverId",
					"network_topology",
					"installation Server has no active attachment for private access",
				)
			}
			if err != nil {
				return err
			}
			address = strings.TrimSpace(attachmentAddress)
			if address != endpoint.Address {
				return domainError(
					"serverId",
					"private_access",
					"remove this Resource from its private network before changing its WireGuard attachment address",
				)
			}
		}
		if _, err := models.ResourceEndpoint.Update(ctx, db, models.UpdateResourceEndpointData{
			ID: endpoint.ID, Name: endpoint.Name, Role: endpoint.Role, Address: address,
			Port: mapping.HostPort, Protocol: definition.DefaultProtocol, TlsMode: endpoint.TLSMode,
			Settings: endpoint.Settings, ArchivedAt: endpoint.ArchivedAt, ResourceID: resource.ID,
			PrivateNetworkID: endpoint.PrivateNetworkID,
		}); err != nil {
			return err
		}
	}
	if origins != 1 {
		return domainError(
			"endpoint",
			"primary",
			"managed Resource requires exactly one primary origin endpoint",
		)
	}
	if privateEndpoints > 1 {
		return domainError(
			"endpoint",
			"private",
			"managed Resource supports at most one private endpoint",
		)
	}
	return nil
}

func nullableString(value string) sql.NullString {
	value = strings.TrimSpace(value)
	return sql.NullString{String: value, Valid: value != ""}
}

func (service *ResourceManagement) credentialPayload(
	input ResourceCredentialInput,
	definition models.ResourceEngineDefinition,
) ([]byte, []byte, error) {
	values := make(map[string]string)
	for key, value := range input.SecretValues {
		key = strings.TrimSpace(key)
		if key != "" && value != "" {
			values[key] = value
		}
	}
	if len(values) == 0 {
		return nil, nil, domainError(
			"secretValues",
			"required",
			"at least one credential value is required",
		)
	}
	allowed := make(map[string]models.ResourceCredentialField, len(definition.CredentialFields))
	for _, field := range definition.CredentialFields {
		allowed[field.Name] = field
		if field.Required && values[field.Name] == "" {
			return nil, nil, domainError(
				"secretValues."+field.Name,
				"required",
				field.Label+" is required",
			)
		}
	}
	for key := range values {
		if _, ok := allowed[key]; !ok {
			return nil, nil, domainError(
				"secretValues."+key,
				"unsupported",
				"credential field is not supported by this resource kind",
			)
		}
	}
	payload, err := json.Marshal(struct {
		SchemaVersion int               `json:"schema_version"`
		Values        map[string]string `json:"values"`
	}{SchemaVersion: 1, Values: values})
	if err != nil {
		return nil, nil, err
	}
	encrypted, err := secretcrypto.EncryptForPurpose(
		payload,
		service.config.App.SessionEncryptionKey,
		resourceCredentialPurpose,
	)
	if err != nil {
		return nil, nil, err
	}
	key, err := hex.DecodeString(service.config.App.SessionEncryptionKey)
	if err != nil || len(key) != 32 {
		return nil, nil, errors.New("resource credential digest key is invalid")
	}
	digest := hmac.New(sha256.New, key)
	_, _ = digest.Write(payload)
	return encrypted, digest.Sum(nil), nil
}

func (service *ResourceManagement) credentialSecretValues(
	credential models.ResourceCredentialEntity,
) (map[string]string, error) {
	plaintext, err := secretcrypto.DecryptForPurpose(
		credential.EncPayload,
		service.config.App.SessionEncryptionKey,
		resourceCredentialPurpose,
	)
	if err != nil {
		return nil, fmt.Errorf("decrypt Resource credential: %w", err)
	}
	var payload struct {
		SchemaVersion int               `json:"schema_version"`
		Values        map[string]string `json:"values"`
	}
	if err := json.Unmarshal(plaintext, &payload); err != nil || payload.SchemaVersion != 1 {
		return nil, errors.New("Resource credential payload is invalid")
	}
	return payload.Values, nil
}

func (service *ResourceManagement) resourceAdministratorCredential(
	ctx context.Context,
	db storage.Executor,
	resourceID uuid.UUID,
) (models.ResourceCredentialEntity, error) {
	credential, err := models.ResourceCredential.FindAdministrator(ctx, db, resourceID)
	if errors.Is(err, sql.ErrNoRows) {
		return models.ResourceCredentialEntity{}, errors.New(
			"Resource administrator credential is required",
		)
	}
	return credential, err
}

func (service *ResourceManagement) postgreSQLAdministratorConnection(
	ctx context.Context,
	db storage.Executor,
	resource models.ResourceEntity,
) (postgresqlclient.Connection, error) {
	administrator, err := service.resourceAdministratorCredential(ctx, db, resource.ID)
	if err != nil {
		return postgresqlclient.Connection{}, err
	}
	values, err := service.credentialSecretValues(administrator)
	if err != nil {
		return postgresqlclient.Connection{}, err
	}
	endpoint, err := models.ResourceEndpoint.FindPrimary(ctx, db, resource.ID)
	if err != nil {
		return postgresqlclient.Connection{}, err
	}
	if !administrator.Username.Valid || values["password"] == "" {
		return postgresqlclient.Connection{}, errors.New(
			"PostgreSQL Resource administrator credential is incomplete",
		)
	}
	return postgresqlclient.Connection{
		Host:     endpoint.Address,
		Port:     endpoint.Port,
		Username: administrator.Username.String,
		Password: values["password"],
	}, nil
}

func (service *ResourceManagement) CreateDatabase(
	ctx context.Context,
	resourceID uuid.UUID,
	input CreateResourceDatabaseInput,
) (result models.ResourceEntity, err error) {
	database := input.Database
	database.Name = strings.TrimSpace(database.Name)
	database.Encoding = strings.TrimSpace(database.Encoding)
	database.Collation = strings.TrimSpace(database.Collation)
	if (input.CredentialID == nil) == (input.Credential == nil) {
		return models.ResourceEntity{}, domainError(
			"credential",
			"required",
			"select an existing application credential or create a new one",
		)
	}
	tx, err := service.db.BeginTx(ctx, nil)
	if err != nil {
		return models.ResourceEntity{}, err
	}
	defer tx.Rollback()
	resource, err := service.loadResource(ctx, tx, resourceID, true)
	if err != nil {
		return models.ResourceEntity{}, err
	}
	if resource.ResourceType != models.ResourceTypeDatabase || resource.Engine() != "postgresql" {
		return models.ResourceEntity{}, domainError(
			"database",
			"resource_type",
			"database creation currently requires a PostgreSQL Resource",
		)
	}
	if database.Name == "" || strings.EqualFold(database.Name, "postgres") ||
		resourceHasDatabase(resource, database.Name) {
		return models.ResourceEntity{}, domainError(
			"name",
			"unavailable",
			"database name must be unique and cannot be postgres",
		)
	}
	connection, err := service.postgreSQLAdministratorConnection(ctx, tx, resource)
	if err != nil {
		return models.ResourceEntity{}, err
	}
	createdDatabase, err := service.postgres.CreateDatabase(
		ctx,
		connection,
		database.Name,
		database.Encoding,
		database.Collation,
	)
	if err != nil {
		return models.ResourceEntity{}, err
	}
	completed := false
	var reconciledCredential *models.ResourceCredentialEntity
	var previousCredential *models.ResourceCredentialEntity
	defer func() {
		if completed {
			return
		}
		compensationContext := context.WithoutCancel(ctx)
		compensationErrors := make([]error, 0, 3)
		if previousCredential != nil && reconciledCredential != nil {
			compensationErrors = append(
				compensationErrors,
				service.reconcilePostgreSQLCredential(
					compensationContext,
					service.db.Executor(),
					resource,
					*previousCredential,
					reconciledCredential,
				),
			)
		}
		if createdDatabase {
			compensationErrors = append(
				compensationErrors,
				service.postgres.DropDatabase(compensationContext, connection, database.Name),
			)
		}
		if previousCredential == nil && reconciledCredential != nil &&
			reconciledCredential.Username.Valid {
			if !createdDatabase {
				compensationErrors = append(
					compensationErrors,
					service.postgres.RevokeLoginRoleDatabase(
						compensationContext,
						connection,
						database.Name,
						reconciledCredential.Username.String,
					),
				)
			}
			compensationErrors = append(
				compensationErrors,
				service.postgres.DropLoginRole(
					compensationContext,
					connection,
					reconciledCredential.Username.String,
				),
			)
		}
		err = errors.Join(err, errors.Join(compensationErrors...))
	}()
	configuration := resource.ParsedConfiguration()
	configuration.Databases = append(configuration.Databases, database)
	encoded, err := json.Marshal(configuration)
	if err != nil {
		return models.ResourceEntity{}, err
	}
	updated, err := models.Resource.Update(ctx, tx, models.UpdateResourceData{
		ID:                    resource.ID,
		Name:                  resource.Name,
		Slug:                  resource.Slug,
		ResourceType:          resource.ResourceType,
		Configuration:         encoded,
		SystemManaged:         resource.SystemManaged,
		EnvironmentAttachable: resource.EnvironmentAttachable,
		ArchivedAt:            resource.ArchivedAt,
	})
	if err != nil {
		return models.ResourceEntity{}, err
	}
	resource = updated
	if input.CredentialID != nil {
		current, findErr := models.ResourceCredential.LockActiveForResource(
			ctx, tx, resource.ID, *input.CredentialID,
		)
		if errors.Is(findErr, sql.ErrNoRows) {
			return models.ResourceEntity{}, domainError(
				"credentialId",
				"unavailable",
				"selected application credential is unavailable",
			)
		}
		if findErr != nil {
			return models.ResourceEntity{}, findErr
		}
		if resourceCredentialMetadataPurpose(current.Metadata) != "application" {
			return models.ResourceEntity{}, domainError(
				"credentialId",
				"application",
				"only application credentials can be attached to a Database",
			)
		}
		metadata, metadataErr := resourceCredentialMetadataForDatabase(
			current.Metadata,
			database.Name,
		)
		if metadataErr != nil {
			return models.ResourceEntity{}, metadataErr
		}
		candidate := current
		candidate.Metadata = metadata
		credentialInput := ResourceCredentialInput{
			Name: current.Name, Username: current.Username.String,
			Metadata: metadata,
		}
		if validationErr := service.validatePostgreSQLCredential(
			ctx,
			tx,
			resource,
			credentialInput,
			&current.ID,
		); validationErr != nil {
			return models.ResourceEntity{}, validationErr
		}
		if reconcileErr := service.reconcilePostgreSQLCredential(
			ctx,
			tx,
			resource,
			candidate,
			&current,
		); reconcileErr != nil {
			return models.ResourceEntity{}, reconcileErr
		}
		reconciledCredential = &candidate
		previousCredential = &current
		if _, updateErr := models.ResourceCredential.Update(
			ctx,
			tx,
			models.UpdateResourceCredentialData{
				ID: current.ID, Name: current.Name, Username: current.Username,
				Metadata: metadata, EncPayload: current.EncPayload, Digest: current.Digest,
				ArchivedAt: current.ArchivedAt, ResourceID: resource.ID,
			},
		); updateErr != nil {
			return models.ResourceEntity{}, mapResourceConflict(updateErr)
		}
	} else {
		credentialInput := *input.Credential
		metadata, metadataErr := resourceCredentialMetadataForDatabase(
			credentialInput.Metadata,
			database.Name,
		)
		if metadataErr != nil {
			return models.ResourceEntity{}, metadataErr
		}
		credentialInput.Metadata = metadata
		createdCredential, createCredentialErr := service.createCredential(
			ctx,
			tx,
			resource,
			credentialInput,
		)
		if createCredentialErr != nil {
			return models.ResourceEntity{}, prefixResourceValidation(
				createCredentialErr,
				"credential",
			)
		}
		if reconcileErr := service.reconcilePostgreSQLCredential(
			ctx,
			tx,
			resource,
			createdCredential,
			nil,
		); reconcileErr != nil {
			return models.ResourceEntity{}, prefixResourceValidation(
				reconcileErr,
				"credential",
			)
		}
		reconciledCredential = &createdCredential
	}
	if err := tx.Commit(); err != nil {
		return models.ResourceEntity{}, err
	}
	completed = true
	return updated, nil
}

func (service *ResourceManagement) DestroyDatabase(
	ctx context.Context,
	resourceID uuid.UUID,
	databaseName string,
) error {
	databaseName = strings.TrimSpace(databaseName)
	if databaseName == "" || strings.EqualFold(databaseName, "postgres") {
		return domainError(
			"database",
			"unavailable",
			"select a configured application Database",
		)
	}
	tx, err := service.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	resource, err := service.loadResource(ctx, tx, resourceID, true)
	if err != nil {
		return err
	}
	if resource.ResourceType != models.ResourceTypeDatabase || resource.Engine() != "postgresql" {
		return domainError(
			"database",
			"resource_type",
			"database deletion currently requires a PostgreSQL Resource",
		)
	}
	databaseIndex := -1
	configuration := resource.ParsedConfiguration()
	for index, database := range configuration.Databases {
		if strings.EqualFold(database.Name, databaseName) {
			databaseIndex = index
			databaseName = database.Name
			break
		}
	}
	if databaseIndex < 0 {
		return models.ErrNotFound
	}
	if _, policyErr := models.BackupPolicy.FindForResourceDatabase(
		ctx,
		tx,
		resource.ID,
		databaseName,
	); policyErr == nil {
		return domainError(
			"database",
			"dependency",
			"archive the Database backup policy before deleting this Database",
		)
	} else if !errors.Is(policyErr, sql.ErrNoRows) {
		return policyErr
	}
	activeRestores, err := models.ResourceRestore.ActiveCountForResourceDatabase(
		ctx,
		tx,
		resource.ID,
		databaseName,
	)
	if err != nil {
		return err
	}
	if activeRestores > 0 {
		return domainError(
			"database",
			"dependency",
			"wait for the active Database restore to finish before deleting this Database",
		)
	}
	credentials, err := models.ResourceCredential.LockActiveApplicationsForDatabase(
		ctx,
		tx,
		resource.ID,
		databaseName,
	)
	if err != nil {
		return err
	}
	for _, credential := range credentials {
		dependencies, dependencyErr := models.ResourceCredential.ActiveDependencyCount(
			ctx,
			tx,
			credential.ID,
		)
		if dependencyErr != nil {
			return dependencyErr
		}
		if dependencies > 0 {
			return domainError(
				"database",
				"dependency",
				"detach Environments and health checks using this Database before deleting it",
			)
		}
	}
	connection, err := service.postgreSQLAdministratorConnection(ctx, tx, resource)
	if err != nil {
		return err
	}
	configuration.Databases = slices.Delete(configuration.Databases, databaseIndex, databaseIndex+1)
	encoded, err := json.Marshal(configuration)
	if err != nil {
		return err
	}
	if _, err := models.Resource.Update(ctx, tx, models.UpdateResourceData{
		ID: resource.ID, Name: resource.Name, Slug: resource.Slug,
		ResourceType: resource.ResourceType, Configuration: encoded,
		SystemManaged: resource.SystemManaged, EnvironmentAttachable: resource.EnvironmentAttachable,
		ArchivedAt: resource.ArchivedAt,
	}); err != nil {
		return err
	}
	now := time.Now().UTC()
	for _, credential := range credentials {
		if err := models.ResourceCredential.ArchiveID(ctx, tx, credential.ID, now); err != nil {
			return err
		}
	}
	if err := service.postgres.DropDatabase(ctx, connection, databaseName); err != nil {
		return err
	}
	for _, credential := range credentials {
		if !credential.Username.Valid {
			continue
		}
		if cleanupErr := service.postgres.DropLoginRole(
			ctx,
			connection,
			credential.Username.String,
		); cleanupErr != nil {
			slog.WarnContext(
				ctx,
				"database deleted but PostgreSQL credential role cleanup failed",
				"resource_id", resource.ID,
				"database", databaseName,
				"credential_id", credential.ID,
				"username", credential.Username.String,
				"error", cleanupErr,
			)
		}
	}
	return tx.Commit()
}

func (service *ResourceManagement) reconcilePostgreSQLCredential(
	ctx context.Context,
	db storage.Executor,
	resource models.ResourceEntity,
	credential models.ResourceCredentialEntity,
	previous *models.ResourceCredentialEntity,
) error {
	purpose := resourceCredentialMetadataPurpose(credential.Metadata)
	if purpose == "administrator" {
		if previous == nil {
			return nil
		}
		if resourceCredentialMetadataPurpose(previous.Metadata) != "administrator" ||
			!credential.Username.Valid ||
			!previous.Username.Valid ||
			!strings.EqualFold(
				strings.TrimSpace(credential.Username.String),
				strings.TrimSpace(previous.Username.String),
			) {
			return domainError(
				"metadata.purpose",
				"administrator",
				"PostgreSQL administrator identity cannot be changed",
			)
		}
		currentValues, err := service.credentialSecretValues(*previous)
		if err != nil {
			return fmt.Errorf("load current PostgreSQL administrator credential: %w", err)
		}
		desiredValues, err := service.credentialSecretValues(credential)
		if err != nil {
			return fmt.Errorf("load desired PostgreSQL administrator credential: %w", err)
		}
		currentPassword, desiredPassword := currentValues["password"], desiredValues["password"]
		if currentPassword == "" || desiredPassword == "" {
			return errors.New("PostgreSQL Resource administrator credential is incomplete")
		}
		if currentPassword == desiredPassword {
			return nil
		}
		endpoint, err := models.ResourceEndpoint.FindPrimary(ctx, db, resource.ID)
		if err != nil {
			return fmt.Errorf("load PostgreSQL Resource primary endpoint: %w", err)
		}
		connection := postgresqlclient.Connection{
			Host: endpoint.Address, Port: endpoint.Port,
			Username: previous.Username.String, Password: currentPassword,
		}
		if err := service.postgres.RotateAdministratorPassword(
			ctx,
			connection,
			desiredPassword,
		); err != nil {
			return err
		}
		return nil
	}
	if purpose != "application" {
		return domainError(
			"metadata.purpose",
			"unsupported",
			"PostgreSQL credential purpose must be administrator or application",
		)
	}
	databaseName := resourceCredentialMetadataDatabase(credential.Metadata)
	if databaseName == "" || !resourceHasDatabase(resource, databaseName) {
		return domainError(
			"metadata.database",
			"database",
			"application credentials must select a configured Resource database",
		)
	}
	reconciliation, err := service.preparePostgreSQLCredentialReconciliation(
		ctx,
		db,
		resource,
		credential,
		previous,
	)
	if err != nil {
		return err
	}
	if err := service.postgres.ReconcileLoginRoleAcrossDatabases(
		ctx,
		reconciliation.Connection,
		[]string{databaseName},
		reconciliation.Username,
		reconciliation.Password,
		reconciliation.PreviousPassword,
	); err != nil {
		return fmt.Errorf(
			"reconcile PostgreSQL login role %q across Resource Databases: %w",
			reconciliation.Username,
			err,
		)
	}
	previousDatabase := ""
	if previous != nil && resourceCredentialMetadataPurpose(previous.Metadata) == "application" {
		previousDatabase = resourceCredentialMetadataDatabase(previous.Metadata)
	}
	if previousDatabase == "" || strings.EqualFold(previousDatabase, databaseName) {
		return nil
	}
	if err := service.postgres.RevokeLoginRoleDatabase(
		ctx,
		reconciliation.Connection,
		previousDatabase,
		reconciliation.Username,
	); err != nil {
		rollbackReconciliation, prepareErr := service.preparePostgreSQLCredentialReconciliation(
			context.WithoutCancel(ctx),
			db,
			resource,
			*previous,
			&credential,
		)
		var rollbackErr error
		if prepareErr == nil {
			rollbackErr = service.postgres.ReconcileLoginRoleAcrossDatabases(
				context.WithoutCancel(
					ctx,
				),
				rollbackReconciliation.Connection,
				[]string{previousDatabase},
				rollbackReconciliation.Username,
				rollbackReconciliation.Password,
				rollbackReconciliation.PreviousPassword,
			)
		}
		revokeTargetErr := service.postgres.RevokeLoginRoleDatabase(
			context.WithoutCancel(ctx),
			reconciliation.Connection,
			databaseName,
			reconciliation.Username,
		)
		return errors.Join(
			fmt.Errorf(
				"revoke PostgreSQL database %q access from role %q: %w",
				previousDatabase,
				reconciliation.Username,
				err,
			),
			prepareErr,
			rollbackErr,
			revokeTargetErr,
		)
	}
	return nil
}

func (service *ResourceManagement) reconcilePostgreSQLDatabaseCredentials(
	ctx context.Context,
	resourceID, installationID uuid.UUID,
	database string,
) error {
	resource, err := models.Resource.Find(ctx, service.db.Executor(), resourceID)
	if err != nil {
		return err
	}
	if resource.ArchivedAt.Valid || resource.Engine() != "postgresql" {
		return errors.New(
			"PostgreSQL credential reconciliation requires an active managed Resource",
		)
	}
	_ = installationID
	credentials, err := models.ResourceCredential.ActiveForResourceAll(
		ctx, service.db.Executor(), resourceID,
	)
	if err != nil {
		return err
	}
	for _, credential := range credentials {
		if resourceCredentialMetadataPurpose(credential.Metadata) != "application" ||
			!strings.EqualFold(resourceCredentialMetadataDatabase(credential.Metadata), database) {
			continue
		}
		if err := service.reconcilePostgreSQLCredentialInDatabase(
			ctx,
			service.db.Executor(),
			resource,
			credential,
			database,
		); err != nil {
			return err
		}
	}
	return nil
}

func (service *ResourceManagement) reconcilePostgreSQLCredentialInDatabase(
	ctx context.Context,
	db storage.Executor,
	resource models.ResourceEntity,
	credential models.ResourceCredentialEntity,
	database string,
) error {
	reconciliation, err := service.preparePostgreSQLCredentialReconciliation(
		ctx,
		db,
		resource,
		credential,
		&credential,
	)
	if err != nil {
		return err
	}
	if err := service.postgres.GrantLoginRoleDatabase(
		ctx,
		reconciliation.Connection,
		database,
		reconciliation.Username,
	); err != nil {
		return fmt.Errorf(
			"grant PostgreSQL database %q access to role %q: %w",
			database,
			reconciliation.Username,
			err,
		)
	}
	return nil
}

type postgreSQLCredentialReconciliation struct {
	Connection       postgresqlclient.Connection
	Username         string
	Password         string
	PreviousPassword string
}

func (service *ResourceManagement) preparePostgreSQLCredentialReconciliation(
	ctx context.Context,
	db storage.Executor,
	resource models.ResourceEntity,
	credential models.ResourceCredentialEntity,
	previous *models.ResourceCredentialEntity,
) (postgreSQLCredentialReconciliation, error) {
	topology, err := models.Resource.PostgreSQLCredentialTopology(ctx, db, resource.ID)
	if err != nil {
		return postgreSQLCredentialReconciliation{}, fmt.Errorf(
			"load Resource administrator and primary endpoint: %w",
			err,
		)
	}
	administratorPlaintext, err := secretcrypto.DecryptForPurpose(
		topology.AdministratorPayload,
		service.config.App.SessionEncryptionKey,
		resourceCredentialPurpose,
	)
	if err != nil {
		return postgreSQLCredentialReconciliation{}, fmt.Errorf(
			"decrypt Resource administrator credential: %w",
			err,
		)
	}
	var administratorPayload struct {
		SchemaVersion int               `json:"schema_version"`
		Values        map[string]string `json:"values"`
	}
	if json.Unmarshal(administratorPlaintext, &administratorPayload) != nil ||
		administratorPayload.SchemaVersion != 1 {
		return postgreSQLCredentialReconciliation{}, errors.New(
			"Resource administrator credential is invalid",
		)
	}
	administratorPassword := administratorPayload.Values["password"]
	if administratorPassword == "" {
		return postgreSQLCredentialReconciliation{}, errors.New(
			"Resource administrator credential has no PostgreSQL password",
		)
	}
	targetValues, err := service.credentialSecretValues(credential)
	if err != nil {
		return postgreSQLCredentialReconciliation{}, fmt.Errorf(
			"load PostgreSQL login credential: %w",
			err,
		)
	}
	if !credential.Username.Valid || targetValues["password"] == "" {
		return postgreSQLCredentialReconciliation{}, errors.New(
			"PostgreSQL login credential requires a username and password",
		)
	}
	if strings.EqualFold(
		strings.TrimSpace(credential.Username.String),
		strings.TrimSpace(topology.AdministratorUsername),
	) {
		return postgreSQLCredentialReconciliation{}, domainError(
			"username",
			"administrator",
			"Application username must be different from the Resource administrator",
		)
	}
	previousPassword := ""
	if previous != nil && previous.Username.Valid &&
		strings.EqualFold(
			strings.TrimSpace(previous.Username.String),
			strings.TrimSpace(credential.Username.String),
		) {
		previousValues, err := service.credentialSecretValues(*previous)
		if err != nil {
			return postgreSQLCredentialReconciliation{}, fmt.Errorf(
				"load previous PostgreSQL login credential: %w",
				err,
			)
		}
		previousPassword = previousValues["password"]
	}
	return postgreSQLCredentialReconciliation{
		Connection: postgresqlclient.Connection{
			Host: topology.Address, Port: topology.Port,
			Username: topology.AdministratorUsername, Password: administratorPassword,
		},
		Username: strings.TrimSpace(
			credential.Username.String,
		),
		Password:         targetValues["password"],
		PreviousPassword: previousPassword,
	}, nil
}

func (service *ResourceManagement) CreateEndpoint(
	ctx context.Context,
	resourceID uuid.UUID,
	input ResourceEndpointInput,
) (models.ResourceEndpointEntity, error) {
	tx, err := service.db.BeginTx(ctx, nil)
	if err != nil {
		return models.ResourceEndpointEntity{}, err
	}
	defer tx.Rollback()
	resource, err := service.loadResource(ctx, tx, resourceID, true)
	if err != nil {
		return models.ResourceEndpointEntity{}, err
	}
	endpoint, err := service.createEndpoint(ctx, tx, resource, input)
	if err != nil {
		return models.ResourceEndpointEntity{}, err
	}
	if err := tx.Commit(); err != nil {
		return models.ResourceEndpointEntity{}, mapResourceConflict(err)
	}
	return endpoint, nil
}

func (service *ResourceManagement) createEndpoint(
	ctx context.Context,
	db storage.Executor,
	resource models.ResourceEntity,
	input ResourceEndpointInput,
) (models.ResourceEndpointEntity, error) {
	input.Settings = normalizedJSON(input.Settings)
	entity := models.ResourceEndpointEntity{
		Name: input.Name, Role: input.Role, Address: input.Address, Port: input.Port,
		Protocol: input.Protocol, TLSMode: input.TLSMode, Settings: input.Settings,
		ResourceID:       resource.ID,
		PrivateNetworkID: input.PrivateNetworkID,
	}
	if err := entity.ValidateForKind(resource.Engine()); err != nil {
		return models.ResourceEndpointEntity{}, errors.Join(models.ErrDomainValidation, err)
	}
	caddySettings := entity.ParsedSettings().Caddy
	isCaddyEndpoint := caddySettings != nil && caddySettings.Managed
	if !isCaddyEndpoint && input.Role == "primary" && input.PrivateNetworkID == nil {
		primaryEndpoints, err := models.ResourceEndpoint.ActivePrimaryPublicCount(
			ctx,
			db,
			resource.ID,
		)
		if err != nil {
			return models.ResourceEndpointEntity{}, err
		}
		if primaryEndpoints != 0 {
			return models.ResourceEndpointEntity{}, domainError(
				"role",
				"primary",
				"Resource already has a primary origin endpoint",
			)
		}
	}
	if err := service.validateEndpointTopology(ctx, db, resource, nil, input); err != nil {
		return models.ResourceEndpointEntity{}, err
	}
	created, err := models.ResourceEndpoint.Create(ctx, db, models.CreateResourceEndpointData{
		Name: entity.Name, Role: entity.Role, Address: entity.Address, Port: entity.Port,
		Protocol: entity.Protocol, TlsMode: entity.TLSMode, Settings: entity.Settings,
		ResourceID:       resource.ID,
		PrivateNetworkID: entity.PrivateNetworkID,
	})
	return created, mapResourceConflict(err)
}

func (service *ResourceManagement) UpdateEndpoint(
	ctx context.Context,
	resourceID, endpointID uuid.UUID,
	input ResourceEndpointInput,
) (models.ResourceEndpointEntity, error) {
	tx, err := service.db.BeginTx(ctx, nil)
	if err != nil {
		return models.ResourceEndpointEntity{}, err
	}
	defer tx.Rollback()
	resource, err := service.loadResource(ctx, tx, resourceID, true)
	if err != nil {
		return models.ResourceEndpointEntity{}, err
	}
	current, err := models.ResourceEndpoint.LockActiveForResource(ctx, tx, resourceID, endpointID)
	if errors.Is(err, sql.ErrNoRows) {
		return models.ResourceEndpointEntity{}, models.ErrNotFound
	}
	if err != nil {
		return models.ResourceEndpointEntity{}, err
	}
	if current.Role == "wireguard" && current.PrivateNetworkID != nil &&
		current.Name == "Private access" {
		return models.ResourceEndpointEntity{}, domainError(
			"endpoint",
			"managed",
			"WireGuard access is managed through the Endpoints page",
		)
	}
	currentCaddySettings := current.ParsedSettings().Caddy
	currentIsCaddyEndpoint := currentCaddySettings != nil && currentCaddySettings.Managed
	if !currentIsCaddyEndpoint && current.Role == "primary" && current.PrivateNetworkID == nil &&
		(input.Role != "primary" || input.PrivateNetworkID != nil) {
		return models.ResourceEndpointEntity{}, domainError(
			"role",
			"primary",
			"external Resource primary origin cannot be changed into another endpoint type",
		)
	}
	input.Settings = normalizedJSON(input.Settings)
	entity := models.ResourceEndpointEntity{
		ID:               current.ID,
		Name:             input.Name,
		Role:             input.Role,
		Address:          input.Address,
		Port:             input.Port,
		Protocol:         input.Protocol,
		TLSMode:          input.TLSMode,
		Settings:         input.Settings,
		ResourceID:       resource.ID,
		PrivateNetworkID: input.PrivateNetworkID,
		ArchivedAt:       current.ArchivedAt,
	}
	if err := entity.ValidateForKind(resource.Engine()); err != nil {
		return models.ResourceEndpointEntity{}, errors.Join(models.ErrDomainValidation, err)
	}
	inputCaddySettings := entity.ParsedSettings().Caddy
	inputIsCaddyEndpoint := inputCaddySettings != nil && inputCaddySettings.Managed
	if !inputIsCaddyEndpoint && input.Role == "primary" && input.PrivateNetworkID == nil &&
		(current.Role != "primary" || current.PrivateNetworkID != nil) {
		primaryEndpoints, countErr := models.ResourceEndpoint.ActivePrimaryPublicCount(
			ctx,
			tx,
			resourceID,
		)
		if countErr != nil {
			return models.ResourceEndpointEntity{}, countErr
		}
		if primaryEndpoints != 0 {
			return models.ResourceEndpointEntity{}, domainError(
				"role",
				"primary",
				"Resource already has a primary origin endpoint",
			)
		}
	}
	if err := service.validateEndpointTopology(ctx, tx, resource, &current.ID, input); err != nil {
		return models.ResourceEndpointEntity{}, err
	}
	updated, err := models.ResourceEndpoint.Update(ctx, tx, models.UpdateResourceEndpointData{
		ID:               current.ID,
		Name:             entity.Name,
		Role:             entity.Role,
		Address:          entity.Address,
		Port:             entity.Port,
		Protocol:         entity.Protocol,
		TlsMode:          entity.TLSMode,
		Settings:         entity.Settings,
		ArchivedAt:       current.ArchivedAt,
		ResourceID:       resource.ID,
		PrivateNetworkID: entity.PrivateNetworkID,
	})
	if err != nil {
		return models.ResourceEndpointEntity{}, mapResourceConflict(err)
	}
	connections, err := models.EnvironmentResource.ActiveForEndpointID(ctx, tx, endpointID)
	if err != nil {
		return models.ResourceEndpointEntity{}, err
	}
	if err := service.reconcileEnvironmentResourceConnections(
		ctx,
		tx,
		resource,
		connections,
	); err != nil {
		return models.ResourceEndpointEntity{}, err
	}
	if err := tx.Commit(); err != nil {
		return models.ResourceEndpointEntity{}, mapResourceConflict(err)
	}
	return updated, nil
}

func (service *ResourceManagement) validateEndpointTopology(
	ctx context.Context,
	db storage.Executor,
	resource models.ResourceEntity,
	endpointID *uuid.UUID,
	input ResourceEndpointInput,
) error {
	if input.PrivateNetworkID != nil {
		exists, err := models.PrivateNetwork.ActiveExists(ctx, db, *input.PrivateNetworkID)
		if err != nil {
			return err
		}
		if err := requireChild(
			boolCount(exists),
			"privateNetworkId",
			"private network is unavailable",
		); err != nil {
			return err
		}
		enabled, err := models.ResourceEndpoint.WireGuardGatewayExists(
			ctx, db, resource.ID, *input.PrivateNetworkID,
		)
		if err != nil {
			return err
		}
		if err := requireChild(
			boolCount(enabled),
			"privateNetworkId",
			"turn on WireGuard access before publishing an endpoint through this private network",
		); err != nil {
			return err
		}
		if endpointID != nil {
			incompatible, err := models.EnvironmentResource.IncompatibleEndpointNetworkCount(
				ctx, db, *endpointID, *input.PrivateNetworkID,
			)
			if err != nil {
				return err
			}
			if incompatible > 0 {
				return domainError(
					"privateNetworkId",
					"topology",
					"an existing Connected Environment cannot reach this private network",
				)
			}
		}
	}
	return nil
}

func (service *ResourceManagement) ArchiveEndpoint(
	ctx context.Context,
	resourceID, endpointID uuid.UUID,
) error {
	return service.archiveEndpoint(ctx, resourceID, endpointID, false)
}

func (service *ResourceManagement) ArchiveSystemEndpoint(
	ctx context.Context,
	resourceID, endpointID uuid.UUID,
) error {
	return service.archiveEndpoint(ctx, resourceID, endpointID, true)
}

func (service *ResourceManagement) archiveEndpoint(
	ctx context.Context,
	resourceID, endpointID uuid.UUID,
	systemManaged bool,
) error {
	tx, err := service.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	resource, err := service.loadResourceWithSystemPolicy(ctx, tx, resourceID, true, systemManaged)
	if err != nil {
		return err
	}
	if resource.SystemManaged != systemManaged {
		return models.ErrNotFound
	}
	endpoint, err := models.ResourceEndpoint.LockActiveForResource(ctx, tx, resourceID, endpointID)
	if errors.Is(err, sql.ErrNoRows) {
		return models.ErrNotFound
	}
	if err != nil {
		return err
	}
	if endpoint.Role == "wireguard" && endpoint.PrivateNetworkID != nil &&
		endpoint.Name == "Private access" {
		return domainError(
			"endpoint",
			"managed",
			"turn off WireGuard access instead of archiving its gateway endpoint",
		)
	}
	dependencies, err := models.ResourceEndpoint.ActiveDependencyCount(ctx, tx, endpointID)
	if err != nil {
		return err
	}
	if dependencies > 0 {
		return domainError(
			"endpoint",
			"dependency",
			"endpoint is selected by an active binding or health check",
		)
	}
	if !systemManaged && endpoint.Role == "primary" && endpoint.PrivateNetworkID == nil {
		primaryEndpoints, countErr := models.ResourceEndpoint.ActivePrimaryPublicCount(
			ctx,
			tx,
			resourceID,
		)
		if countErr != nil {
			return countErr
		}
		if primaryEndpoints == 1 {
			return domainError(
				"endpoint",
				"required",
				"external Resources must retain one primary origin endpoint",
			)
		}
	}
	now := time.Now().UTC()
	if _, err := models.ResourceEndpoint.ArchiveActive(ctx, tx, endpointID, now); err != nil {
		return err
	}
	return tx.Commit()
}

func (service *ResourceManagement) CreateCredential(
	ctx context.Context,
	resourceID uuid.UUID,
	input ResourceCredentialInput,
) (models.ResourceCredentialEntity, error) {
	tx, err := service.db.BeginTx(ctx, nil)
	if err != nil {
		return models.ResourceCredentialEntity{}, err
	}
	defer tx.Rollback()
	resource, err := service.loadResource(ctx, tx, resourceID, true)
	if err != nil {
		return models.ResourceCredentialEntity{}, err
	}
	credential, err := service.createCredential(ctx, tx, resource, input)
	if err != nil {
		return models.ResourceCredentialEntity{}, err
	}
	if resource.Engine() == "postgresql" {
		if err := service.reconcilePostgreSQLCredential(
			ctx,
			tx,
			resource,
			credential,
			nil,
		); err != nil {
			return models.ResourceCredentialEntity{}, err
		}
	}
	if err := tx.Commit(); err != nil {
		return models.ResourceCredentialEntity{}, mapResourceConflict(err)
	}
	return credential, nil
}

func (service *ResourceManagement) createCredential(
	ctx context.Context,
	db storage.Executor,
	resource models.ResourceEntity,
	input ResourceCredentialInput,
) (models.ResourceCredentialEntity, error) {
	definition, ok := models.FindResourceEngine(resource.Engine())
	if !ok {
		return models.ResourceCredentialEntity{}, domainError(
			"kind",
			"unsupported",
			"resource kind is not supported",
		)
	}
	if resource.Engine() == "postgresql" {
		if err := service.validatePostgreSQLCredential(ctx, db, resource, input, nil); err != nil {
			return models.ResourceCredentialEntity{}, err
		}
	}
	encrypted, digest, err := service.credentialPayload(input, definition)
	if err != nil {
		return models.ResourceCredentialEntity{}, err
	}
	created, err := models.ResourceCredential.Create(ctx, db, models.CreateResourceCredentialData{
		Name:       input.Name,
		Username:   nullableString(input.Username),
		Metadata:   normalizedJSON(input.Metadata),
		EncPayload: encrypted,
		Digest:     digest,
		ResourceID: resource.ID,
	})
	return created, mapResourceConflict(err)
}

func (service *ResourceManagement) validatePostgreSQLCredential(
	ctx context.Context,
	db storage.Executor,
	resource models.ResourceEntity,
	input ResourceCredentialInput,
	credentialID *uuid.UUID,
) error {
	username := strings.TrimSpace(input.Username)
	if username == "" {
		return domainError("username", "required", "PostgreSQL credentials require a username")
	}
	purpose := resourceCredentialMetadataPurpose(input.Metadata)
	if purpose != "administrator" && purpose != "application" {
		return domainError(
			"metadata.purpose",
			"unsupported",
			"PostgreSQL credential purpose must be administrator or application",
		)
	}
	if purpose == "application" &&
		!resourceHasDatabase(resource, resourceCredentialMetadataDatabase(input.Metadata)) {
		return domainError(
			"metadata.database",
			"database",
			"application credentials must select a configured Resource database",
		)
	}

	counts, err := models.ResourceCredential.PostgreSQLCounts(
		ctx, db, resource.ID, username, credentialID,
	)
	if err != nil {
		return err
	}
	administrators := counts.Administrators
	if purpose == "administrator" {
		administrators++
	}
	if administrators == 0 {
		return domainError(
			"metadata.purpose",
			"required",
			"PostgreSQL Resources must retain an administrator credential",
		)
	}
	if administrators > 1 {
		return domainError(
			"metadata.purpose",
			"unique",
			"database Resource already has an administrator credential",
		)
	}
	if counts.Usernames != 0 {
		return domainError(
			"username",
			"unique",
			"an active PostgreSQL credential already uses this username",
		)
	}
	return nil
}

func (service *ResourceManagement) UpdateCredentialMetadata(
	ctx context.Context,
	resourceID, credentialID uuid.UUID,
	input ResourceCredentialInput,
) (models.ResourceCredentialEntity, error) {
	return service.updateCredential(ctx, resourceID, credentialID, input, false)
}

func (service *ResourceManagement) RotateCredential(
	ctx context.Context,
	resourceID, credentialID uuid.UUID,
	input ResourceCredentialInput,
) (models.ResourceCredentialEntity, error) {
	return service.updateCredential(ctx, resourceID, credentialID, input, true)
}

func (service *ResourceManagement) updateCredential(
	ctx context.Context,
	resourceID, credentialID uuid.UUID,
	input ResourceCredentialInput,
	rotate bool,
) (models.ResourceCredentialEntity, error) {
	tx, err := service.db.BeginTx(ctx, nil)
	if err != nil {
		return models.ResourceCredentialEntity{}, err
	}
	defer tx.Rollback()
	resource, err := service.loadResource(ctx, tx, resourceID, true)
	if err != nil {
		return models.ResourceCredentialEntity{}, err
	}
	current, err := models.ResourceCredential.LockActiveForResource(
		ctx,
		tx,
		resourceID,
		credentialID,
	)
	if errors.Is(err, sql.ErrNoRows) {
		return models.ResourceCredentialEntity{}, models.ErrNotFound
	}
	if err != nil {
		return models.ResourceCredentialEntity{}, err
	}
	if resource.Engine() == "postgresql" {
		if !current.Username.Valid ||
			!strings.EqualFold(
				strings.TrimSpace(current.Username.String),
				strings.TrimSpace(input.Username),
			) {
			return models.ResourceCredentialEntity{}, domainError(
				"username",
				"immutable",
				"PostgreSQL credential usernames cannot be changed",
			)
		}
		if err := service.validatePostgreSQLCredential(
			ctx,
			tx,
			resource,
			input,
			&current.ID,
		); err != nil {
			return models.ResourceCredentialEntity{}, err
		}
	}
	encrypted, digest := current.EncPayload, current.Digest
	if rotate {
		definition, _ := models.FindResourceEngine(resource.Engine())
		encrypted, digest, err = service.credentialPayload(input, definition)
		if err != nil {
			return models.ResourceCredentialEntity{}, err
		}
	}
	candidate := models.ResourceCredentialEntity{
		ID: current.ID, Name: input.Name, Username: nullableString(input.Username),
		Metadata: normalizedJSON(input.Metadata), EncPayload: encrypted, Digest: digest,
		ArchivedAt: current.ArchivedAt, ResourceID: resourceID,
	}
	if err := candidate.Validate(); err != nil {
		return models.ResourceCredentialEntity{}, errors.Join(models.ErrDomainValidation, err)
	}
	reconciledPostgreSQL := false
	if resource.Engine() == "postgresql" {
		if err := service.reconcilePostgreSQLCredential(
			ctx,
			tx,
			resource,
			candidate,
			&current,
		); err != nil {
			return models.ResourceCredentialEntity{}, err
		}
		reconciledPostgreSQL = true
	}
	compensatePostgreSQL := func(db storage.Executor) error {
		if !reconciledPostgreSQL || !current.Username.Valid || !candidate.Username.Valid ||
			!strings.EqualFold(
				strings.TrimSpace(current.Username.String),
				strings.TrimSpace(candidate.Username.String),
			) {
			return nil
		}
		return service.reconcilePostgreSQLCredential(
			context.WithoutCancel(ctx),
			db,
			resource,
			current,
			&candidate,
		)
	}
	updated, err := models.ResourceCredential.Update(ctx, tx, models.UpdateResourceCredentialData{
		ID: current.ID, Name: input.Name, Username: nullableString(input.Username),
		Metadata: normalizedJSON(input.Metadata), EncPayload: encrypted, Digest: digest,
		ArchivedAt: current.ArchivedAt, ResourceID: resourceID,
	})
	if err != nil {
		return models.ResourceCredentialEntity{}, errors.Join(
			mapResourceConflict(err),
			compensatePostgreSQL(service.db.Executor()),
		)
	}
	connections, err := models.EnvironmentResource.ActiveForCredentialID(ctx, tx, credentialID)
	if err != nil {
		return models.ResourceCredentialEntity{}, errors.Join(
			err,
			compensatePostgreSQL(service.db.Executor()),
		)
	}
	if err := service.reconcileEnvironmentResourceConnections(
		ctx,
		tx,
		resource,
		connections,
	); err != nil {
		return models.ResourceCredentialEntity{}, errors.Join(
			err,
			compensatePostgreSQL(service.db.Executor()),
		)
	}
	if err := tx.Commit(); err != nil {
		return models.ResourceCredentialEntity{}, errors.Join(
			mapResourceConflict(err),
			compensatePostgreSQL(service.db.Executor()),
		)
	}
	return updated, nil
}

func connectionEnvironmentKeys(
	resource models.ResourceEntity,
	configuration environmentResourceConfiguration,
) map[string]string {
	keys := resource.EnvironmentKeys()
	maps.Copy(keys, configuration.EnvironmentKeyOverrides)
	return keys
}

func normalizeConnectionEnvironmentKeys(
	resource models.ResourceEntity,
	requested map[string]string,
) (map[string]string, map[string]string, error) {
	configuration := resource.ParsedConfiguration()
	configuration.EnvironmentKeys = maps.Clone(requested)
	encoded, err := json.Marshal(configuration)
	if err != nil {
		return nil, nil, err
	}
	candidate := resource
	candidate.Configuration = encoded
	if err := validation.Validate(&candidate); err != nil {
		return nil, nil, errors.Join(models.ErrDomainValidation, err)
	}
	effective := candidate.EnvironmentKeys()
	defaults := resource.EnvironmentKeys()
	overrides := make(map[string]string)
	for logicalName, key := range effective {
		if defaults[logicalName] != key {
			overrides[logicalName] = key
		}
	}
	return effective, overrides, nil
}

func (service *ResourceManagement) UpdateConnectionEnvironmentKeys(
	ctx context.Context,
	resourceID, connectionID uuid.UUID,
	requested map[string]string,
) error {
	tx, err := service.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	resource, err := service.loadResource(ctx, tx, resourceID, true)
	if err != nil {
		return err
	}
	connection, err := models.EnvironmentResource.LockActiveConnection(
		ctx,
		tx,
		resourceID,
		connectionID,
	)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return errors.Join(
				models.ErrDomainValidation,
				errors.New("Resource connection is unavailable"),
			)
		}
		return err
	}
	effectiveKeys, overrides, err := normalizeConnectionEnvironmentKeys(resource, requested)
	if err != nil {
		return err
	}
	configuration, err := parseEnvironmentResourceConfiguration(connection.Configuration)
	if err != nil {
		return err
	}
	endpoint, err := models.ResourceEndpoint.Find(ctx, tx, connection.ResourceEndpointID)
	if err != nil || endpoint.ArchivedAt.Valid || endpoint.ResourceID != resource.ID {
		return errors.New("Environment Resource endpoint is unavailable during projection")
	}
	var credential *models.ResourceCredentialEntity
	credentialValues := make(map[string]string)
	if connection.ResourceCredentialID != nil {
		selected, findErr := models.ResourceCredential.Find(
			ctx,
			tx,
			*connection.ResourceCredentialID,
		)
		if findErr != nil || selected.ArchivedAt.Valid || selected.ResourceID != resource.ID {
			return errors.New("Environment Resource credential is unavailable during projection")
		}
		credentialValues, err = service.credentialSecretValues(selected)
		if err != nil {
			return err
		}
		credential = &selected
	}
	values, projectedKeys, err := service.resourceProjectionValuesForEnvironment(
		connection.EnvironmentID,
		resource,
		endpoint,
		credential,
		credentialValues,
		configuration.CredentialProjection,
		effectiveKeys,
	)
	if err != nil {
		return err
	}
	database := ""
	if credential != nil {
		database = resourceCredentialMetadataDatabase(credential.Metadata)
	}
	if err := service.secrets.ReconcileManagedResource(
		ctx,
		tx,
		connection,
		values,
		projectedKeys,
		overrides,
		database,
	); err != nil {
		return err
	}
	return tx.Commit()
}

func (service *ResourceManagement) reconcileEnvironmentResourceConnections(
	ctx context.Context,
	db storage.Executor,
	resource models.ResourceEntity,
	connections []models.EnvironmentResourceEntity,
) error {
	for _, connection := range connections {
		if connection.ResourceID != resource.ID {
			return errors.New(
				"Environment Resource connection does not belong to the changed Resource",
			)
		}
		endpoint, err := models.ResourceEndpoint.Find(ctx, db, connection.ResourceEndpointID)
		if err != nil || endpoint.ArchivedAt.Valid || endpoint.ResourceID != resource.ID {
			return errors.New("Environment Resource endpoint is unavailable during projection")
		}
		var credential *models.ResourceCredentialEntity
		credentialValues := make(map[string]string)
		if connection.ResourceCredentialID != nil {
			selected, findErr := models.ResourceCredential.Find(
				ctx,
				db,
				*connection.ResourceCredentialID,
			)
			if findErr != nil || selected.ArchivedAt.Valid || selected.ResourceID != resource.ID {
				return errors.New(
					"Environment Resource credential is unavailable during projection",
				)
			}
			credentialValues, err = service.credentialSecretValues(selected)
			if err != nil {
				return err
			}
			credential = &selected
		}
		configuration, err := parseEnvironmentResourceConfiguration(connection.Configuration)
		if err != nil {
			return err
		}
		effectiveKeys := connectionEnvironmentKeys(resource, configuration)
		values, environmentKeys, err := service.resourceProjectionValuesForEnvironment(
			connection.EnvironmentID,
			resource,
			endpoint,
			credential,
			credentialValues,
			configuration.CredentialProjection,
			effectiveKeys,
		)
		if err != nil {
			return err
		}
		database := ""
		if credential != nil {
			database = resourceCredentialMetadataDatabase(credential.Metadata)
		}
		if err := service.secrets.ReconcileManagedResource(
			ctx,
			db,
			connection,
			values,
			environmentKeys,
			configuration.EnvironmentKeyOverrides,
			database,
		); err != nil {
			return err
		}
	}
	return nil
}

func (service *ResourceManagement) resourceProjectionValuesForEnvironment(
	environmentID uuid.UUID,
	resource models.ResourceEntity,
	endpoint models.ResourceEndpointEntity,
	credential *models.ResourceCredentialEntity,
	credentialValues map[string]string,
	projection string,
	resourceKeys map[string]string,
) (map[string]string, map[string]string, error) {
	if resource.Engine() == "opentelemetry" {
		if credential == nil ||
			resourceCredentialMetadataEnvironmentID(credential.Metadata) != environmentID {
			return nil, nil, errors.New(
				"OpenTelemetry Resource credential does not belong to this Environment",
			)
		}
	}
	identityToken := credentialValues["token"]
	return resourceProjectionValues(
		resource,
		endpoint,
		credential,
		credentialValues,
		projection,
		resourceKeys,
		identityToken,
	)
}

func (service *ResourceManagement) ArchiveCredential(
	ctx context.Context,
	resourceID, credentialID uuid.UUID,
) error {
	tx, err := service.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if _, err := service.loadResource(ctx, tx, resourceID, true); err != nil {
		return err
	}
	_, err = models.ResourceCredential.LockActiveForResource(ctx, tx, resourceID, credentialID)
	if errors.Is(err, sql.ErrNoRows) {
		return models.ErrNotFound
	}
	if err != nil {
		return err
	}
	dependencies, err := models.ResourceCredential.ActiveDependencyCount(ctx, tx, credentialID)
	if err != nil {
		return err
	}
	if dependencies > 0 {
		return domainError(
			"credential",
			"dependency",
			"credential is selected by an active binding or health check",
		)
	}
	now := time.Now().UTC()
	if err := models.ResourceCredential.ArchiveID(ctx, tx, credentialID, now); err != nil {
		return err
	}
	return tx.Commit()
}

func (service *ResourceManagement) CreateInstallation(
	ctx context.Context,
	resourceID uuid.UUID,
	input ResourceInstallationInput,
) (models.ResourceInstallationEntity, error) {
	tx, err := service.db.BeginTx(ctx, nil)
	if err != nil {
		return models.ResourceInstallationEntity{}, err
	}
	defer tx.Rollback()
	resource, err := service.loadResource(ctx, tx, resourceID, true)
	if err != nil {
		return models.ResourceInstallationEntity{}, err
	}
	installation, err := service.createInstallation(ctx, tx, resource, input)
	if err != nil {
		return models.ResourceInstallationEntity{}, err
	}
	endpoint, err := models.ResourceEndpoint.FindActivePrimaryPublic(ctx, tx, resource.ID)
	if errors.Is(err, sql.ErrNoRows) {
		endpoint, err = service.createManagedPrimaryEndpoint(ctx, tx, resource, installation)
	} else if err == nil {
		err = service.syncManagedEndpoints(ctx, tx, resource, installation)
	}
	if err != nil {
		return models.ResourceInstallationEntity{}, err
	}
	healthChecks, err := models.ResourceHealthCheck.ActiveKindCount(
		ctx,
		tx,
		resource.ID,
		resource.Engine(),
	)
	if err != nil {
		return models.ResourceInstallationEntity{}, err
	}
	if healthChecks == 0 {
		var credential *models.ResourceCredentialEntity
		if resource.ResourceType == models.ResourceTypeDatabase {
			administrator, credentialErr := service.resourceAdministratorCredential(
				ctx,
				tx,
				resource.ID,
			)
			if credentialErr != nil {
				return models.ResourceInstallationEntity{}, credentialErr
			}
			credential = &administrator
		}
		healthInput := defaultResourceHealthCheckInput(resource, endpoint, credential)
		if healthInput != nil {
			if _, err := service.createHealthCheck(ctx, tx, resource, *healthInput); err != nil {
				return models.ResourceInstallationEntity{}, err
			}
		}
	}
	if err := tx.Commit(); err != nil {
		return models.ResourceInstallationEntity{}, mapResourceConflict(err)
	}
	return installation, nil
}

func (service *ResourceManagement) createInstallation(
	ctx context.Context,
	db storage.Executor,
	resource models.ResourceEntity,
	input ResourceInstallationInput,
) (models.ResourceInstallationEntity, error) {
	installations, err := models.ResourceInstallation.ActiveCountForResource(ctx, db, resource.ID)
	if err != nil {
		return models.ResourceInstallationEntity{}, err
	}
	if installations > 0 {
		return models.ResourceInstallationEntity{}, domainError(
			"installation",
			"topology",
			"only one active Docker installation is supported for a Resource right now",
		)
	}
	if err := service.validatePlacement(
		ctx,
		db,
		input.ServerID,
		managedResourceCapability(resource.Engine()),
	); err != nil {
		return models.ResourceInstallationEntity{}, err
	}
	if input.RegistryCredentialID != nil {
		exists, err := models.Credential.ActiveExists(ctx, db, *input.RegistryCredentialID)
		if err != nil {
			return models.ResourceInstallationEntity{}, err
		}
		if err := requireChild(
			boolCount(exists),
			"registryCredentialId",
			"registry credential is unavailable",
		); err != nil {
			return models.ResourceInstallationEntity{}, err
		}
	}
	configuration, err := resourceInstallationConfiguration(input)
	if err != nil {
		return models.ResourceInstallationEntity{}, err
	}
	if _, err := managedPrimaryPortMapping(resource.Engine(), configuration); err != nil {
		return models.ResourceInstallationEntity{}, err
	}
	created, err := models.ResourceInstallation.Create(
		ctx,
		db,
		models.CreateResourceInstallationData{
			ImageReference: input.ImageReference, ImageDigest: nullableString(input.ImageDigest),
			ContainerName: input.ContainerName, RestartPolicy: input.RestartPolicy,
			Configuration: configuration, ResourceID: resource.ID,
			ServerID: input.ServerID, RegistryCredentialID: input.RegistryCredentialID,
		},
	)
	return created, mapResourceConflict(err)
}

func (service *ResourceManagement) UpdateInstallation(
	ctx context.Context,
	resourceID, installationID uuid.UUID,
	input ResourceInstallationInput,
) (models.ResourceInstallationEntity, error) {
	tx, err := service.db.BeginTx(ctx, nil)
	if err != nil {
		return models.ResourceInstallationEntity{}, err
	}
	defer tx.Rollback()
	if err := service.requireNoActiveRestore(ctx, tx, resourceID, &installationID); err != nil {
		return models.ResourceInstallationEntity{}, err
	}
	resource, err := service.loadResource(ctx, tx, resourceID, true)
	if err != nil {
		return models.ResourceInstallationEntity{}, err
	}
	current, err := models.ResourceInstallation.LockActiveForResource(
		ctx,
		tx,
		resourceID,
		installationID,
	)
	if errors.Is(err, sql.ErrNoRows) {
		return models.ResourceInstallationEntity{}, models.ErrNotFound
	}
	if err != nil {
		return models.ResourceInstallationEntity{}, err
	}
	if err := service.validatePlacement(
		ctx,
		tx,
		input.ServerID,
		managedResourceCapability(resource.Engine()),
	); err != nil {
		return models.ResourceInstallationEntity{}, err
	}
	if input.RegistryCredentialID != nil {
		exists, countErr := models.Credential.ActiveExists(ctx, tx, *input.RegistryCredentialID)
		if countErr != nil {
			return models.ResourceInstallationEntity{}, countErr
		}
		if err := requireChild(
			boolCount(exists),
			"registryCredentialId",
			"registry credential is unavailable",
		); err != nil {
			return models.ResourceInstallationEntity{}, err
		}
	}
	if current.ServerID != input.ServerID {
		activePolicy, policyErr := service.installationHasActiveBackupPolicy(
			ctx,
			tx,
			installationID,
		)
		if policyErr != nil {
			return models.ResourceInstallationEntity{}, policyErr
		}
		if activePolicy {
			return models.ResourceInstallationEntity{}, domainError(
				"serverId",
				"backup_policy",
				"pause or archive the active backup policy before moving this installation",
			)
		}
		if err := service.validateInstallationMove(
			ctx,
			tx,
			installationID,
			input.ServerID,
		); err != nil {
			return models.ResourceInstallationEntity{}, err
		}
	}
	configuration, err := resourceInstallationConfiguration(input)
	if err != nil {
		return models.ResourceInstallationEntity{}, err
	}
	currentMapping, err := managedPrimaryPortMapping(resource.Engine(), current.Configuration)
	if err != nil {
		return models.ResourceInstallationEntity{}, err
	}
	nextMapping, err := managedPrimaryPortMapping(resource.Engine(), configuration)
	if err != nil {
		return models.ResourceInstallationEntity{}, err
	}
	privateEndpoints, err := models.ResourceEndpoint.ActivePrivateCount(
		ctx,
		tx,
		resourceID,
		uuid.Nil,
	)
	if err != nil {
		return models.ResourceInstallationEntity{}, err
	}
	if privateEndpoints > 0 &&
		(current.ServerID != input.ServerID || currentMapping.HostPort != nextMapping.HostPort) {
		return models.ResourceInstallationEntity{}, domainError(
			"installation",
			"private_access",
			"remove this Resource from its private network before changing the installation Server or host port",
		)
	}
	updated, err := models.ResourceInstallation.Update(
		ctx,
		tx,
		models.UpdateResourceInstallationData{
			ID:                   current.ID,
			ImageReference:       input.ImageReference,
			ImageDigest:          nullableString(input.ImageDigest),
			ContainerName:        input.ContainerName,
			RestartPolicy:        input.RestartPolicy,
			Configuration:        configuration,
			ArchivedAt:           current.ArchivedAt,
			ResourceID:           resourceID,
			ServerID:             input.ServerID,
			RegistryCredentialID: input.RegistryCredentialID,
		},
	)
	if err != nil {
		return models.ResourceInstallationEntity{}, mapResourceConflict(err)
	}
	if err := service.syncManagedEndpoints(ctx, tx, resource, updated); err != nil {
		return models.ResourceInstallationEntity{}, err
	}
	if err := tx.Commit(); err != nil {
		return models.ResourceInstallationEntity{}, mapResourceConflict(err)
	}
	return updated, nil
}

func (service *ResourceManagement) RunInstallation(
	ctx context.Context,
	resourceID, installationID uuid.UUID,
) error {
	if err := service.requireNoActiveRestore(
		ctx,
		service.db.Executor(),
		resourceID,
		&installationID,
	); err != nil {
		return err
	}
	resource, err := service.loadResource(ctx, service.db.Executor(), resourceID, false)
	if err != nil {
		return err
	}
	installation, err := models.ResourceInstallation.FindActiveForResourceID(
		ctx, service.db.Executor(), resourceID, installationID,
	)
	if errors.Is(err, sql.ErrNoRows) {
		return models.ErrNotFound
	}
	if err != nil {
		return err
	}
	if installation.RegistryCredentialID != nil {
		return errors.New("running an installation with a registry credential is not supported yet")
	}

	mapping, err := managedPrimaryPortMapping(resource.Engine(), installation.Configuration)
	if err != nil {
		return err
	}
	portMappings := []containerclient.PortMapping{
		{
			HostPort:      mapping.HostPort,
			ContainerPort: mapping.ContainerPort,
			Protocol:      mapping.Protocol,
		},
	}

	volumeMounts, err := models.ResourceVolumeMount.ActiveWithVolumeForInstallation(
		ctx, service.db.Executor(), installationID,
	)
	if err != nil {
		return err
	}
	mounts := make([]containerclient.VolumeMount, 0, len(volumeMounts))
	for _, mount := range volumeMounts {
		mounts = append(mounts, containerclient.VolumeMount{
			Name: mount.Name, MountPath: mount.MountPath, ReadOnly: mount.ReadOnly,
		})
	}

	environment := make(map[string]string)
	if resource.Engine() == "postgresql" || resource.Engine() == "clickhouse" {
		administrator, adminErr := service.resourceAdministratorCredential(
			ctx,
			service.db.Executor(),
			resource.ID,
		)
		if adminErr != nil {
			return adminErr
		}
		values, valuesErr := service.credentialSecretValues(administrator)
		if valuesErr != nil {
			return valuesErr
		}
		if !administrator.Username.Valid ||
			strings.TrimSpace(administrator.Username.String) == "" ||
			values["password"] == "" {
			return fmt.Errorf(
				"%s Resource administrator credential is incomplete",
				resource.Engine(),
			)
		}
		switch resource.Engine() {
		case "postgresql":
			environment["POSTGRES_USER"] = administrator.Username.String
			environment["POSTGRES_PASSWORD"] = values["password"]
		case "clickhouse":
			environment["CLICKHOUSE_USER"] = administrator.Username.String
			environment["CLICKHOUSE_PASSWORD"] = values["password"]
		}
	}
	imageReference := installation.ImageReference
	if installation.ImageDigest.Valid && !strings.Contains(imageReference, "@") {
		imageReference += "@" + installation.ImageDigest.String
	}
	if err := service.container.Run(
		ctx,
		installation.ServerID,
		managedResourceCapability(resource.Engine()),
		containerclient.RunSpec{
			InstallationID: installation.ID.String(), ContainerName: installation.ContainerName,
			ImageReference: imageReference, RestartPolicy: installation.RestartPolicy,
			PortMappings: portMappings, VolumeMounts: mounts, Environment: environment,
		},
	); err != nil {
		return err
	}
	status, err := service.observeDockerInstallation(ctx, installation)
	if err != nil {
		return err
	}
	if status.ServiceState != "running" {
		var state containerclient.State
		_ = json.Unmarshal(status.Details, &state)
		return fmt.Errorf(
			"container did not stay running: state %s, exit code %d; open container logs for details",
			status.ServiceState,
			state.ExitCode,
		)
	}
	return nil
}

func (service *ResourceManagement) StopInstallation(
	ctx context.Context,
	resourceID, installationID uuid.UUID,
) error {
	return service.controlInstallation(ctx, resourceID, installationID, "stop")
}

func (service *ResourceManagement) RestartInstallation(
	ctx context.Context,
	resourceID, installationID uuid.UUID,
) error {
	return service.controlInstallation(ctx, resourceID, installationID, "restart")
}

func (service *ResourceManagement) RemoveInstallationContainer(
	ctx context.Context,
	resourceID, installationID uuid.UUID,
) error {
	return service.controlInstallation(ctx, resourceID, installationID, "remove")
}

func (service *ResourceManagement) controlInstallation(
	ctx context.Context,
	resourceID, installationID uuid.UUID,
	action string,
) error {
	if err := service.requireNoActiveRestore(
		ctx,
		service.db.Executor(),
		resourceID,
		&installationID,
	); err != nil {
		return err
	}
	installation, err := service.loadInstallationForControl(ctx, resourceID, installationID)
	if err != nil {
		return err
	}
	capability, err := service.resourceCapability(ctx, resourceID)
	if err != nil {
		return err
	}
	switch action {
	case "stop":
		err = service.container.Stop(
			ctx,
			installation.ServerID,
			capability,
			installation.ID.String(),
			installation.ContainerName,
		)
	case "restart":
		err = service.container.Restart(
			ctx,
			installation.ServerID,
			capability,
			installation.ID.String(),
			installation.ContainerName,
		)
	case "remove":
		err = service.container.Remove(
			ctx,
			installation.ServerID,
			capability,
			installation.ID.String(),
			installation.ContainerName,
		)
	default:
		err = errors.New("unsupported container action")
	}
	if err != nil {
		return err
	}
	_, err = service.observeDockerInstallation(ctx, installation)
	return err
}

func (service *ResourceManagement) loadInstallationForControl(
	ctx context.Context,
	resourceID, installationID uuid.UUID,
) (models.ResourceInstallationEntity, error) {
	if _, err := service.loadResource(ctx, service.db.Executor(), resourceID, false); err != nil {
		return models.ResourceInstallationEntity{}, err
	}
	installation, err := models.ResourceInstallation.FindActiveForResourceID(
		ctx, service.db.Executor(), resourceID, installationID,
	)
	if errors.Is(err, sql.ErrNoRows) {
		return models.ResourceInstallationEntity{}, models.ErrNotFound
	}
	return installation, err
}

func (service *ResourceManagement) InstallationLogs(
	ctx context.Context,
	resourceID, installationID uuid.UUID,
	tail int,
) (string, error) {
	installation, err := service.loadInstallationForControl(ctx, resourceID, installationID)
	if err != nil {
		return "", err
	}
	capability, err := service.resourceCapability(ctx, resourceID)
	if err != nil {
		return "", err
	}
	return service.container.Logs(
		ctx,
		installation.ServerID,
		capability,
		installation.ID.String(),
		installation.ContainerName,
		tail,
	)
}

func (service *ResourceManagement) observeInstallation(
	ctx context.Context,
	detail *models.ResourceInstallationDetail,
) {
	detail.CanControl = true
	status, err := service.observeDockerInstallation(ctx, detail.ResourceInstallationEntity)
	if err != nil {
		detail.State = "unavailable"
		detail.ServiceState = "unknown"
		detail.Health = "unknown"
		detail.HealthReason = err.Error()
		detail.ObservedAt = sql.NullTime{Time: time.Now().UTC(), Valid: true}
		return
	}
	detail.State = status.State
	detail.ServiceState = status.ServiceState
	detail.Health = status.Health
	detail.HealthReason = status.HealthReason.String
	detail.ContainerDetails = status.Details
	detail.ObservedAt = sql.NullTime{Time: status.ObservedAt, Valid: true}
}

func (service *ResourceManagement) observeDockerInstallation(
	ctx context.Context,
	installation models.ResourceInstallationEntity,
) (models.ResourceInstallationStatusEntity, error) {
	capability, err := service.resourceCapability(ctx, installation.ResourceID)
	if err != nil {
		return models.ResourceInstallationStatusEntity{}, err
	}
	state, err := service.container.Inspect(
		ctx,
		installation.ServerID,
		capability,
		installation.ID.String(),
		installation.ContainerName,
	)
	if err != nil {
		return models.ResourceInstallationStatusEntity{}, err
	}
	details, err := json.Marshal(state)
	if err != nil {
		return models.ResourceInstallationStatusEntity{}, err
	}
	now := time.Now().UTC()
	serviceState := state.Status
	stateValue := "installed"
	health := state.Health
	reason := state.Error
	if !state.Exists {
		stateValue = "missing"
		serviceState = "not-created"
		health = "unknown"
		reason = "No Docker container exists for this installation."
	} else if health == "" {
		health = "unknown"
	}
	status, err := models.ResourceInstallationStatus.Upsert(
		ctx,
		service.db.Executor(),
		models.CreateResourceInstallationStatusData{
			ResourceInstallationID: installation.ID,
			ExternalID:             nullableString(state.ID), State: stateValue,
			InstalledVersion: nullableString(state.ImageID), ServiceState: serviceState,
			Health: health, Source: "docker", HealthReason: nullableString(reason),
			Details: details, ObservedAt: now, ExpiresAt: now.Add(30 * time.Second),
		},
	)
	return status, err
}

func (service *ResourceManagement) DeployResource(ctx context.Context, resourceID uuid.UUID) error {
	_, err := service.loadResource(ctx, service.db.Executor(), resourceID, false)
	if err != nil {
		return err
	}
	installations, err := models.ResourceInstallation.ActiveForResource(
		ctx, service.db.Executor(), resourceID,
	)
	if err != nil {
		return err
	}
	if len(installations) == 0 {
		return errors.New("Resource has no active installations to deploy")
	}
	for _, installation := range installations {
		if err := service.RunInstallation(ctx, resourceID, installation.ID); err != nil {
			return fmt.Errorf("deploy installation %q: %w", installation.ContainerName, err)
		}
	}
	return nil
}

func (service *ResourceManagement) validatePlacement(
	ctx context.Context,
	db storage.Executor,
	serverID uuid.UUID,
	capability models.ServerCapability,
) error {
	_, err := models.RequireServerCapability(ctx, db, serverID, capability)
	return err
}

func managedResourceCapability(kind string) models.ServerCapability {
	if kind == "registry" {
		return models.ServerCapabilityRepository
	}
	return models.ServerCapabilityResource
}

func (service *ResourceManagement) resourceCapability(
	ctx context.Context,
	resourceID uuid.UUID,
) (models.ServerCapability, error) {
	kind, err := models.Resource.EngineForID(ctx, service.db.Executor(), resourceID)
	if err != nil {
		return "", err
	}
	return managedResourceCapability(kind), nil
}

func (service *ResourceManagement) validateInstallationMove(
	ctx context.Context,
	db storage.Executor,
	installationID, targetServerID uuid.UUID,
) error {
	conflicts, err := models.ResourceInstallation.MoveConflicts(
		ctx,
		db,
		installationID,
		targetServerID,
	)
	if err != nil {
		return err
	}
	if conflicts.UnreachableNetworks > 0 {
		return domainError(
			"serverId",
			"network_topology",
			"target Server cannot reach every endpoint private network",
		)
	}
	if conflicts.IncompatibleVolumes > 0 {
		return domainError(
			"serverId",
			"storage_topology",
			"mounted server-local Volume requires explicit migration or reassignment",
		)
	}
	return nil
}

func (service *ResourceManagement) ArchiveInstallation(
	ctx context.Context,
	resourceID, installationID uuid.UUID,
) error {
	tx, err := service.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if err := service.requireNoActiveRestore(ctx, tx, resourceID, &installationID); err != nil {
		return err
	}
	if _, err := service.loadResource(ctx, tx, resourceID, true); err != nil {
		return err
	}
	_, err = models.ResourceInstallation.LockActiveForResource(ctx, tx, resourceID, installationID)
	if errors.Is(err, sql.ErrNoRows) {
		return models.ErrNotFound
	}
	if err != nil {
		return err
	}
	dependencies, err := models.ResourceInstallation.ActiveMountDependencyCount(
		ctx,
		tx,
		installationID,
	)
	if err != nil {
		return err
	}
	if dependencies > 0 {
		return domainError(
			"installation",
			"dependency",
			"installation has active endpoints, credentials, mounts, or health checks",
		)
	}
	activePolicy, err := service.installationHasActiveBackupPolicy(ctx, tx, installationID)
	if err != nil {
		return err
	}
	if activePolicy {
		return domainError(
			"installation",
			"backup_policy",
			"pause or archive the active backup policy before archiving this installation",
		)
	}
	now := time.Now().UTC()
	if err := models.ResourceInstallation.ArchiveID(ctx, tx, installationID, now); err != nil {
		return err
	}
	return tx.Commit()
}

func (service *ResourceManagement) installationHasActiveBackupPolicy(
	ctx context.Context,
	db storage.Executor,
	installationID uuid.UUID,
) (bool, error) {
	return false, nil
}

func (service *ResourceManagement) requireNoActiveRestore(
	ctx context.Context,
	db storage.Executor,
	resourceID uuid.UUID,
	installationID *uuid.UUID,
) error {
	count, err := models.ResourceRestore.ActiveCountForResource(ctx, db, resourceID)
	if err != nil {
		return err
	}
	if count > 0 {
		return domainError(
			"resource",
			"restore_active",
			"Resource lifecycle changes are unavailable while a database restore is active",
		)
	}
	return nil
}

func (service *ResourceManagement) CreateVolume(
	ctx context.Context,
	resourceID uuid.UUID,
	input ResourceVolumeInput,
) (models.ResourceVolumeEntity, error) {
	tx, err := service.db.BeginTx(ctx, nil)
	if err != nil {
		return models.ResourceVolumeEntity{}, err
	}
	defer tx.Rollback()
	resource, err := service.loadResource(ctx, tx, resourceID, true)
	if err != nil {
		return models.ResourceVolumeEntity{}, err
	}
	volume, err := service.createVolume(ctx, tx, resource, input)
	if err != nil {
		return models.ResourceVolumeEntity{}, err
	}
	if err := tx.Commit(); err != nil {
		return models.ResourceVolumeEntity{}, mapResourceConflict(err)
	}
	return volume, nil
}

func (service *ResourceManagement) createVolume(
	ctx context.Context,
	db storage.Executor,
	resource models.ResourceEntity,
	input ResourceVolumeInput,
) (models.ResourceVolumeEntity, error) {
	volumes, err := models.ResourceVolume.ActiveCountForResource(ctx, db, resource.ID)
	if err != nil {
		return models.ResourceVolumeEntity{}, err
	}
	if volumes > 0 {
		return models.ResourceVolumeEntity{}, domainError(
			"volume",
			"topology",
			"only one active volume is supported for a Resource right now",
		)
	}
	if err := service.validatePlacement(
		ctx,
		db,
		input.ServerID,
		managedResourceCapability(resource.Engine()),
	); err != nil {
		return models.ResourceVolumeEntity{}, err
	}
	created, err := models.ResourceVolume.Create(ctx, db, models.CreateResourceVolumeData{
		Name: input.Name, Driver: input.Driver, Configuration: normalizedJSON(input.Configuration),
		ResourceID: resource.ID, ServerID: input.ServerID,
	})
	return created, mapResourceConflict(err)
}

func (service *ResourceManagement) UpdateVolume(
	ctx context.Context,
	resourceID, volumeID uuid.UUID,
	input ResourceVolumeInput,
) (models.ResourceVolumeEntity, error) {
	tx, err := service.db.BeginTx(ctx, nil)
	if err != nil {
		return models.ResourceVolumeEntity{}, err
	}
	defer tx.Rollback()
	resource, err := service.loadResource(ctx, tx, resourceID, true)
	if err != nil {
		return models.ResourceVolumeEntity{}, err
	}
	current, err := models.ResourceVolume.LockActiveForResource(ctx, tx, resourceID, volumeID)
	if errors.Is(err, sql.ErrNoRows) {
		return models.ResourceVolumeEntity{}, models.ErrNotFound
	}
	if err != nil {
		return models.ResourceVolumeEntity{}, err
	}
	if err := service.validatePlacement(
		ctx,
		tx,
		input.ServerID,
		managedResourceCapability(resource.Engine()),
	); err != nil {
		return models.ResourceVolumeEntity{}, err
	}
	if current.ServerID != input.ServerID {
		mounts, countErr := models.ResourceVolumeMount.ActiveCountForVolume(ctx, tx, volumeID)
		if countErr != nil {
			return models.ResourceVolumeEntity{}, countErr
		}
		if mounts > 0 {
			return models.ResourceVolumeEntity{}, domainError(
				"serverId",
				"topology",
				"archive volume mounts before changing Servers",
			)
		}
	}
	updated, err := models.ResourceVolume.Update(ctx, tx, models.UpdateResourceVolumeData{
		ID:            current.ID,
		Name:          input.Name,
		Driver:        input.Driver,
		Configuration: normalizedJSON(input.Configuration),
		ArchivedAt:    current.ArchivedAt,
		ResourceID:    resourceID,
		ServerID:      input.ServerID,
	})
	if err != nil {
		return models.ResourceVolumeEntity{}, mapResourceConflict(err)
	}
	if err := tx.Commit(); err != nil {
		return models.ResourceVolumeEntity{}, mapResourceConflict(err)
	}
	return updated, nil
}

func (service *ResourceManagement) ArchiveVolume(
	ctx context.Context,
	resourceID, volumeID uuid.UUID,
) error {
	tx, err := service.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if _, err := service.loadResource(ctx, tx, resourceID, true); err != nil {
		return err
	}
	_, err = models.ResourceVolume.LockActiveForResource(ctx, tx, resourceID, volumeID)
	if errors.Is(err, sql.ErrNoRows) {
		return models.ErrNotFound
	}
	if err != nil {
		return err
	}
	mounts, err := models.ResourceVolumeMount.ActiveCountForVolume(ctx, tx, volumeID)
	if err != nil {
		return err
	}
	if mounts > 0 {
		return domainError("volume", "dependency", "volume has active mounts")
	}
	now := time.Now().UTC()
	if err := models.ResourceVolume.ArchiveID(ctx, tx, volumeID, now); err != nil {
		return err
	}
	return tx.Commit()
}

func (service *ResourceManagement) CreateMount(
	ctx context.Context,
	resourceID uuid.UUID,
	input ResourceMountInput,
) (models.ResourceVolumeMountEntity, error) {
	tx, err := service.db.BeginTx(ctx, nil)
	if err != nil {
		return models.ResourceVolumeMountEntity{}, err
	}
	defer tx.Rollback()
	resource, err := service.loadResource(ctx, tx, resourceID, true)
	if err != nil {
		return models.ResourceVolumeMountEntity{}, err
	}
	mount, err := service.createMount(ctx, tx, resource, input)
	if err != nil {
		return models.ResourceVolumeMountEntity{}, err
	}
	if err := tx.Commit(); err != nil {
		return models.ResourceVolumeMountEntity{}, mapResourceConflict(err)
	}
	return mount, nil
}

func (service *ResourceManagement) createMount(
	ctx context.Context,
	db storage.Executor,
	resource models.ResourceEntity,
	input ResourceMountInput,
) (models.ResourceVolumeMountEntity, error) {
	mounts, err := models.ResourceVolumeMount.ActiveCountForResource(ctx, db, resource.ID)
	if err != nil {
		return models.ResourceVolumeMountEntity{}, err
	}
	if mounts > 0 {
		return models.ResourceVolumeMountEntity{}, domainError(
			"mount",
			"topology",
			"only one active volume mount is supported for a Resource right now",
		)
	}
	topology, err := models.ResourceVolumeMount.Topology(
		ctx, db, input.ResourceVolumeID, input.ResourceInstallationID,
	)
	if errors.Is(err, sql.ErrNoRows) {
		return models.ResourceVolumeMountEntity{}, domainError(
			"mount",
			"topology",
			"volume and installation must be active",
		)
	}
	if err != nil {
		return models.ResourceVolumeMountEntity{}, err
	}
	if topology.VolumeResourceID != resource.ID || topology.InstallationResourceID != resource.ID ||
		topology.VolumeServerID != topology.InstallationServerID {
		return models.ResourceVolumeMountEntity{}, domainError(
			"mount",
			"topology",
			"volume and installation must belong to the same Resource and Server",
		)
	}
	created, err := models.ResourceVolumeMount.Create(ctx, db, models.CreateResourceVolumeMountData{
		MountPath:              input.MountPath,
		ReadOnly:               input.ReadOnly,
		ResourceVolumeID:       input.ResourceVolumeID,
		ResourceInstallationID: input.ResourceInstallationID,
	})
	return created, mapResourceConflict(err)
}

func (service *ResourceManagement) UpdateMount(
	ctx context.Context,
	resourceID, mountID uuid.UUID,
	input ResourceMountInput,
) (models.ResourceVolumeMountEntity, error) {
	tx, err := service.db.BeginTx(ctx, nil)
	if err != nil {
		return models.ResourceVolumeMountEntity{}, err
	}
	defer tx.Rollback()
	resource, err := service.loadResource(ctx, tx, resourceID, true)
	if err != nil {
		return models.ResourceVolumeMountEntity{}, err
	}
	current, err := models.ResourceVolumeMount.LockActive(ctx, tx, mountID)
	if errors.Is(err, sql.ErrNoRows) {
		return models.ResourceVolumeMountEntity{}, models.ErrNotFound
	}
	if err != nil {
		return models.ResourceVolumeMountEntity{}, err
	}
	owned, err := models.ResourceVolume.OwnedByResource(
		ctx,
		tx,
		current.ResourceVolumeID,
		resourceID,
	)
	if err != nil || !owned {
		if err != nil {
			return models.ResourceVolumeMountEntity{}, err
		}
		return models.ResourceVolumeMountEntity{}, models.ErrNotFound
	}
	if _, err := service.createMountValidation(ctx, tx, resource, input); err != nil {
		return models.ResourceVolumeMountEntity{}, err
	}
	updated, err := models.ResourceVolumeMount.Update(ctx, tx, models.UpdateResourceVolumeMountData{
		ID:                     current.ID,
		MountPath:              input.MountPath,
		ReadOnly:               input.ReadOnly,
		ArchivedAt:             current.ArchivedAt,
		ResourceVolumeID:       input.ResourceVolumeID,
		ResourceInstallationID: input.ResourceInstallationID,
	})
	if err != nil {
		return models.ResourceVolumeMountEntity{}, mapResourceConflict(err)
	}
	if err := tx.Commit(); err != nil {
		return models.ResourceVolumeMountEntity{}, mapResourceConflict(err)
	}
	return updated, nil
}

func (service *ResourceManagement) createMountValidation(
	ctx context.Context,
	db storage.Executor,
	resource models.ResourceEntity,
	input ResourceMountInput,
) (models.ResourceVolumeMountEntity, error) {
	entity := models.ResourceVolumeMountEntity{
		MountPath:              input.MountPath,
		ReadOnly:               input.ReadOnly,
		ResourceVolumeID:       input.ResourceVolumeID,
		ResourceInstallationID: input.ResourceInstallationID,
	}
	if err := entity.Validate(); err != nil {
		return models.ResourceVolumeMountEntity{}, errors.Join(models.ErrDomainValidation, err)
	}
	topology, err := models.ResourceVolumeMount.Topology(
		ctx, db, input.ResourceVolumeID, input.ResourceInstallationID,
	)
	if errors.Is(err, sql.ErrNoRows) {
		return models.ResourceVolumeMountEntity{}, domainError(
			"mount",
			"topology",
			"volume and installation must be active",
		)
	}
	if err != nil {
		return models.ResourceVolumeMountEntity{}, err
	}
	if topology.VolumeResourceID != resource.ID || topology.InstallationResourceID != resource.ID ||
		topology.VolumeServerID != topology.InstallationServerID {
		return models.ResourceVolumeMountEntity{}, domainError(
			"mount",
			"topology",
			"volume and installation must belong to the same Resource and Server",
		)
	}
	return entity, nil
}

func (service *ResourceManagement) ArchiveMount(
	ctx context.Context,
	resourceID, mountID uuid.UUID,
) error {
	tx, err := service.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if _, err := service.loadResource(ctx, tx, resourceID, true); err != nil {
		return err
	}
	_, err = models.ResourceVolumeMount.LockActiveForResource(ctx, tx, resourceID, mountID)
	if errors.Is(err, sql.ErrNoRows) {
		return models.ErrNotFound
	}
	if err != nil {
		return err
	}
	now := time.Now().UTC()
	if err := models.ResourceVolumeMount.ArchiveID(ctx, tx, mountID, now); err != nil {
		return err
	}
	return tx.Commit()
}

func (service *ResourceManagement) CreateHealthCheck(
	ctx context.Context,
	resourceID uuid.UUID,
	input ResourceHealthCheckInput,
) (models.ResourceHealthCheckEntity, error) {
	tx, err := service.db.BeginTx(ctx, nil)
	if err != nil {
		return models.ResourceHealthCheckEntity{}, err
	}
	defer tx.Rollback()
	resource, err := service.loadResource(ctx, tx, resourceID, true)
	if err != nil {
		return models.ResourceHealthCheckEntity{}, err
	}
	check, err := service.createHealthCheck(ctx, tx, resource, input)
	if err != nil {
		return models.ResourceHealthCheckEntity{}, err
	}
	if err := tx.Commit(); err != nil {
		return models.ResourceHealthCheckEntity{}, mapResourceConflict(err)
	}
	return check, nil
}

func (service *ResourceManagement) createHealthCheck(
	ctx context.Context,
	db storage.Executor,
	resource models.ResourceEntity,
	input ResourceHealthCheckInput,
) (models.ResourceHealthCheckEntity, error) {
	entity := models.ResourceHealthCheckEntity{
		Name:                 input.Name,
		Kind:                 input.Kind,
		Configuration:        normalizedJSON(input.Configuration),
		IntervalSeconds:      input.IntervalSeconds,
		TimeoutSeconds:       input.TimeoutSeconds,
		FailureThreshold:     input.FailureThreshold,
		SuccessThreshold:     input.SuccessThreshold,
		Enabled:              input.Enabled,
		ResourceID:           resource.ID,
		ResourceEndpointID:   input.ResourceEndpointID,
		ResourceCredentialID: input.ResourceCredentialID,
	}
	if err := entity.ValidateForKind(resource.Engine()); err != nil {
		return models.ResourceHealthCheckEntity{}, errors.Join(models.ErrDomainValidation, err)
	}
	if err := service.validateHealthTopology(ctx, db, resource.ID, input); err != nil {
		return models.ResourceHealthCheckEntity{}, err
	}
	created, err := models.ResourceHealthCheck.Create(ctx, db, models.CreateResourceHealthCheckData{
		Name:                 entity.Name,
		Kind:                 entity.Kind,
		Configuration:        entity.Configuration,
		IntervalSeconds:      entity.IntervalSeconds,
		TimeoutSeconds:       entity.TimeoutSeconds,
		FailureThreshold:     entity.FailureThreshold,
		SuccessThreshold:     entity.SuccessThreshold,
		Enabled:              entity.Enabled,
		ResourceID:           entity.ResourceID,
		ResourceEndpointID:   entity.ResourceEndpointID,
		ResourceCredentialID: entity.ResourceCredentialID,
	})
	return created, mapResourceConflict(err)
}

func (service *ResourceManagement) validateHealthTopology(
	ctx context.Context,
	db storage.Executor,
	resourceID uuid.UUID,
	input ResourceHealthCheckInput,
) error {
	resourceType, err := models.Resource.TypeForID(ctx, db, resourceID)
	if err != nil {
		return err
	}
	if input.ResourceEndpointID == nil {
		return domainError("resourceEndpointId", "required", "health checks require an endpoint")
	}
	if resourceType == models.ResourceTypeDatabase && input.ResourceCredentialID == nil {
		return domainError(
			"resourceCredentialId",
			"required",
			"database Resource access checks require a credential",
		)
	}
	if input.ResourceEndpointID != nil {
		belongs, countErr := models.ResourceEndpoint.ActiveBelongsToResource(
			ctx, db, *input.ResourceEndpointID, resourceID,
		)
		if countErr != nil {
			return countErr
		}
		if err := requireChild(
			boolCount(belongs),
			"resourceEndpointId",
			"endpoint does not belong to this installation topology",
		); err != nil {
			return err
		}
	}
	if input.ResourceCredentialID != nil {
		belongs, countErr := models.ResourceCredential.ActiveBelongsToResource(
			ctx, db, *input.ResourceCredentialID, resourceID,
		)
		if countErr != nil {
			return countErr
		}
		if err := requireChild(
			boolCount(belongs),
			"resourceCredentialId",
			"credential does not belong to this installation topology",
		); err != nil {
			return err
		}
	}
	return nil
}

func (service *ResourceManagement) UpdateHealthCheck(
	ctx context.Context,
	resourceID, healthCheckID uuid.UUID,
	input ResourceHealthCheckInput,
) (models.ResourceHealthCheckEntity, error) {
	tx, err := service.db.BeginTx(ctx, nil)
	if err != nil {
		return models.ResourceHealthCheckEntity{}, err
	}
	defer tx.Rollback()
	resource, err := service.loadResource(ctx, tx, resourceID, true)
	if err != nil {
		return models.ResourceHealthCheckEntity{}, err
	}
	current, err := models.ResourceHealthCheck.LockActiveForResource(
		ctx, tx, resourceID, healthCheckID,
	)
	if errors.Is(err, sql.ErrNoRows) {
		return models.ResourceHealthCheckEntity{}, models.ErrNotFound
	}
	if err != nil {
		return models.ResourceHealthCheckEntity{}, err
	}
	entity := models.ResourceHealthCheckEntity{
		ID:                   current.ID,
		Name:                 input.Name,
		Kind:                 input.Kind,
		Configuration:        normalizedJSON(input.Configuration),
		IntervalSeconds:      input.IntervalSeconds,
		TimeoutSeconds:       input.TimeoutSeconds,
		FailureThreshold:     input.FailureThreshold,
		SuccessThreshold:     input.SuccessThreshold,
		Enabled:              input.Enabled,
		ArchivedAt:           current.ArchivedAt,
		ResourceID:           resource.ID,
		ResourceEndpointID:   input.ResourceEndpointID,
		ResourceCredentialID: input.ResourceCredentialID,
	}
	if err := entity.ValidateForKind(resource.Engine()); err != nil {
		return models.ResourceHealthCheckEntity{}, errors.Join(models.ErrDomainValidation, err)
	}
	if err := service.validateHealthTopology(ctx, tx, resourceID, input); err != nil {
		return models.ResourceHealthCheckEntity{}, err
	}
	updated, err := models.ResourceHealthCheck.Update(ctx, tx, models.UpdateResourceHealthCheckData{
		ID:                   current.ID,
		Name:                 entity.Name,
		Kind:                 entity.Kind,
		Configuration:        entity.Configuration,
		IntervalSeconds:      entity.IntervalSeconds,
		TimeoutSeconds:       entity.TimeoutSeconds,
		FailureThreshold:     entity.FailureThreshold,
		SuccessThreshold:     entity.SuccessThreshold,
		Enabled:              entity.Enabled,
		ArchivedAt:           current.ArchivedAt,
		ResourceID:           entity.ResourceID,
		ResourceEndpointID:   entity.ResourceEndpointID,
		ResourceCredentialID: entity.ResourceCredentialID,
	})
	if err != nil {
		return models.ResourceHealthCheckEntity{}, mapResourceConflict(err)
	}
	if err := tx.Commit(); err != nil {
		return models.ResourceHealthCheckEntity{}, mapResourceConflict(err)
	}
	return updated, nil
}

func (service *ResourceManagement) ArchiveHealthCheck(
	ctx context.Context,
	resourceID, healthCheckID uuid.UUID,
) error {
	tx, err := service.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if _, err := service.loadResource(ctx, tx, resourceID, true); err != nil {
		return err
	}
	_, err = models.ResourceHealthCheck.LockActiveForResource(ctx, tx, resourceID, healthCheckID)
	if errors.Is(err, sql.ErrNoRows) {
		return models.ErrNotFound
	}
	if err != nil {
		return err
	}
	now := time.Now().UTC()
	if err := models.ResourceHealthCheck.ArchiveID(ctx, tx, healthCheckID, now); err != nil {
		return err
	}
	return tx.Commit()
}

func requireChild(count int, field, message string) error {
	if count != 1 {
		return domainError(field, "topology", message)
	}
	return nil
}

func boolCount(value bool) int {
	if value {
		return 1
	}
	return 0
}
