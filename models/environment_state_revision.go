package models

import (
	"bytes"
	"context"
	"crypto/hmac"
	"deploycrate-ce/internal/storage"
	"deploycrate-ce/internal/validation"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"slices"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/uptrace/bun"
)

const EnvironmentStateSchemaVersion = 3

type EnvironmentSecretDescriptor struct {
	ID         uuid.UUID `json:"id"`
	Key        string    `json:"key"`
	Digest     string    `json:"digest"`
	SourceType string    `json:"source_type"`
	SourceID   uuid.UUID `json:"source_id"`
}

type EnvironmentRuntimeState struct {
	Runtime       string            `json:"runtime"`
	BPGOTargets   []GoProcessTarget `json:"bp_go_targets,omitempty"`
	RestartPolicy string            `json:"restart_policy"`
}

type EnvironmentProcessState struct {
	Name           string   `json:"name"`
	Kind           string   `json:"kind"`
	Command        *string  `json:"command,omitempty"`
	Arguments      []string `json:"arguments"`
	Replicas       int32    `json:"replicas"`
	ContainerPort  int32    `json:"container_port,omitempty"`
	HealthPath     string   `json:"health_path,omitempty"`
	TimeoutSeconds int32    `json:"timeout_seconds,omitempty"`
}

type EnvironmentDomainState struct {
	ID       uuid.UUID `json:"id"`
	Hostname string    `json:"hostname"`
	Primary  bool      `json:"primary"`
}

type EnvironmentResourceState struct {
	EnvironmentResourceID uuid.UUID         `json:"environment_resource_id"`
	ResourceID            uuid.UUID         `json:"resource_id"`
	Kind                  string            `json:"kind"`
	EndpointID            uuid.UUID         `json:"endpoint_id"`
	CredentialID          *uuid.UUID        `json:"credential_id,omitempty"`
	Alias                 string            `json:"alias"`
	Database              string            `json:"database,omitempty"`
	EnvironmentKeys       map[string]string `json:"environment_keys,omitempty"`
	Variables             map[string]string `json:"variables,omitempty"`
}

type EnvironmentDesiredState struct {
	SchemaVersion int                           `json:"schema_version"`
	Runtime       EnvironmentRuntimeState       `json:"runtime"`
	Processes     []EnvironmentProcessState     `json:"processes"`
	Domain        EnvironmentDomainState        `json:"domain"`
	Resources     []EnvironmentResourceState    `json:"resources"`
	Secrets       []EnvironmentSecretDescriptor `json:"secrets"`
}

type EnvironmentStateRevisionEntity struct {
	bun.BaseModel `bun:"table:environment_state_revisions,alias:environment_state_revisions"`
	ID            uuid.UUID       `bun:"id,pk,type:uuid"`
	CreatedAt     time.Time       `bun:"created_at"`
	UpdatedAt     time.Time       `bun:"updated_at"`
	State         json.RawMessage `bun:"state,type:jsonb"`
	EnvironmentID uuid.UUID       `bun:"environment_id,type:uuid"`
	ChangeID      uuid.UUID       `bun:"change_id,type:uuid"`
}

func (e *EnvironmentStateRevisionEntity) Validate() error {
	builder := validation.NewBuilder()
	if e.EnvironmentID == uuid.Nil {
		builder.Add("environmentId", "required", "Environment is required")
	}
	if e.ChangeID == uuid.Nil {
		builder.Add("changeId", "required", "change is required")
	}
	if _, err := ParseEnvironmentDesiredState(e.State); err != nil {
		builder.Add("state", "invalid", err.Error())
	}
	return builder.Err()
}

