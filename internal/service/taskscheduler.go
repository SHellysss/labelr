package service

import (
	"fmt"
	"os/exec"
	"strings"
)

const taskName = "labelr"

type TaskSchedulerManager struct{}

func (m *TaskSchedulerManager) Install(binaryPath string) error {
	// Use PowerShell to launch the daemon without a visible console window.
	tr := `powershell -WindowStyle Hidden -Command "& '` + binaryPath + `' daemon"`
	if err := exec.Command("schtasks", "/create",
		"/tn", taskName,
		"/tr", tr,
		"/sc", "onlogon",
		"/rl", "LIMITED",
		"/f",
	).Run(); err != nil {
		return fmt.Errorf("%w — try running your terminal as Administrator", err)
	}
	return nil
}

func (m *TaskSchedulerManager) Uninstall() error {
	m.Stop()
	if err := exec.Command("schtasks", "/delete", "/tn", taskName, "/f").Run(); err != nil {
		return fmt.Errorf("%w — try running your terminal as Administrator", err)
	}
	return nil
}

func (m *TaskSchedulerManager) Start() error {
	if err := exec.Command("schtasks", "/run", "/tn", taskName).Run(); err != nil {
		return fmt.Errorf("%w — try running your terminal as Administrator", err)
	}
	return nil
}

func (m *TaskSchedulerManager) Stop() error {
	if err := exec.Command("schtasks", "/end", "/tn", taskName).Run(); err != nil {
		return fmt.Errorf("%w — try running your terminal as Administrator", err)
	}
	return nil
}

func (m *TaskSchedulerManager) IsRunning() (bool, error) {
	out, err := exec.Command("schtasks", "/query", "/tn", taskName, "/fo", "CSV", "/nh").Output()
	if err != nil {
		return false, nil
	}
	return strings.Contains(string(out), "Running"), nil
}
