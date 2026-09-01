package services

import (
	"deploycrate-ce/models"

	"github.com/google/uuid"
)

func environmentSecretDeploymentStatus(
	descriptor models.EnvironmentSecretDescriptor,
	targetStates []models.EnvironmentTargetStateEntity,
	appliedByTarget []map[string]models.EnvironmentSecretDescriptor,
	applyingByTarget []map[string]models.EnvironmentSecretDescriptor,
	desiredRevisionFailed bool,
) string {
	if environmentSecretDeployedOnTargets(descriptor, targetStates, appliedByTarget) {
		return "deployed"
	}
	if desiredRevisionFailed {
		return "failed"
	}
	if environmentSecretDeployingOnTargets(descriptor, applyingByTarget) {
		return "deploying"
	}
	return "pending"
}

func environmentSecretDeployedOnTargets(
	descriptor models.EnvironmentSecretDescriptor,
	targetStates []models.EnvironmentTargetStateEntity,
	appliedByTarget []map[string]models.EnvironmentSecretDescriptor,
) bool {
	checkedTarget := false
	for index := range targetStates {
		if targetStates[index].AppliedRevisionID == nil {
			continue
		}
		checkedTarget = true
		if !sameEnvironmentSecretDescriptor(
			descriptor,
			appliedByTarget[index][descriptor.Key],
		) {
			return false
		}
	}
	return checkedTarget
}

func environmentSecretDeployingOnTargets(
	descriptor models.EnvironmentSecretDescriptor,
	applyingByTarget []map[string]models.EnvironmentSecretDescriptor,
) bool {
	for _, applying := range applyingByTarget {
		if sameEnvironmentSecretDescriptor(descriptor, applying[descriptor.Key]) {
			return true
		}
	}
	return false
}

func sameEnvironmentSecretDescriptor(left, right models.EnvironmentSecretDescriptor) bool {
	return right.ID != uuid.Nil && left.Key == right.Key && left.Digest == right.Digest
}
