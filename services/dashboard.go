package services

import (
	"context"
	"fmt"
	"time"

	"deploycrate-ce/internal/storage"
	"deploycrate-ce/models"

	"github.com/google/uuid"
)

type DashboardMetrics struct {
	Applications      int64   `json:"applications" bun:"applications"`
	Environments      int64   `json:"environments" bun:"environments"`
	Deployments       int64   `json:"deployments" bun:"deployments"`
	ActiveDeployments int64   `json:"activeDeployments" bun:"active_deployments"`
	SuccessfulDeploys int64   `json:"successfulDeployments" bun:"successful_deployments"`
	FinishedDeploys   int64   `json:"finishedDeployments" bun:"finished_deployments"`
	DeploymentSuccess float64 `json:"deploymentSuccess"`
	Resources         int64   `json:"resources" bun:"resources"`
	Nodes             int64   `json:"nodes" bun:"nodes"`
}

type DashboardDeploymentActivity struct {
	Day       string `json:"day" bun:"day"`
	Total     int64  `json:"total" bun:"total"`
	Succeeded int64  `json:"succeeded" bun:"succeeded"`
	Failed    int64  `json:"failed" bun:"failed"`
}

type DashboardDeployment struct {
	ID              uuid.UUID `json:"id" bun:"id"`
	ApplicationID   uuid.UUID `json:"applicationId" bun:"application_id"`
	ApplicationName string    `json:"applicationName" bun:"application_name"`
	EnvironmentID   uuid.UUID `json:"environmentId" bun:"environment_id"`
	EnvironmentName string    `json:"environmentName" bun:"environment_name"`
	EnvironmentKind string    `json:"environmentKind" bun:"environment_kind"`
	Status          string    `json:"status" bun:"status"`
	CurrentStep     string    `json:"currentStep" bun:"current_step"`
	SourceRevision  string    `json:"sourceRevision" bun:"source_revision"`
	CreatedAt       time.Time `json:"createdAt" bun:"created_at"`
}

type DashboardApplication struct {
	ID                     uuid.UUID `json:"id" bun:"id"`
	Name                   string    `json:"name" bun:"name"`
	Slug                   string    `json:"slug" bun:"slug"`
	EnvironmentCount       int64     `json:"environmentCount" bun:"environment_count"`
	DeploymentCount        int64     `json:"deploymentCount" bun:"deployment_count"`
	LatestDeploymentStatus string    `json:"latestDeploymentStatus" bun:"latest_deployment_status"`
	LatestDeploymentAt     time.Time `json:"latestDeploymentAt" bun:"latest_deployment_at"`
}

type DashboardSnapshot struct {
	Metrics            DashboardMetrics              `json:"metrics"`
	DeploymentActivity []DashboardDeploymentActivity `json:"deploymentActivity"`
	RecentDeployments  []DashboardDeployment         `json:"recentDeployments"`
	Applications       []DashboardApplication        `json:"applications"`
}

type Dashboard struct {
	db storage.Pool
}

func NewDashboard(db storage.Pool) *Dashboard {
	return &Dashboard{db: db}
}

