// internal/client/store/config_test.go
package store_test

import (
	"testing"
)

func TestInitConfig(t *testing.T) {
	s := newTestStore(t)

	err := s.InitConfig("device-uuid-abc", "https://trasker.example.com", "api-key-123")
	if err != nil {
		t.Fatalf("InitConfig() error: %v", err)
	}

	cfg, err := s.GetConfig()
	if err != nil {
		t.Fatalf("GetConfig() error: %v", err)
	}
	if cfg.DeviceID != "device-uuid-abc" {
		t.Errorf("DeviceID = %q, want %q", cfg.DeviceID, "device-uuid-abc")
	}
	if cfg.ServerURL != "https://trasker.example.com" {
		t.Errorf("ServerURL = %q, want %q", cfg.ServerURL, "https://trasker.example.com")
	}
	if cfg.APIKey != "api-key-123" {
		t.Errorf("APIKey = %q, want %q", cfg.APIKey, "api-key-123")
	}
	if !cfg.TrackingOn {
		t.Error("TrackingOn = false, want true (default)")
	}
	if cfg.Autostart {
		t.Error("Autostart = true, want false (default)")
	}
}

func TestInitConfig_Idempotent(t *testing.T) {
	s := newTestStore(t)

	_ = s.InitConfig("device-1", "https://example.com", "key-1")

	// Second call should not error (INSERT OR IGNORE)
	err := s.InitConfig("device-2", "https://other.com", "key-2")
	if err != nil {
		t.Fatalf("second InitConfig() error: %v", err)
	}

	// Original values should be preserved
	cfg, _ := s.GetConfig()
	if cfg.DeviceID != "device-1" {
		t.Errorf("DeviceID = %q, want %q (first init should win)", cfg.DeviceID, "device-1")
	}
}

func TestGetConfig_BeforeInit(t *testing.T) {
	s := newTestStore(t)

	_, err := s.GetConfig()
	if err == nil {
		t.Fatal("expected error before InitConfig(), got nil")
	}
}

func TestUpdateTrackingState(t *testing.T) {
	s := newTestStore(t)
	_ = s.InitConfig("dev-1", "https://example.com", "key-1")

	err := s.UpdateTrackingState(false)
	if err != nil {
		t.Fatalf("UpdateTrackingState() error: %v", err)
	}

	cfg, _ := s.GetConfig()
	if cfg.TrackingOn {
		t.Error("TrackingOn = true, want false after update")
	}
}

func TestUpdateAutostart(t *testing.T) {
	s := newTestStore(t)
	_ = s.InitConfig("dev-1", "https://example.com", "key-1")

	err := s.UpdateAutostart(true)
	if err != nil {
		t.Fatalf("UpdateAutostart() error: %v", err)
	}

	cfg, _ := s.GetConfig()
	if !cfg.Autostart {
		t.Error("Autostart = false, want true after update")
	}
}
