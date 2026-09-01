package services

import (
	"testing"

	"deploycrate-ce/models"

	"github.com/google/uuid"
)

func TestEnvironmentSecretDeploymentStatus(t *testing.T) {
	t.Parallel()

	secretID := uuid.New()
	descriptor := models.EnvironmentSecretDescriptor{
		ID:         secretID,
		Key:        "API_KEY",
		Digest:     "hmac-sha256:aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa",
		SourceType: models.EnvironmentSecretSourceUser,
		SourceID:   uuid.New(),
	}
	appliedRevisionID := uuid.New()
	targetStates := []models.EnvironmentTargetStateEntity{{
		AppliedRevisionID: &appliedRevisionID,
	}}
	appliedByTarget := []map[string]models.EnvironmentSecretDescriptor{{
		"API_KEY": descriptor,
	}}
	applyingByTarget := []map[string]models.EnvironmentSecretDescriptor{{}}

	status := environmentSecretDeploymentStatus(
		descriptor,
		targetStates,
		appliedByTarget,
		applyingByTarget,
		false,
	)
	if status != "deployed" {
		t.Fatalf("status = %q, want deployed", status)
	}

	newSecret := models.EnvironmentSecretDescriptor{
		ID:         uuid.New(),
		Key:        "NEW_SECRET",
		Digest:     "hmac-sha256:bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb",
		SourceType: models.EnvironmentSecretSourceUser,
		SourceID:   uuid.New(),
	}
	status = environmentSecretDeploymentStatus(
		newSecret,
		targetStates,
		appliedByTarget,
		applyingByTarget,
		false,
	)
	if status != "pending" {
		t.Fatalf("new secret status = %q, want pending", status)
	}
}

func TestEnvironmentSecretDeploymentStatusSkipsTargetsWithoutAppliedRevision(t *testing.T) {
	t.Parallel()

	secretID := uuid.New()
	descriptor := models.EnvironmentSecretDescriptor{
		ID:         secretID,
		Key:        "API_KEY",
		Digest:     "hmac-sha256:aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa",
		SourceType: models.EnvironmentSecretSourceUser,
		SourceID:   uuid.New(),
	}
	appliedRevisionID := uuid.New()
	targetStates := []models.EnvironmentTargetStateEntity{
		{AppliedRevisionID: &appliedRevisionID},
		{AppliedRevisionID: nil},
	}
	appliedByTarget := []map[string]models.EnvironmentSecretDescriptor{
		{"API_KEY": descriptor},
		{},
	}

	status := environmentSecretDeploymentStatus(
		descriptor,
		targetStates,
		appliedByTarget,
		[]map[string]models.EnvironmentSecretDescriptor{{}, {}},
		false,
	)
	if status != "deployed" {
		t.Fatalf("status = %q, want deployed when only applied targets are checked", status)
	}
}

func TestEnvironmentSecretDeploymentStatusOnlyChangedSecretPendingAfterAdd(t *testing.T) {
	t.Parallel()

	existing := models.EnvironmentSecretDescriptor{
		ID:         uuid.New(),
		Key:        "EXISTING",
		Digest:     "hmac-sha256:cccccccccccccccccccccccccccccccccccccccccccccccccccccccccccccccc",
		SourceType: models.EnvironmentSecretSourceUser,
		SourceID:   uuid.New(),
	}
	added := models.EnvironmentSecretDescriptor{
		ID:         uuid.New(),
		Key:        "ADDED",
		Digest:     "hmac-sha256:dddddddddddddddddddddddddddddddddddddddddddddddddddddddddddddddd",
		SourceType: models.EnvironmentSecretSourceUser,
		SourceID:   uuid.New(),
	}
	appliedRevisionID := uuid.New()
	targetStates := []models.EnvironmentTargetStateEntity{{
		AppliedRevisionID: &appliedRevisionID,
	}}
	appliedByTarget := []map[string]models.EnvironmentSecretDescriptor{{
		"EXISTING": existing,
	}}
	applyingByTarget := []map[string]models.EnvironmentSecretDescriptor{{}}

	existingStatus := environmentSecretDeploymentStatus(
		existing,
		targetStates,
		appliedByTarget,
		applyingByTarget,
		false,
	)
	if existingStatus != "deployed" {
		t.Fatalf("existing secret status = %q, want deployed", existingStatus)
	}

	addedStatus := environmentSecretDeploymentStatus(
		added,
		targetStates,
		appliedByTarget,
		applyingByTarget,
		false,
	)
	if addedStatus != "pending" {
		t.Fatalf("added secret status = %q, want pending", addedStatus)
	}
}