func ParseEnvironmentDesiredState(raw json.RawMessage) (EnvironmentDesiredState, error) {
	var state EnvironmentDesiredState
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&state); err != nil {
		return EnvironmentDesiredState{}, errors.New(
			"Environment state must use the supported schema",
		)
	}
	if err := decoder.Decode(&struct{}{}); !errors.Is(err, io.EOF) {
		return EnvironmentDesiredState{}, errors.New(
			"Environment state must contain one JSON value",
		)
	}
	if err := validateEnvironmentDesiredState(&state); err != nil {
		return EnvironmentDesiredState{}, err
	}
	return state, nil
}

func CanonicalEnvironmentDesiredState(state EnvironmentDesiredState) (json.RawMessage, error) {
	state.SchemaVersion = EnvironmentStateSchemaVersion
	for index := range state.Secrets {
		state.Secrets[index].Key = NormalizeEnvironmentSecretKey(state.Secrets[index].Key)
	}
	slices.SortFunc(state.Secrets, func(left, right EnvironmentSecretDescriptor) int {
		return strings.Compare(left.Key, right.Key)
	})
	slices.SortFunc(state.Resources, func(left, right EnvironmentResourceState) int {
		return strings.Compare(left.Alias, right.Alias)
	})
	for index := range state.Processes {
		state.Processes[index].Name = strings.ToLower(
			strings.TrimSpace(state.Processes[index].Name),
		)
		state.Processes[index].Kind = strings.ToLower(
			strings.TrimSpace(state.Processes[index].Kind),
		)
		if state.Processes[index].Arguments == nil {
			state.Processes[index].Arguments = []string{}
		}
	}
	if err := validateEnvironmentDesiredState(&state); err != nil {
		return nil, err
	}
	encoded, err := json.Marshal(state)
	if err != nil {
		return nil, fmt.Errorf("encode Environment desired state: %w", err)
	}
	return encoded, nil
}

