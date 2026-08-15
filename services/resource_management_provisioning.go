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
	"strings"

	postgresqlclient "deploycrate-ce/clients/postgresql"
	"deploycrate-ce/internal/secretcrypto"
	"deploycrate-ce/internal/storage"
	"deploycrate-ce/models"

	"github.com/google/uuid"
)

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
