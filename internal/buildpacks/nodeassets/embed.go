package nodeassets

import (
	"embed"
	"errors"
	"fmt"
	"os"
	"path/filepath"
)

//go:embed buildpack.toml bin/detect bin/build
var files embed.FS

func Materialize(parent string) (string, error) {
	if !filepath.IsAbs(parent) {
		return "", errors.New("node assets buildpack directory must be absolute")
	}
	root := filepath.Join(parent, "node-assets")
	if err := os.MkdirAll(filepath.Join(root, "bin"), 0o755); err != nil {
		return "", fmt.Errorf("create node assets buildpack directory: %w", err)
	}
	for _, file := range []struct {
		name string
		mode os.FileMode
	}{
		{name: "buildpack.toml", mode: 0o644},
		{name: "bin/detect", mode: 0o755},
		{name: "bin/build", mode: 0o755},
	} {
		value, err := files.ReadFile(file.name)
		if err != nil {
			return "", fmt.Errorf("read embedded node assets buildpack file %s: %w", file.name, err)
		}
		if err := os.WriteFile(
			filepath.Join(root, filepath.FromSlash(file.name)),
			value,
			file.mode,
		); err != nil {
			return "", fmt.Errorf("materialize node assets buildpack file %s: %w", file.name, err)
		}
	}
	return root, nil
}