func validateEnvironmentDesiredState(state *EnvironmentDesiredState) error {
	builder := validation.NewBuilder()
	if state.SchemaVersion != EnvironmentStateSchemaVersion {
		builder.Add(
			"schemaVersion",
			"unsupported",
			"Environment state schema version is unsupported",
		)
	}
	if strings.TrimSpace(state.Runtime.Runtime) != "go" {
		builder.Add("runtime.runtime", "unsupported", "only the Go runtime is supported")
	}
	if state.Runtime.RestartPolicy != "unless-stopped" {
		builder.Add("runtime.restartPolicy", "unsupported", "restart policy must be unless-stopped")
	}
	if err := ValidateGoProcessTargets(state.Runtime.BPGOTargets); err != nil {
		builder.Add("runtime.bpGoTargets", "invalid", err.Error())
	}
	processInputs := make([]EnvironmentProcessInput, 0, len(state.Processes))
	for _, process := range state.Processes {
		input := EnvironmentProcessInput{
			Name:       process.Name,
			Kind:       process.Kind,
			Command:    process.Command,
			Arguments:  process.Arguments,
			Replicas:   process.Replicas,
			HealthPath: process.HealthPath,
		}
		if process.Kind == EnvironmentProcessWeb {
			input.ContainerPort = &process.ContainerPort
		}
		if process.Kind == EnvironmentProcessRelease {
			input.TimeoutSeconds = &process.TimeoutSeconds
		}
		processInputs = append(processInputs, input)
	}
	if _, err := ValidateEnvironmentProcessFormation(processInputs); err != nil {
		builder.Add("processes", "invalid", err.Error())
	}
	if state.Domain.ID == uuid.Nil || strings.TrimSpace(state.Domain.Hostname) == "" ||
		!state.Domain.Primary {
		builder.Add("domain", "invalid", "one primary Environment domain is required")
	}
	secretKeys := make(map[string]struct{}, len(state.Secrets))
	secretIDs := make(map[uuid.UUID]struct{}, len(state.Secrets))
	resourceConnectionIDs := make(map[uuid.UUID]struct{}, len(state.Resources))
	resourceSecretKeys := make(map[uuid.UUID]map[string]struct{}, len(state.Resources))
	resourceMappedKeys := make(map[string]uuid.UUID)
	legacyVariableKeys := make(map[string]struct{})
	for _, resource := range state.Resources {
		if resource.EnvironmentResourceID != uuid.Nil {
			resourceConnectionIDs[resource.EnvironmentResourceID] = struct{}{}
		}
		if len(resource.EnvironmentKeys) > 0 {
			keys := make(map[string]struct{}, len(resource.EnvironmentKeys))
			for _, key := range resource.EnvironmentKeys {
				normalized := NormalizeEnvironmentSecretKey(key)
				keys[normalized] = struct{}{}
				if owner, exists := resourceMappedKeys[normalized]; exists &&
					owner != resource.EnvironmentResourceID {
					builder.Add(
						"resources",
						"duplicate",
						"Resource Environment key mappings must be unique",
					)
				}
				resourceMappedKeys[normalized] = resource.EnvironmentResourceID
			}
			resourceSecretKeys[resource.EnvironmentResourceID] = keys
		}
		for key := range resource.Variables {
			legacyVariableKeys[NormalizeEnvironmentSecretKey(key)] = struct{}{}
		}
	}
	for key := range resourceMappedKeys {
		if _, exists := legacyVariableKeys[key]; exists {
			builder.Add(
				"resources",
				"duplicate",
				"Resource Environment key mapping conflicts with a legacy Resource variable",
			)
		}
	}
	for index := range state.Secrets {
		descriptor := &state.Secrets[index]
		descriptor.Key = NormalizeEnvironmentSecretKey(descriptor.Key)
		field := fmt.Sprintf("secrets.%d", index)
		if descriptor.ID == uuid.Nil || descriptor.SourceID == uuid.Nil {
			builder.Add(field, "invalid", "secret identity and owner are required")
		}
		if err := ValidateEnvironmentSecretKey(descriptor.Key, true); err != nil {
			builder.Add(field+".key", "invalid", "secret key is invalid")
		}
		if descriptor.SourceType != EnvironmentSecretSourceUser &&
			descriptor.SourceType != EnvironmentSecretSourceResource {
			builder.Add(field+".sourceType", "unsupported", "secret source type is unsupported")
		}
		if descriptor.SourceType == EnvironmentSecretSourceResource {
			if _, exists := resourceConnectionIDs[descriptor.SourceID]; !exists {
				builder.Add(
					field+".sourceId",
					"ownership",
					"Resource-managed secret owner must be present in the same revision",
				)
			}
			if keys, mapped := resourceSecretKeys[descriptor.SourceID]; mapped {
				if _, exists := keys[descriptor.Key]; !exists {
					builder.Add(
						field+".key",
						"ownership",
						"Resource-managed secret key must belong to its Resource mapping",
					)
				}
			}
		}
		if !validEnvironmentSecretDigest(descriptor.Digest) {
			builder.Add(field+".digest", "invalid", "secret digest descriptor is invalid")
		}
		if _, exists := secretKeys[descriptor.Key]; exists {
			builder.Add(field+".key", "duplicate", "secret keys must be unique")
		}
		if _, exists := legacyVariableKeys[descriptor.Key]; exists {
			builder.Add(
				field+".key",
				"duplicate",
				"secret key conflicts with a legacy Resource variable",
			)
		}
		if _, exists := secretIDs[descriptor.ID]; exists {
			builder.Add(field+".id", "duplicate", "secret identifiers must be unique")
		}
		secretKeys[descriptor.Key] = struct{}{}
		secretIDs[descriptor.ID] = struct{}{}
	}
	resourceAliases := make(map[string]struct{}, len(state.Resources))
	for index, resource := range state.Resources {
		field := fmt.Sprintf("resources.%d", index)
		alias := strings.ToUpper(strings.TrimSpace(resource.Alias))
		if resource.EnvironmentResourceID == uuid.Nil || resource.ResourceID == uuid.Nil ||
			resource.EndpointID == uuid.Nil ||
			alias == "" ||
			strings.TrimSpace(resource.Kind) == "" {
			builder.Add(
				field,
				"invalid",
				"Resource connection identities, kind, and alias are required",
			)
		}
		if _, exists := resourceAliases[alias]; exists {
			builder.Add(field+".alias", "duplicate", "Resource aliases must be unique")
		}
		resourceAliases[alias] = struct{}{}
		mappedKeys := make(map[string]struct{}, len(resource.EnvironmentKeys))
		for logicalName, key := range resource.EnvironmentKeys {
			normalized := NormalizeEnvironmentSecretKey(key)
			if strings.TrimSpace(logicalName) == "" || normalized == "" {
				builder.Add(
					field+".environmentKeys",
					"invalid",
					"Resource Environment key roles and names are required",
				)
				continue
			}
			if err := ValidateEnvironmentSecretKey(
				normalized,
				false,
			); err != nil ||
				normalized != key {
				builder.Add(
					field+".environmentKeys",
					"invalid",
					"Resource Environment keys must be normalized and valid",
				)
			}
			if _, exists := mappedKeys[normalized]; exists {
				builder.Add(
					field+".environmentKeys",
					"duplicate",
					"Resource Environment keys must be unique",
				)
			}
			mappedKeys[normalized] = struct{}{}
		}
		if len(resource.EnvironmentKeys) > 0 && len(resource.Variables) > 0 {
			builder.Add(
				field+".variables",
				"secret",
				"Resource projections with owned key mappings must use Environment secrets",
			)
		}
		for key := range resource.Variables {
			normalized := NormalizeEnvironmentSecretKey(key)
			lower := strings.ToLower(normalized)
			if normalized != key || strings.Contains(lower, "password") ||
				strings.Contains(lower, "secret") ||
				strings.Contains(lower, "token") ||
				strings.Contains(lower, "credential") {
				builder.Add(
					field+".variables",
					"secret",
					"Resource variables must be normalized and non-secret",
				)
			}
		}
	}
	return builder.Err()
}

