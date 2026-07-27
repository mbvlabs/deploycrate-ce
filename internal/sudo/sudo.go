// Package sudo constructs non-interactive privileged commands.
package sudo

import (
	"context"
	"os/exec"
)

const executable = "/usr/bin/sudo"

func CommandContext(ctx context.Context, command string, arguments ...string) *exec.Cmd {
	return commandContext(ctx, false, command, arguments...)
}

func CommandContextPreserveEnvironment(
	ctx context.Context,
	command string,
	arguments ...string,
) *exec.Cmd {
	return commandContext(ctx, true, command, arguments...)
}

func commandContext(
	ctx context.Context,
	preserveEnvironment bool,
	command string,
	arguments ...string,
) *exec.Cmd {
	sudoArguments := []string{"-n"}
	if preserveEnvironment {
		sudoArguments = append(sudoArguments, "-E")
	}
	sudoArguments = append(sudoArguments, "--", command)
	sudoArguments = append(sudoArguments, arguments...)
	return exec.CommandContext(ctx, executable, sudoArguments...)
}
