package graph

import (
	"os"
	"path/filepath"
	"testing"
)

func TestWpFillRulesShouldFillDefaultsToTrue(t *testing.T) {
	t.Parallel()

	rules := WpFillRules{}

	if !rules.ShouldFill(2, "areaid", "所属区域") {
		t.Fatal("expected default fill to allow areaid")
	}
}

func TestWpFillRulesShouldSkipConfiguredFields(t *testing.T) {
	t.Parallel()

	rules := WpFillRules{
		ByWpTypeID: map[int]WpFillRule{
			2: {
				SkipFields: []string{"operationunit"},
			},
		},
	}

	if rules.ShouldFill(2, "operationunit", "作业单位") {
		t.Fatal("expected operationunit to be skipped")
	}
	if !rules.ShouldFill(2, "areaid", "所属区域") {
		t.Fatal("expected areaid to remain fillable")
	}
}

func TestLoadWpFillRules(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	path := filepath.Join(dir, "wp_fill_rules.json")
	if err := os.WriteFile(path, []byte(`{"2":{"skip_fields":["operationunit"],"fill_fields":["areaid"]}}`), 0o600); err != nil {
		t.Fatalf("write rules file: %v", err)
	}

	rules, err := LoadWpFillRules(path)
	if err != nil {
		t.Fatalf("LoadWpFillRules() error = %v", err)
	}

	if rules.ShouldFill(2, "operationunit", "作业单位") {
		t.Fatal("expected loaded rule to skip operationunit")
	}
	if !rules.ShouldFill(2, "areaid", "所属区域") {
		t.Fatal("expected loaded rule to allow areaid")
	}
	if !rules.ShouldFill(2, "starttime", "作业计划开始时间") {
		t.Fatal("expected unspecified field to default to fill")
	}
}

func TestWpFillRulesShouldMatchByLabel(t *testing.T) {
	t.Parallel()

	rules := WpFillRules{
		ByWpTypeID: map[int]WpFillRule{
			2: {
				SkipFields: []string{"作业单位"},
			},
		},
	}

	if rules.ShouldFill(2, "operationunit", "作业单位") {
		t.Fatal("expected label match to skip operationunit")
	}
}
