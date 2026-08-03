package graph

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	einotool "github.com/cloudwego/eino/components/tool"
)

type wpTemplateResponse struct {
	Success bool            `json:"success"`
	Message string          `json:"message"`
	Data    json.RawMessage `json:"data"`
}

type wpTemplateData struct {
	ID                   int            `json:"id"`
	WpTypeID             int            `json:"wptypeid"`
	WpTypeName           string         `json:"wptypename"`
	Name                 string         `json:"templetname"`
	IsDefault            bool           `json:"isdefault"`
	IDPrefix             string         `json:"idprefix"`
	UpdatedBy            string         `json:"updatedby"`
	UpdateTime           int64          `json:"updatetime"`
	Version              int            `json:"version"`
	State                int            `json:"state"`
	SiteIDs              string         `json:"siteids"`
	HandwrittenSignature int            `json:"handwrittensignature"`
	ApplyContent         string         `json:"applycontent"`
	ApplyContentObject   map[string]any `json:"applycontent_object"`
}

func newSearchWpTemplateNode(templateDetailTool einotool.InvokableTool) func(context.Context, *WpGraphState) (*WpGraphState, error) {
	return func(ctx context.Context, state *WpGraphState) (*WpGraphState, error) {
		if templateDetailTool == nil {
			return nil, fmt.Errorf("work permit template detail tool is nil")
		}

		templates := make([]WpTemplate, 0, len(state.IntentWpTypes))
		for _, meta := range state.IntentWpTypes {
			if meta.Id == 0 {
				return nil, fmt.Errorf("work permit template id is empty")
			}

			raw, err := templateDetailTool.InvokableRun(ctx, fmt.Sprintf(`{"id":%d}`, meta.Id))
			if err != nil {
				return nil, fmt.Errorf("query work permit template %d: %w", meta.Id, err)
			}

			template, err := decodeWpTemplateResponse(raw, meta)
			if err != nil {
				return nil, fmt.Errorf("decode work permit template %d: %w", meta.Id, err)
			}
			templates = append(templates, template)
		}

		state.WpTemplates = templates
		state.CurrentNode = nodeSearchWpTemplate
		return state, nil
	}
}

func decodeWpTemplateResponse(raw string, meta WpMetaInfo) (WpTemplate, error) {
	var response wpTemplateResponse
	if err := json.Unmarshal([]byte(raw), &response); err != nil {
		return WpTemplate{}, fmt.Errorf("unmarshal response: %w", err)
	}
	if !response.Success {
		if response.Message == "" {
			return WpTemplate{}, fmt.Errorf("template query failed")
		}
		return WpTemplate{}, fmt.Errorf("template query failed: %s", response.Message)
	}
	if len(response.Data) == 0 || string(response.Data) == "null" {
		return WpTemplate{}, fmt.Errorf("template data is empty")
	}

	var data wpTemplateData
	if err := json.Unmarshal(response.Data, &data); err != nil {
		return WpTemplate{}, fmt.Errorf("unmarshal template data: %w", err)
	}

	id := data.ID
	if id == 0 {
		id = meta.Id
	}
	wpTypeID := data.WpTypeID
	if wpTypeID == 0 {
		wpTypeID = meta.WpTypeId
	}
	wpTypeName := strings.TrimSpace(data.WpTypeName)
	if wpTypeName == "" {
		wpTypeName = strings.TrimSpace(meta.WpTypeName)
	}
	name := strings.TrimSpace(data.Name)
	if name == "" {
		name = strings.TrimSpace(meta.TemplateName)
	}
	return WpTemplate{
		ID:                   id,
		WpTypeID:             wpTypeID,
		WpTypeName:           wpTypeName,
		Name:                 name,
		IsDefault:            data.IsDefault,
		IDPrefix:             strings.TrimSpace(data.IDPrefix),
		UpdatedBy:            strings.TrimSpace(data.UpdatedBy),
		UpdateTime:           data.UpdateTime,
		Version:              data.Version,
		State:                data.State,
		SiteIDs:              strings.TrimSpace(data.SiteIDs),
		HandwrittenSignature: data.HandwrittenSignature,
		ApplyContent:         data.ApplyContent,
		ApplyContentObject:   data.ApplyContentObject,
	}, nil
}
