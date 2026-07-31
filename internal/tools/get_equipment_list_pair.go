package tools

import (
	"net/url"

	einotool "github.com/cloudwego/eino/components/tool"
)

func NewGetEquipmentListPairTool(cfg ToolConfig) (einotool.InvokableTool, error) {
	return newGetTool[areaEquipmentInput](
		cfg,
		"get_equipment_list_pair",
		"根据工厂和区域查询设备列表，返回设备 id 和 name。",
		"/getEquipmentListPair",
		func(c *client, input areaEquipmentInput) (url.Values, error) {
			siteID, err := c.resolveSiteID(input.SiteID)
			if err != nil {
				return nil, err
			}

			query := url.Values{}
			intQuery("siteid", siteID, query)
			intQuery("areaid", input.AreaID, query)
			return query, nil
		},
	)
}
