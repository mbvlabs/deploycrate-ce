package hostcommand

import (
	"context"
	"fmt"
	"os"
	"strings"

	"deploycrate-ce/internal/sudo"
)

func Run(ctx context.Context, arguments ...string) (string, error) {
	executable, err := os.Executable()
	if err != nil {
		return "", fmt.Errorf("resolve DeployCrate executable: %w", err)
	}
	commandArguments := append([]string{"host-resource-access"}, arguments...)
	output, err := sudo.CommandContext(ctx, executable, commandArguments...).CombinedOutput()
	if err != nil {
		message := strings.TrimSpace(string(output))
		if message == "" {
			message = err.Error()
		}
		return "", fmt.Errorf("apply resource access host state: %s", message)
	}
	return strings.TrimSpace(string(output)), nil
}
