package graph

import (
	"context"
	"encoding/json"
	"fmt"
	"strconv"
	"strings"
	"time"

	einotool "github.com/cloudwego/eino/components/tool"
	"github.com/cloudwego/eino/compose"
)

func newSubmitWpsNode(memoryStore StructuredMemoryStore, updateWpInfoTool einotool.InvokableTool, linkWpInfoTool einotool.InvokableTool) func(context.Context, *WpGraphState) (*WpGraphState, error) {
	return func(ctx context.Context, state *WpGraphState) (*WpGraphState, error) {
		if memoryStore == nil {
			return nil, fmt.Errorf("structured memory store is nil")
		}
		if updateWpInfoTool == nil {
			return nil, fmt.Errorf("update work permit tool is nil")
		}
		if linkWpInfoTool == nil {
			return nil, fmt.Errorf("link work permit tool is nil")
		}
		if state == nil {
			if wasInterrupted, hasState, interruptedState := compose.GetInterruptState[*WpGraphState](ctx); wasInterrupted && hasState {
				state = interruptedState
			}
		}
		if state == nil {
			return nil, fmt.Errorf("work permit graph state is nil")
		}

		if isResume, hasData, approval := compose.GetResumeContext[SubmitWpsApproval](ctx); isResume {
			if !hasData {
				return nil, fmt.Errorf("submit approval data is missing")
			}
			if !approval.Approved {
				state.CurrentNode = nodeSubmitWps
				state.ErrorMsg = "work permit submission rejected by user"
				return state, nil
			}
			permitTypeID := requiredPermitTypeID(state.IntentWpTypes)
			if permitTypeID == 0 {
				return nil, fmt.Errorf("required work permit type id is missing")
			}
			if err := submitApprovedWps(ctx, updateWpInfoTool, linkWpInfoTool, memoryStore, state.WpTemplates, permitTypeID, approval); err != nil {
				return nil, err
			}

			state.CurrentNode = nodeSubmitWps
			state.ErrorMsg = ""
			return state, nil
		}

		preview, err := json.Marshal(state.FilledWps)
		if err != nil {
			return nil, fmt.Errorf("marshal work permit preview: %w", err)
		}
		info := map[string]any{
			"operation":  "submit_work_permits",
			"message":    "请审核作业票预览。确认后请回传完整 payloads；如需编辑，请回传编辑后的完整 payloads。",
			"filled_wps": state.FilledWps,
			"preview":    json.RawMessage(preview),
		}
		return nil, compose.StatefulInterrupt(ctx, info, state)
	}
}

func submitApprovedWps(ctx context.Context, updateWpInfoTool einotool.InvokableTool, linkWpInfoTool einotool.InvokableTool, memoryStore StructuredMemoryStore, templates []WpTemplate, permitTypeID int, approval SubmitWpsApproval) error {
	if len(approval.Payloads) == 0 {
		return fmt.Errorf("approved work permit payloads are empty")
	}

	ids := make([]int, 0, len(approval.Payloads))
	childIndexes := make([]int, 0, len(approval.Payloads))
	for i, payload := range approval.Payloads {
		if len(payload) == 0 {
			return fmt.Errorf("approved work permit payload %d is empty", i)
		}
		arguments, err := json.Marshal(map[string]any{"payload": payload})
		if err != nil {
			return fmt.Errorf("marshal work permit payload %d: %w", i, err)
		}
		result, err := updateWpInfoTool.InvokableRun(ctx, string(arguments))
		if err != nil {
			return fmt.Errorf("submit work permit payload %d: %w", i, err)
		}
		id, err := decodeCreatedWpID(result)
		if err != nil {
			return fmt.Errorf("decode created work permit id for payload %d: %w", i, err)
		}
		ids = append(ids, id)
		if payloadTypeID(payload) != permitTypeID {
			childIndexes = append(childIndexes, i)
		}
	}

	parentIndex := -1
	for i, payload := range approval.Payloads {
		if payloadTypeID(payload) == permitTypeID {
			parentIndex = i
			break
		}
	}
	if len(childIndexes) > 0 && parentIndex < 0 {
		return fmt.Errorf("work permit parent payload is missing")
	}
	for _, childIndex := range childIndexes {
		arguments := fmt.Sprintf(`{"id":%d,"fid":%d}`, ids[childIndex], ids[parentIndex])
		if _, err := linkWpInfoTool.InvokableRun(ctx, arguments); err != nil {
			return fmt.Errorf("link child work permit %d to parent %d: %w", ids[childIndex], ids[parentIndex], err)
		}
	}
	if err := persistStructuredMemoryFromPayloads(ctx, memoryStore, templates, approval.Payloads, ids); err != nil {
		return err
	}
	return nil
}

