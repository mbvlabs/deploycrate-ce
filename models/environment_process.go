package models

import (
	"context"
	"database/sql"
	"deploycrate-ce/internal/storage"
	"deploycrate-ce/internal/validation"
	"encoding/json"
	"errors"
	"fmt"
	"regexp"
	"slices"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/google/uuid"
	"github.com/uptrace/bun"
)

const (
	EnvironmentProcessWeb     = "web"
	EnvironmentProcessWorker  = "worker"
	EnvironmentProcessRelease = "release"
	MaxProcessArguments       = 128
	MaxProcessArgumentBytes   = 4096
	MaxProcessCommandBytes    = 1024
	DefaultReleaseTimeout     = int32(900)
)

var environmentProcessNamePattern = regexp.MustCompile(`^[a-z][a-z0-9_-]{0,62}$`)

type EnvironmentProcessEntity struct {
	bun.BaseModel  `bun:"table:environment_processes,alias:environment_processes"`
	ID             uuid.UUID       `json:"id" bun:"id,pk,type:uuid"`
	CreatedAt      time.Time       `json:"createdAt" bun:"created_at"`
	UpdatedAt      time.Time       `json:"updatedAt" bun:"updated_at"`
	ArchivedAt     sql.NullTime    `json:"-" bun:"archived_at"`
	Name           string          `json:"name" bun:"name"`
	Kind           string          `json:"kind" bun:"kind"`
	Command        sql.NullString  `json:"-" bun:"command"`
	Arguments      json.RawMessage `json:"arguments" bun:"arguments,type:jsonb"`
	Replicas       int32           `json:"replicas" bun:"replicas"`
	ContainerPort  sql.NullInt32   `json:"-" bun:"container_port"`
	HealthPath     sql.NullString  `json:"-" bun:"health_path"`
	TimeoutSeconds sql.NullInt32   `json:"-" bun:"timeout_seconds"`
	EnvironmentID  uuid.UUID       `json:"-" bun:"environment_id,type:uuid"`
}

type EnvironmentProcessInput struct {
	Name           string   `json:"name"`
	Kind           string   `json:"kind"`
	Command        *string  `json:"command"`
	Arguments      []string `json:"arguments"`
	Replicas       int32    `json:"replicas"`
	ContainerPort  *int32   `json:"containerPort,omitempty"`
	HealthPath     string   `json:"healthPath,omitempty"`
	TimeoutSeconds *int32   `json:"timeoutSeconds,omitempty"`
}

func NormalizeEnvironmentProcessInput(input EnvironmentProcessInput) EnvironmentProcessInput {
	input.Name = strings.ToLower(strings.TrimSpace(input.Name))
	input.Kind = strings.ToLower(strings.TrimSpace(input.Kind))
	if input.Command != nil {
		command := strings.TrimSpace(*input.Command)
		if command == "" {
			input.Command = nil
		} else {
			input.Command = &command
		}
	}
	if input.Arguments == nil {
		input.Arguments = []string{}
	}
	input.HealthPath = strings.TrimSpace(input.HealthPath)
	return input
}

func ValidateEnvironmentProcessFormation(inputs []EnvironmentProcessInput) ([]EnvironmentProcessInput, error) {
	normalized := make([]EnvironmentProcessInput, len(inputs))
	names := make(map[string]struct{}, len(inputs))
	webCount := 0
	releaseCount := 0
	builder := validation.NewBuilder()
	for index, value := range inputs {
		value = NormalizeEnvironmentProcessInput(value)
		normalized[index] = value
		field := fmt.Sprintf("processes.%d", index)
		if !environmentProcessNamePattern.MatchString(value.Name) {
			builder.Add(field+".name", "invalid", "process name must be a lowercase identifier")
		}
		if _, exists := names[value.Name]; exists {
			builder.Add(field+".name", "duplicate", "process names must be unique")
		}
		names[value.Name] = struct{}{}
		if !slices.Contains([]string{EnvironmentProcessWeb, EnvironmentProcessWorker, EnvironmentProcessRelease}, value.Kind) {
			builder.Add(field+".kind", "invalid", "process kind is invalid")
		}
		if value.Command != nil && (len(*value.Command) > MaxProcessCommandBytes || strings.ContainsRune(*value.Command, '\x00') || !utf8.ValidString(*value.Command)) {
			builder.Add(field+".command", "invalid", "process command is invalid")
		}
		if len(value.Arguments) > MaxProcessArguments {
			builder.Add(field+".arguments", "limit", "process has too many arguments")
		}
		for _, argument := range value.Arguments {
			if len(argument) > MaxProcessArgumentBytes || strings.ContainsRune(argument, '\x00') || !utf8.ValidString(argument) {
				builder.Add(field+".arguments", "invalid", "process arguments are invalid")
				break
			}
		}
		switch value.Kind {
		case EnvironmentProcessWeb:
			webCount++
			if value.Name != EnvironmentProcessWeb || value.Replicas != 1 || value.ContainerPort == nil || *value.ContainerPort < 1 || *value.ContainerPort > 65535 {
				builder.Add(field, "invalid", "web must be named web with one replica and a valid container port")
			}
			if value.TimeoutSeconds != nil {
				builder.Add(field+".timeoutSeconds", "unsupported", "web does not use a timeout")
			}
			if value.HealthPath != "" && (!strings.HasPrefix(value.HealthPath, "/") || len(value.HealthPath) > 2048 || strings.ContainsAny(value.HealthPath, " \t\r\n\x00") || !utf8.ValidString(value.HealthPath)) {
				builder.Add(field+".healthPath", "invalid", "web health path must be an absolute path without whitespace")
			}
		case EnvironmentProcessWorker:
			if value.Command == nil || value.Replicas < 1 || value.Replicas > 32 {
				builder.Add(field, "invalid", "worker requires a command and between 1 and 32 replicas")
			}
			if value.Name == EnvironmentProcessRelease {
				builder.Add(field+".name", "reserved", "release is reserved for the release command")
			}
			if value.ContainerPort != nil || value.HealthPath != "" || value.TimeoutSeconds != nil {
				builder.Add(field, "unsupported", "worker cannot publish a port, health path, or timeout")
			}
		case EnvironmentProcessRelease:
			releaseCount++
			if value.Name != EnvironmentProcessRelease || value.Command == nil || value.Replicas != 1 || value.TimeoutSeconds == nil || *value.TimeoutSeconds < 30 || *value.TimeoutSeconds > 3600 {
				builder.Add(field, "invalid", "release must be named release with one execution, a command, and a timeout from 30 to 3600 seconds")
			}
			if value.ContainerPort != nil || value.HealthPath != "" {
				builder.Add(field, "unsupported", "release cannot publish a port or health path")
			}
		}
	}
	if webCount != 1 {
		builder.Add("processes", "web", "exactly one web process is required")
	}
	if releaseCount > 1 {
		builder.Add("processes", "release", "at most one release process is allowed")
	}
	if err := builder.Err(); err != nil {
		return nil, errors.Join(ErrDomainValidation, err)
	}
	return normalized, nil
}

