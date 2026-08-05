package models

import "slices"

type ResourceCredentialField struct {
	Name     string
	Label    string
	Required bool
	Secret   bool
}

type ResourceEnvironmentKeyDefinition struct {
	Name       string
	Label      string
	DefaultKey string
}

type ResourceEngineDefinition struct {
	Engine           string
	Label            string
	ResourceType     ResourceTypeEnum
	Protocols        []string
	EndpointRoles    []string
	TLSModes         []string
	CredentialFields []ResourceCredentialField
	EnvironmentKeys  []ResourceEnvironmentKeyDefinition
	HealthCheckKinds []string
	DefaultPort      int32
	DefaultProtocol  string
	DefaultTLSMode   string
}

var resourceEngineCatalog = []ResourceEngineDefinition{
	{Engine: "postgresql", Label: "PostgreSQL", ResourceType: ResourceTypeDatabase, Protocols: []string{"postgresql", "tcp"}, EndpointRoles: []string{"primary", "replica"}, TLSModes: resourceTLSModes(), CredentialFields: databaseCredentialFields(), EnvironmentKeys: resourceEnvironmentKeys("POSTGRESQL", true, true, databaseCredentialFields()), HealthCheckKinds: []string{"tcp", "postgresql"}, DefaultPort: 5432, DefaultProtocol: "postgresql", DefaultTLSMode: "prefer"},
	{Engine: "mysql", Label: "MySQL", ResourceType: ResourceTypeDatabase, Protocols: []string{"mysql", "tcp"}, EndpointRoles: []string{"primary", "replica"}, TLSModes: resourceTLSModes(), CredentialFields: databaseCredentialFields(), EnvironmentKeys: resourceEnvironmentKeys("MYSQL", true, false, databaseCredentialFields()), HealthCheckKinds: []string{"tcp", "mysql"}, DefaultPort: 3306, DefaultProtocol: "mysql", DefaultTLSMode: "prefer"},
	{Engine: "redis", Label: "Redis", ResourceType: ResourceTypeCache, Protocols: []string{"redis", "tcp"}, EndpointRoles: []string{"primary", "replica"}, TLSModes: resourceTLSModes(), CredentialFields: []ResourceCredentialField{{Name: "password", Label: "Password", Required: false, Secret: true}}, EnvironmentKeys: resourceEnvironmentKeys("REDIS", false, false, []ResourceCredentialField{{Name: "password", Label: "Password", Required: false, Secret: true}}), HealthCheckKinds: []string{"tcp", "redis"}, DefaultPort: 6379, DefaultProtocol: "redis", DefaultTLSMode: "disable"},
	{Engine: "clickhouse", Label: "ClickHouse", ResourceType: ResourceTypeDatabase, Protocols: []string{"clickhouse", "http", "https", "tcp"}, EndpointRoles: []string{"primary", "replica", "wireguard"}, TLSModes: resourceTLSModes(), CredentialFields: databaseCredentialFields(), EnvironmentKeys: resourceEnvironmentKeys("CLICKHOUSE", true, false, databaseCredentialFields()), HealthCheckKinds: []string{"tcp", "http", "clickhouse"}, DefaultPort: 8123, DefaultProtocol: "http", DefaultTLSMode: "disable"},
	{Engine: "registry", Label: "OCI Registry", ResourceType: ResourceTypeService, Protocols: []string{"http", "https"}, EndpointRoles: []string{"primary"}, TLSModes: resourceTLSModes(), CredentialFields: []ResourceCredentialField{{Name: "password", Label: "Password", Required: true, Secret: true}}, EnvironmentKeys: resourceEnvironmentKeys("REGISTRY", false, false, []ResourceCredentialField{{Name: "password", Label: "Password", Required: true, Secret: true}}), HealthCheckKinds: []string{"http", "tcp"}, DefaultPort: 5000, DefaultProtocol: "http", DefaultTLSMode: "disable"},
	{Engine: "opentelemetry", Label: "OpenTelemetry", ResourceType: ResourceTypeService, Protocols: []string{"http", "https"}, EndpointRoles: []string{"local", "wireguard"}, TLSModes: resourceTLSModes(), CredentialFields: []ResourceCredentialField{{Name: "token", Label: "Environment identity token", Required: true, Secret: true}}, EnvironmentKeys: []ResourceEnvironmentKeyDefinition{{Name: "endpoint", Label: "OTLP endpoint", DefaultKey: "OTEL_EXPORTER_OTLP_ENDPOINT"}, {Name: "protocol", Label: "OTLP protocol", DefaultKey: "OTEL_EXPORTER_OTLP_PROTOCOL"}, {Name: "headers", Label: "OTLP headers", DefaultKey: "OTEL_EXPORTER_OTLP_HEADERS"}}, HealthCheckKinds: []string{"http", "tcp"}, DefaultPort: 4318, DefaultProtocol: "http", DefaultTLSMode: "disable"},
	{Engine: "http", Label: "HTTP", ResourceType: ResourceTypeService, Protocols: []string{"http", "https"}, EndpointRoles: []string{"primary", "replica"}, TLSModes: resourceTLSModes(), CredentialFields: []ResourceCredentialField{{Name: "token", Label: "Token", Required: false, Secret: true}}, EnvironmentKeys: resourceEnvironmentKeys("HTTP", false, false, []ResourceCredentialField{{Name: "token", Label: "Token", Required: false, Secret: true}}), HealthCheckKinds: []string{"http", "tcp"}, DefaultPort: 80, DefaultProtocol: "http", DefaultTLSMode: "disable"},
	{Engine: "tcp", Label: "Generic TCP", ResourceType: ResourceTypeService, Protocols: []string{"tcp"}, EndpointRoles: []string{"primary", "replica"}, TLSModes: resourceTLSModes(), CredentialFields: []ResourceCredentialField{}, EnvironmentKeys: resourceEnvironmentKeys("TCP", false, false, nil), HealthCheckKinds: []string{"tcp"}, DefaultPort: 1, DefaultProtocol: "tcp", DefaultTLSMode: "disable"},
}

