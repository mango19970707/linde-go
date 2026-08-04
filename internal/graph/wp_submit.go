package graph

import (
	"context"
	"encoding/json"
	"fmt"

	einotool "github.com/cloudwego/eino/components/tool"
	"github.com/cloudwego/eino/compose"
)

func newSubmitWpsNode(updateWpInfoTool einotool.InvokableTool, linkWpInfoTool einotool.InvokableTool) func(context.Context, *WpGraphState) (*WpGraphState, error) {
	return func(ctx context.Context, state *WpGraphState) (*WpGraphState, error) {
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
			if err := submitApprovedWps(ctx, updateWpInfoTool, linkWpInfoTool, permitTypeID, approval); err != nil {
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

func submitApprovedWps(ctx context.Context, updateWpInfoTool einotool.InvokableTool, linkWpInfoTool einotool.InvokableTool, permitTypeID int, approval SubmitWpsApproval) error {
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
	return nil
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
