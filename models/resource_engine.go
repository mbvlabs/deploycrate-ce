package models

import "slices"

type ResourceCredentialField struct {
	Name     string
	Label    string
	Required bool
	Secret   bool
}

type ResourceEngineDefinition struct {
	Engine           string
	Label            string
	ResourceType     ResourceTypeEnum
	Protocols        []string
	EndpointRoles    []string
	TLSModes         []string
	CredentialFields []ResourceCredentialField
	HealthCheckKinds []string
	DefaultPort      int32
	DefaultProtocol  string
	DefaultTLSMode   string
}

var resourceEngineCatalog = []ResourceEngineDefinition{
	{Engine: "postgresql", Label: "PostgreSQL", ResourceType: ResourceTypeDatabase, Protocols: []string{"postgresql", "tcp"}, EndpointRoles: []string{"primary", "replica"}, TLSModes: resourceTLSModes(), CredentialFields: databaseCredentialFields(), HealthCheckKinds: []string{"tcp", "postgresql"}, DefaultPort: 5432, DefaultProtocol: "postgresql", DefaultTLSMode: "prefer"},
	{Engine: "mysql", Label: "MySQL", ResourceType: ResourceTypeDatabase, Protocols: []string{"mysql", "tcp"}, EndpointRoles: []string{"primary", "replica"}, TLSModes: resourceTLSModes(), CredentialFields: databaseCredentialFields(), HealthCheckKinds: []string{"tcp", "mysql"}, DefaultPort: 3306, DefaultProtocol: "mysql", DefaultTLSMode: "prefer"},
	{Engine: "redis", Label: "Redis", ResourceType: ResourceTypeCache, Protocols: []string{"redis", "tcp"}, EndpointRoles: []string{"primary", "replica"}, TLSModes: resourceTLSModes(), CredentialFields: []ResourceCredentialField{{Name: "password", Label: "Password", Required: false, Secret: true}}, HealthCheckKinds: []string{"tcp", "redis"}, DefaultPort: 6379, DefaultProtocol: "redis", DefaultTLSMode: "disable"},
	{Engine: "clickhouse", Label: "ClickHouse", ResourceType: ResourceTypeDatabase, Protocols: []string{"clickhouse", "http", "https", "tcp"}, EndpointRoles: []string{"primary", "replica", "wireguard"}, TLSModes: resourceTLSModes(), CredentialFields: databaseCredentialFields(), HealthCheckKinds: []string{"tcp", "http", "clickhouse"}, DefaultPort: 8123, DefaultProtocol: "http", DefaultTLSMode: "disable"},
	{Engine: "registry", Label: "OCI Registry", ResourceType: ResourceTypeService, Protocols: []string{"http", "https"}, EndpointRoles: []string{"primary"}, TLSModes: resourceTLSModes(), CredentialFields: []ResourceCredentialField{{Name: "password", Label: "Password", Required: true, Secret: true}}, HealthCheckKinds: []string{"http", "tcp"}, DefaultPort: 5000, DefaultProtocol: "http", DefaultTLSMode: "disable"},
	{Engine: "http", Label: "HTTP", ResourceType: ResourceTypeService, Protocols: []string{"http", "https"}, EndpointRoles: []string{"primary", "replica"}, TLSModes: resourceTLSModes(), CredentialFields: []ResourceCredentialField{{Name: "token", Label: "Token", Required: false, Secret: true}}, HealthCheckKinds: []string{"http", "tcp"}, DefaultPort: 80, DefaultProtocol: "http", DefaultTLSMode: "disable"},
	{Engine: "tcp", Label: "Generic TCP", ResourceType: ResourceTypeService, Protocols: []string{"tcp"}, EndpointRoles: []string{"primary", "replica"}, TLSModes: resourceTLSModes(), CredentialFields: []ResourceCredentialField{}, HealthCheckKinds: []string{"tcp"}, DefaultPort: 1, DefaultProtocol: "tcp", DefaultTLSMode: "disable"},
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
