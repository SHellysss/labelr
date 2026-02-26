package service

import "runtime"

type Manager interface {
	Install(binaryPath string) error
	Uninstall() error
	Start() error
	Stop() error
	IsRunning() (bool, error)
}

func Detect() Manager {
	switch runtime.GOOS {
	case "darwin":
		return &LaunchdManager{}
	case "linux":
		return &SystemdManager{}
	case "windows":
		return &TaskSchedulerManager{}
	default:
		return nil
	}
}
