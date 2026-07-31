package tools

import (
	"net/url"

	einotool "github.com/cloudwego/eino/components/tool"
)

func NewQueryWpcompanyPairTool(cfg ToolConfig) (einotool.InvokableTool, error) {
	return newGetTool[siteIDInput](
		cfg,
		"query_wp_company_pair",
		"查询作业单位列表，返回作业单位 id 和 name。",
		"/wp/queryWpcompanyPair",
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
