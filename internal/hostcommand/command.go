package hostcommand

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"strings"

	"deploycrate-ce/internal/sudo"
)

func Run(ctx context.Context, arguments ...string) (string, error) {
	return run(ctx, nil, arguments...)
}

func RunWithInput(ctx context.Context, input []byte, arguments ...string) (string, error) {
	return run(ctx, input, arguments...)
}

func run(ctx context.Context, input []byte, arguments ...string) (string, error) {
	executable, err := os.Executable()
	if err != nil {
		return "", fmt.Errorf("resolve DeployCrate executable: %w", err)
	}
	commandArguments := append([]string{"host-resource-access"}, arguments...)
	command := sudo.CommandContext(ctx, executable, commandArguments...)
	if input != nil {
		command.Stdin = bytes.NewReader(input)
	}
	output, err := command.CombinedOutput()
	if err != nil {
		message := strings.TrimSpace(string(output))
		if message == "" {
			message = err.Error()
		}
		return "", fmt.Errorf("apply resource access host state: %s", message)
	}
	return strings.TrimSpace(string(output)), nil
}
