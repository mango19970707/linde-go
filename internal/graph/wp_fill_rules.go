package graph

import (
	"encoding/json"
	"fmt"
	"os"
	"sort"
	"strconv"
	"strings"
)

type WpFillRule struct {
	FillFields []string `json:"fill_fields"`
	SkipFields []string `json:"skip_fields"`
}

type WpFillRules struct {
	ByWpTypeID map[int]WpFillRule `json:"-"`
}

func LoadWpFillRules(path string) (WpFillRules, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return WpFillRules{}, fmt.Errorf("read wp fill rules %s: %w", path, err)
	}

	raw := make(map[string]WpFillRule)
	if err := json.Unmarshal(data, &raw); err != nil {
		return WpFillRules{}, fmt.Errorf("unmarshal wp fill rules %s: %w", path, err)
	}

	rules := WpFillRules{ByWpTypeID: make(map[int]WpFillRule, len(raw))}
	for key, rule := range raw {
		wpTypeID, err := strconv.Atoi(strings.TrimSpace(key))
		if err != nil {
			return WpFillRules{}, fmt.Errorf("parse wp type id %q: %w", key, err)
		}
		rules.ByWpTypeID[wpTypeID] = normalizeWpFillRule(rule)
	}

	return rules, nil
}

func (r WpFillRules) ShouldFill(wpTypeID int, fieldName, fieldLabel string) bool {
	name := strings.TrimSpace(fieldName)
	label := strings.TrimSpace(fieldLabel)
	if name == "" && label == "" {
		return false
	}

	rule, ok := r.ByWpTypeID[wpTypeID]
	if !ok {
		return true
	}
	rule = normalizeWpFillRule(rule)

	if containsString(rule.SkipFields, name) || containsString(rule.SkipFields, label) {
		return false
	}
	if containsString(rule.FillFields, name) || containsString(rule.FillFields, label) {
		return true
	}
	return true
}

func normalizeWpFillRule(rule WpFillRule) WpFillRule {
	rule.FillFields = normalizeStrings(rule.FillFields)
	rule.SkipFields = normalizeStrings(rule.SkipFields)
	return rule
}

func normalizeStrings(values []string) []string {
	if len(values) == 0 {
		return nil
	}

	normalized := make([]string, 0, len(values))
	seen := make(map[string]struct{}, len(values))
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value == "" {
			continue
		}
		if _, ok := seen[value]; ok {
			continue
		}
		seen[value] = struct{}{}
		normalized = append(normalized, value)
	}
	sort.Strings(normalized)
	return normalized
}

func containsString(values []string, target string) bool {
	for _, value := range values {
		if value == target {
			return true
		}
	}
	return false
}
