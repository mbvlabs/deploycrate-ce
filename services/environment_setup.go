package services

import (
	"context"
	"database/sql"
	registryclient "deploycrate-ce/clients/registry"
	"deploycrate-ce/config"
	"deploycrate-ce/internal/storage"
	"deploycrate-ce/models"
	"encoding/json"
	"errors"
	"regexp"
	"strings"

	buildpacksclient "deploycrate-ce/clients/buildpacks"

	"github.com/google/uuid"
)

var goTargetsPattern = regexp.MustCompile(`^[A-Za-z0-9_./,:*-]*$`)

const (
	resourceCredentialProjectionConnectionURL   = "connection_url"
	resourceCredentialProjectionIndividualParts = "individual_parts"
)

type environmentResourceConfiguration struct {
	SchemaVersion           int               `json:"schema_version"`
	CredentialSource        string            `json:"credential_source"`
	CredentialProjection    string            `json:"credential_projection"`
	EnvironmentKeyOverrides map[string]string `json:"environment_key_overrides,omitempty"`
	EnvironmentKeys         map[string]string `json:"environment_keys"`
}

func encodeEnvironmentResourceConfiguration(
	projection string,
	credentialID *uuid.UUID,
	credentialSource string,
	environmentKeys, environmentKeyOverrides map[string]string,
) (json.RawMessage, error) {
	credentialSource = strings.ToLower(strings.TrimSpace(credentialSource))
	if credentialSource == "" && credentialID != nil {
		credentialSource = "managed"
	} else if credentialSource == "" {
		credentialSource = "none"
	}
	return json.Marshal(environmentResourceConfiguration{
		SchemaVersion: 1, CredentialSource: credentialSource,
		CredentialProjection: projection, EnvironmentKeys: environmentKeys,
		EnvironmentKeyOverrides: environmentKeyOverrides,
	})
}

func parseEnvironmentResourceConfiguration(
	raw json.RawMessage,
) (environmentResourceConfiguration, error) {
	var configuration environmentResourceConfiguration
	if json.Unmarshal(raw, &configuration) != nil || configuration.CredentialProjection == "" {
		return environmentResourceConfiguration{}, errors.New(
			"Environment Resource configuration is invalid",
		)
	}
	return configuration, nil
}

type EnvironmentSetup struct {
	db          storage.Pool
	queue       storage.InsertQueue
	jobControl  BuildJobControl
	github      *GitHubConnection
	secrets     *EnvironmentSecrets
	resources   *ResourceManagement
	dns         *EnvironmentDNS
	caddy       CaddyRouteService
	workloads   *WorkloadExecution
	buildpacks  buildpacksclient.Client
	servers     *ServerExecution
	builds      *BuildExecution
	registry    registryclient.Client
	telemetry   *TelemetryIdentity
	releases    *ReleaseDeployment
	deployments *DeploymentExecution
	config      config.Config
}

func NewEnvironmentSetup(
	db storage.Pool,
	queue storage.InsertQueue,
	jobControl BuildJobControl,
	github *GitHubConnection,
	secrets *EnvironmentSecrets,
	resources *ResourceManagement,
	dns *EnvironmentDNS,
	caddy CaddyRouteService,
	workloads *WorkloadExecution,
	servers *ServerExecution,
	builds *BuildExecution,
	telemetry *TelemetryIdentity,
	releases *ReleaseDeployment,
	deployments *DeploymentExecution,
	cfg config.Config,
) *EnvironmentSetup {
	return &EnvironmentSetup{
		db:          db,
		queue:       queue,
		jobControl:  jobControl,
		github:      github,
		secrets:     secrets,
		resources:   resources,
		dns:         dns,
		caddy:       caddy,
		workloads:   workloads,
		buildpacks:  buildpacksclient.New(),
		servers:     servers,
		builds:      builds,
		registry:    registryclient.New(),
		telemetry:   telemetry,
		releases:    releases,
		deployments: deployments,
		config:      cfg,
	}
}

type BuildJobControl interface {
	CancelJob(context.Context, int64) error
	CancelJobTx(context.Context, *sql.Tx, int64) error
	DeleteJob(context.Context, int64) error
	DeleteJobTx(context.Context, *sql.Tx, int64) error
	RetryJobTx(context.Context, *sql.Tx, int64) error
}

type EnvironmentSetupSecretInput struct {
	Key   string `json:"key"`
	Value string `json:"value"`
}

type EnvironmentSetupResourceInput struct {
	ResourceID           uuid.UUID  `json:"resourceId"`
	EndpointID           uuid.UUID  `json:"endpointId"`
	CredentialID         *uuid.UUID `json:"credentialId"`
	Alias                string     `json:"alias"`
	Database             string     `json:"database"`
	CredentialProjection string     `json:"credentialProjection"`
}

type EnvironmentSetupInput struct {
	ServerID      uuid.UUID
	ServerIDs     []uuid.UUID
	Hostname      string
	ContainerPort int32
	HealthPath    string
	Processes     []models.EnvironmentProcessInput
	Resources     []EnvironmentSetupResourceInput
	Secrets       []EnvironmentSetupSecretInput
	Deploy        bool
	DNS           EnvironmentDNSInput
}