func ImportStructuredMemoryPayloads(ctx context.Context, memoryStore StructuredMemoryStore, payloads []map[string]any, sourcePrefix string) (int, error) {
	if len(payloads) == 0 {
		return 0, fmt.Errorf("payloads are empty")
	}
	ids := make([]int, 0, len(payloads))
	for i := range payloads {
		sourceID := i + 1
		if strings.TrimSpace(sourcePrefix) != "" {
			sourceID = i + 1
		}
		ids = append(ids, sourceID)
	}
	normalizedPrefix := strings.TrimSpace(sourcePrefix)
	labelIndexes := map[int]map[string]string{}
	total := 0
	for i, payload := range payloads {
		sourceID := strconv.Itoa(ids[i])
		if normalizedPrefix != "" {
			sourceID = normalizedPrefix + "-" + sourceID
		}
		entries, err := extractStructuredMemoryEntries(payload, sourceID, labelIndexes[templateIDFromPayload(payload)])
		if err != nil {
			return 0, fmt.Errorf("extract structured memory entries for payload %d: %w", i, err)
		}
		for _, entry := range entries {
			if err := memoryStore.Upsert(ctx, entry); err != nil {
				return 0, fmt.Errorf("persist structured memory entry %q for payload %d: %w", entry.FieldName, i, err)
			}
			total++
		}
	}
	return total, nil
}

func ExportStructuredMemoryPayloads(ctx context.Context, memoryStore StructuredMemoryStore, wpTypeID int, area string) ([]map[string]any, error) {
	if memoryStore == nil {
		return nil, fmt.Errorf("structured memory store is nil")
	}
	entries, err := memoryStore.FindByTypeAndArea(ctx, wpTypeID, area)
	if err != nil {
		return nil, err
	}
	if len(entries) == 0 {
		return []map[string]any{}, nil
	}

	grouped := make(map[string][]StructuredMemoryEntry)
	order := make([]string, 0)
	for _, entry := range entries {
		source := strings.TrimSpace(entry.Source)
		if source == "" {
			source = "unknown"
		}
		if _, ok := grouped[source]; !ok {
			order = append(order, source)
		}
		grouped[source] = append(grouped[source], entry)
	}

	payloads := make([]map[string]any, 0, len(grouped))
	for _, source := range order {
		group := grouped[source]
		payload := map[string]any{
			"wptypeid":   wpTypeID,
			"wptypename": group[0].WpTypeName,
		}
		for _, entry := range group {
			if err := setValueAtPath(payload, entry.FieldName, entry.FieldValue); err != nil {
				return nil, fmt.Errorf("rebuild payload field %q: %w", entry.FieldName, err)
			}
		}
		payloads = append(payloads, payload)
	}
	return payloads, nil
}

func persistStructuredMemoryFromPayloads(ctx context.Context, memoryStore StructuredMemoryStore, templates []WpTemplate, payloads []map[string]any, ids []int) error {
	if memoryStore == nil {
		return fmt.Errorf("structured memory store is nil")
	}
	if len(payloads) != len(ids) {
		return fmt.Errorf("created work permit ids do not match payload count")
	}

	labelIndexes := buildTemplateLabelIndexes(templates)
	for i, payload := range payloads {
		entries, err := extractStructuredMemoryEntries(payload, strconv.Itoa(ids[i]), labelIndexes[templateIDFromPayload(payload)])
		if err != nil {
			return fmt.Errorf("extract structured memory entries for payload %d: %w", i, err)
		}
		for _, entry := range entries {
			if err := memoryStore.Upsert(ctx, entry); err != nil {
				return fmt.Errorf("persist structured memory entry %q for payload %d: %w", entry.FieldName, i, err)
			}
		}
	}
	return nil
}

func buildTemplateLabelIndexes(templates []WpTemplate) map[int]map[string]string {
	indexes := make(map[int]map[string]string, len(templates))
	for _, template := range templates {
		if template.ID == 0 || template.ApplyContentObject == nil {
			continue
		}
		labels := make(map[string]string)
		collectFieldLabels(template.ApplyContentObject, "", labels)
		indexes[template.ID] = labels
	}
	return indexes
}

