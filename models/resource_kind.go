package models

import "slices"

type ResourceCredentialField struct {
	Name     string
	Label    string
	Required bool
	Secret   bool
}

type ResourceKindDefinition struct {
	Kind             string
	Label            string
	Category         string
	Protocols        []string
	EndpointRoles    []string
	TLSModes         []string
	CredentialFields []ResourceCredentialField
	HealthCheckKinds []string
	DefaultPort      int32
	DefaultProtocol  string
	DefaultTLSMode   string
}

var resourceKindCatalog = []ResourceKindDefinition{
	{Kind: "postgresql", Label: "PostgreSQL", Category: "database", Protocols: []string{"postgresql", "tcp"}, EndpointRoles: []string{"primary", "replica"}, TLSModes: resourceTLSModes(), CredentialFields: databaseCredentialFields(), HealthCheckKinds: []string{"tcp", "postgresql"}, DefaultPort: 5432, DefaultProtocol: "postgresql", DefaultTLSMode: "prefer"},
	{Kind: "mysql", Label: "MySQL", Category: "database", Protocols: []string{"mysql", "tcp"}, EndpointRoles: []string{"primary", "replica"}, TLSModes: resourceTLSModes(), CredentialFields: databaseCredentialFields(), HealthCheckKinds: []string{"tcp", "mysql"}, DefaultPort: 3306, DefaultProtocol: "mysql", DefaultTLSMode: "prefer"},
	{Kind: "redis", Label: "Redis", Category: "cache", Protocols: []string{"redis", "tcp"}, EndpointRoles: []string{"primary", "replica"}, TLSModes: resourceTLSModes(), CredentialFields: []ResourceCredentialField{{Name: "password", Label: "Password", Required: false, Secret: true}}, HealthCheckKinds: []string{"tcp", "redis"}, DefaultPort: 6379, DefaultProtocol: "redis", DefaultTLSMode: "disable"},
	{Kind: "clickhouse", Label: "ClickHouse", Category: "database", Protocols: []string{"clickhouse", "http", "https", "tcp"}, EndpointRoles: []string{"primary", "replica", "wireguard"}, TLSModes: resourceTLSModes(), CredentialFields: databaseCredentialFields(), HealthCheckKinds: []string{"tcp", "http", "clickhouse"}, DefaultPort: 8123, DefaultProtocol: "http", DefaultTLSMode: "disable"},
	{Kind: "registry", Label: "OCI Registry", Category: "service", Protocols: []string{"http", "https"}, EndpointRoles: []string{"primary"}, TLSModes: resourceTLSModes(), CredentialFields: []ResourceCredentialField{{Name: "password", Label: "Password", Required: true, Secret: true}}, HealthCheckKinds: []string{"http", "tcp"}, DefaultPort: 5000, DefaultProtocol: "http", DefaultTLSMode: "disable"},
	{Kind: "http", Label: "HTTP", Category: "service", Protocols: []string{"http", "https"}, EndpointRoles: []string{"primary", "replica"}, TLSModes: resourceTLSModes(), CredentialFields: []ResourceCredentialField{{Name: "token", Label: "Token", Required: false, Secret: true}}, HealthCheckKinds: []string{"http", "tcp"}, DefaultPort: 80, DefaultProtocol: "http", DefaultTLSMode: "disable"},
	{Kind: "tcp", Label: "Generic TCP", Category: "service", Protocols: []string{"tcp"}, EndpointRoles: []string{"primary", "replica"}, TLSModes: resourceTLSModes(), CredentialFields: []ResourceCredentialField{}, HealthCheckKinds: []string{"tcp"}, DefaultPort: 1, DefaultProtocol: "tcp", DefaultTLSMode: "disable"},
}

func resourceTLSModes() []string {
	return []string{"disable", "prefer", "require", "verify-ca", "verify-full"}
}

func databaseCredentialFields() []ResourceCredentialField {
	return []ResourceCredentialField{
		{Name: "password", Label: "Password", Required: true, Secret: true},
	}
}

func ResourceKindCatalog() []ResourceKindDefinition {
	result := make([]ResourceKindDefinition, len(resourceKindCatalog))
	copy(result, resourceKindCatalog)
	return result
}

func FindResourceKind(kind string) (ResourceKindDefinition, bool) {
	for _, definition := range resourceKindCatalog {
		if definition.Kind == kind {
			return definition, true
		}
	}
	return ResourceKindDefinition{}, false
}

func ResourceCategoryKindSupported(category, kind string) bool {
	definition, ok := FindResourceKind(kind)
	return ok && definition.Category == category
}

func (definition ResourceKindDefinition) SupportsProtocol(protocol string) bool {
	return slices.Contains(definition.Protocols, protocol)
}

func (definition ResourceKindDefinition) SupportsEndpointRole(role string) bool {
	return slices.Contains(definition.EndpointRoles, role)
}

func (definition ResourceKindDefinition) SupportsHealthCheck(kind string) bool {
	return slices.Contains(definition.HealthCheckKinds, kind)
}
