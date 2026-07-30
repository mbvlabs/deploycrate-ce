package hostcommand

import (
	"bytes"
	"context"
	"fmt"
	"io"
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

func RunStreaming(
	ctx context.Context,
	header []byte,
	stdin io.Reader,
	stdout io.Writer,
	arguments ...string,
) error {
	executable, err := os.Executable()
	if err != nil {
		return fmt.Errorf("resolve DeployCrate executable: %w", err)
	}
	commandArguments := append([]string{"host-resource-access"}, arguments...)
	command := sudo.CommandContext(ctx, executable, commandArguments...)
	command.Stdin = io.MultiReader(bytes.NewReader(header), strings.NewReader("\n"), stdin)
	command.Stdout = stdout
	stderr := &limitedWriter{remaining: 800}
	command.Stderr = stderr
	if err := command.Run(); err != nil {
		message := strings.TrimSpace(stderr.value.String())
		if message == "" {
			message = err.Error()
		}
		return fmt.Errorf("apply resource access host state: %s", message)
	}
	return nil
}

type limitedWriter struct {
	value     strings.Builder
	remaining int
}

func (writer *limitedWriter) Write(value []byte) (int, error) {
	written := len(value)
	if writer.remaining > 0 {
		kept := min(len(value), writer.remaining)
		_, _ = writer.value.Write(value[:kept])
		writer.remaining -= kept
	}
	return written, nil
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
