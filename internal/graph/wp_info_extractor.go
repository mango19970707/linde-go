package graph

import (
	"context"
	"encoding/json"
	"fmt"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/cloudwego/eino/components/model"
	"github.com/cloudwego/eino/schema"
)

const (
	wpInfoExtractionPrompt = `你是作业票信息提取器。请从用户输入中提取作业区域名称、开始时间、结束时间、作业描述、设备名称、作业单位名称。
只返回一个合法 JSON 对象，不要返回 Markdown、解释或额外文本。

要求：
1. 时间字段只填写作业时间，不填写解释。
2. 如果用户使用“今天、明天、后天”等相对时间，请先结合当前日期理解后再输出。
3. 如果无法确定某字段，返回空字符串或空数组。
4. 不要猜测区域、设备、公司的 ID。

JSON 结构必须是：
{
  "area_name": "",
  "start_time": "",
  "end_time": "",
  "description": "",
  "equipment_names": [],
  "company_name": ""
}`
	wpTimeOutputLayout = "2006/01/02-15:04:05"
)

var timeOnlyPattern = regexp.MustCompile(`(?i)^\s*(\d{1,2})(?:[:点时](\d{1,2}))?(?:[:分](\d{1,2}))?\s*(?:秒?(\d{1,2}))?\s*(?:点|时)?\s*$`)

type wpInfoExtractionJSON struct {
	AreaName       string   `json:"area_name"`
	StartTime      string   `json:"start_time"`
	EndTime        string   `json:"end_time"`
	Description    string   `json:"description"`
	EquipmentNames []string `json:"equipment_names"`
	CompanyName    string   `json:"company_name"`
}

type llmWpInfoExtractor struct {
	chatModel model.BaseChatModel
	now       func() time.Time
	location  *time.Location
}

func NewLLMWpInfoExtractor(chatModel model.BaseChatModel) (WpInfoExtractor, error) {
	location, err := time.LoadLocation("Asia/Shanghai")
	if err != nil {
		return nil, fmt.Errorf("load Asia/Shanghai location: %w", err)
	}
	return newLLMWpInfoExtractor(chatModel, time.Now().In(location))
}

func newLLMWpInfoExtractor(chatModel model.BaseChatModel, current time.Time) (WpInfoExtractor, error) {
	if chatModel == nil {
		return nil, fmt.Errorf("chat model is nil")
	}

	location := current.Location()
	return &llmWpInfoExtractor{
		chatModel: chatModel,
		now: func() time.Time {
			return current
		},
		location: location,
	}, nil
}

func (e *llmWpInfoExtractor) Extract(ctx context.Context, input string) (WpInfoExtraction, error) {
	input = strings.TrimSpace(input)
	if input == "" {
		return WpInfoExtraction{}, fmt.Errorf("user input is empty")
	}

	response, err := e.chatModel.Generate(ctx, []*schema.Message{
		schema.SystemMessage(e.prompt()),
		schema.UserMessage(input),
	})
	if err != nil {
		return WpInfoExtraction{}, fmt.Errorf("generate extraction response: %w", err)
	}
	if response == nil {
		return WpInfoExtraction{}, fmt.Errorf("extraction response is nil")
	}
	if strings.TrimSpace(response.Content) == "" {
		return WpInfoExtraction{}, fmt.Errorf("extraction response is empty")
	}

	var payload wpInfoExtractionJSON
	if err := json.Unmarshal([]byte(response.Content), &payload); err != nil {
		return WpInfoExtraction{}, fmt.Errorf("decode extraction response: %w", err)
	}

	startTime := normalizeWorkPermitTime(strings.TrimSpace(payload.StartTime), e.referenceNow(), nil)
	endTime := normalizeWorkPermitTime(strings.TrimSpace(payload.EndTime), e.referenceNow(), parseNormalizedDate(startTime, e.location))

	return WpInfoExtraction{
		AreaName:       strings.TrimSpace(payload.AreaName),
		StartTime:      startTime,
		EndTime:        endTime,
		Description:    strings.TrimSpace(payload.Description),
		EquipmentNames: payload.EquipmentNames,
		CompanyName:    strings.TrimSpace(payload.CompanyName),
	}, nil
}

