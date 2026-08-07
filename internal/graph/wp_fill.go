package graph

import (
	"context"
	"encoding/json"
	"fmt"
	"slices"
	"strings"
)

func newFillWpsNode(memoryStore StructuredMemoryStore, fillRules WpFillRules) func(context.Context, *WpGraphState) (*WpGraphState, error) {
	return func(ctx context.Context, state *WpGraphState) (*WpGraphState, error) {
		if memoryStore == nil {
			return nil, fmt.Errorf("structured memory store is nil")
		}

		filledWps := make([]FilledWp, 0, len(state.WpTemplates))
		for _, template := range state.WpTemplates {
			filled, err := fillSingleWp(ctx, state, template, memoryStore, fillRules)
			if err != nil {
				return nil, err
			}
			filledWps = append(filledWps, filled)
		}

		state.FilledWps = filledWps
		state.CurrentNode = nodeFillWps
		return state, nil
	}
}

func fillSingleWp(ctx context.Context, state *WpGraphState, template WpTemplate, memoryStore StructuredMemoryStore, fillRules WpFillRules) (FilledWp, error) {
	content, err := cloneMap(template.ApplyContentObject)
	if err != nil {
		return FilledWp{}, fmt.Errorf("clone template %d content: %w", template.ID, err)
	}

	sqxx, ok := content["sqxx"].([]any)
	if !ok {
		return FilledWp{}, fmt.Errorf("template %d sqxx is missing or invalid", template.ID)
	}

	memoryEntries, err := memoryStore.Lookup(ctx, template.WpTypeID)
	if err != nil {
		return FilledWp{}, fmt.Errorf("lookup structured memory for template %d: %w", template.ID, err)
	}
	memoryIndex := buildMemoryIndex(memoryEntries)

	content["siteid"] = state.SiteID
	content["userid"] = state.UserID
	content["wptypeid"] = template.WpTypeID
	content["wptempletid"] = template.ID

	filledFields := make([]FilledField, 0)
	unfilledFields := make([]UnfilledField, 0)

	for i, rawField := range sqxx {
		field, ok := rawField.(map[string]any)
		if !ok {
			return FilledWp{}, fmt.Errorf("template %d field %d is invalid", template.ID, i)
		}
		name, _ := field["name"].(string)
		if strings.TrimSpace(name) == "" || !supportsFill(field) {
			continue
		}
		label, _ := field["label"].(string)
		displayName := strings.TrimSpace(label)
		if displayName == "" {
			displayName = name
		}
		if !fillRules.ShouldFill(template.WpTypeID, name, displayName) {
			continue
		}

		value, source, found := resolveFieldValue(name, state, memoryIndex)
		if found {
			field["value"] = value
			filledFields = append(filledFields, FilledField{Label: displayName, Value: value, Source: source})
			continue
		}

		if isEmptyFieldValue(field["value"]) {
			unfilledFields = append(unfilledFields, UnfilledField{Label: displayName})
		}
	}

	content["sqxx"] = sqxx
	return FilledWp{
		WpType:         readableWpType(template),
		Name:           template.Name,
		Content:        content,
		FilledFields:   filledFields,
		UnfilledFields: unfilledFields,
	}, nil
}

func readableWpType(template WpTemplate) string {
	if strings.TrimSpace(template.WpTypeName) != "" {
		return template.WpTypeName
	}
	if strings.TrimSpace(template.Name) != "" {
		return template.Name
	}
	return fmt.Sprintf("%d", template.WpTypeID)
}

func cloneMap(value map[string]any) (map[string]any, error) {
	if value == nil {
		return nil, fmt.Errorf("template content is empty")
	}
	raw, err := json.Marshal(value)
	if err != nil {
		return nil, err
	}
	var cloned map[string]any
	if err := json.Unmarshal(raw, &cloned); err != nil {
		return nil, err
	}
	return cloned, nil
}

func buildMemoryIndex(entries []StructuredMemoryEntry) map[string]StructuredMemoryEntry {
	index := make(map[string]StructuredMemoryEntry, len(entries))
	for _, entry := range entries {
		existing, ok := index[entry.FieldName]
		if !ok || existing.UpdatedAt < entry.UpdatedAt {
			index[entry.FieldName] = entry
		}
	}
	return index
}

func supportsFill(field map[string]any) bool {
	fieldType, _ := field["type"].(string)
	return slices.Contains([]string{"input", "datepicker", "selector", "checkbox", "radio", "textarea"}, fieldType)
}

func resolveFieldValue(name string, state *WpGraphState, memoryIndex map[string]StructuredMemoryEntry) (any, string, bool) {
	if value, ok := resolveStateFieldValue(name, state); ok {
		return value, "state", true
	}
	if entry, ok := memoryIndex[name]; ok {
		source := entry.Source
		if strings.TrimSpace(source) == "" {
			source = "structured_memory"
		}
		return entry.FieldValue, source, true
	}
	return nil, "", false
}

func resolveStateFieldValue(name string, state *WpGraphState) (any, bool) {
	switch name {
	case "operationunit":
		if state.WpCompany.ID == 0 && strings.TrimSpace(state.WpCompany.Name) == "" {
			return nil, false
		}
		return []any{selectorValue(state.WpCompany.ID, state.WpCompany.Name)}, true
	case "areaid":
		if strings.TrimSpace(state.Area) == "" {
			return nil, false
		}
		return state.Area, true
	case "equipmentid":
		if len(state.Equipments) == 0 {
			return nil, false
		}
		return []any{selectorValue(state.Equipments[0].ID, state.Equipments[0].Name)}, true
	case "discription":
		if strings.TrimSpace(state.Description) == "" {
			return nil, false
		}
		return state.Description, true
	case "starttime":
		if strings.TrimSpace(state.StartTime) == "" {
			return nil, false
		}
		return state.StartTime, true
	case "endtime":
		if strings.TrimSpace(state.EndTime) == "" {
			return nil, false
		}
		return state.EndTime, true
	}
	return nil, false
}

func selectorValue(id int, name string) map[string]any {
	return map[string]any{
		"id":   id,
		"name": name,
	}
}

func isEmptyFieldValue(value any) bool {
	switch typed := value.(type) {
	case nil:
		return true
	case string:
		return strings.TrimSpace(typed) == ""
	case []any:
		return len(typed) == 0
	case []string:
		return len(typed) == 0
	default:
		return false
	}
}