func (entity *EnvironmentProcessEntity) Validate() error {
	arguments := []string{}
	if json.Unmarshal(entity.Arguments, &arguments) != nil {
		return errors.Join(ErrDomainValidation, errors.New("process arguments must be a JSON string array"))
	}
	input := EnvironmentProcessInput{Name: entity.Name, Kind: entity.Kind, Arguments: arguments, Replicas: entity.Replicas}
	if entity.Command.Valid {
		input.Command = &entity.Command.String
	}
	if entity.ContainerPort.Valid {
		input.ContainerPort = &entity.ContainerPort.Int32
	}
	if entity.HealthPath.Valid {
		input.HealthPath = entity.HealthPath.String
	}
	if entity.TimeoutSeconds.Valid {
		input.TimeoutSeconds = &entity.TimeoutSeconds.Int32
	}
	_, err := ValidateEnvironmentProcessFormation([]EnvironmentProcessInput{input, defaultCompanionForValidation(input.Kind)})
	if entity.EnvironmentID == uuid.Nil {
		return errors.Join(err, errors.Join(ErrDomainValidation, errors.New("Environment is required")))
	}
	return err
}

func defaultCompanionForValidation(kind string) EnvironmentProcessInput {
	port := int32(8080)
	if kind == EnvironmentProcessWeb {
		command := "/bin/worker"
		return EnvironmentProcessInput{Name: "validation-worker", Kind: EnvironmentProcessWorker, Command: &command, Arguments: []string{}, Replicas: 1}
	}
	return EnvironmentProcessInput{Name: EnvironmentProcessWeb, Kind: EnvironmentProcessWeb, Arguments: []string{}, Replicas: 1, ContainerPort: &port}
}

func (environmentProcess) ActiveForEnvironment(ctx context.Context, db storage.Executor, environmentID uuid.UUID) ([]EnvironmentProcessEntity, error) {
	entities := make([]EnvironmentProcessEntity, 0)
	err := db.NewSelect().Model(&entities).Where("environment_id = ?", environmentID).Where("archived_at IS NULL").OrderExpr("created_at, id").Scan(ctx)
	return entities, err
}

func (environmentProcess) ReplaceActive(ctx context.Context, db storage.Executor, environmentID uuid.UUID, inputs []EnvironmentProcessInput) ([]EnvironmentProcessEntity, error) {
	switch db.(type) {
	case bun.Tx, *bun.Tx:
	default:
		return nil, errors.New("process formation replacement requires a transaction")
	}
	inputs, err := ValidateEnvironmentProcessFormation(inputs)
	if err != nil {
		return nil, err
	}
	now := time.Now().UTC()
	if _, err := db.ExecContext(ctx, "SELECT pg_advisory_xact_lock(hashtextextended(?, 0))", "environment-processes:"+environmentID.String()); err != nil {
		return nil, err
	}
	if _, err := db.NewUpdate().TableExpr("environment_processes").Set("archived_at = ?", now).Set("updated_at = ?", now).Where("environment_id = ?", environmentID).Where("archived_at IS NULL").Exec(ctx); err != nil {
		return nil, err
	}
	entities := make([]EnvironmentProcessEntity, 0, len(inputs))
	for _, input := range inputs {
		arguments, _ := json.Marshal(input.Arguments)
		entity := EnvironmentProcessEntity{ID: uuid.New(), CreatedAt: now, UpdatedAt: now, Name: input.Name, Kind: input.Kind, Arguments: arguments, Replicas: input.Replicas, EnvironmentID: environmentID}
		if input.Command != nil {
			entity.Command = sql.NullString{String: *input.Command, Valid: true}
		}
		if input.ContainerPort != nil {
			entity.ContainerPort = sql.NullInt32{Int32: *input.ContainerPort, Valid: true}
		}
		if input.HealthPath != "" {
			entity.HealthPath = sql.NullString{String: input.HealthPath, Valid: true}
		}
		if input.TimeoutSeconds != nil {
			entity.TimeoutSeconds = sql.NullInt32{Int32: *input.TimeoutSeconds, Valid: true}
		}
		if _, err := db.NewInsert().Model(&entity).Exec(ctx); err != nil {
			return nil, err
		}
		entities = append(entities, entity)
	}
	return entities, nil
}
