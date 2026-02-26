package service

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

const (
	launchdLabel = "com.labelr.daemon"
)

type LaunchdManager struct{}

func (m *LaunchdManager) plistPath() string {
	home, _ := os.UserHomeDir()
	return filepath.Join(home, "Library", "LaunchAgents", launchdLabel+".plist")
}

func (m *LaunchdManager) plistContent(binaryPath string) string {
	return fmt.Sprintf(`<?xml version="1.0" encoding="UTF-8"?>
<!DOCTYPE plist PUBLIC "-//Apple//DTD PLIST 1.0//EN" "http://www.apple.com/DTDs/PropertyList-1.0.dtd">
<plist version="1.0">
<dict>
    <key>Label</key>
    <string>%s</string>
    <key>ProgramArguments</key>
    <array>
        <string>%s</string>
        <string>daemon</string>
    </array>
    <key>RunAtLoad</key>
    <true/>
    <key>KeepAlive</key>
    <true/>
    <key>StandardErrorPath</key>
    <string>/tmp/labelr-stderr.log</string>
</dict>
</plist>`, launchdLabel, binaryPath)
}

func (m *LaunchdManager) Install(binaryPath string) error {
	content := m.plistContent(binaryPath)
	path := m.plistPath()
	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		return err
	}
	return os.WriteFile(path, []byte(content), 0644)
}

func (m *LaunchdManager) Uninstall() error {
	m.Stop()
	return os.Remove(m.plistPath())
}

func (m *LaunchdManager) Start() error {
	return exec.Command("launchctl", "load", m.plistPath()).Run()
}

func (m *LaunchdManager) Stop() error {
	return exec.Command("launchctl", "unload", m.plistPath()).Run()
}

func (m *LaunchdManager) IsRunning() (bool, error) {
	out, err := exec.Command("launchctl", "list").Output()
	if err != nil {
		return false, err
	}
	return strings.Contains(string(out), launchdLabel), nil
}
