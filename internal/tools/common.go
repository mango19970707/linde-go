package tools

import (
	"context"
	"encoding/json"
	"fmt"
	"net/url"
	"strconv"

	einotool "github.com/cloudwego/eino/components/tool"
	"github.com/cloudwego/eino/components/tool/utils"
	"github.com/cloudwego/eino/schema"
)

type siteIDInput struct {
	SiteID int `json:"siteid,omitempty" jsonschema_description:"工厂 ID；未传时使用工具默认 siteid"`
}

type areaEquipmentInput struct {
	SiteID int `json:"siteid,omitempty" jsonschema_description:"工厂 ID；未传时使用工具默认 siteid"`
	AreaID int `json:"areaid" jsonschema:"required" jsonschema_description:"区域 ID"`
}

type templateIDInput struct {
	ID int `json:"id" jsonschema:"required" jsonschema_description:"模板 ID"`
}

type linkExistInput struct {
	ID  int `json:"id" jsonschema:"required" jsonschema_description:"子票 ID"`
	FID int `json:"fid" jsonschema:"required" jsonschema_description:"父票 ID"`
}

type updateWpInfoInput struct {
	Payload map[string]any `json:"payload" jsonschema:"required" jsonschema_description:"完整的创建作业票 JSON 请求体对象"`
}

func newGetTool[T any](cfg ToolConfig, name, desc, path string, buildQuery func(*client, T) (url.Values, error)) (einotool.InvokableTool, error) {
	c, err := newClient(cfg)
	if err != nil {
		return nil, err
	}

	return utils.InferTool[T, basicResponse](name, desc, func(ctx context.Context, input T) (basicResponse, error) {
		query, err := buildQuery(c, input)
		if err != nil {
			return basicResponse{}, err
		}
		return c.get(ctx, path, query)
	})
}

func intQuery(key string, value int, query url.Values) {
	query.Set(key, strconv.Itoa(value))
}

func marshalJSONMap(value any) (map[string]any, error) {
	raw, err := json.Marshal(value)
	if err != nil {
		return nil, fmt.Errorf("marshal value: %w", err)
	}

	var result map[string]any
	if err := json.Unmarshal(raw, &result); err != nil {
		return nil, fmt.Errorf("unmarshal value to map: %w", err)
	}

	return result, nil
}

func newUpdateWpInfoToolInfo() *schema.ToolInfo {
	return &schema.ToolInfo{
		Name: "update_wp_info",
		Desc: "根据模板详情中的 applycontent 和业务字段值，提交完整 JSON 请求体以创建作业票。",
		ParamsOneOf: schema.NewParamsOneOfByParams(map[string]*schema.ParameterInfo{
			"payload": {
				Type:     schema.Object,
				Desc:     "完整的创建作业票 JSON 请求体对象。",
				Required: true,
			},
		}),
	}
}
