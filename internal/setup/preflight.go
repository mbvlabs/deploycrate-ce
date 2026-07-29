package setup

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"os/exec"
	"strconv"
	"strings"
	"syscall"
	"time"
)

const publicIPv4Endpoint = "https://api.ipify.org"

type HostInfo struct {
	Distribution string
	Version      string
	Architecture string
	MemoryMB     uint64
	DiskFreeMB   uint64
	PublicIPv4   string
}

type InstallLock struct {
	file *os.File
}

func AcquireInstallLock(dryRun bool) (*InstallLock, error) {
	if dryRun {
		return &InstallLock{}, nil
	}
	_, _, stateDir := ConfigPaths()
	if err := os.MkdirAll(stateDir, 0o700); err != nil {
		return nil, fmt.Errorf("create installer state directory: %w", err)
	}
	file, err := os.OpenFile(stateDir+"/install.lock", os.O_CREATE|os.O_RDWR, 0o600)
	if err != nil {
		return nil, fmt.Errorf("open installer lock: %w", err)
	}
	if err := syscall.Flock(int(file.Fd()), syscall.LOCK_EX|syscall.LOCK_NB); err != nil {
		file.Close()
		return nil, errors.New("another DeployCrate CE installer process is running")
	}
	return &InstallLock{file: file}, nil
}

func (lock *InstallLock) Close() error {
	if lock == nil || lock.file == nil {
		return nil
	}
	_ = syscall.Flock(int(lock.file.Fd()), syscall.LOCK_UN)
	return lock.file.Close()
}

func Preflight(ctx context.Context, dryRun bool) (HostInfo, error) {
	if dryRun {
		return HostInfo{
			Distribution: "debian",
			Version:      "13",
			Architecture: "amd64",
			MemoryMB:     4096,
			DiskFreeMB:   20480,
			PublicIPv4:   "203.0.113.10",
		}, nil
	}
	if os.Geteuid() != 0 {
		return HostInfo{}, errors.New("run the installer as root with sudo")
	}

	osRelease, err := parseOSRelease()
	if err != nil {
		return HostInfo{}, err
	}
	if osRelease["ID"] != "debian" || !strings.HasPrefix(osRelease["VERSION_ID"], "13") {
		return HostInfo{}, fmt.Errorf(
			"unsupported operating system %s %s; this release supports Debian 13",
			osRelease["ID"],
			osRelease["VERSION_ID"],
		)
	}
	for _, command := range []string{"apt-get", "bash", "systemctl", "sshd"} {
		if _, err := exec.LookPath(command); err != nil {
			return HostInfo{}, fmt.Errorf("required command %s is not available", command)
		}
	}

	architectureBytes, err := exec.CommandContext(ctx, "dpkg", "--print-architecture").Output()
	if err != nil {
		return HostInfo{}, fmt.Errorf("detect architecture: %w", err)
	}
	architecture := strings.TrimSpace(string(architectureBytes))
	if architecture != "amd64" && architecture != "arm64" {
		return HostInfo{}, fmt.Errorf("unsupported architecture %s", architecture)
	}

	var stat syscall.Statfs_t
	if err := syscall.Statfs("/", &stat); err != nil {
		return HostInfo{}, fmt.Errorf("inspect root filesystem: %w", err)
	}
	diskFreeMB := stat.Bavail * uint64(stat.Bsize) / 1024 / 1024
	if diskFreeMB < 10240 {
		return HostInfo{}, fmt.Errorf(
			"at least 10240 MB of free disk is required, found %d MB",
			diskFreeMB,
		)
	}
	memoryMB, err := availableMemoryMB()
	if err != nil {
		return HostInfo{}, err
	}
	publicIPv4, err := detectPublicIPv4(ctx)
	if err != nil {
		return HostInfo{}, err
	}
	return HostInfo{
		Distribution: osRelease["ID"], Version: osRelease["VERSION_ID"], Architecture: architecture,
		MemoryMB: memoryMB, DiskFreeMB: diskFreeMB, PublicIPv4: publicIPv4,
	}, nil
}

func detectPublicIPv4(ctx context.Context) (string, error) {
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, publicIPv4Endpoint, nil)
	if err != nil {
		return "", fmt.Errorf("create public IPv4 detection request: %w", err)
	}
	client := &http.Client{Timeout: 10 * time.Second}
	response, err := client.Do(request)
	if err != nil {
		return "", fmt.Errorf("detect public IPv4: %w", err)
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		return "", fmt.Errorf("detect public IPv4: unexpected HTTP status %s", response.Status)
	}
	body, err := io.ReadAll(io.LimitReader(response.Body, 64))
	if err != nil {
		return "", fmt.Errorf("read detected public IPv4: %w", err)
	}
	value := strings.TrimSpace(string(body))
	address := net.ParseIP(value)
	if address == nil || address.To4() == nil || address.IsPrivate() || address.IsLoopback() {
		return "", fmt.Errorf("detect public IPv4: received invalid address %q", value)
	}
	return address.String(), nil
}

func parseOSRelease() (map[string]string, error) {
	file, err := os.Open("/etc/os-release")
	if err != nil {
		return nil, fmt.Errorf("read operating system release: %w", err)
	}
	defer file.Close()
	values := make(map[string]string)
	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		key, value, ok := strings.Cut(scanner.Text(), "=")
		if ok {
			values[key] = strings.Trim(strings.TrimSpace(value), "\"")
		}
	}
	if err := scanner.Err(); err != nil {
		return nil, fmt.Errorf("scan operating system release: %w", err)
	}
	return values, nil
}

func availableMemoryMB() (uint64, error) {
	file, err := os.Open("/proc/meminfo")
	if err != nil {
		return 0, fmt.Errorf("read memory information: %w", err)
	}
	defer file.Close()
	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		fields := strings.Fields(scanner.Text())
		if len(fields) >= 2 && fields[0] == "MemAvailable:" {
			kilobytes, err := strconv.ParseUint(fields[1], 10, 64)
			if err != nil {
				return 0, fmt.Errorf("parse available memory: %w", err)
			}
			return kilobytes / 1024, nil
		}
	}
	return 0, errors.New("MemAvailable was not found in /proc/meminfo")
}