func collectFieldLabels(value any, currentPath string, labels map[string]string) {
	switch typed := value.(type) {
	case map[string]any:
		if _, hasValue := typed["value"]; hasValue {
			if label, _ := typed["label"].(string); strings.TrimSpace(label) != "" {
				labels[joinPath(currentPath, "value")] = strings.TrimSpace(label)
			}
		}
		for key, child := range typed {
			collectFieldLabels(child, joinPath(currentPath, key), labels)
		}
	case []any:
		for i, child := range typed {
			collectFieldLabels(child, joinPath(currentPath, strconv.Itoa(i)), labels)
		}
	}
}

func extractStructuredMemoryEntries(payload map[string]any, source string, templateLabels map[string]string) ([]StructuredMemoryEntry, error) {
	wpTypeID := payloadTypeID(payload)
	if wpTypeID == 0 {
		return nil, fmt.Errorf("payload work permit type id is missing")
	}

	wpTypeName := strings.TrimSpace(stringValue(payload["wptypename"]))
	if wpTypeName == "" {
		wpTypeName = strings.TrimSpace(stringValue(payload["wptype"]))
	}

	flatFields := make([]StructuredMemoryEntry, 0)
	now := time.Now().Unix()
	area := extractPayloadArea(payload)
	description := extractPayloadDescription(payload)
	collectStructuredMemoryEntries(payload, "", wpTypeID, wpTypeName, area, description, source, now, templateLabels, &flatFields)
	return flatFields, nil
}

func collectStructuredMemoryEntries(value any, currentPath string, wpTypeID int, wpTypeName, area, description, source string, updatedAt int64, templateLabels map[string]string, entries *[]StructuredMemoryEntry) {
	switch typed := value.(type) {
	case map[string]any:
		if fieldValue, hasValue := typed["value"]; hasValue && !isEmptyFieldValue(fieldValue) {
			fieldPath := joinPath(currentPath, "value")
			fieldDescription := strings.TrimSpace(stringValue(typed["label"]))
			if fieldDescription == "" && templateLabels != nil {
				fieldDescription = strings.TrimSpace(templateLabels[fieldPath])
			}
			if fieldDescription == "" {
				fieldDescription = strings.TrimSpace(stringValue(typed["name"]))
			}
			*entries = append(*entries, StructuredMemoryEntry{
				WpTypeID:         wpTypeID,
				WpTypeName:       wpTypeName,
				Area:             area,
				WorkDescription:  description,
				FieldDescription: fieldDescription,
				FieldName:        fieldPath,
				FieldValue:       fieldValue,
				Source:           source,
				UpdatedAt:        updatedAt,
			})
		}
		for key, child := range typed {
			collectStructuredMemoryEntries(child, joinPath(currentPath, key), wpTypeID, wpTypeName, area, description, source, updatedAt, templateLabels, entries)
		}
	case []any:
		for i, child := range typed {
			collectStructuredMemoryEntries(child, joinPath(currentPath, strconv.Itoa(i)), wpTypeID, wpTypeName, area, description, source, updatedAt, templateLabels, entries)
		}
	}
}

func extractPayloadArea(payload map[string]any) string {
	value, ok := findFieldValueByName(payload, "areaid")
	if !ok {
		return ""
	}
	return displayValue(value)
}

func extractPayloadDescription(payload map[string]any) string {
	value, ok := findFieldValueByName(payload, "discription")
	if !ok {
		return ""
	}
	return strings.TrimSpace(stringValue(value))
}

func findFieldValueByName(value any, target string) (any, bool) {
	switch typed := value.(type) {
	case map[string]any:
		if name, _ := typed["name"].(string); strings.TrimSpace(name) == target {
			fieldValue, ok := typed["value"]
			if ok {
				return fieldValue, true
			}
		}
		for _, child := range typed {
			if found, ok := findFieldValueByName(child, target); ok {
				return found, true
			}
		}
	case []any:
		for _, child := range typed {
			if found, ok := findFieldValueByName(child, target); ok {
				return found, true
			}
		}
	}
	return nil, false
}

func displayValue(value any) string {
	switch typed := value.(type) {
	case string:
		return strings.TrimSpace(typed)
	case []any:
		names := make([]string, 0, len(typed))
		for _, item := range typed {
			name := displayValue(item)
			if name != "" {
				names = append(names, name)
			}
		}
		return strings.Join(names, ",")
	case map[string]any:
		if name := strings.TrimSpace(stringValue(typed["name"])); name != "" {
			return name
		}
		if label := strings.TrimSpace(stringValue(typed["label"])); label != "" {
			return label
		}
	}
	return strings.TrimSpace(stringValue(value))
}

