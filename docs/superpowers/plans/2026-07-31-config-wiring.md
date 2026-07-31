# Config Wiring Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Load model settings and work-permit API base URL from `configs/config.yaml`, and expose config-backed constructors for chat models and tools.

**Architecture:** Add a small `internal/config` package that reads YAML into strongly typed structs. Keep `internal/chatmodel` and `internal/tools` focused on their own domains by adding config-to-runtime conversion helpers instead of introducing a global singleton.

**Tech Stack:** Go, `gopkg.in/yaml.v3`, existing Eino model/tool packages

---

### Task 1: Add failing tests for config loading

**Files:**
- Create: `internal/config/config_test.go`
- Create: `internal/config/config.go`

- [ ] **Step 1: Write the failing test**

```go
func TestLoad(t *testing.T) {
	cfg, err := Load("testdata/config.yaml")
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}
	if cfg.ChatModel.Provider != "anthropic" {
		t.Fatalf("provider = %s", cfg.ChatModel.Provider)
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/config`
Expected: FAIL with `undefined: Load`

- [ ] **Step 3: Write minimal implementation**

```go
func Load(path string) (*AppConfig, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var cfg AppConfig
	if err := yaml.Unmarshal(data, &cfg); err != nil {
		return nil, err
	}
	return &cfg, nil
}
```

- [ ] **Step 4: Run test to verify it passes**

Run: `go test ./internal/config`
Expected: PASS

### Task 2: Add failing tests for config-backed chat model settings

**Files:**
- Modify: `internal/chatmodel/chatmodel.go`
- Create: `internal/chatmodel/chatmodel_config_test.go`

- [ ] **Step 1: Write the failing test**

```go
func TestBuildModelConfigFromAppConfig(t *testing.T) {
	cfg := &config.AppConfig{...}
	modelCfg, err := BuildModelConfigFromAppConfig(cfg, "")
	if err != nil {
		t.Fatalf("BuildModelConfigFromAppConfig() error = %v", err)
	}
	if modelCfg.BaseURL != "https://example.com" {
		t.Fatalf("BaseURL = %s", modelCfg.BaseURL)
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/chatmodel`
Expected: FAIL with `undefined: BuildModelConfigFromAppConfig`

- [ ] **Step 3: Write minimal implementation**

```go
func BuildModelConfigFromAppConfig(appCfg *config.AppConfig, provider string) (*ModelConfig, error) {
	selected := provider
	if selected == "" {
		selected = appCfg.ChatModel.Provider
	}
	src, err := appCfg.GetProviderConfig(selected)
	if err != nil {
		return nil, err
	}
	return &ModelConfig{Provider: selected, BaseURL: src.BaseURL, ...}, nil
}
```

- [ ] **Step 4: Run test to verify it passes**

Run: `go test ./internal/chatmodel`
Expected: PASS

### Task 3: Add failing tests for tool config mapping

**Files:**
- Modify: `internal/tools/client.go`
- Create: `internal/tools/config_test.go`

- [ ] **Step 1: Write the failing test**

```go
func TestToolConfigFromAppConfig(t *testing.T) {
	appCfg := &config.AppConfig{...}
	got := ToolConfigFromAppConfig(appCfg)
	if got.BaseURL != "http://127.0.0.1:8083" {
		t.Fatalf("BaseURL = %s", got.BaseURL)
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/tools`
Expected: FAIL with `undefined: ToolConfigFromAppConfig`

- [ ] **Step 3: Write minimal implementation**

```go
func ToolConfigFromAppConfig(appCfg *config.AppConfig) ToolConfig {
	return ToolConfig{
		BaseURL: appCfg.WorkPermitAPI.BaseURL,
		SiteID:  appCfg.WorkPermitAPI.SiteID,
		UserID:  appCfg.WorkPermitAPI.UserID,
		Timeout: appCfg.WorkPermitAPI.Timeout,
		Headers: appCfg.WorkPermitAPI.Headers,
	}
}
```

- [ ] **Step 4: Run test to verify it passes**

Run: `go test ./internal/tools`
Expected: PASS

### Task 4: Populate the YAML config and verify the repository

**Files:**
- Modify: `configs/config.yaml`

- [ ] **Step 1: Write the config file**

```yaml
chat_model:
  provider: anthropic
  providers:
    anthropic:
      base_url: ""
      api_key: ""
      model: claude-opus-4-1
work_permit_api:
  base_url: http://110.40.170.130:8083
```

- [ ] **Step 2: Run repository verification**

Run: `go test ./...`
Expected: PASS
