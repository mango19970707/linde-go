package graph

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"slices"
	"strconv"
	"strings"

	"github.com/cloudwego/eino/components/model"
	"github.com/cloudwego/eino/schema"
	"github.com/tidwall/gjson"
)

const (
	structuredMemorySourceExact     = "structured_memory_exact"
	structuredMemorySourceAreaGuess = "structured_memory_area_guess"
	llmFallbackSource               = "llm_fallback"
	locationGuessMinConfidence      = 0.70
)

type fillCandidateField struct {
	Path         string
	Name         string
	Label        string
	Type         string
	Required     bool
	Value        any
	Field        map[string]any
	AllowFill    bool
	DisplayLabel string
}

type llmAreaSelectionResult struct {
	Area       string  `json:"area"`
	Confidence float64 `json:"confidence"`
	Reason     string  `json:"reason"`
}

type llmFieldFillResult struct {
	Fields map[string]any `json:"fields"`
}

type structuredMemorySample struct {
	Area            string         `json:"area"`
	WorkDescription string         `json:"work_description"`
	Source          string         `json:"source"`
	Fields          map[string]any `json:"fields"`
}

type wpFillLLMHelper struct {
	chatModel   model.BaseChatModel
	promptStore PromptStore
}

func newFillWpsNode(chatModel model.BaseChatModel, args ...any) func(context.Context, *WpGraphState) (*WpGraphState, error) {
	promptStore := newDefaultPromptStore()
	var memoryStore StructuredMemoryStore
	var fillRules WpFillRules

	switch len(args) {
	case 2:
		store, ok := args[0].(StructuredMemoryStore)
		if !ok {
			return func(context.Context, *WpGraphState) (*WpGraphState, error) {
				return nil, fmt.Errorf("structured memory store has invalid type %T", args[0])
			}
		}
		rules, ok := args[1].(WpFillRules)
		if !ok {
			return func(context.Context, *WpGraphState) (*WpGraphState, error) {
				return nil, fmt.Errorf("fill rules has invalid type %T", args[1])
			}
		}
		memoryStore = store
		fillRules = rules
	case 3:
		store, ok := args[0].(PromptStore)
		if !ok {
			return func(context.Context, *WpGraphState) (*WpGraphState, error) {
				return nil, fmt.Errorf("prompt store has invalid type %T", args[0])
			}
		}
		memory, ok := args[1].(StructuredMemoryStore)
		if !ok {
			return func(context.Context, *WpGraphState) (*WpGraphState, error) {
				return nil, fmt.Errorf("structured memory store has invalid type %T", args[1])
			}
		}
		rules, ok := args[2].(WpFillRules)
		if !ok {
			return func(context.Context, *WpGraphState) (*WpGraphState, error) {
				return nil, fmt.Errorf("fill rules has invalid type %T", args[2])
			}
		}
		promptStore = store
		memoryStore = memory
		fillRules = rules
	default:
		return func(context.Context, *WpGraphState) (*WpGraphState, error) {
			return nil, fmt.Errorf("unexpected fill node argument count %d", len(args))
		}
	}

	helper := wpFillLLMHelper{chatModel: chatModel, promptStore: promptStore}
	return func(ctx context.Context, state *WpGraphState) (*WpGraphState, error) {
		if chatModel == nil {
			return nil, fmt.Errorf("chat model is nil")
		}
		if memoryStore == nil {
			return nil, fmt.Errorf("structured memory store is nil")
		}

		filledWps := make([]FilledWp, 0, len(state.WpTemplates))
		for _, template := range state.WpTemplates {
			filled, err := fillSingleWp(ctx, state, template, memoryStore, helper, fillRules)
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

func fillSingleWp(ctx context.Context, state *WpGraphState, template WpTemplate, memoryStore StructuredMemoryStore, helper wpFillLLMHelper, fillRules WpFillRules) (FilledWp, error) {
	content, err := cloneMap(template.ApplyContentObject)
	if err != nil {
		return FilledWp{}, fmt.Errorf("clone template %d content: %w", template.ID, err)
	}

	content["siteid"] = state.SiteID
	content["userid"] = state.UserID
	content["wptypeid"] = template.WpTypeID
	content["wptempletid"] = template.ID

	// step 1：递归遍历整张模板 JSON，收集所有可填字段。
	fields := collectTemplateFields(content, "", fillRules, template.WpTypeID)
	if len(fields) == 0 {
		return FilledWp{}, fmt.Errorf("template %d does not contain any fillable field", template.ID)
	}
	fieldsByPath := indexFieldsByPath(fields)

	filledFields := make([]FilledField, 0)
	filledPathSet := make(map[string]struct{}, len(fieldsByPath))

	// step 2：优先使用图状态中的明确字段值进行填充。
	fillStateFields(fields, state, &filledFields, filledPathSet)

	// step 3：优先按作业票类型 + 区域精确查询结构化记忆，并按字段路径回填。
	exactEntries, err := memoryStore.FindByTypeAndArea(ctx, template.WpTypeID, strings.TrimSpace(state.Area))
	if err != nil {
		return FilledWp{}, fmt.Errorf("find structured memory by type and area for template %d: %w", template.ID, err)
	}
	applyStructuredMemoryEntries(content, fieldsByPath, exactEntries, structuredMemorySourceExact, &filledFields, filledPathSet)

	// step 4：如果精确地点没有命中，则按作业票类型列出候选地点，让大模型选择最可能地点后再回填。
	if len(exactEntries) == 0 {
		candidateAreas, err := memoryStore.ListAreasByType(ctx, template.WpTypeID)
		if err != nil {
			return FilledWp{}, fmt.Errorf("list structured memory areas for template %d: %w", template.ID, err)
		}
		selectedArea, confidence, err := helper.selectClosestArea(ctx, template, state, candidateAreas)
		if err != nil {
			if !isRecoverableLLMEnrichmentError(ctx, err) {
				return FilledWp{}, fmt.Errorf("select closest area for template %d: %w", template.ID, err)
			}
		}
		if strings.TrimSpace(selectedArea) != "" && confidence >= locationGuessMinConfidence {
			guessedEntries, err := memoryStore.FindByTypeAndArea(ctx, template.WpTypeID, selectedArea)
			if err != nil {
				return FilledWp{}, fmt.Errorf("find structured memory by guessed area for template %d: %w", template.ID, err)
			}
			applyStructuredMemoryEntries(content, fieldsByPath, guessedEntries, structuredMemorySourceAreaGuess, &filledFields, filledPathSet)
		}
	}

	// step 5：如果仍存在必填空字段，则参考同类型历史样本，让大模型只返回字段路径到字段值的映射。
	requiredPaths := collectMissingRequiredPaths(fieldsByPath, filledPathSet)
	if len(requiredPaths) > 0 {
		historyEntries, err := memoryStore.FindByType(ctx, template.WpTypeID)
		if err != nil {
			return FilledWp{}, fmt.Errorf("find structured memory history for template %d: %w", template.ID, err)
		}
		historySamples := buildStructuredMemorySamples(historyEntries)
		fallbackValues, err := helper.fillFields(ctx, template, state, fieldsByPath, requiredPaths, historySamples)
		if err != nil {
			if !isRecoverableLLMEnrichmentError(ctx, err) {
				return FilledWp{}, fmt.Errorf("fill required fields with llm for template %d: %w", template.ID, err)
			}
		} else {
			applyResolvedValues(fieldsByPath, fallbackValues, llmFallbackSource, &filledFields, filledPathSet)
		}
	}

	return FilledWp{
		WpType:         readableWpType(template),
		Name:           template.Name,
		Content:        content,
		FilledFields:   filledFields,
		UnfilledFields: collectUnfilledFields(fieldsByPath),
	}, nil
}

func collectTemplateFields(root map[string]any, basePath string, fillRules WpFillRules, wpTypeID int) []fillCandidateField {
	fields := make([]fillCandidateField, 0)
	collectTemplateFieldsRecursive(root, basePath, fillRules, wpTypeID, &fields)
	return fields
}

func collectTemplateFieldsRecursive(value any, currentPath string, fillRules WpFillRules, wpTypeID int, fields *[]fillCandidateField) {
	switch typed := value.(type) {
	case map[string]any:
		if field, ok := buildFieldCandidate(typed, currentPath, fillRules, wpTypeID); ok {
			*fields = append(*fields, field)
		}
		for key, child := range typed {
			nextPath := joinPath(currentPath, key)
			collectTemplateFieldsRecursive(child, nextPath, fillRules, wpTypeID, fields)
		}
	case []any:
		for index, item := range typed {
			nextPath := joinPath(currentPath, strconv.Itoa(index))
			collectTemplateFieldsRecursive(item, nextPath, fillRules, wpTypeID, fields)
		}
	}
}

func buildFieldCandidate(field map[string]any, currentPath string, fillRules WpFillRules, wpTypeID int) (fillCandidateField, bool) {
	name, _ := field["name"].(string)
	fieldType, _ := field["type"].(string)
	if strings.TrimSpace(name) == "" || strings.TrimSpace(fieldType) == "" {
		return fillCandidateField{}, false
	}
	value, hasValue := field["value"]
	if !hasValue {
		return fillCandidateField{}, false
	}
	if !supportsFill(field) {
		return fillCandidateField{}, false
	}
	label, _ := field["label"].(string)
	displayName := strings.TrimSpace(label)
	if displayName == "" {
		displayName = name
	}

	required, _ := field["required"].(bool)
	path := joinPath(currentPath, "value")
	return fillCandidateField{
		Path:         path,
		Name:         name,
		Label:        label,
		Type:         fieldType,
		Required:     required,
		Value:        value,
		Field:        field,
		AllowFill:    fillRules.ShouldFill(wpTypeID, name, displayName),
		DisplayLabel: displayName,
	}, true
}

func indexFieldsByPath(fields []fillCandidateField) map[string]*fillCandidateField {
	index := make(map[string]*fillCandidateField, len(fields))
	for i := range fields {
		field := fields[i]
		index[field.Path] = &fields[i]
	}
	return index
}

func fillStateFields(fields []fillCandidateField, state *WpGraphState, filledFields *[]FilledField, filledPathSet map[string]struct{}) {
	for i := range fields {
		field := fields[i]
		if !field.AllowFill || !isEmptyFieldValue(field.Field["value"]) {
			continue
		}
		value, ok := resolveStateFieldValue(field.Name, state)
		if !ok {
			continue
		}
		field.Field["value"] = value
		*filledFields = append(*filledFields, FilledField{Label: field.DisplayLabel, Value: value, Source: "state"})
		filledPathSet[field.Path] = struct{}{}
	}
}

func applyStructuredMemoryEntries(content map[string]any, fieldsByPath map[string]*fillCandidateField, entries []StructuredMemoryEntry, source string, filledFields *[]FilledField, filledPathSet map[string]struct{}) int {
	if len(entries) == 0 {
		return 0
	}
	rawContent, err := json.Marshal(content)
	if err != nil {
		return 0
	}
	filledCount := 0
	for _, entry := range chooseLatestMemoryEntries(entries) {
		path := strings.TrimSpace(entry.FieldName)
		field, ok := fieldsByPath[path]
		if !ok || !field.AllowFill {
			continue
		}
		if !gjson.GetBytes(rawContent, path).Exists() {
			continue
		}
		if !isEmptyFieldValue(field.Field["value"]) {
			continue
		}
		field.Field["value"] = entry.FieldValue
		*filledFields = append(*filledFields, FilledField{
			Label:  field.DisplayLabel,
			Value:  entry.FieldValue,
			Source: source,
		})
		filledPathSet[path] = struct{}{}
		filledCount++
	}
	return filledCount
}

func chooseLatestMemoryEntries(entries []StructuredMemoryEntry) []StructuredMemoryEntry {
	index := make(map[string]StructuredMemoryEntry, len(entries))
	for _, entry := range entries {
		existing, ok := index[entry.FieldName]
		if !ok || existing.UpdatedAt < entry.UpdatedAt {
			index[entry.FieldName] = entry
		}
	}
	result := make([]StructuredMemoryEntry, 0, len(index))
	for _, entry := range index {
		result = append(result, entry)
	}
	return result
}

func collectMissingRequiredPaths(fieldsByPath map[string]*fillCandidateField, filledPathSet map[string]struct{}) []string {
	paths := make([]string, 0)
	for path, field := range fieldsByPath {
		if !field.AllowFill || !field.Required {
			continue
		}
		if _, ok := filledPathSet[path]; ok {
			continue
		}
		if !isEmptyFieldValue(field.Field["value"]) {
			continue
		}
		paths = append(paths, path)
	}
	slices.Sort(paths)
	return paths
}

func buildStructuredMemorySamples(entries []StructuredMemoryEntry) []structuredMemorySample {
	type sampleKey struct {
		area   string
		source string
		desc   string
	}
	index := make(map[sampleKey]*structuredMemorySample)
	for _, entry := range entries {
		key := sampleKey{
			area:   strings.TrimSpace(entry.Area),
			source: strings.TrimSpace(entry.Source),
			desc:   strings.TrimSpace(entry.WorkDescription),
		}
		sample, ok := index[key]
		if !ok {
			sample = &structuredMemorySample{
				Area:            key.area,
				WorkDescription: key.desc,
				Source:          key.source,
				Fields:          make(map[string]any),
			}
			index[key] = sample
		}
		sample.Fields[entry.FieldName] = entry.FieldValue
	}
	samples := make([]structuredMemorySample, 0, len(index))
	for _, sample := range index {
		samples = append(samples, *sample)
	}
	return samples
}

func (h wpFillLLMHelper) selectClosestArea(ctx context.Context, template WpTemplate, state *WpGraphState, candidateAreas []string) (string, float64, error) {
	if len(candidateAreas) == 0 {
		return "", 0, nil
	}

	payload, err := json.Marshal(map[string]any{
		"wp_type_name": readableWpType(template),
		"area":         strings.TrimSpace(state.Area),
		"description":  strings.TrimSpace(state.Description),
		"candidates":   candidateAreas,
	})
	if err != nil {
		return "", 0, fmt.Errorf("marshal area selection payload: %w", err)
	}

	prompt, err := h.promptStore.Get(ctx, PromptKeyWpFillAreaSelection)
	if err != nil {
		return "", 0, fmt.Errorf("load prompt %q: %w", PromptKeyWpFillAreaSelection, err)
	}

	response, err := h.chatModel.Generate(ctx, []*schema.Message{
		schema.SystemMessage(prompt.Content),
		schema.UserMessage(string(payload)),
	})
	if err != nil {
		return "", 0, fmt.Errorf("generate area selection response: %w", err)
	}
	if response == nil || strings.TrimSpace(response.Content) == "" {
		return "", 0, fmt.Errorf("area selection response is empty")
	}

	var result llmAreaSelectionResult
	if err := json.Unmarshal([]byte(response.Content), &result); err != nil {
		return "", 0, fmt.Errorf("decode area selection response: %w", err)
	}
	return strings.TrimSpace(result.Area), result.Confidence, nil
}

func (h wpFillLLMHelper) fillFields(ctx context.Context, template WpTemplate, state *WpGraphState, fieldsByPath map[string]*fillCandidateField, requiredPaths []string, historySamples []structuredMemorySample) (map[string]any, error) {
	if len(requiredPaths) == 0 {
		return nil, nil
	}

	requiredFields := make([]map[string]any, 0, len(requiredPaths))
	for _, path := range requiredPaths {
		field := fieldsByPath[path]
		requiredFields = append(requiredFields, map[string]any{
			"path":     path,
			"name":     field.Name,
			"label":    field.DisplayLabel,
			"type":     field.Type,
			"required": field.Required,
		})
	}

	payload, err := json.Marshal(map[string]any{
		"wp_type_name":      readableWpType(template),
		"template_name":     template.Name,
		"area":              strings.TrimSpace(state.Area),
		"description":       strings.TrimSpace(state.Description),
		"start_time":        strings.TrimSpace(state.StartTime),
		"end_time":          strings.TrimSpace(state.EndTime),
		"required_fields":   requiredFields,
		"structured_samples": historySamples,
	})
	if err != nil {
		return nil, fmt.Errorf("marshal llm fill payload: %w", err)
	}

	prompt, err := h.promptStore.Get(ctx, PromptKeyWpFillFieldCompletion)
	if err != nil {
		return nil, fmt.Errorf("load prompt %q: %w", PromptKeyWpFillFieldCompletion, err)
	}

	response, err := h.chatModel.Generate(ctx, []*schema.Message{
		schema.SystemMessage(prompt.Content),
		schema.UserMessage(string(payload)),
	})
	if err != nil {
		return nil, fmt.Errorf("generate llm field fill response: %w", err)
	}
	if response == nil || strings.TrimSpace(response.Content) == "" {
		return nil, fmt.Errorf("llm field fill response is empty")
	}

	var result llmFieldFillResult
	if err := json.Unmarshal([]byte(response.Content), &result); err != nil {
		return nil, fmt.Errorf("decode llm field fill response: %w", err)
	}
	return result.Fields, nil
}

func applyResolvedValues(fieldsByPath map[string]*fillCandidateField, values map[string]any, source string, filledFields *[]FilledField, filledPathSet map[string]struct{}) {
	for path, value := range values {
		field, ok := fieldsByPath[path]
		if !ok || !field.AllowFill {
			continue
		}
		if !isEmptyFieldValue(field.Field["value"]) {
			continue
		}
		if isEmptyFieldValue(value) {
			continue
		}
		field.Field["value"] = value
		*filledFields = append(*filledFields, FilledField{
			Label:  field.DisplayLabel,
			Value:  value,
			Source: source,
		})
		filledPathSet[path] = struct{}{}
	}
}

func collectUnfilledFields(fieldsByPath map[string]*fillCandidateField) []UnfilledField {
	unfilled := make([]UnfilledField, 0)
	seen := make(map[string]struct{})
	for _, field := range fieldsByPath {
		if !field.Required || !field.AllowFill {
			continue
		}
		if !isEmptyFieldValue(field.Field["value"]) {
			continue
		}
		if _, exists := seen[field.DisplayLabel]; exists {
			continue
		}
		seen[field.DisplayLabel] = struct{}{}
		unfilled = append(unfilled, UnfilledField{Label: field.DisplayLabel})
	}
	return unfilled
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

func supportsFill(field map[string]any) bool {
	fieldType, _ := field["type"].(string)
	return slices.Contains([]string{"input", "datepicker", "selector", "checkbox", "radio", "textarea"}, fieldType)
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
	case map[string]any:
		return len(typed) == 0
	default:
		return false
	}
}

func joinPath(base, next string) string {
	if strings.TrimSpace(base) == "" {
		return next
	}
	return base + "." + next
}

func isRecoverableLLMEnrichmentError(ctx context.Context, err error) bool {
	if err == nil {
		return false
	}
	if isLLMTimeoutError(ctx, err) {
		return true
	}
	var syntaxErr *json.SyntaxError
	if errors.As(err, &syntaxErr) {
		return true
	}
	var unmarshalTypeErr *json.UnmarshalTypeError
	if errors.As(err, &unmarshalTypeErr) {
		return true
	}
	message := err.Error()
	return strings.Contains(message, "unexpected end of JSON input") ||
		strings.Contains(message, "response is empty")
}

func isLLMTimeoutError(ctx context.Context, err error) bool {
	if err == nil {
		return false
	}
	if ctx != nil && ctx.Err() != nil && !errors.Is(err, ctx.Err()) {
		return false
	}
	if errors.Is(err, context.DeadlineExceeded) {
		return true
	}
	if os.IsTimeout(err) {
		return true
	}
	message := err.Error()
	return strings.Contains(message, "Client.Timeout exceeded") ||
		strings.Contains(message, "context deadline exceeded")
}
