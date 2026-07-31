package tools

import (
	"context"

	einotool "github.com/cloudwego/eino/components/tool"
	"github.com/cloudwego/eino/components/tool/utils"
)

func NewLinkexistWpinfoTool(cfg ToolConfig) (einotool.InvokableTool, error) {
	c, err := newClient(cfg)
	if err != nil {
		return nil, err
	}

	return utils.InferTool[linkExistInput, basicResponse](
		"link_exist_wpinfo",
		"建立大票和小票的父子关联关系。fid 为父票 ID，id 为子票 ID。",
		func(ctx context.Context, input linkExistInput) (basicResponse, error) {
			body := map[string]any{
				"id":  input.ID,
				"fid": input.FID,
			}
			return c.post(ctx, "/wp/linkexistWpinfo", body)
		},
	)
}
