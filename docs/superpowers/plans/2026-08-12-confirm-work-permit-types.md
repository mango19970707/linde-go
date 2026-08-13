# Work Permit Type Confirmation Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Pause after work permit intent recognition so users can confirm or edit recognized permit types, controlled by a default-on configuration flag.

**Architecture:** Keep the interaction in the existing Eino stateful-interrupt graph. The intent node handles both initial recognition and resume data, while the HTTP layer serializes the interrupt payload and converts user JSON into a typed confirmation object. Configuration uses `*bool` so omission can retain the default-on behavior.

**Tech Stack:** Go, Eino compose stateful interrupts, Gin, YAML, Go tests.

---

### Task 1: Add failing graph and config tests

**Files:**
- Modify: `internal/graph/graph_test.go`
- Modify: `internal/config/config_test.go`

- [ ] Add tests proving default confirmation interrupts, edited types are applied on resume, disabled confirmation continues, and config omission defaults to enabled.
- [ ] Run the focused tests and verify they fail because the confirmation types and configuration field do not exist.

### Task 2: Implement graph confirmation state and node behavior

**Files:**
- Modify: `internal/graph/graph.go`

- [ ] Add `WpTypeConfirmation`, `IntentWpTypes` state interrupt metadata, and default-on confirmation handling.
- [ ] Make `newIntentRecognitionNode` recognize, interrupt when enabled, and apply typed resume data after validating submitted types against site metadata.
- [ ] Keep the node's existing two-argument callers source-compatible by making the confirmation flag optional.
- [ ] Run graph tests and verify the new behavior passes without regressing existing flows.

### Task 3: Wire configuration into graph construction

**Files:**
- Modify: `internal/config/config.go`
- Modify: `main.go`
- Modify: `configs/config.yaml`

- [ ] Add optional `confirm_wp_types` to `WorkPermitAPIConfig`.
- [ ] Pass the configured value into `BuildWpGraph`, preserving default `true` when omitted.
- [ ] Add the explicit setting to the sample configuration.
- [ ] Run config and package tests.

### Task 4: Extend HTTP confirmation contract

**Files:**
- Modify: `internal/httpapi/work_permit.go`
- Modify: `internal/httpapi/work_permit_test.go`

- [ ] Accept `confirmed` and `intent_wp_types` in resume requests.
- [ ] Return `intent_wp_types` and `need_wp_type_confirmation` for the new interrupt operation.
- [ ] Add tests for preview and modified resume payload mapping.
- [ ] Run the HTTP package tests.

### Task 5: Verify the complete change

**Files:**
- No additional files.

- [ ] Run `gofmt` on modified Go files.
- [ ] Run `go test ./...`.
- [ ] Inspect `git diff` for unintended changes and verify each requirement against the tests and implementation.
