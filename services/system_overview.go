package services

import (
	"context"
	"fmt"

	"deploycrate-ce/models"
)

type SystemOverview = models.SystemOverview

func (s *SelfUpdate) Overview(ctx context.Context) (SystemOverview, error) {
	if _, err := models.Application.FindSystem(ctx, s.db.Executor()); err != nil {
		return SystemOverview{}, fmt.Errorf("find DeployCrate CE system application: %w", err)
	}

	overview, err := models.Application.FindSystemOverview(ctx, s.db.Executor())
	if err != nil {
		return SystemOverview{}, fmt.Errorf("load DeployCrate CE system overview: %w", err)
	}

	return overview, nil
}
