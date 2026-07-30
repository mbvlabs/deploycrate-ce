package resourceaccess

import (
	"encoding/base64"
	"errors"
	"fmt"
	"net/netip"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/google/uuid"
)

const (
	interfaceName   = "wg0"
	wireGuardServer = "10.99.0.1"
)

func RunHostCommand(arguments []string) error {
	if os.Geteuid() != 0 {
		return errors.New("host resource access command must run as root")
	}
	if len(arguments) == 0 {
		return errors.New("host resource access operation is required")
	}
	switch arguments[0] {
	case "peer-apply":
		if len(arguments) != 3 {
			return errors.New("usage: host-resource-access peer-apply PUBLIC_KEY PRIVATE_ADDRESS")
		}
		if err := validatePublicKey(arguments[1]); err != nil {
			return err
		}
		if err := validateDeviceAddress(arguments[2]); err != nil {
			return err
		}
		if err := run("/usr/bin/wg", "set", interfaceName, "peer", arguments[1], "allowed-ips", arguments[2]+"/32"); err != nil {
			return err
		}
		return run("/usr/bin/wg-quick", "save", interfaceName)
	case "peer-remove":
		if len(arguments) != 2 {
			return errors.New("usage: host-resource-access peer-remove PUBLIC_KEY")
		}
		if err := validatePublicKey(arguments[1]); err != nil {
			return err
		}
		if err := run("/usr/bin/wg", "set", interfaceName, "peer", arguments[1], "remove"); err != nil {
			return err
		}
		return run("/usr/bin/wg-quick", "save", interfaceName)
	case "peer-handshake":
		if len(arguments) != 2 {
			return errors.New("usage: host-resource-access peer-handshake PUBLIC_KEY")
		}
		if err := validatePublicKey(arguments[1]); err != nil {
			return err
		}
		output, err := exec.Command("/usr/bin/wg", "show", interfaceName, "latest-handshakes").Output()
		if err != nil {
			return fmt.Errorf("read WireGuard handshakes: %w", err)
		}
		for line := range strings.SplitSeq(string(output), "\n") {
			fields := strings.Fields(line)
			if len(fields) == 2 && fields[0] == arguments[1] {
				fmt.Println(fields[1])
				return nil
			}
		}
		fmt.Println("missing")
		return nil
	case "firewall-apply", "firewall-remove":
		if len(arguments) != 5 {
			return errors.New("usage: host-resource-access firewall-(apply|remove) GRANT_ID SOURCE_ADDRESS DESTINATION_PORT tcp")
		}
		if _, err := uuid.Parse(arguments[1]); err != nil {
			return errors.New("grant ID must be a UUID")
		}
		if err := validateDeviceAddress(arguments[2]); err != nil {
			return err
		}
		port, err := validatePort(arguments[3])
		if err != nil {
			return err
		}
		if arguments[4] != "tcp" {
			return errors.New("only TCP firewall rules are supported")
		}
		ufwArguments := []string{"allow", "in", "on", interfaceName, "from", arguments[2], "to", wireGuardServer, "port", strconv.Itoa(port), "proto", "tcp", "comment", "deploycrate-grant-" + arguments[1]}
		if arguments[0] == "firewall-remove" {
			ufwArguments = append([]string{"--force", "delete"}, ufwArguments...)
			return runAllowMissingFirewallRule("/usr/sbin/ufw", ufwArguments...)
		}
		return run("/usr/sbin/ufw", ufwArguments...)
	case "listener-apply":
		if len(arguments) != 6 {
			return errors.New("usage: host-resource-access listener-apply RESOURCE_ID WIREGUARD_ADDRESS PORT ORIGIN_ADDRESS ORIGIN_PORT")
		}
		resourceID, err := uuid.Parse(arguments[1])
		if err != nil {
			return errors.New("resource ID must be a UUID")
		}
		if arguments[2] != wireGuardServer {
			return errors.New("listener must bind to the DeployCrate WireGuard address")
		}
		port, err := validatePort(arguments[3])
		if err != nil {
			return err
		}
		originValue := arguments[4]
		if originValue == "localhost" {
			originValue = "127.0.0.1"
		}
		origin, err := netip.ParseAddr(originValue)
		if err != nil || !origin.IsLoopback() {
			return errors.New("listener origin must be a loopback address")
		}
		originPort, err := validatePort(arguments[5])
		if err != nil {
			return err
		}
		return applyListener(resourceID, arguments[2], port, origin.String(), originPort)
	case "listener-remove":
		if len(arguments) != 2 {
			return errors.New("usage: host-resource-access listener-remove RESOURCE_ID")
		}
		resourceID, err := uuid.Parse(arguments[1])
		if err != nil {
			return errors.New("resource ID must be a UUID")
		}
		return removeListener(resourceID)
	case "container-run":
		if len(arguments) != 1 {
			return errors.New("usage: host-resource-access container-run")
		}
		return runContainer(os.Stdin)
	case "container-inspect":
		if len(arguments) != 3 {
			return errors.New("usage: host-resource-access container-inspect INSTALLATION_ID CONTAINER_NAME")
		}
		return printContainerInspection(arguments[1], arguments[2])
	case "container-exec":
		if len(arguments) != 1 {
			return errors.New("usage: host-resource-access container-exec")
		}
		return execContainer(os.Stdin, os.Stdout)
	case "container-start", "container-stop", "container-restart", "container-remove":
		if len(arguments) != 3 {
			return errors.New("usage: host-resource-access container-(start|stop|restart|remove) INSTALLATION_ID CONTAINER_NAME")
		}
		return controlContainer(strings.TrimPrefix(arguments[0], "container-"), arguments[1], arguments[2])
	default:
		return fmt.Errorf("unknown host resource access operation %q", arguments[0])
	}
}