func (state EnvironmentDesiredState) WebProcess() (EnvironmentProcessState, bool) {
	for _, process := range state.Processes {
		if process.Kind == EnvironmentProcessWeb {
			return process, true
		}
	}
	return EnvironmentProcessState{}, false
}

func (state EnvironmentDesiredState) ReleaseProcess() (EnvironmentProcessState, bool) {
	for _, process := range state.Processes {
		if process.Kind == EnvironmentProcessRelease {
			return process, true
		}
	}
	return EnvironmentProcessState{}, false
}

func (state EnvironmentDesiredState) LongRunningProcesses() []EnvironmentProcessState {
	processes := make([]EnvironmentProcessState, 0, len(state.Processes))
	for _, process := range state.Processes {
		if process.Kind == EnvironmentProcessWeb || process.Kind == EnvironmentProcessWorker {
			processes = append(processes, process)
		}
	}
	return processes
}

func (esr environmentStateRevision) Find(
	ctx context.Context,
	db storage.Executor,
	id uuid.UUID,
) (EnvironmentStateRevisionEntity, error) {
	var entity EnvironmentStateRevisionEntity
	if err := db.NewSelect().Model(&entity).Where("id = ?", id).Scan(ctx); err != nil {
		return EnvironmentStateRevisionEntity{}, err
	}
	return entity, nil
}

func (esr environmentStateRevision) LatestCommitted(
	ctx context.Context,
	db storage.Executor,
	environmentID uuid.UUID,
) (EnvironmentStateRevisionEntity, error) {
	var entity EnvironmentStateRevisionEntity
	if err := db.NewSelect().Model(&entity).
		Join("JOIN changes AS change ON change.id = environment_state_revisions.change_id").
		Where("environment_state_revisions.environment_id = ?", environmentID).
		Where("change.committed_at IS NOT NULL").Where("change.cancelled_at IS NULL").
		OrderExpr("change.sequence DESC").Limit(1).Scan(ctx); err != nil {
		return EnvironmentStateRevisionEntity{}, err
	}
	return entity, nil
}

