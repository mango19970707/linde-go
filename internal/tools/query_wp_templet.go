package tools

import (
	"context"
	"encoding/json"
	"fmt"
	"net/url"

	einotool "github.com/cloudwego/eino/components/tool"
	"github.com/cloudwego/eino/components/tool/utils"
)

func NewQueryWptempletTool(cfg ToolConfig) (einotool.InvokableTool, error) {
	c, err := newClient(cfg)
	if err != nil {
		return nil, err
	}

	return utils.InferTool[templateIDInput, basicResponse](
		"query_wp_templet",
		"查询单个作业票模板详情；返回模板字段，并额外把 applycontent 从 JSON 字符串解析为 applycontent_object，便于后续直接编辑和创建。",
		func(ctx context.Context, input templateIDInput) (basicResponse, error) {
			query := url.Values{}
			intQuery("id", input.ID, query)

			resp, err := c.get(ctx, "/wp/queryWptemplet", query)
			if err != nil {
				return basicResponse{}, err
			}

			if !resp.Success || resp.Data == nil {
				return resp, nil
			}

			dataMap, err := marshalJSONMap(resp.Data)
			if err != nil {
				return basicResponse{}, err
			}

			applycontent, _ := dataMap["applycontent"].(string)
			if applycontent != "" {
				var applycontentObject map[string]any
				if err := json.Unmarshal([]byte(applycontent), &applycontentObject); err != nil {
					return basicResponse{}, fmt.Errorf("parse applycontent: %w", err)
				}
				dataMap["applycontent_object"] = applycontentObject
			}

			resp.Data = dataMap
			return resp, nil
		},
	)
}
