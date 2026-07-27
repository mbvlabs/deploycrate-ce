package wireguard

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

func Apply(ctx context.Context, interfaceName, configurationPath string, desired []byte) error {
	if interfaceName == "" || strings.ContainsAny(interfaceName, "/\\\r\n\x00") {
		return errors.New("WireGuard interface name is invalid")
	}
	if len(desired) == 0 {
		return errors.New("WireGuard desired configuration is empty")
	}
	directory := filepath.Dir(configurationPath)
	if err := os.MkdirAll(directory, 0o700); err != nil {
		return fmt.Errorf("create WireGuard configuration directory: %w", err)
	}
	staged, err := os.CreateTemp(directory, ".wg0.conf.")
	if err != nil {
		return fmt.Errorf("stage WireGuard configuration: %w", err)
	}
	stagedPath := staged.Name()
	defer os.Remove(stagedPath)
	if err := staged.Chmod(0o600); err != nil {
		staged.Close()
		return fmt.Errorf("protect staged WireGuard configuration: %w", err)
	}
	if _, err := staged.Write(desired); err != nil {
		staged.Close()
		return fmt.Errorf("write staged WireGuard configuration: %w", err)
	}
	if err := staged.Sync(); err != nil {
		staged.Close()
		return fmt.Errorf("sync staged WireGuard configuration: %w", err)
	}
	if err := staged.Close(); err != nil {
		return fmt.Errorf("close staged WireGuard configuration: %w", err)
	}

	stripped, err := exec.CommandContext(ctx, "wg-quick", "strip", stagedPath).Output()
	if err != nil {
		return fmt.Errorf("strip staged WireGuard configuration: %w", err)
	}
	live, err := os.CreateTemp(directory, ".wg0.syncconf.")
	if err != nil {
		return fmt.Errorf("stage live WireGuard configuration: %w", err)
	}
	livePath := live.Name()
	defer os.Remove(livePath)
	if err := live.Chmod(0o600); err != nil {
		live.Close()
		return fmt.Errorf("protect live WireGuard configuration: %w", err)
	}
	if _, err := live.Write(stripped); err != nil {
		live.Close()
		return fmt.Errorf("write live WireGuard configuration: %w", err)
	}
	if err := live.Close(); err != nil {
		return fmt.Errorf("close live WireGuard configuration: %w", err)
	}
	if output, err := exec.CommandContext(ctx, "wg", "syncconf", interfaceName, livePath).CombinedOutput(); err != nil {
		return fmt.Errorf("apply live WireGuard peers: %w: %s", err, strings.TrimSpace(string(output)))
	}
	if err := os.Rename(stagedPath, configurationPath); err != nil {
		return fmt.Errorf("persist WireGuard configuration after live apply: %w", err)
	}
	return nil
}