func stringValue(value any) string {
	switch typed := value.(type) {
	case string:
		return typed
	case json.Number:
		return typed.String()
	case float64:
		return strconv.FormatFloat(typed, 'f', -1, 64)
	case float32:
		return strconv.FormatFloat(float64(typed), 'f', -1, 32)
	case int:
		return strconv.Itoa(typed)
	case int64:
		return strconv.FormatInt(typed, 10)
	case int32:
		return strconv.FormatInt(int64(typed), 10)
	case bool:
		if typed {
			return "true"
		}
		return "false"
	default:
		return ""
	}
}

func templateIDFromPayload(payload map[string]any) int {
	switch value := payload["wptempletid"].(type) {
	case float64:
		return int(value)
	case int:
		return value
	case json.Number:
		id, _ := value.Int64()
		return int(id)
	default:
		return 0
	}
}

func setValueAtPath(root map[string]any, path string, value any) error {
	parts := strings.Split(strings.TrimSpace(path), ".")
	if len(parts) == 0 || parts[0] == "" {
		return fmt.Errorf("path is empty")
	}

	_, err := setValueAtPathRecursive(root, parts, value)
	return err
}

func setValueAtPathRecursive(current any, parts []string, value any) (any, error) {
	if len(parts) == 0 {
		return value, nil
	}

	part := parts[0]
	last := len(parts) == 1

	switch typed := current.(type) {
	case map[string]any:
		if typed == nil {
			typed = map[string]any{}
		}
		if last {
			typed[part] = value
			return typed, nil
		}
		next, ok := typed[part]
		if !ok {
			next = emptyContainerForPathPart(parts[1])
		}
		updatedNext, err := setValueAtPathRecursive(next, parts[1:], value)
		if err != nil {
			return nil, err
		}
		typed[part] = updatedNext
		return typed, nil
	case []any:
		index, err := strconv.Atoi(part)
		if err != nil {
			return nil, fmt.Errorf("path segment %q is not array index", part)
		}
		for len(typed) <= index {
			typed = append(typed, nil)
		}
		if last {
			typed[index] = value
			return typed, nil
		}
		next := typed[index]
		if next == nil {
			next = emptyContainerForPathPart(parts[1])
		}
		updatedNext, err := setValueAtPathRecursive(next, parts[1:], value)
		if err != nil {
			return nil, err
		}
		typed[index] = updatedNext
		return typed, nil
	case nil:
		return setValueAtPathRecursive(emptyContainerForPathPart(part), parts, value)
	default:
		return nil, fmt.Errorf("unsupported container type %T", current)
	}
}

func emptyContainerForPathPart(part string) any {
	if _, err := strconv.Atoi(part); err == nil {
		return []any{}
	}
	return map[string]any{}
}

type createdWpResponse struct {
	Success bool           `json:"success"`
	Data    map[string]any `json:"data"`
}

func decodeCreatedWpID(raw string) (int, error) {
	var response createdWpResponse
	if err := json.Unmarshal([]byte(raw), &response); err != nil {
		return 0, fmt.Errorf("unmarshal response: %w", err)
	}
	if !response.Success {
		return 0, fmt.Errorf("create work permit failed")
	}
	if response.Data == nil {
		return 0, fmt.Errorf("response data is empty")
	}
	return decodeCreatedWpIDFromMap(response.Data)
}

func decodeCreatedWpIDFromMap(data map[string]any) (int, error) {
	switch value := data["id"].(type) {
	case float64:
		return int(value), nil
	case int:
		return value, nil
	case json.Number:
		id, err := value.Int64()
		if err != nil {
			return 0, fmt.Errorf("parse response id: %w", err)
		}
		return int(id), nil
	default:
		return 0, fmt.Errorf("response data is not an id: %T", data)
	}
}

func payloadTypeID(payload map[string]any) int {
	switch value := payload["wptypeid"].(type) {
	case float64:
		return int(value)
	case int:
		return value
	case json.Number:
		id, _ := value.Int64()
		return int(id)
	default:
		return 0
	}
}

func requiredPermitTypeID(types []WpMetaInfo) int {
	for _, meta := range types {
		if meta.WpTypeId != 0 && (meta.TemplateName == "作业许可证" || meta.WpTypeName == "作业许可证") {
			return meta.WpTypeId
		}
	}
	return 0
}
