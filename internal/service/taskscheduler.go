package service

import (
	"os/exec"
	"strings"
)

const taskName = "labelr"

type TaskSchedulerManager struct{}

func (m *TaskSchedulerManager) Install(binaryPath string) error {
	// Use PowerShell to launch the daemon without a visible console window.
	tr := `powershell -WindowStyle Hidden -Command "& '` + binaryPath + `' daemon"`
	return exec.Command("schtasks", "/create",
		"/tn", taskName,
		"/tr", tr,
		"/sc", "onlogon",
		"/rl", "LIMITED",
		"/f",
	).Run()
}

func (m *TaskSchedulerManager) Uninstall() error {
	m.Stop()
	return exec.Command("schtasks", "/delete", "/tn", taskName, "/f").Run()
}

func (m *TaskSchedulerManager) Start() error {
	return exec.Command("schtasks", "/run", "/tn", taskName).Run()
}

func (m *TaskSchedulerManager) Stop() error {
	return exec.Command("schtasks", "/end", "/tn", taskName).Run()
}

func (m *TaskSchedulerManager) IsRunning() (bool, error) {
	out, err := exec.Command("schtasks", "/query", "/tn", taskName, "/fo", "CSV", "/nh").Output()
	if err != nil {
		return false, nil
	}
	return strings.Contains(string(out), "Running"), nil
}