type CreateEnvironmentStateRevisionData struct {
	State         json.RawMessage
	EnvironmentID uuid.UUID
	ChangeID      uuid.UUID
}

func (esr environmentStateRevision) Create(
	ctx context.Context,
	db storage.Executor,
	data CreateEnvironmentStateRevisionData,
) (EnvironmentStateRevisionEntity, error) {
	state, err := ParseEnvironmentDesiredState(data.State)
	if err != nil {
		return EnvironmentStateRevisionEntity{}, errors.Join(ErrDomainValidation, err)
	}
	canonical, err := CanonicalEnvironmentDesiredState(state)
	if err != nil {
		return EnvironmentStateRevisionEntity{}, errors.Join(ErrDomainValidation, err)
	}
	entity := EnvironmentStateRevisionEntity{
		ID: uuid.New(), CreatedAt: time.Now().UTC(), UpdatedAt: time.Now().UTC(),
		State: canonical, EnvironmentID: data.EnvironmentID, ChangeID: data.ChangeID,
	}
	if err := validation.Validate(&entity); err != nil {
		return EnvironmentStateRevisionEntity{}, errors.Join(ErrDomainValidation, err)
	}
	if _, err := esr.ResolveSecrets(ctx, db, entity); err != nil {
		return EnvironmentStateRevisionEntity{}, errors.Join(ErrDomainValidation, err)
	}
	if _, err := db.NewInsert().Model(&entity).Exec(ctx); err != nil {
		return EnvironmentStateRevisionEntity{}, err
	}
	return entity, nil
}

func (esr environmentStateRevision) ResolveSecrets(
	ctx context.Context,
	db storage.Executor,
	revision EnvironmentStateRevisionEntity,
) ([]EnvironmentSecretEntity, error) {
	state, err := ParseEnvironmentDesiredState(revision.State)
	if err != nil {
		return nil, err
	}
	resolved := make([]EnvironmentSecretEntity, 0, len(state.Secrets))
	for _, descriptor := range state.Secrets {
		secret, err := EnvironmentSecret.FindForEnvironment(
			ctx,
			db,
			revision.EnvironmentID,
			descriptor.ID,
		)
		if err != nil {
			return nil, fmt.Errorf("resolve Environment secret %s: %w", descriptor.Key, err)
		}
		digest, _ := environmentSecretDigestBytes(descriptor.Digest)
		if secret.Key != descriptor.Key || secret.SourceType != descriptor.SourceType ||
			secret.SourceID != descriptor.SourceID ||
			!hmac.Equal(secret.Digest, digest) {
			return nil, fmt.Errorf(
				"Environment secret descriptor %s does not match its immutable value",
				descriptor.Key,
			)
		}
		resolved = append(resolved, secret)
	}
	return resolved, nil
}

func EnvironmentSecretDescriptorFromEntity(
	secret EnvironmentSecretEntity,
) EnvironmentSecretDescriptor {
	return EnvironmentSecretDescriptor{
		ID: secret.ID, Key: secret.Key,
		Digest:     "hmac-sha256:" + hex.EncodeToString(secret.Digest),
		SourceType: secret.SourceType, SourceID: secret.SourceID,
	}
}

func validEnvironmentSecretDigest(value string) bool {
	digest, err := environmentSecretDigestBytes(value)
	return err == nil && len(digest) == 32
}

func environmentSecretDigestBytes(value string) ([]byte, error) {
	const prefix = "hmac-sha256:"
	if !strings.HasPrefix(value, prefix) {
		return nil, errors.New("secret digest algorithm is unsupported")
	}
	return hex.DecodeString(strings.TrimPrefix(value, prefix))
}
