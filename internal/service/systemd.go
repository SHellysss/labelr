package service

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

const systemdServiceName = "labelr.service"

type SystemdManager struct{}

func (m *SystemdManager) unitPath() string {
	home, _ := os.UserHomeDir()
	return filepath.Join(home, ".config", "systemd", "user", systemdServiceName)
}

func (m *SystemdManager) unitContent(binaryPath string) string {
	return fmt.Sprintf(`[Unit]
Description=labelr - AI Gmail Labeler
After=network-online.target

[Service]
Type=simple
ExecStart=%s daemon
Restart=always
RestartSec=5

[Install]
WantedBy=default.target
`, binaryPath)
}

func (m *SystemdManager) Install(binaryPath string) error {
	content := m.unitContent(binaryPath)
	path := m.unitPath()
	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		return err
	}
	if err := os.WriteFile(path, []byte(content), 0644); err != nil {
		return err
	}
	return exec.Command("systemctl", "--user", "daemon-reload").Run()
}

func (m *SystemdManager) Uninstall() error {
	exec.Command("systemctl", "--user", "disable", "labelr").Run()
	m.Stop()
	return os.Remove(m.unitPath())
}

func (m *SystemdManager) Start() error {
	return exec.Command("systemctl", "--user", "enable", "--now", "labelr").Run()
}

func (m *SystemdManager) Stop() error {
	return exec.Command("systemctl", "--user", "stop", "labelr").Run()
}

func (m *SystemdManager) IsRunning() (bool, error) {
	out, err := exec.Command("systemctl", "--user", "is-active", "labelr").Output()
	if err != nil {
		return false, nil // not running
	}
	return strings.TrimSpace(string(out)) == "active", nil
}
