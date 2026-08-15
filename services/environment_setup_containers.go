package services

import (
	"context"
	"database/sql"
	"deploycrate-ce/models"
	"errors"

	"github.com/google/uuid"
)

type EnvironmentServingContainer struct {
	InstanceID   uuid.UUID `json:"instanceId"`
	DeploymentID uuid.UUID `json:"deploymentId"`
	TargetID     uuid.UUID `json:"targetId"`
	ServerID     uuid.UUID `json:"serverId"`
	Exists       bool      `json:"exists"`
	Running      bool      `json:"running"`
}

func (service *EnvironmentSetup) ServingContainer(
	ctx context.Context,
	applicationID, environmentID uuid.UUID,
) (EnvironmentServingContainer, error) {
	container := EnvironmentServingContainer{}
	if _, err := models.Environment.FindForApplication(
		ctx,
		service.db.Executor(),
		applicationID,
		environmentID,
	); err != nil {
		return container, err
	}
	instance, err := models.Instance.ServingForEnvironment(ctx, service.db.Executor(), environmentID)
	if errors.Is(err, sql.ErrNoRows) {
		return container, nil
	}
	if err != nil {
		return container, err
	}
	target, err := models.EnvironmentTarget.Find(
		ctx,
		service.db.Executor(),
		instance.EnvironmentTargetID,
	)
	if err != nil {
		return container, err
	}
	container.InstanceID = instance.ID
	container.DeploymentID = instance.DeploymentID
	container.TargetID = instance.EnvironmentTargetID
	container.ServerID = target.ServerID
	state, err := service.workloads.Find(
		ctx,
		target.ServerID,
		instance.DeploymentID,
		instance.ID,
	)
	if err != nil {
		return container, err
	}
	container.Exists = state.Exists
	container.Running = state.Exists && state.Running
	return container, nil
}

func (service *EnvironmentSetup) RestartServingContainer(
	ctx context.Context,
	applicationID, environmentID uuid.UUID,
) error {
	container, err := service.ServingContainer(ctx, applicationID, environmentID)
	if err != nil {
		return err
	}
	if container.InstanceID == uuid.Nil || !container.Exists {
		return errors.New("no serving container is available to restart")
	}
	_, err = service.workloads.Restart(
		ctx,
		container.ServerID,
		container.DeploymentID,
		container.InstanceID,
	)
	return err
}

func (service *EnvironmentSetup) StartServingContainer(
	ctx context.Context,
	applicationID, environmentID uuid.UUID,
) error {
	container, err := service.ServingContainer(ctx, applicationID, environmentID)
	if err != nil {
		return err
	}
	if container.InstanceID == uuid.Nil || !container.Exists {
		return errors.New("no serving container is available to start")
	}
	if container.Running {
		return nil
	}
	_, err = service.workloads.Start(
		ctx,
		container.ServerID,
		container.DeploymentID,
		container.InstanceID,
	)
	return err
}
