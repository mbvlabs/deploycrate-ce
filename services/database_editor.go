package services

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	postgresqlclient "deploycrate-ce/clients/postgresql"
	"deploycrate-ce/config"
	"deploycrate-ce/internal/secretcrypto"
	"deploycrate-ce/internal/storage"
	"deploycrate-ce/models"

	"github.com/google/uuid"
)

const (
	databaseEditorMaximumStatementBytes = 100_000
	databaseEditorMaximumRows           = 500
	databaseEditorMaximumResultBytes    = 2_000_000
)

var (
	ErrDatabaseEditorNotFound    = errors.New("Database editor target was not found")
	ErrDatabaseEditorUnavailable = errors.New("Database editor is unavailable")
	ErrDatabaseEditorInvalidSQL  = errors.New("Database editor SQL is invalid")
)

type DatabaseEditorQueryError = postgresqlclient.QueryError
type DatabaseEditorQueryResult = postgresqlclient.QueryResult
type DatabaseEditorCatalog = postgresqlclient.Catalog

type DatabaseEditorDetails struct {
	Resource       models.ResourceEntity
	Database       models.ResourceDatabaseDefinition
	CredentialName string
	CredentialUser string
	Catalog        DatabaseEditorCatalog
}

type databaseEditorPostgreSQL interface {
	Catalog(
		context.Context,
		postgresqlclient.Connection,
		string,
	) (postgresqlclient.Catalog, error)
	ExecuteReadOnly(
		context.Context,
		postgresqlclient.Connection,
		string,
		string,
		int,
		int,
	) (postgresqlclient.QueryResult, error)
}

type DatabaseEditor struct {
	db       storage.Pool
	config   config.Config
	postgres databaseEditorPostgreSQL
}

func NewDatabaseEditor(db storage.Pool, cfg config.Config) *DatabaseEditor {
	return &DatabaseEditor{db: db, config: cfg, postgres: postgresqlclient.New()}
}

func (service *DatabaseEditor) Details(
	ctx context.Context,
	resourceID uuid.UUID,
	databaseName string,
) (DatabaseEditorDetails, error) {
	target, err := service.target(ctx, resourceID, databaseName)
	if err != nil {
		return DatabaseEditorDetails{}, err
	}
	operationContext, cancel := context.WithTimeout(ctx, 20*time.Second)
	defer cancel()
	catalog, err := service.postgres.Catalog(
		operationContext,
		target.connection,
		target.database.Name,
	)
	if err != nil {
		return DatabaseEditorDetails{}, fmt.Errorf("load PostgreSQL editor catalog: %w", err)
	}
	return DatabaseEditorDetails{
		Resource: target.resource, Database: target.database,
		CredentialName: target.credential.Name,
		CredentialUser: target.connection.Username,
		Catalog:        catalog,
	}, nil
}

func (service *DatabaseEditor) Execute(
	ctx context.Context,
	resourceID uuid.UUID,
	databaseName, statement string,
) (DatabaseEditorQueryResult, error) {
	statement = strings.TrimSpace(statement)
	if statement == "" || len([]byte(statement)) > databaseEditorMaximumStatementBytes ||
		strings.ContainsRune(statement, '\x00') {
		return DatabaseEditorQueryResult{}, ErrDatabaseEditorInvalidSQL
	}
	target, err := service.target(ctx, resourceID, databaseName)
	if err != nil {
		return DatabaseEditorQueryResult{}, err
	}
	operationContext, cancel := context.WithTimeout(ctx, 20*time.Second)
	defer cancel()
	return service.postgres.ExecuteReadOnly(
		operationContext,
		target.connection,
		target.database.Name,
		statement,
		databaseEditorMaximumRows,
		databaseEditorMaximumResultBytes,
	)
}

type databaseEditorTarget struct {
	resource   models.ResourceEntity
	database   models.ResourceDatabaseDefinition
	credential models.ResourceCredentialEntity
	connection postgresqlclient.Connection
}

func (service *DatabaseEditor) target(
	ctx context.Context,
	resourceID uuid.UUID,
	databaseName string,
) (databaseEditorTarget, error) {
	resource, err := models.Resource.FindActive(ctx, service.db.Executor(), resourceID, false)
	if errors.Is(err, models.ErrNotFound) || errors.Is(err, sql.ErrNoRows) {
		return databaseEditorTarget{}, ErrDatabaseEditorNotFound
	}
	if err != nil {
		return databaseEditorTarget{}, fmt.Errorf("load Database editor Resource: %w", err)
	}
	if resource.SystemManaged || resource.ResourceType != models.ResourceTypeDatabase ||
		resource.Engine() != "postgresql" {
		return databaseEditorTarget{}, ErrDatabaseEditorNotFound
	}

	databaseName = strings.TrimSpace(databaseName)
	var database models.ResourceDatabaseDefinition
	found := false
	for _, candidate := range resource.Databases() {
		if candidate.Name == databaseName {
			database = candidate
			found = true
			break
		}
	}
	if !found {
		return databaseEditorTarget{}, ErrDatabaseEditorNotFound
	}

	credential, err := models.ResourceCredential.FindActiveApplicationForDatabase(
		ctx,
		service.db.Executor(),
		resource.ID,
		database.Name,
	)
	if errors.Is(err, sql.ErrNoRows) {
		return databaseEditorTarget{}, fmt.Errorf(
			"%w: attach an application credential to this database",
			ErrDatabaseEditorUnavailable,
		)
	}
	if err != nil {
		return databaseEditorTarget{}, fmt.Errorf("load Database editor credential: %w", err)
	}
	endpoint, err := models.ResourceEndpoint.FindPrimary(
		ctx,
		service.db.Executor(),
		resource.ID,
	)
	if errors.Is(err, sql.ErrNoRows) {
		return databaseEditorTarget{}, fmt.Errorf(
			"%w: configure a primary PostgreSQL endpoint",
			ErrDatabaseEditorUnavailable,
		)
	}
	if err != nil {
		return databaseEditorTarget{}, fmt.Errorf("load Database editor endpoint: %w", err)
	}
	values, err := service.credentialValues(credential)
	if err != nil {
		return databaseEditorTarget{}, err
	}
	username := strings.TrimSpace(credential.Username.String)
	password := values["password"]
	if username == "" || password == "" {
		return databaseEditorTarget{}, fmt.Errorf(
			"%w: the application credential is incomplete",
			ErrDatabaseEditorUnavailable,
		)
	}
	return databaseEditorTarget{
		resource: resource, database: database, credential: credential,
		connection: postgresqlclient.Connection{
			Host: endpoint.Address, Port: endpoint.Port,
			Username: username, Password: password, TLSMode: endpoint.TLSMode,
		},
	}, nil
}

func (service *DatabaseEditor) credentialValues(
	credential models.ResourceCredentialEntity,
) (map[string]string, error) {
	plaintext, err := secretcrypto.DecryptForPurpose(
		credential.EncPayload,
		service.config.App.SessionEncryptionKey,
		resourceCredentialPurpose,
	)
	if err != nil {
		return nil, fmt.Errorf(
			"%w: the application credential cannot be decrypted",
			ErrDatabaseEditorUnavailable,
		)
	}
	var payload struct {
		SchemaVersion int               `json:"schema_version"`
		Values        map[string]string `json:"values"`
	}
	if json.Unmarshal(plaintext, &payload) != nil || payload.SchemaVersion != 1 {
		return nil, fmt.Errorf(
			"%w: the application credential is invalid",
			ErrDatabaseEditorUnavailable,
		)
	}
	return payload.Values, nil
}