func (e *llmWpInfoExtractor) prompt() string {
	return wpInfoExtractionPrompt + "\n当前时间：" + e.referenceNow().Format("2006-01-02 15:04:05")
}

func (e *llmWpInfoExtractor) referenceNow() time.Time {
	if e.now == nil {
		return time.Now()
	}
	now := e.now()
	if e.location != nil {
		return now.In(e.location)
	}
	return now
}

func normalizeWorkPermitTime(raw string, reference time.Time, fallbackDate *time.Time) string {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return ""
	}

	if parsed, ok := parseAbsoluteDateTime(raw, reference.Location()); ok {
		return parsed.Format(wpTimeOutputLayout)
	}

	if parsed, ok := parseRelativeDateTime(raw, reference, fallbackDate); ok {
		return parsed.Format(wpTimeOutputLayout)
	}

	return ""
}

func parseAbsoluteDateTime(raw string, location *time.Location) (time.Time, bool) {
	replacer := strings.NewReplacer("T", " ", "年", "/", "月", "/", "日", "", "时", ":", "点", ":", "分", ":", "秒", "", ".", "/", "：", ":")
	normalized := strings.TrimSpace(replacer.Replace(raw))
	normalized = strings.ReplaceAll(normalized, "--", "-")
	normalized = strings.ReplaceAll(normalized, " /", "/")

	layouts := []string{
		wpTimeOutputLayout,
		"2006/01/02 15:04:05",
		"2006/01/02 15:04",
		"2006-01-02 15:04:05",
		"2006-01-02 15:04",
		"2006/1/2 15:04:05",
		"2006/1/2 15:04",
		"2006-1-2 15:04:05",
		"2006-1-2 15:04",
	}
	for _, layout := range layouts {
		if parsed, err := time.ParseInLocation(layout, normalized, location); err == nil {
			return parsed, true
		}
	}
	return time.Time{}, false
}

func parseRelativeDateTime(raw string, reference time.Time, fallbackDate *time.Time) (time.Time, bool) {
	baseDate := time.Date(reference.Year(), reference.Month(), reference.Day(), 0, 0, 0, 0, reference.Location())

	switch {
	case strings.Contains(raw, "明天"), strings.Contains(raw, "明日"):
		baseDate = baseDate.AddDate(0, 0, 1)
	case strings.Contains(raw, "后天"):
		baseDate = baseDate.AddDate(0, 0, 2)
	case strings.Contains(raw, "今天"), strings.Contains(raw, "今日"):
	default:
		if fallbackDate != nil {
			baseDate = time.Date(fallbackDate.Year(), fallbackDate.Month(), fallbackDate.Day(), 0, 0, 0, 0, fallbackDate.Location())
		}
	}

	clockRaw := strings.NewReplacer("今天", "", "今日", "", "明天", "", "明日", "", "后天", "").Replace(raw)
	hour, minute, second, ok := parseClock(clockRaw)
	if !ok {
		return time.Time{}, false
	}
	return time.Date(baseDate.Year(), baseDate.Month(), baseDate.Day(), hour, minute, second, 0, baseDate.Location()), true
}

func parseClock(raw string) (int, int, int, bool) {
	matches := timeOnlyPattern.FindStringSubmatch(strings.TrimSpace(raw))
	if len(matches) == 0 {
		return 0, 0, 0, false
	}

	hour, err := strconv.Atoi(matches[1])
	if err != nil || hour < 0 || hour > 23 {
		return 0, 0, 0, false
	}

	minute := 0
	if matches[2] != "" {
		minute, err = strconv.Atoi(matches[2])
		if err != nil || minute < 0 || minute > 59 {
			return 0, 0, 0, false
		}
	}

	second := 0
	if matches[3] != "" {
		second, err = strconv.Atoi(matches[3])
		if err != nil || second < 0 || second > 59 {
			return 0, 0, 0, false
		}
	}

	if matches[4] != "" {
		second, err = strconv.Atoi(matches[4])
		if err != nil || second < 0 || second > 59 {
			return 0, 0, 0, false
		}
	}

	return hour, minute, second, true
}

func parseNormalizedDate(raw string, location *time.Location) *time.Time {
	if strings.TrimSpace(raw) == "" {
		return nil
	}
	parsed, err := time.ParseInLocation(wpTimeOutputLayout, raw, location)
	if err != nil {
		return nil
	}
	return &parsed
}
