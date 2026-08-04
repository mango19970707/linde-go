package graph

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	einotool "github.com/cloudwego/eino/components/tool"
)

type toolAreaResolver struct {
	tool einotool.InvokableTool
}

func NewToolAreaResolver(tool einotool.InvokableTool) (AreaResolver, error) {
	if tool == nil {
		return nil, fmt.Errorf("area tool is nil")
	}
	return &toolAreaResolver{tool: tool}, nil
}

func (r *toolAreaResolver) Resolve(ctx context.Context, siteID int, userInput, extractedName string) (AreaInfo, error) {
	raw, err := r.tool.InvokableRun(ctx, fmt.Sprintf(`{"siteid":%d}`, siteID))
	if err != nil {
		return AreaInfo{}, fmt.Errorf("query area list: %w", err)
	}

	areas, err := decodeAreaListResponse(raw)
	if err != nil {
		return AreaInfo{}, err
	}

	return matchAreaInfo(areas, userInput, extractedName), nil
}

type areaListItem struct {
	ID   int    `json:"id"`
	Name string `json:"name"`
}

func decodeAreaListResponse(raw string) ([]AreaInfo, error) {
	var payload struct {
		Success bool           `json:"success"`
		Data    []areaListItem `json:"data"`
	}
	if err := json.Unmarshal([]byte(raw), &payload); err != nil {
		return nil, fmt.Errorf("decode area list response: %w", err)
	}

	areas := make([]AreaInfo, 0, len(payload.Data))
	for _, item := range payload.Data {
		name := strings.TrimSpace(item.Name)
		if item.ID == 0 || name == "" {
			continue
		}
		areas = append(areas, AreaInfo{ID: item.ID, Name: name})
	}
	return areas, nil
}

func matchAreaInfo(areas []AreaInfo, userInput, extractedName string) AreaInfo {
	extractedName = strings.TrimSpace(extractedName)
	userInput = strings.TrimSpace(userInput)

	for _, area := range areas {
		if extractedName != "" && strings.EqualFold(area.Name, extractedName) {
			return area
		}
	}
	for _, area := range areas {
		if extractedName != "" && strings.Contains(extractedName, area.Name) {
			return area
		}
	}
	for _, area := range areas {
		if userInput != "" && strings.Contains(userInput, area.Name) {
			return area
		}
	}
	return AreaInfo{}
}