type EnvironmentSetupResult struct {
	Environment        models.EnvironmentEntity              `json:"environment"`
	Revision           models.EnvironmentStateRevisionEntity `json:"revision"`
	Build              models.BuildEntity                    `json:"build"`
	Release            models.ReleaseEntity                  `json:"release"`
	Deployment         models.DeploymentEntity               `json:"deployment"`
	DeploymentDeferred bool                                  `json:"deploymentDeferred"`
}

type EnvironmentSetupResourceOption struct {
	ID                    uuid.UUID         `json:"id"                    bun:"id"`
	Name                  string            `json:"name"                  bun:"name"`
	Engine                string            `json:"engine"                bun:"engine"`
	Database              string            `json:"database"              bun:"database_name"`
	EndpointID            uuid.UUID         `json:"endpointId"            bun:"endpoint_id"`
	Endpoint              string            `json:"endpoint"              bun:"endpoint"`
	CredentialID          *uuid.UUID        `json:"credentialId"          bun:"credential_id"`
	Credential            string            `json:"credential"            bun:"credential"`
	ServerID              *uuid.UUID        `json:"serverId"              bun:"server_id"`
	ResourceConfiguration json.RawMessage   `json:"-"                     bun:"resource_configuration"`
	CredentialFields      []string          `json:"credentialFields"      bun:"-"`
	EnvironmentKeys       map[string]string `json:"environmentKeys"       bun:"-"`
	SupportsConnectionURL bool              `json:"supportsConnectionUrl" bun:"-"`
}

type EnvironmentSetupServerOption = models.EnvironmentSetupServerOption

type EnvironmentSetupOptions struct {
	Resources []EnvironmentSetupResourceOption `json:"resources"`
	Servers   []EnvironmentSetupServerOption   `json:"servers"`
	DNSZones  []EnvironmentDNSOption           `json:"dnsZones"`
}

type EnvironmentEditConfiguration struct {
	Runtime       models.BuildpackRuntime          `json:"runtime"`
	Name          string                           `json:"name"`
	Slug          string                           `json:"slug"`
	Kind          string                           `json:"kind"`
	Hostname      string                           `json:"hostname"`
	ContainerPort int32                            `json:"containerPort"`
	HealthPath    string                           `json:"healthPath"`
	Processes     []models.EnvironmentProcessInput `json:"processes"`
	Resources     []EnvironmentSetupResourceInput  `json:"resources"`
	ServerIDs     []uuid.UUID                      `json:"serverIds"`
	ServerNames   []string                         `json:"serverNames"`
	DNSMode       string                           `json:"dnsMode"`
	DNSZoneID     *uuid.UUID                       `json:"dnsZoneId"`
}

type EnvironmentEditData struct {
	Overview      EnvironmentOverview          `json:"overview"`
	Configuration EnvironmentEditConfiguration `json:"configuration"`
}

type EnvironmentEditInput struct {
	Name          string
	Slug          string
	Kind          string
	ServerIDs     []uuid.UUID
	Hostname      string
	ContainerPort int32
	HealthPath    string
	Processes     []models.EnvironmentProcessInput
	Resources     []EnvironmentSetupResourceInput
	DNS           EnvironmentDNSInput
}

type EnvironmentOverview struct {
	ApplicationID                uuid.UUID                           `json:"applicationId"`
	ApplicationName              string                              `json:"applicationName"`
	SourceType                   string                              `json:"sourceType"`
	Environment                  models.EnvironmentEntity            `json:"environment"`
	SetupComplete                bool                                `json:"setupComplete"`
	Repository                   string                              `json:"repository"`
	Reference                    string                              `json:"reference"`
	ContextPath                  string                              `json:"contextPath"`
	RegistryName                 string                              `json:"registryName"`
	RegistryEndpoint             string                              `json:"registryEndpoint"`
	RuntimeServerIDs             []uuid.UUID                         `json:"runtimeServerIds"`
	RuntimeTargetIDs             []uuid.UUID                         `json:"runtimeTargetIds"`
	RuntimeServers               []string                            `json:"runtimeServers"`
	Deployability                models.EnvironmentDeployability     `json:"deployability"`
	Secrets                      []EnvironmentSecretActivity         `json:"secrets"`
	Variables                    []EnvironmentVariableActivity       `json:"variables"`
	Domain                       string                              `json:"domain"`
	Resources                    []EnvironmentResourceActivity       `json:"resources"`
	Builds                       []EnvironmentBuildActivity          `json:"builds"`
	Releases                     []EnvironmentReleaseActivity        `json:"releases"`
	Deployments                  []EnvironmentDeploymentActivity     `json:"deployments"`
	Instances                    []EnvironmentInstanceActivity       `json:"instances"`
	Processes                    []models.EnvironmentProcessState    `json:"processes"`
	ReleaseCommands              []EnvironmentReleaseCommandActivity `json:"releaseCommands"`
	APITokenPrefix               string                              `json:"apiTokenPrefix"`
	DNS                          EnvironmentDNSStatus                `json:"dns"`
	CanPromoteToProduction       bool                                `json:"canPromoteToProduction"`
	PromotionTargetName          string                              `json:"promotionTargetName"`
	LatestSuccessfulDeploymentID *uuid.UUID                          `json:"latestSuccessfulDeploymentId,omitempty"`
	LatestSuccessfulReleaseID    *uuid.UUID                          `json:"latestSuccessfulReleaseId,omitempty"`
}

type EnvironmentListItem = models.EnvironmentListItem
