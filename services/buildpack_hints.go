package services

import (
	"context"
	"encoding/json"
	"errors"
	"path"
	"strings"

	githubclient "deploycrate-ce/clients/github"
	"deploycrate-ce/models"

	"github.com/google/uuid"
)

type BuildpackRepositoryHints struct {
	HasGoMod                   bool     `json:"hasGoMod"`
	HasPackageJSON             bool     `json:"hasPackageJson"`
	PackageManager             string   `json:"packageManager,omitempty"`
	HasLockfile                bool     `json:"hasLockfile"`
	Scripts                    []string `json:"scripts,omitempty"`
	HasBuildScript             bool     `json:"hasBuildScript"`
	HasSSRScript               bool     `json:"hasSSRScript"`
	SuggestedGoTargets         []string `json:"suggestedGoTargets,omitempty"`
	SuggestedFrontendDirectory string   `json:"suggestedFrontendDirectory,omitempty"`
	Warnings                   []string `json:"warnings,omitempty"`
}

func (service *ApplicationSetup) RepositoryBuildHints(
	ctx context.Context,
	repositoryID uuid.UUID,
	reference string,
	contextPath string,
) (BuildpackRepositoryHints, error) {
	hints := BuildpackRepositoryHints{}
	reference = strings.TrimSpace(reference)
	contextPath = strings.TrimSpace(contextPath)
	if contextPath == "" {
		contextPath = "."
	}
	cleanContext := path.Clean(contextPath)
	if cleanContext == ".." || strings.HasPrefix(cleanContext, "../") {
		return hints, errors.New("build context cannot leave the repository root")
	}

	repository, err := models.GitHubRepository.Find(ctx, service.db.Executor(), repositoryID)
	if err != nil {
		return hints, err
	}
	if repository.RemovedAt.Valid {
		return hints, errors.New("GitHub repository is unavailable")
	}
	installation, err := models.GitHubInstallation.Find(
		ctx,
		service.db.Executor(),
		repository.GitHubInstallationID,
	)
	if err != nil {
		return hints, err
	}
	if installation.ArchivedAt.Valid || installation.SuspendedAt.Valid {
		return hints, errors.New("GitHub installation is unavailable")
	}
	app, err := models.GitHubApp.Find(ctx, service.db.Executor(), installation.GitHubAppID)
	if err != nil {
		return hints, err
	}
	connection := NewGitHubConnection(service.db, service.cfg, githubclient.NewClient())
	if reference == "" {
		reference = repository.DefaultBranch
	}
	revision, err := connection.ResolveRevision(ctx, installation, repository, reference)
	if err != nil {
		return hints, err
	}
	authentication, err := connection.authentication(ctx, app)
	if err != nil {
		return hints, err
	}
	client := githubclient.NewClient()
	prefix := cleanContext
	if prefix == "." {
		prefix = ""
	}
	joinPath := func(relative string) string {
		if prefix == "" {
			return relative
		}
		return path.Join(prefix, relative)
	}

	if _, found, err := client.GetFileContent(
		ctx,
		authentication,
		installation.ExternalID,
		repository.FullName,
		joinPath("go.mod"),
		revision,
	); err != nil {
		return hints, err
	} else if found {
		hints.HasGoMod = true
	} else {
		hints.Warnings = append(hints.Warnings, "No go.mod found in the build context.")
	}

	packageJSONPath := joinPath("package.json")
	packageJSON, found, err := client.GetFileContent(
		ctx,
		authentication,
		installation.ExternalID,
		repository.FullName,
		packageJSONPath,
		revision,
	)
	if err != nil {
		return hints, err
	}
	if found {
		hints.HasPackageJSON = true
		hints.SuggestedFrontendDirectory = "."
		if prefix != "" {
			hints.SuggestedFrontendDirectory = prefix
		}
		var manifest struct {
			Scripts map[string]string `json:"scripts"`
		}
		if err := json.Unmarshal(packageJSON, &manifest); err != nil {
			hints.Warnings = append(hints.Warnings, "package.json could not be parsed.")
		} else {
			for name, command := range manifest.Scripts {
				if strings.TrimSpace(command) == "" {
					continue
				}
				hints.Scripts = append(hints.Scripts, name)
				switch name {
				case "build":
					hints.HasBuildScript = true
				case "build:ssr":
					hints.HasSSRScript = true
				}
			}
		}
		lockfiles := map[string]string{
			"package-lock.json":   "npm",
			"npm-shrinkwrap.json": "npm",
			"pnpm-lock.yaml":      "pnpm",
			"yarn.lock":           "yarn",
			"bun.lock":            "bun",
			"bun.lockb":           "bun",
		}
		foundManagers := make([]string, 0, 1)
		for lockfile, manager := range lockfiles {
			_, exists, lockErr := client.GetFileContent(
				ctx,
				authentication,
				installation.ExternalID,
				repository.FullName,
				joinPath(lockfile),
				revision,
			)
			if lockErr != nil {
				return hints, lockErr
			}
			if exists {
				hints.HasLockfile = true
				foundManagers = append(foundManagers, manager)
			}
		}
		switch len(foundManagers) {
		case 0:
			hints.Warnings = append(
				hints.Warnings,
				"No supported lockfile found beside package.json.",
			)
		case 1:
			hints.PackageManager = foundManagers[0]
		default:
			hints.Warnings = append(
				hints.Warnings,
				"Multiple lockfiles found. Keep only the one for your package manager.",
			)
		}
	} else if prefix != "" {
		rootPackageJSON, rootFound, rootErr := client.GetFileContent(
			ctx,
			authentication,
			installation.ExternalID,
			repository.FullName,
			"package.json",
			revision,
		)
		if rootErr != nil {
			return hints, rootErr
		}
		if rootFound {
			hints.Warnings = append(
				hints.Warnings,
				"package.json lives at the repository root, not inside the build context.",
			)
			hints.SuggestedFrontendDirectory = "."
			var manifest struct {
				Scripts map[string]string `json:"scripts"`
			}
			if err := json.Unmarshal(rootPackageJSON, &manifest); err == nil {
				hints.HasPackageJSON = true
				for name, command := range manifest.Scripts {
					if strings.TrimSpace(command) == "" {
						continue
					}
					hints.Scripts = append(hints.Scripts, name)
					switch name {
					case "build":
						hints.HasBuildScript = true
					case "build:ssr":
						hints.HasSSRScript = true
					}
				}
			}
		}
	}

	goTargets := []string{"cmd/server", "cmd/app", "cmd/web", "cmd/api"}
	for _, target := range goTargets {
		mainPath := joinPath(path.Join(target, "main.go"))
		_, exists, targetErr := client.GetFileContent(
			ctx,
			authentication,
			installation.ExternalID,
			repository.FullName,
			mainPath,
			revision,
		)
		if targetErr != nil {
			return hints, targetErr
		}
		if exists {
			hints.SuggestedGoTargets = append(hints.SuggestedGoTargets, "./"+target)
		}
	}
	return hints, nil
}
