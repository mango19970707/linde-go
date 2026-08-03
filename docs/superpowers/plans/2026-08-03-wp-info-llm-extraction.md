# Work Permit Info LLM Extraction Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Extract work permit base information from `WpGraphState.UserInput` through an injected LLM-backed interface.

**Architecture:** `graph` owns the small `WpInfoExtractor` interface and state mapping. The concrete extractor sends a strict JSON-only prompt to an injected Eino `model.BaseChatModel`, decodes the response, and returns names/times/description without guessing backend IDs. The validation node checks required fields and routes incomplete input back to the ask node.

**Tech Stack:** Go 1.25, Eino `schema.Message`, Eino `model.BaseChatModel`, standard-library JSON decoding, Go tests.

---

### Task 1: Add the extractor contract and state input

**Files:**
- Modify: `internal/graph/graph.go`
- Test: `internal/graph/graph_test.go`

- [ ] **Step 1: Write failing tests** for mapping extractor output into state and rejecting missing required fields.
- [ ] **Step 2: Run `go test ./internal/graph` and verify the tests fail because the contract and behavior do not exist.**
- [ ] **Step 3: Add `UserInput`, `WpInfoExtraction`, and `WpInfoExtractor`; inject the extractor through `BuildWpGraph(extractor)` and map extraction results in `askWpInfoNode`.
- [ ] **Step 4: Implement validation for area, start time, end time, description, equipment, and company.
- [ ] **Step 5: Run `go test ./internal/graph` and verify it passes.

### Task 2: Implement the Eino JSON extractor

**Files:**
- Create: `internal/graph/wp_info_extractor.go`
- Test: `internal/graph/wp_info_extractor_test.go`

- [ ] **Step 1: Write a failing test using a fake `model.BaseChatModel` that returns a JSON response.**
- [ ] **Step 2: Run the focused test and verify it fails because the concrete extractor does not exist.**
- [ ] **Step 3: Implement `NewLLMWpInfoExtractor` and `Extract` with a system prompt requiring JSON and strict decoding of the six fields.
- [ ] **Step 4: Add errors for nil model, empty input, model generation failure, empty response, and invalid JSON.
- [ ] **Step 5: Run `go test ./internal/graph` and verify it passes.

### Task 3: Verify repository behavior

**Files:**
- Modify: `internal/graph/graph.go`
- Modify: `internal/graph/wp_info_extractor.go`
- Test: `internal/graph/*_test.go`

- [ ] **Step 1: Run `gofmt` on changed Go files.**
- [ ] **Step 2: Run `go test ./...`.
- [ ] **Step 3: Run `go vet ./...`.
- [ ] **Step 4: Inspect `git diff` and confirm only the planned files changed.
