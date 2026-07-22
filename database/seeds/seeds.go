package seeds

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"os"
	"slices"
	"strings"
	"time"

	"deploycrate-ce/internal/storage"
	"deploycrate-ce/models"
	"deploycrate-ce/models/factories"
)

const Default = "development"

const (
	developmentAdminEmail    = "admin@deploycrate.com"
	developmentAdminPassword = "password123"
)

type Runner func(context.Context, storage.Executor) error

var Registry = map[string]Runner{
	"default":     Development,
	"development": Development,
	"test":        Test,
	"ui":          UI,
}

func Names() []string {
	names := make([]string, 0, len(Registry))
	for name := range Registry {
		names = append(names, name)
	}
	slices.Sort(names)
	return names
}

func Run(ctx context.Context, exec storage.Executor, name string) error {
	if name == "" {
		name = Default
	}

	runner, ok := Registry[name]
	if !ok {
		return fmt.Errorf("unknown seed %q (available: %s)", name, strings.Join(Names(), ", "))
	}

	return runner(ctx, exec)
}

func Development(ctx context.Context, exec storage.Executor) error {
	pepper := os.Getenv("PEPPER")
	if pepper == "" {
		pepper = factories.TestPepper
	}

	password, err := models.HashPassword(developmentAdminPassword, pepper)
	if err != nil {
		return fmt.Errorf("hash development admin password: %w", err)
	}

	admin, err := models.User.FindByEmail(ctx, exec, developmentAdminEmail)
	if err == nil {
		validatedAt := admin.EmailValidatedAt
		if !validatedAt.Valid {
			validatedAt = sql.NullTime{Time: time.Now(), Valid: true}
		}

		admin, err = models.User.Update(ctx, exec, models.UpdateUserData{
			ID:               admin.ID,
			Email:            developmentAdminEmail,
			EmailValidatedAt: validatedAt,
			Password:         []byte(password),
			IsAdmin:          true,
		})
		if err != nil {
			return fmt.Errorf("update development admin user: %w", err)
		}

		fmt.Printf("Updated development admin user: %s\n", admin.Email)
		return nil
	}
	if !errors.Is(err, models.ErrNotFound) {
		return fmt.Errorf("find development admin user: %w", err)
	}

	admin, err = factories.CreateUser(ctx, exec,
		factories.WithEmail(developmentAdminEmail),
		factories.WithPassword([]byte(password)),
		factories.WithIsAdmin(true),
		factories.WithValidatedEmail(),
	)
	if err != nil {
		return fmt.Errorf("create development admin user: %w", err)
	}
	fmt.Printf("Created development admin user: %s\n", admin.Email)

	return nil
}

func Test(ctx context.Context, exec storage.Executor) error {
	_, err := factories.CreateUser(ctx, exec,
		factories.WithEmail("test@example.com"),
		factories.WithValidatedEmail(),
	)
	if err != nil {
		return fmt.Errorf("failed to create test user: %w", err)
	}

	return nil
}
