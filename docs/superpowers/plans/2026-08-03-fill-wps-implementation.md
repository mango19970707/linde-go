# Fill Wps Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Implement `fillWpsNode` so it fills work permit template fields from graph state and structured memory, and reports filled versus unfilled fields.

**Architecture:** `graph` will own the fill-node orchestration, template field traversal, and field-source reporting. A small structured-memory interface will be injected into the graph builder, and the node will mutate cloned `ApplyContentObject` payloads into `FilledWp` results without guessing unresolved values.

**Tech Stack:** Go 1.25, standard library JSON cloning, existing graph package types and tests.

---

### Task 1: Define fill result and memory types

**Files:**
- Modify: `internal/graph/graph.go`
- Test: `internal/graph/graph_test.go`

- [ ] **Step 1: Write failing tests** for state-first filling, structured-memory fallback, and unfilled field reporting.
- [ ] **Step 2: Run `go test ./internal/graph` and verify the tests fail because fill logic and supporting types do not exist.**
- [ ] **Step 3: Add `StructuredMemoryEntry`, `StructuredMemoryStore`, `FilledField`, and `UnfilledField` types; extend `FilledWp` to carry filled/unfilled metadata.**
- [ ] **Step 4: Update graph builder signatures to inject the structured-memory dependency.**
- [ ] **Step 5: Run `go test ./internal/graph` and confirm the tests still fail only on node behavior.**

### Task 2: Implement fill node behavior

**Files:**
- Modify: `internal/graph/graph.go`
- Create: `internal/graph/wp_fill.go`
- Test: `internal/graph/graph_test.go`

- [ ] **Step 1: Write the failing test** for malformed template handling.
- [ ] **Step 2: Run `go test ./internal/graph -run FillWps` and verify the failure is due to missing fill behavior.**
- [ ] **Step 3: Implement `newFillWpsNode(memoryStore)` to clone template content, fill known fields from state first, then structured memory, then leave blank.**
- [ ] **Step 4: Record per-field sources and unfilled field names in each `FilledWp`.**
- [ ] **Step 5: Run `go test ./internal/graph -run FillWps` and verify the tests pass.**

### Task 3: Verify repository state

**Files:**
- Modify: `internal/graph/*.go`
- Test: `internal/graph/*.go`

- [ ] **Step 1: Run `gofmt -w internal/graph/*.go`.**
- [ ] **Step 2: Run `go test ./...`.**
- [ ] **Step 3: Run `go vet ./...`.**
- [ ] **Step 4: Run `git diff --check`.**