func validatePublicKey(value string) error {
	decoded, err := base64.StdEncoding.DecodeString(strings.TrimSpace(value))
	if err != nil || len(decoded) != 32 {
		return errors.New("public key must be a base64-encoded 32-byte WireGuard key")
	}
	return nil
}

func validateDeviceAddress(value string) error {
	address, err := netip.ParseAddr(strings.TrimSpace(value))
	network := netip.MustParsePrefix("10.99.0.0/16")
	if err != nil || !address.Is4() || !network.Contains(address) || address.String() == wireGuardServer || address == network.Addr() {
		return errors.New("device address must be an allocatable host in 10.99.0.0/16")
	}
	return nil
}

func validatePort(value string) (int, error) {
	port, err := strconv.Atoi(value)
	if err != nil || port < 1 || port > 65535 {
		return 0, errors.New("port must be between 1 and 65535")
	}
	return port, nil
}

func applyListener(resourceID uuid.UUID, address string, port int, origin string, originPort int) error {
	proxy, err := socketProxyPath()
	if err != nil {
		return err
	}
	unit := "deploycrate-resource-" + resourceID.String()
	socketPath := filepath.Join("/etc/systemd/system", unit+".socket")
	servicePath := filepath.Join("/etc/systemd/system", unit+".service")
	socket := fmt.Sprintf("[Unit]\nDescription=DeployCrate private resource listener %s\nAfter=wg-quick@wg0.service\nRequires=wg-quick@wg0.service\n\n[Socket]\nListenStream=%s:%d\nBindToDevice=%s\nFreeBind=true\nNoDelay=true\n\n[Install]\nWantedBy=sockets.target\n", resourceID, address, port, interfaceName)
	service := fmt.Sprintf("[Unit]\nDescription=DeployCrate private resource proxy %s\n\n[Service]\nExecStart=%s %s:%d\nPrivateTmp=true\nProtectSystem=strict\nProtectHome=true\nNoNewPrivileges=true\n", resourceID, proxy, origin, originPort)
	if err := os.WriteFile(socketPath, []byte(socket), 0o644); err != nil {
		return fmt.Errorf("write resource socket unit: %w", err)
	}
	if err := os.WriteFile(servicePath, []byte(service), 0o644); err != nil {
		return fmt.Errorf("write resource service unit: %w", err)
	}
	if err := run("/usr/bin/systemctl", "daemon-reload"); err != nil {
		return err
	}
	return run("/usr/bin/systemctl", "enable", "--now", unit+".socket")
}

func removeListener(resourceID uuid.UUID) error {
	unit := "deploycrate-resource-" + resourceID.String()
	if err := runAllowInactive("/usr/bin/systemctl", "disable", "--now", unit+".socket", unit+".service"); err != nil {
		return err
	}
	for _, suffix := range []string{".socket", ".service"} {
		path := filepath.Join("/etc/systemd/system", unit+suffix)
		if err := os.Remove(path); err != nil && !errors.Is(err, os.ErrNotExist) {
			return fmt.Errorf("remove resource listener unit: %w", err)
		}
	}
	return run("/usr/bin/systemctl", "daemon-reload")
}

func socketProxyPath() (string, error) {
	for _, candidate := range []string{"/usr/lib/systemd/systemd-socket-proxyd", "/lib/systemd/systemd-socket-proxyd"} {
		if info, err := os.Stat(candidate); err == nil && !info.IsDir() {
			return candidate, nil
		}
	}
	return "", errors.New("systemd-socket-proxyd is not installed")
}

func run(name string, arguments ...string) error {
	output, err := exec.Command(name, arguments...).CombinedOutput()
	if err != nil {
		return fmt.Errorf("run %s: %w: %s", name, err, strings.TrimSpace(string(output)))
	}
	return nil
}

func runAllowInactive(name string, arguments ...string) error {
	output, err := exec.Command(name, arguments...).CombinedOutput()
	if err != nil && !strings.Contains(string(output), "not loaded") && !strings.Contains(string(output), "does not exist") {
		return fmt.Errorf("run %s: %w: %s", name, err, strings.TrimSpace(string(output)))
	}
	return nil
}

func runAllowMissingFirewallRule(name string, arguments ...string) error {
	output, err := exec.Command(name, arguments...).CombinedOutput()
	message := string(output)
	if err != nil && !strings.Contains(message, "Could not delete non-existent rule") && !strings.Contains(message, "Skipping") {
		return fmt.Errorf("run %s: %w: %s", name, err, strings.TrimSpace(message))
	}
	return nil
}
