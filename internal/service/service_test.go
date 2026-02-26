package service

import (
	"runtime"
	"testing"
)

func TestDetectManager(t *testing.T) {
	mgr := Detect()
	if mgr == nil {
		t.Fatal("Detect() returned nil")
	}

	switch runtime.GOOS {
	case "darwin":
		if _, ok := mgr.(*LaunchdManager); !ok {
			t.Error("expected LaunchdManager on darwin")
		}
	case "linux":
		if _, ok := mgr.(*SystemdManager); !ok {
			t.Error("expected SystemdManager on linux")
		}
	case "windows":
		if _, ok := mgr.(*TaskSchedulerManager); !ok {
			t.Error("expected TaskSchedulerManager on windows")
		}
	}
}

func TestLaunchdPlistContent(t *testing.T) {
	mgr := &LaunchdManager{}
	content := mgr.plistContent("/usr/local/bin/labelr")
	if content == "" {
		t.Error("plist content is empty")
	}
}
