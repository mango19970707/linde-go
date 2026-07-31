package tools

import (
	"net/url"

	einotool "github.com/cloudwego/eino/components/tool"
)

func NewGetAreaListPairTool(cfg ToolConfig) (einotool.InvokableTool, error) {
	return newGetTool[siteIDInput](
		cfg,
		"get_area_list_pair",
		"查询区域列表。先于设备查询调用，返回区域 id 和 name。",
		"/getAreaListPair",
		func(c *client, input siteIDInput) (url.Values, error) {
			siteID, err := c.resolveSiteID(input.SiteID)
			if err != nil {
				return nil, err
			}

			query := url.Values{}
			intQuery("siteid", siteID, query)
			return query, nil
		},
	)
}