func (service *Dashboard) Snapshot(ctx context.Context) (DashboardSnapshot, error) {
	exec := service.db.Executor()
	metrics := DashboardMetrics{}
	if err := exec.NewSelect().
		ColumnExpr("(SELECT COUNT(*) FROM applications WHERE archived_at IS NULL AND slug <> ?) AS applications", models.SystemApplicationSlug).
		ColumnExpr("(SELECT COUNT(*) FROM environments AS environment JOIN applications AS application ON application.id = environment.application_id WHERE environment.archived_at IS NULL AND application.archived_at IS NULL AND application.slug <> ?) AS environments", models.SystemApplicationSlug).
		ColumnExpr("(SELECT COUNT(*) FROM deployments AS deployment JOIN releases AS release ON release.id = deployment.release_id JOIN environments AS environment ON environment.id = release.environment_id JOIN applications AS application ON application.id = environment.application_id WHERE application.slug <> ?) AS deployments", models.SystemApplicationSlug).
		ColumnExpr("(SELECT COUNT(*) FROM deployments AS deployment JOIN releases AS release ON release.id = deployment.release_id JOIN environments AS environment ON environment.id = release.environment_id JOIN applications AS application ON application.id = environment.application_id WHERE application.slug <> ? AND deployment.status IN ('queued', 'running', 'in_progress')) AS active_deployments", models.SystemApplicationSlug).
		ColumnExpr("(SELECT COUNT(*) FROM deployments AS deployment JOIN releases AS release ON release.id = deployment.release_id JOIN environments AS environment ON environment.id = release.environment_id JOIN applications AS application ON application.id = environment.application_id WHERE application.slug <> ? AND deployment.status = 'succeeded') AS successful_deployments", models.SystemApplicationSlug).
		ColumnExpr("(SELECT COUNT(*) FROM deployments AS deployment JOIN releases AS release ON release.id = deployment.release_id JOIN environments AS environment ON environment.id = release.environment_id JOIN applications AS application ON application.id = environment.application_id WHERE application.slug <> ? AND deployment.status IN ('succeeded', 'failed')) AS finished_deployments", models.SystemApplicationSlug).
		ColumnExpr("(SELECT COUNT(*) FROM resources WHERE archived_at IS NULL AND system_managed = FALSE) AS resources").
		ColumnExpr("(SELECT COUNT(*) FROM servers WHERE archived_at IS NULL) AS nodes").
		Scan(ctx, &metrics); err != nil {
		return DashboardSnapshot{}, fmt.Errorf("load dashboard metrics: %w", err)
	}
	if metrics.FinishedDeploys > 0 {
		metrics.DeploymentSuccess = float64(metrics.SuccessfulDeploys) / float64(metrics.FinishedDeploys) * 100
	}

	activityRows := make([]DashboardDeploymentActivity, 0, 7)
	if err := exec.NewSelect().
		TableExpr("deployments AS deployment").
		ColumnExpr("TO_CHAR((deployment.created_at AT TIME ZONE 'UTC')::date, 'YYYY-MM-DD') AS day").
		ColumnExpr("COUNT(*) AS total").
		ColumnExpr("COUNT(*) FILTER (WHERE deployment.status = 'succeeded') AS succeeded").
		ColumnExpr("COUNT(*) FILTER (WHERE deployment.status = 'failed') AS failed").
		Join("JOIN releases AS release ON release.id = deployment.release_id").
		Join("JOIN environments AS environment ON environment.id = release.environment_id").
		Join("JOIN applications AS application ON application.id = environment.application_id").
		Where("application.slug <> ?", models.SystemApplicationSlug).
		Where("deployment.created_at >= CURRENT_DATE - INTERVAL '6 days'").
		GroupExpr("(deployment.created_at AT TIME ZONE 'UTC')::date").
		OrderExpr("(deployment.created_at AT TIME ZONE 'UTC')::date").
		Scan(ctx, &activityRows); err != nil {
		return DashboardSnapshot{}, fmt.Errorf("load dashboard deployment activity: %w", err)
	}
	activityByDay := make(map[string]DashboardDeploymentActivity, len(activityRows))
	for _, row := range activityRows {
		activityByDay[row.Day] = row
	}
	activity := make([]DashboardDeploymentActivity, 0, 7)
	today := time.Now().UTC()
	for offset := -6; offset <= 0; offset++ {
		day := today.AddDate(0, 0, offset).Format("2006-01-02")
		row := activityByDay[day]
		row.Day = day
		activity = append(activity, row)
	}

	recentDeployments := make([]DashboardDeployment, 0, 6)
	if err := exec.NewSelect().
		TableExpr("deployments AS deployment").
		ColumnExpr("deployment.id, deployment.status, COALESCE(deployment.current_step, '') AS current_step, deployment.created_at").
		ColumnExpr("application.id AS application_id, application.name AS application_name").
		ColumnExpr("environment.id AS environment_id, environment.name AS environment_name, environment.kind AS environment_kind").
		ColumnExpr("COALESCE(release.source_revision, release.version, '') AS source_revision").
		Join("JOIN releases AS release ON release.id = deployment.release_id").
		Join("JOIN environments AS environment ON environment.id = release.environment_id AND environment.archived_at IS NULL").
		Join("JOIN applications AS application ON application.id = environment.application_id AND application.archived_at IS NULL").
		Where("application.slug <> ?", models.SystemApplicationSlug).
		OrderExpr("deployment.created_at DESC").
		Limit(6).
		Scan(ctx, &recentDeployments); err != nil {
		return DashboardSnapshot{}, fmt.Errorf("load recent dashboard deployments: %w", err)
	}

	applications := make([]DashboardApplication, 0, 5)
	if err := exec.NewSelect().
		TableExpr("applications AS application").
		ColumnExpr("application.id, application.name, application.slug").
		ColumnExpr("COUNT(DISTINCT environment.id) AS environment_count").
		ColumnExpr("COUNT(DISTINCT deployment.id) AS deployment_count").
		ColumnExpr("COALESCE((ARRAY_AGG(deployment.status ORDER BY deployment.created_at DESC) FILTER (WHERE deployment.id IS NOT NULL))[1], '') AS latest_deployment_status").
		ColumnExpr("COALESCE(MAX(deployment.created_at), application.created_at) AS latest_deployment_at").
		Join("LEFT JOIN environments AS environment ON environment.application_id = application.id AND environment.archived_at IS NULL").
		Join("LEFT JOIN releases AS release ON release.environment_id = environment.id").
		Join("LEFT JOIN deployments AS deployment ON deployment.release_id = release.id").
		Where("application.archived_at IS NULL").
		Where("application.slug <> ?", models.SystemApplicationSlug).
		GroupExpr("application.id, application.name, application.slug, application.created_at").
		OrderExpr("MAX(deployment.created_at) DESC NULLS LAST, application.created_at DESC").
		Limit(5).
		Scan(ctx, &applications); err != nil {
		return DashboardSnapshot{}, fmt.Errorf("load dashboard applications: %w", err)
	}

	return DashboardSnapshot{
		Metrics: metrics, DeploymentActivity: activity,
		RecentDeployments: recentDeployments, Applications: applications,
	}, nil
}
