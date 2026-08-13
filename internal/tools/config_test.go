package tools

import (
	"testing"
	"time"

	appconfig "linde-go-v2/internal/config"
)

func TestToolConfigFromAppConfig(t *testing.T) {
	appCfg := &appconfig.AppConfig{
		WorkPermitAPI: appconfig.WorkPermitAPIConfig{
			BaseURL: "http://127.0.0.1:8083",
			Timeout: 20 * time.Second,
			Headers: map[string]string{
				"X-Test": "value",
			},
		},
	}

	got := ToolConfigFromAppConfig(appCfg)

	if got.BaseURL != "http://127.0.0.1:8083" {
		t.Fatalf("BaseURL = %s", got.BaseURL)
	}
	if got.Timeout != 20*time.Second {
		t.Fatalf("Timeout = %s", got.Timeout)
	}
	if got.Headers["X-Test"] != "value" {
		t.Fatalf("Header X-Test = %s", got.Headers["X-Test"])
	}
}
