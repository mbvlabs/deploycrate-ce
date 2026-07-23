package setup

import (
	"embed"
	"fmt"
)

//go:embed scripts/*.sh
var setupScripts embed.FS

func loadScript(name string) (string, error) {
	content, err := setupScripts.ReadFile("scripts/" + name)
	if err != nil {
		return "", fmt.Errorf("load setup script %s: %w", name, err)
	}
	return string(content), nil
}
