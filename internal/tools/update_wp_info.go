package tools

import (
	"context"

	einotool "github.com/cloudwego/eino/components/tool"
	"github.com/cloudwego/eino/components/tool/utils"
)

func NewUpdateWpInfoTool(cfg ToolConfig) (einotool.InvokableTool, error) {
	c, err := newClient(cfg)
	if err != nil {
		return nil, err
	}

	return utils.NewTool[updateWpInfoInput, basicResponse](
		newUpdateWpInfoToolInfo(),
		func(ctx context.Context, input updateWpInfoInput) (basicResponse, error) {
			return c.post(ctx, "/wp/updateWpInfo", input.Payload)
		},
	), nil
}
