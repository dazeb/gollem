package protocol

import "testing"

func TestDefaultInitializeResponseIncludesCodexCompatibilityFields(t *testing.T) {
	response := DefaultInitializeResponse(
		ImplementationInfo{Name: "gollem-appserver", Version: "1.2.3"},
		"/workspace/.gollem",
	)
	if response.UserAgent != "gollem-appserver/1.2.3" {
		t.Errorf("userAgent = %q", response.UserAgent)
	}
	if response.CodexHome != "/workspace/.gollem" {
		t.Errorf("codexHome = %q", response.CodexHome)
	}
	if response.PlatformFamily == "" || response.PlatformOS == "" {
		t.Errorf("platform = %q/%q", response.PlatformFamily, response.PlatformOS)
	}
	if response.ProtocolVersion != ProtocolVersion || response.ServerInfo.Name != "gollem-appserver" {
		t.Errorf("Gollem metadata = %+v", response)
	}
	if !response.Capabilities.ProtocolSchema || len(response.Methods) == 0 {
		t.Errorf("capabilities/methods = %+v/%d", response.Capabilities, len(response.Methods))
	}
}

func TestInitializePlatformNamesMatchPublicWireValues(t *testing.T) {
	tests := []struct {
		goos       string
		wantFamily string
		wantOS     string
	}{
		{goos: "darwin", wantFamily: "unix", wantOS: "macos"},
		{goos: "linux", wantFamily: "unix", wantOS: "linux"},
		{goos: "windows", wantFamily: "windows", wantOS: "windows"},
	}
	for _, test := range tests {
		t.Run(test.goos, func(t *testing.T) {
			if got := initializePlatformFamily(test.goos); got != test.wantFamily {
				t.Errorf("family = %q, want %q", got, test.wantFamily)
			}
			if got := initializePlatformOS(test.goos); got != test.wantOS {
				t.Errorf("os = %q, want %q", got, test.wantOS)
			}
		})
	}
}
