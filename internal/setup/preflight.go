package setup

import "context"

type HostInfo struct {
	Distribution string
	Version      string
	Architecture string
	MemoryMB     uint64
	DiskFreeMB   uint64
}

type InstallLock struct{}

func AcquireInstallLock(bool) (*InstallLock, error) {
	return &InstallLock{}, nil
}

func (*InstallLock) Close() error {
	return nil
}

func Preflight(context.Context, bool) (HostInfo, error) {
	return HostInfo{Distribution: "debian", Version: "13", Architecture: "amd64", MemoryMB: 4096, DiskFreeMB: 20480}, nil
}
