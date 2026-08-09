package services

import (
	"context"

	"deploycrate-ce/internal/storage"
	"deploycrate-ce/models"
)

type DashboardMetrics = models.DashboardMetrics
type DashboardDeploymentActivity = models.DashboardDeploymentActivity
type DashboardDeployment = models.DashboardDeployment
type DashboardApplication = models.DashboardApplication
type DashboardSnapshot = models.DashboardSnapshot

type Dashboard struct {
	db storage.Pool
}

func NewDashboard(db storage.Pool) *Dashboard {
	return &Dashboard{db: db}
}

func (service *Dashboard) Snapshot(ctx context.Context) (DashboardSnapshot, error) {
	return models.Dashboard.Snapshot(ctx, service.db.Executor())
}
