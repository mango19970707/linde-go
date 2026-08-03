package graph

import (
	"encoding/json"
	"fmt"
	"strings"
)

type wpMetaInfoResponse struct {
	Success bool         `json:"success"`
	Message string       `json:"message"`
	Data    []WpMetaInfo `json:"data"`
}

func decodeWpMetaInfoResponse(raw string) ([]WpMetaInfo, error) {
	var response wpMetaInfoResponse
	if err := json.Unmarshal([]byte(raw), &response); err != nil {
		return nil, fmt.Errorf("unmarshal response: %w", err)
	}
	if !response.Success {
		if response.Message == "" {
			return nil, fmt.Errorf("template query failed")
		}
		return nil, fmt.Errorf("template query failed: %s", response.Message)
	}
	return response.Data, nil
}

func selectWpMetaInfos(templates []WpMetaInfo, selectedIDs []int) ([]WpMetaInfo, error) {
	permit, ok := findRequiredPermit(templates)
	if !ok {
		return nil, fmt.Errorf("required work permit template %q not found", "作业许可证")
	}

	byTypeID := make(map[int]WpMetaInfo, len(templates))
	for _, template := range templates {
		if template.WpTypeId == 0 {
			continue
		}
		if _, exists := byTypeID[template.WpTypeId]; !exists {
			byTypeID[template.WpTypeId] = template
		}
	}

	result := make([]WpMetaInfo, 0, len(selectedIDs)+1)
	seen := make(map[int]struct{}, len(selectedIDs)+1)
	appendType := func(typeID int) error {
		template, ok := byTypeID[typeID]
		if !ok {
			return fmt.Errorf("recognized unknown work permit type id: %d", typeID)
		}
		if _, exists := seen[typeID]; exists {
			return nil
		}
		seen[typeID] = struct{}{}
		result = append(result, template)
		return nil
	}

	if err := appendType(permit.WpTypeId); err != nil {
		return nil, err
	}
	for _, typeID := range selectedIDs {
		if err := appendType(typeID); err != nil {
			return nil, err
		}
	}
	return result, nil
}

func findRequiredPermit(templates []WpMetaInfo) (WpMetaInfo, bool) {
	for _, template := range templates {
		if strings.TrimSpace(template.TemplateName) == "作业许可证" ||
			strings.TrimSpace(template.WpTypeName) == "作业许可证" {
			return template, true
		}
	}
	return WpMetaInfo{}, false
}