func resourceEnvironmentKeys(prefix string, database, connectionURL bool, credentialFields []ResourceCredentialField) []ResourceEnvironmentKeyDefinition {
	definitions := []ResourceEnvironmentKeyDefinition{
		{Name: "host", Label: "Host", DefaultKey: prefix + "_HOST"},
		{Name: "port", Label: "Port", DefaultKey: prefix + "_PORT"},
		{Name: "protocol", Label: "Protocol", DefaultKey: prefix + "_PROTOCOL"},
		{Name: "tls_mode", Label: "TLS mode", DefaultKey: prefix + "_TLS_MODE"},
	}
	if database {
		definitions = append(definitions, ResourceEnvironmentKeyDefinition{Name: "database", Label: "Database", DefaultKey: prefix + "_DATABASE"})
	}
	if len(credentialFields) > 0 {
		definitions = append(definitions, ResourceEnvironmentKeyDefinition{Name: "username", Label: "Username", DefaultKey: prefix + "_USER"})
	}
	for _, field := range credentialFields {
		definitions = append(definitions, ResourceEnvironmentKeyDefinition{Name: field.Name, Label: field.Label, DefaultKey: prefix + "_" + NormalizeEnvironmentSecretKey(field.Name)})
	}
	if connectionURL {
		definitions = append(definitions, ResourceEnvironmentKeyDefinition{Name: "url", Label: "Connection URL", DefaultKey: prefix + "_URL"})
	}
	return definitions
}

func (definition ResourceEngineDefinition) DefaultEnvironmentKeys() map[string]string {
	keys := make(map[string]string, len(definition.EnvironmentKeys))
	for _, key := range definition.EnvironmentKeys {
		keys[key.Name] = key.DefaultKey
	}
	return keys
}

func resourceTLSModes() []string {
	return []string{"disable", "prefer", "require", "verify-ca", "verify-full"}
}

func databaseCredentialFields() []ResourceCredentialField {
	return []ResourceCredentialField{
		{Name: "password", Label: "Password", Required: true, Secret: true},
	}
}

func ResourceEngineCatalog() []ResourceEngineDefinition {
	result := make([]ResourceEngineDefinition, len(resourceEngineCatalog))
	copy(result, resourceEngineCatalog)
	return result
}

func FindResourceEngine(engine string) (ResourceEngineDefinition, bool) {
	for _, definition := range resourceEngineCatalog {
		if definition.Engine == engine {
			return definition, true
		}
	}
	return ResourceEngineDefinition{}, false
}

func (definition ResourceEngineDefinition) SupportsProtocol(protocol string) bool {
	return slices.Contains(definition.Protocols, protocol)
}

func (definition ResourceEngineDefinition) SupportsEndpointRole(role string) bool {
	return slices.Contains(definition.EndpointRoles, role)
}

func (definition ResourceEngineDefinition) SupportsHealthCheck(kind string) bool {
	return slices.Contains(definition.HealthCheckKinds, kind)
}
