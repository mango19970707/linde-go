package tools

import (
	"net/url"

	einotool "github.com/cloudwego/eino/components/tool"
)

func NewQueryWptempletsNoContentTool(cfg ToolConfig) (einotool.InvokableTool, error) {
	return newGetTool[siteIDInput](
		cfg,
		"query_wp_templets_no_content",
		"查询作业票模板列表，返回模板 id、wptypeid 和 templetname。",
		"/wp/queryWptempletsNoContent",
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
