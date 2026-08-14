package graph

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"sort"
	"strings"
	"time"

	"github.com/cloudwego/eino/callbacks"
	"github.com/cloudwego/eino/components/model"
	einotool "github.com/cloudwego/eino/components/tool"
	"github.com/cloudwego/eino/compose"
	"github.com/cloudwego/eino/schema"
	"linde-go-v2/internal/sessionrecorder"
)

const (
	nodeAskWpInfo         = "askWpInfo"
	nodeValidateWpInfo    = "validateWpInfo"
	nodeIntentRecognition = "intentRecognition"
	nodeSearchWpTemplate  = "searchWpTemplate"
	nodeFillWps           = "fillWps"
	nodeValidateFilledWps = "validateFilledWps"
	nodeSubmitWps         = "submitWps"

	operationConfirmWpTypes = "confirm_wp_types"
	validationValid         = "valid"
	validationInvalid       = "invalid"
)

type EquipmentInfo struct {
	ID   int
	Name string
}

type WpCompanyInfo struct {
	ID   int
	Name string
}

type WpMetaInfo struct {
	Id           int    `json:"id"`
	TemplateName string `json:"templetname"`
	WpTypeName   string `json:"wptypename"`
	ProcessType  int    `json:"processtype"`
	WpTypeId     int    `json:"wptypeid"`
}

type WpTemplate struct {
	ID                   int
	WpTypeID             int
	WpTypeName           string
	Name                 string
	IsDefault            bool
	IDPrefix             string
	UpdatedBy            string
	UpdateTime           int64
	Version              int
	State                int
	SiteIDs              string
	HandwrittenSignature int
	ApplyContent         string
	ApplyContentObject   map[string]any
}

type FilledWp struct {
	WpType         string
	Name           string
	Content        interface{}
	FilledFields   []FilledField
	UnfilledFields []UnfilledField
}

type FilledField struct {
	Label  string
	Value  any
	Source string
}

type UnfilledField struct {
	Label string
}

type WpInfoExtraction struct {
	AreaName       string
	StartTime      string
	EndTime        string
	Description    string
	EquipmentNames []string
	CompanyName    string
}

type WpInfoExtractor interface {
	Extract(ctx context.Context, input string) (WpInfoExtraction, error)
}

type WpTypeRecognizer interface {
	Recognize(ctx context.Context, description string, templates []WpMetaInfo, candidates []WpMetaInfo) ([]int, error)
}

type StructuredMemoryEntry struct {
	WpTypeID         int    `json:"wp_type_id"`        // 作业票类型ID
	WpTypeName       string `json:"wp_type_name"`      // 作业票类型名称
	Area             string `json:"area"`              // 作业地点名称
	WorkDescription  string `json:"work_description"`  // 作业描述
	FieldDescription string `json:"field_description"` // 字段描述
	FieldName        string `json:"field_name"`        // 字段在模板中的json路径
	FieldValue       any    `json:"field_value"`       // 字段值
	Source           string `json:"source"`            // 来源文件
	UpdatedAt        int64  `json:"updated_at"`        // 更新时间
}

type StructuredMemoryStore interface {
	FindByTypeAndArea(ctx context.Context, wpTypeID int, area string) ([]StructuredMemoryEntry, error)
	ListAreasByType(ctx context.Context, wpTypeID int) ([]string, error)
	FindByType(ctx context.Context, wpTypeID int) ([]StructuredMemoryEntry, error)
	Upsert(ctx context.Context, entry StructuredMemoryEntry) error
	FindIntentMemoryBySite(ctx context.Context, siteID int) ([]StructuredIntentMemoryEntry, error)
	UpsertIntentMemory(ctx context.Context, entry StructuredIntentMemoryEntry) error
}

type StructuredIntentMemoryEntry struct {
	SiteID          int    `json:"site_id"`          // 站点ID
	Area            string `json:"area"`             // 作业区域
	WorkDescription string `json:"work_description"` // 作业描述
	WpTypeIDs       []int  `json:"wp_type_ids"`      // 对应作业票类型ID列表
	Source          string `json:"source"`           // 来源文件
	UpdatedAt       int64  `json:"updated_at"`       // 更新时间
}

type IntentMemoryRetrievalConfig struct {
	ScoreThreshold int
	TopN           int
}

type SubmitWpsApproval struct {
	Approved bool             `json:"approved"`
	Payloads []map[string]any `json:"payloads"`
}

type WpTypeConfirmation struct {
	Confirmed     bool         `json:"confirmed"`
	IntentWpTypes []WpMetaInfo `json:"intent_wp_types"`
}

type WpInfoSupplement struct {
	Input string `json:"input"`
}

type WpGraphState struct {
	SessionID             string
	CurrentNode           string
	UserInput             string
	InitialUserInput      string
	LatestSupplementInput string
	SiteID                int
	UserID                int
	Area                  string
	StartTime             string
	EndTime               string
	Description           string
	Equipments            []EquipmentInfo
	WpCompany             WpCompanyInfo
	IntentWpTypes         []WpMetaInfo
	WpTemplates           []WpTemplate
	FilledWps             []FilledWp
	ValidationStatus      string
	ErrorMsg              string
	RetryCount            int
}

type sessionRecorderOption struct {
	recorder *sessionrecorder.Recorder
}

func WithSessionRecorder(recorder *sessionrecorder.Recorder) any {
	return sessionRecorderOption{recorder: recorder}
}

func WrapChatModelWithSessionRecorder(inner model.BaseChatModel, recorder *sessionrecorder.Recorder) model.BaseChatModel {
	if inner == nil || recorder == nil {
		return inner
	}
	return &sessionRecordedChatModel{inner: inner, recorder: recorder}
}

func WrapToolWithSessionRecorder(inner einotool.InvokableTool, recorder *sessionrecorder.Recorder) einotool.InvokableTool {
	if inner == nil || recorder == nil {
		return inner
	}
	return &sessionRecordedTool{inner: inner, recorder: recorder}
}

func init() {
	schema.RegisterName[*WpGraphState]("work_permit_graph_state")
}

func newAskWpInfoNode(extractor WpInfoExtractor) func(context.Context, *WpGraphState) (*WpGraphState, error) {
	return func(ctx context.Context, state *WpGraphState) (*WpGraphState, error) {
		if extractor == nil {
			return nil, fmt.Errorf("wp info extractor is nil")
		}

		state.CurrentNode = nodeAskWpInfo
		extractionInput := buildWpExtractionInput(state)
		if extractionInput == "" {
			return state, nil
		}

		extracted, err := extractor.Extract(ctx, extractionInput)
		if err != nil {
			return nil, fmt.Errorf("extract work permit info: %w", err)
		}

		if value := strings.TrimSpace(extracted.AreaName); value != "" {
			state.Area = value
		}
		if value := strings.TrimSpace(extracted.StartTime); value != "" {
			state.StartTime = value
		}
		if value := strings.TrimSpace(extracted.EndTime); value != "" {
			state.EndTime = value
		}
		if value := strings.TrimSpace(extracted.Description); value != "" {
			state.Description = value
		}
		return state, nil
	}
}

func validateWpInfoNode(ctx context.Context, state *WpGraphState) (*WpGraphState, error) {
	if state == nil {
		if wasInterrupted, hasState, interruptedState := compose.GetInterruptState[*WpGraphState](ctx); wasInterrupted && hasState {
			state = interruptedState
		}
	}
	if state == nil {
		return nil, fmt.Errorf("work permit graph state is nil")
	}
	state.CurrentNode = nodeValidateWpInfo
	if isResume, hasData, supplement := compose.GetResumeContext[WpInfoSupplement](ctx); isResume {
		if !hasData || strings.TrimSpace(supplement.Input) == "" {
			return nil, fmt.Errorf("work permit info supplement is missing")
		}
		trimmedSupplement := strings.TrimSpace(supplement.Input)
		if strings.TrimSpace(state.InitialUserInput) == "" {
			state.InitialUserInput = strings.TrimSpace(state.UserInput)
		}
		state.UserInput = trimmedSupplement
		state.LatestSupplementInput = trimmedSupplement
		state.ValidationStatus = validationInvalid
		state.ErrorMsg = ""
		return state, nil
	}

	if strings.TrimSpace(state.InitialUserInput) == "" {
		state.InitialUserInput = strings.TrimSpace(state.UserInput)
	}
	if strings.TrimSpace(state.UserInput) != "" && strings.TrimSpace(state.UserInput) != strings.TrimSpace(state.InitialUserInput) {
		state.LatestSupplementInput = strings.TrimSpace(state.UserInput)
	}

	missingFields := make([]string, 0, 6)
	if strings.TrimSpace(state.Area) == "" {
		missingFields = append(missingFields, "area")
	}
	if strings.TrimSpace(state.StartTime) == "" {
		missingFields = append(missingFields, "start_time")
	}
	if strings.TrimSpace(state.EndTime) == "" {
		missingFields = append(missingFields, "end_time")
	}
	if strings.TrimSpace(state.Description) == "" {
		missingFields = append(missingFields, "description")
	}
	if len(missingFields) > 0 {
		state.ValidationStatus = validationInvalid
		state.ErrorMsg = fmt.Sprintf("missing required work permit info: %s", strings.Join(missingFields, ", "))
		return nil, compose.StatefulInterrupt(ctx, map[string]any{
			"operation":      "collect_wp_info",
			"missing_fields": missingFields,
			"question":       buildWpInfoQuestion(missingFields),
			"state":          state,
		}, state)
	}

	state.ValidationStatus = validationValid
	state.ErrorMsg = ""
	return state, nil
}

func buildWpExtractionInput(state *WpGraphState) string {
	if state == nil {
		return ""
	}

	initial := strings.TrimSpace(state.InitialUserInput)
	current := strings.TrimSpace(state.UserInput)
	latestSupplement := strings.TrimSpace(state.LatestSupplementInput)

	if initial == "" {
		initial = current
	}
	if latestSupplement == "" && current != "" && current != initial {
		latestSupplement = current
	}
	if initial == "" {
		return ""
	}
	if latestSupplement == "" || latestSupplement == initial {
		return initial
	}
	return initial + "\n补充信息：" + latestSupplement
}

func newIntentRecognitionNode(templateTool einotool.InvokableTool, memoryStore StructuredMemoryStore, recognizer WpTypeRecognizer, args ...any) func(context.Context, *WpGraphState) (*WpGraphState, error) {
	return func(ctx context.Context, state *WpGraphState) (*WpGraphState, error) {
		if templateTool == nil {
			return nil, fmt.Errorf("work permit template tool is nil")
		}
		if memoryStore == nil {
			return nil, fmt.Errorf("structured memory store is nil")
		}
		if recognizer == nil {
			return nil, fmt.Errorf("work permit type recognizer is nil")
		}
		retrievalConfig := IntentMemoryRetrievalConfig{ScoreThreshold: 10, TopN: 5}
		confirmWpTypes := true
		for _, arg := range args {
			switch value := arg.(type) {
			case IntentMemoryRetrievalConfig:
				if value.ScoreThreshold > 0 {
					retrievalConfig.ScoreThreshold = value.ScoreThreshold
				}
				if value.TopN > 0 {
					retrievalConfig.TopN = value.TopN
				}
			case bool:
				confirmWpTypes = value
			}
		}

		if state == nil {
			if wasInterrupted, hasState, interruptedState := compose.GetInterruptState[*WpGraphState](ctx); wasInterrupted && hasState {
				state = interruptedState
			}
		}
		if state == nil {
			return nil, fmt.Errorf("work permit graph state is nil")
		}

		raw, err := templateTool.InvokableRun(ctx, fmt.Sprintf(`{"siteid":%d}`, state.SiteID))
		if err != nil {
			return nil, fmt.Errorf("query work permit templates: %w", err)
		}

		templates, err := decodeWpMetaInfoResponse(raw)
		if err != nil {
			return nil, fmt.Errorf("decode work permit templates: %w", err)
		}

		intentCandidates, err := findIntentMemoryCandidates(ctx, memoryStore, state.SiteID, state.Area, state.Description, templates, retrievalConfig)
		if err != nil {
			return nil, fmt.Errorf("load intent memory candidates: %w", err)
		}

		if isResume, hasData, confirmationData := compose.GetResumeContext[WpTypeConfirmation](ctx); isResume {
			if !hasData {
				return nil, fmt.Errorf("work permit type confirmation is missing")
			}
			if !confirmationData.Confirmed {
				return nil, fmt.Errorf("work permit type confirmation is not approved")
			}
			state.IntentWpTypes, err = validateWpTypeConfirmation(templates, confirmationData.IntentWpTypes)
			if err != nil {
				return nil, err
			}
			state.CurrentNode = nodeIntentRecognition
			return state, nil
		}

		selectedIDs, err := recognizer.Recognize(ctx, state.Description, templates, intentCandidates)
		if err != nil {
			return nil, fmt.Errorf("recognize work permit types: %w", err)
		}

		state.IntentWpTypes, err = selectWpMetaInfos(templates, selectedIDs)
		if err != nil {
			return nil, err
		}
		state.CurrentNode = nodeIntentRecognition
		if confirmWpTypes {
			return nil, compose.StatefulInterrupt(ctx, map[string]any{
				"operation":       operationConfirmWpTypes,
				"question":        "请确认识别出的作业票类型，必要时可修改后提交",
				"intent_wp_types": state.IntentWpTypes,
				"state":           state,
			}, state)
		}
		return state, nil
	}
}

func validateWpTypeConfirmation(templates []WpMetaInfo, selected []WpMetaInfo) ([]WpMetaInfo, error) {
	if len(selected) == 0 {
		return nil, fmt.Errorf("intent_wp_types is required")
	}

	byTemplateID := make(map[int]WpMetaInfo, len(templates))
	for _, template := range templates {
		byTemplateID[template.Id] = template
	}

	result := make([]WpMetaInfo, 0, len(selected))
	seen := make(map[int]struct{}, len(selected))
	for _, item := range selected {
		canonical, ok := byTemplateID[item.Id]
		if !ok {
			return nil, fmt.Errorf("unknown work permit template id %d", item.Id)
		}
		if _, ok := seen[item.Id]; ok {
			continue
		}
		seen[item.Id] = struct{}{}
		result = append(result, canonical)
	}

	return result, nil
}

func validateFilledWpsNode(ctx context.Context, state *WpGraphState) (*WpGraphState, error) {
	// TODO: Validate generated work permits before submission.
	state.CurrentNode = nodeValidateFilledWps
	state.ValidationStatus = validationValid
	return state, nil
}

func findIntentMemoryCandidates(ctx context.Context, memoryStore StructuredMemoryStore, siteID int, area, description string, templates []WpMetaInfo, config IntentMemoryRetrievalConfig) ([]WpMetaInfo, error) {
	entries, err := memoryStore.FindIntentMemoryBySite(ctx, siteID)
	if err != nil {
		return nil, err
	}
	if len(entries) == 0 {
		return nil, nil
	}

	templateByTypeID := make(map[int]WpMetaInfo, len(templates))
	for _, template := range templates {
		templateByTypeID[template.WpTypeId] = template
	}

	type scoredCandidate struct {
		template WpMetaInfo
		score    int
	}
	seen := make(map[int]struct{})
	scored := make([]scoredCandidate, 0)
	for _, entry := range entries {
		matchScore := scoreIntentMemoryEntry(description, area, entry)
		if matchScore < config.ScoreThreshold {
			continue
		}
		for _, wpTypeID := range entry.WpTypeIDs {
			template, ok := templateByTypeID[wpTypeID]
			if !ok {
				continue
			}
			score := matchScore + 1
			if strings.TrimSpace(template.WpTypeName) != "" && strings.Contains(strings.ToLower(strings.TrimSpace(description)), strings.ToLower(strings.TrimSpace(template.WpTypeName))) {
				score += 2
			}
			if _, ok := seen[template.WpTypeId]; ok {
				continue
			}
			seen[template.WpTypeId] = struct{}{}
			scored = append(scored, scoredCandidate{template: template, score: score})
		}
	}
	if len(scored) == 0 {
		return nil, nil
	}
	sort.SliceStable(scored, func(i, j int) bool {
		if scored[i].score == scored[j].score {
			return scored[i].template.WpTypeId < scored[j].template.WpTypeId
		}
		return scored[i].score > scored[j].score
	})
	result := make([]WpMetaInfo, 0, len(scored))
	for _, item := range scored {
		if config.TopN > 0 && len(result) >= config.TopN {
			break
		}
		result = append(result, item.template)
	}
	return result, nil
}

func scoreIntentMemoryEntry(description, area string, entry StructuredIntentMemoryEntry) int {
	description = normalizeIntentText(description)
	area = normalizeIntentText(area)
	candidateDescription := normalizeIntentText(entry.WorkDescription)
	candidateArea := normalizeIntentText(entry.Area)

	score := textSimilarityScore(description, candidateDescription)
	if score == 0 {
		return 0
	}
	if area != "" && candidateArea != "" {
		if area == candidateArea || strings.Contains(area, candidateArea) || strings.Contains(candidateArea, area) {
			score += 3
		}
	}
	return score
}

func normalizeIntentText(value string) string {
	value = strings.ToLower(strings.TrimSpace(value))
	if value == "" {
		return ""
	}
	var builder strings.Builder
	builder.Grow(len(value))
	for _, r := range value {
		if strings.ContainsRune(" \t\r\n,，。./\\-_()[]{}<>:;：；、", r) {
			continue
		}
		builder.WriteRune(r)
	}
	return builder.String()
}

func textSimilarityScore(left, right string) int {
	if left == "" || right == "" {
		return 0
	}
	if left == right {
		return 100
	}
	if strings.Contains(left, right) || strings.Contains(right, left) {
		shorter := left
		if len([]rune(right)) < len([]rune(left)) {
			shorter = right
		}
		return 50 + len([]rune(shorter))
	}

	leftSet := make(map[rune]struct{})
	for _, r := range left {
		leftSet[r] = struct{}{}
	}
	score := 0
	seen := make(map[rune]struct{})
	for _, r := range right {
		if _, ok := leftSet[r]; ok {
			if _, dup := seen[r]; dup {
				continue
			}
			seen[r] = struct{}{}
			score++
		}
	}
	return score
}

func routeAfterWpInfoValidation(ctx context.Context, state *WpGraphState) (string, error) {
	if state.ValidationStatus == validationValid {
		return nodeIntentRecognition, nil
	}
	return nodeAskWpInfo, nil
}

func routeAfterFilledWpsValidation(ctx context.Context, state *WpGraphState) (string, error) {
	if state.ValidationStatus == validationValid {
		return nodeSubmitWps, nil
	}
	return nodeFillWps, nil
}

// BuildWpGraph builds the intelligent work permit filling graph.
func BuildWpGraph(extractor WpInfoExtractor, templateTool einotool.InvokableTool, recognizer WpTypeRecognizer, templateDetailTool einotool.InvokableTool, chatModel model.BaseChatModel, args ...any) (compose.Runnable[*WpGraphState, *WpGraphState], error) {
	promptStore := newDefaultPromptStore()
	var memoryStore StructuredMemoryStore
	var fillRules WpFillRules
	var updateWpInfoTool einotool.InvokableTool
	var linkWpInfoTool einotool.InvokableTool
	var recorder *sessionrecorder.Recorder
	retrievalConfig := IntentMemoryRetrievalConfig{ScoreThreshold: 10, TopN: 5}
	confirmWpTypes := true
	graphOptions := make([]compose.GraphCompileOption, 0)

	switch {
	case len(args) >= 4:
		store, ok := args[0].(StructuredMemoryStore)
		if !ok {
			// try the new signature below
		} else {
			rules, ok := args[1].(WpFillRules)
			if !ok {
				return nil, fmt.Errorf("fill rules has invalid type %T", args[1])
			}
			updateTool, ok := args[2].(einotool.InvokableTool)
			if !ok {
				return nil, fmt.Errorf("update work permit tool has invalid type %T", args[2])
			}
			linkTool, ok := args[3].(einotool.InvokableTool)
			if !ok {
				return nil, fmt.Errorf("link work permit tool has invalid type %T", args[3])
			}
			memoryStore = store
			fillRules = rules
			updateWpInfoTool = updateTool
			linkWpInfoTool = linkTool
			for _, arg := range args[4:] {
				if value, ok := arg.(IntentMemoryRetrievalConfig); ok {
					if value.ScoreThreshold > 0 {
						retrievalConfig.ScoreThreshold = value.ScoreThreshold
					}
					if value.TopN > 0 {
						retrievalConfig.TopN = value.TopN
					}
					continue
				}
				if value, ok := arg.(bool); ok {
					confirmWpTypes = value
					continue
				}
				if value, ok := arg.(sessionRecorderOption); ok {
					recorder = value.recorder
					continue
				}
				option, ok := arg.(compose.GraphCompileOption)
				if !ok {
					return nil, fmt.Errorf("graph option has invalid type %T", arg)
				}
				graphOptions = append(graphOptions, option)
			}
			goto parsed
		}
		fallthrough
	case len(args) >= 5:
		store, ok := args[0].(PromptStore)
		if !ok {
			return nil, fmt.Errorf("prompt store has invalid type %T", args[0])
		}
		memory, ok := args[1].(StructuredMemoryStore)
		if !ok {
			return nil, fmt.Errorf("structured memory store has invalid type %T", args[1])
		}
		rules, ok := args[2].(WpFillRules)
		if !ok {
			return nil, fmt.Errorf("fill rules has invalid type %T", args[2])
		}
		updateTool, ok := args[3].(einotool.InvokableTool)
		if !ok {
			return nil, fmt.Errorf("update work permit tool has invalid type %T", args[3])
		}
		linkTool, ok := args[4].(einotool.InvokableTool)
		if !ok {
			return nil, fmt.Errorf("link work permit tool has invalid type %T", args[4])
		}
		promptStore = store
		memoryStore = memory
		fillRules = rules
		updateWpInfoTool = updateTool
		linkWpInfoTool = linkTool
		for _, arg := range args[5:] {
			if value, ok := arg.(IntentMemoryRetrievalConfig); ok {
				if value.ScoreThreshold > 0 {
					retrievalConfig.ScoreThreshold = value.ScoreThreshold
				}
				if value.TopN > 0 {
					retrievalConfig.TopN = value.TopN
				}
				continue
			}
			if value, ok := arg.(bool); ok {
				confirmWpTypes = value
				continue
			}
			if value, ok := arg.(sessionRecorderOption); ok {
				recorder = value.recorder
				continue
			}
			option, ok := arg.(compose.GraphCompileOption)
			if !ok {
				return nil, fmt.Errorf("graph option has invalid type %T", arg)
			}
			graphOptions = append(graphOptions, option)
		}
	default:
		return nil, fmt.Errorf("unexpected graph argument count %d", len(args))
	}
parsed:

	if extractor == nil {
		return nil, fmt.Errorf("wp info extractor is nil")
	}
	if templateTool == nil {
		return nil, fmt.Errorf("work permit template tool is nil")
	}
	if recognizer == nil {
		return nil, fmt.Errorf("work permit type recognizer is nil")
	}
	if templateDetailTool == nil {
		return nil, fmt.Errorf("work permit template detail tool is nil")
	}
	if chatModel == nil {
		return nil, fmt.Errorf("chat model is nil")
	}
	if memoryStore == nil {
		return nil, fmt.Errorf("structured memory store is nil")
	}
	if updateWpInfoTool == nil {
		return nil, fmt.Errorf("update work permit tool is nil")
	}
	if linkWpInfoTool == nil {
		return nil, fmt.Errorf("link work permit tool is nil")
	}

	graph := compose.NewGraph[*WpGraphState, *WpGraphState]()

	graph.AddLambdaNode(nodeAskWpInfo, compose.InvokableLambda(newAskWpInfoNode(extractor)))
	graph.AddLambdaNode(nodeValidateWpInfo, compose.InvokableLambda(validateWpInfoNode))
	graph.AddLambdaNode(nodeIntentRecognition, compose.InvokableLambda(newIntentRecognitionNode(templateTool, memoryStore, recognizer, retrievalConfig, confirmWpTypes)))
	graph.AddLambdaNode(nodeSearchWpTemplate, compose.InvokableLambda(newSearchWpTemplateNode(templateDetailTool)))
	graph.AddLambdaNode(nodeFillWps, compose.InvokableLambda(newFillWpsNode(chatModel, promptStore, memoryStore, fillRules)))
	graph.AddLambdaNode(nodeValidateFilledWps, compose.InvokableLambda(validateFilledWpsNode))
	graph.AddLambdaNode(nodeSubmitWps, compose.InvokableLambda(newSubmitWpsNode(memoryStore, updateWpInfoTool, linkWpInfoTool)))

	graph.AddEdge(compose.START, nodeAskWpInfo)
	graph.AddEdge(nodeAskWpInfo, nodeValidateWpInfo)
	graph.AddBranch(nodeValidateWpInfo, compose.NewGraphBranch(
		routeAfterWpInfoValidation,
		map[string]bool{
			nodeAskWpInfo:         true,
			nodeIntentRecognition: true,
		},
	))
	graph.AddEdge(nodeIntentRecognition, nodeSearchWpTemplate)
	graph.AddEdge(nodeSearchWpTemplate, nodeFillWps)
	graph.AddEdge(nodeFillWps, nodeValidateFilledWps)
	graph.AddBranch(nodeValidateFilledWps, compose.NewGraphBranch(
		routeAfterFilledWpsValidation,
		map[string]bool{
			nodeFillWps:   true,
			nodeSubmitWps: true,
		},
	))
	graph.AddEdge(nodeSubmitWps, compose.END)

	graphOptions = append(graphOptions, compose.WithMaxRunSteps(20))
	runnable, err := graph.Compile(context.Background(), graphOptions...)
	if err != nil {
		return nil, err
	}

	return &traceableWpGraphRunnable{
		inner:   runnable,
		handler: newWpGraphTraceHandler(recorder),
	}, nil
}

/*
	智能填充作业票的 Graph 结构

```
START

	-> askWpInfo (提取作业区域、起止时间、作业描述)
	-> validateWpInfo (验证基础信息是否完整)
	|-- valid -> intentRecognition
	`-- invalid -> askWpInfo (重新询问)
	-> intentRecognition (根据作业描述，判断需要哪些作业票)
	-> searchWpTemplate (查询作业票模板)
	-> fillWps (填充作业票，可并行填充)
	-> validateFilledWps (验证填充后的作业票信息是否完整)
	|-- valid -> submitWps
	`-- invalid -> fillWps (重新填充)
	-> submitWps (提交)
	-> END

```
*/
func buildWpInfoQuestion(missingFields []string) string {
	labels := map[string]string{
		"area":        "作业区域",
		"start_time":  "作业开始时间",
		"end_time":    "作业结束时间",
		"description": "作业描述",
	}
	parts := make([]string, 0, len(missingFields))
	for _, field := range missingFields {
		if label, ok := labels[field]; ok {
			parts = append(parts, label)
		}
	}
	return fmt.Sprintf("请补充%s", strings.Join(parts, "、"))
}

type traceableWpGraphRunnable struct {
	inner   compose.Runnable[*WpGraphState, *WpGraphState]
	handler callbacks.Handler
}

type sessionRecordedChatModel struct {
	inner    model.BaseChatModel
	recorder *sessionrecorder.Recorder
}

func (m *sessionRecordedChatModel) Generate(ctx context.Context, input []*schema.Message, opts ...model.Option) (*schema.Message, error) {
	for _, message := range input {
		recordSchemaMessageFromRecorder(ctx, m.recorder, message)
	}

	output, err := m.inner.Generate(ctx, input, opts...)
	if err != nil {
		return nil, err
	}
	recordSchemaMessageFromRecorder(ctx, m.recorder, output)
	return output, nil
}

func (m *sessionRecordedChatModel) Stream(ctx context.Context, input []*schema.Message, opts ...model.Option) (*schema.StreamReader[*schema.Message], error) {
	for _, message := range input {
		recordSchemaMessageFromRecorder(ctx, m.recorder, message)
	}
	return m.inner.Stream(ctx, input, opts...)
}

type sessionRecordedTool struct {
	inner    einotool.InvokableTool
	recorder *sessionrecorder.Recorder
}

func (t *sessionRecordedTool) Info(ctx context.Context) (*schema.ToolInfo, error) {
	return t.inner.Info(ctx)
}

func (t *sessionRecordedTool) InvokableRun(ctx context.Context, argumentsInJSON string, opts ...einotool.Option) (string, error) {
	recordTranscriptEventFromRecorder(ctx, t.recorder, "tool_call", argumentsInJSON)

	output, err := t.inner.InvokableRun(ctx, argumentsInJSON, opts...)
	if err != nil {
		return "", err
	}
	recordTranscriptEventFromRecorder(ctx, t.recorder, "tool_output", output)
	return output, nil
}

func (r *traceableWpGraphRunnable) Invoke(ctx context.Context, input *WpGraphState, opts ...compose.Option) (*WpGraphState, error) {
	if r.handler != nil {
		opts = append(opts, compose.WithCallbacks(r.handler))
	}
	return r.inner.Invoke(ctx, input, opts...)
}

func (r *traceableWpGraphRunnable) Stream(ctx context.Context, input *WpGraphState, opts ...compose.Option) (*schema.StreamReader[*WpGraphState], error) {
	if r.handler != nil {
		opts = append(opts, compose.WithCallbacks(r.handler))
	}
	return r.inner.Stream(ctx, input, opts...)
}

func (r *traceableWpGraphRunnable) Collect(ctx context.Context, input *schema.StreamReader[*WpGraphState], opts ...compose.Option) (*WpGraphState, error) {
	if r.handler != nil {
		opts = append(opts, compose.WithCallbacks(r.handler))
	}
	return r.inner.Collect(ctx, input, opts...)
}

func (r *traceableWpGraphRunnable) Transform(ctx context.Context, input *schema.StreamReader[*WpGraphState], opts ...compose.Option) (*schema.StreamReader[*WpGraphState], error) {
	if r.handler != nil {
		opts = append(opts, compose.WithCallbacks(r.handler))
	}
	return r.inner.Transform(ctx, input, opts...)
}

type wpTraceKey struct{}

type wpTraceMeta struct {
	traceID string
	startAt time.Time
}

func newWpGraphTraceHandler(recorder *sessionrecorder.Recorder) callbacks.Handler {
	return callbacks.NewHandlerBuilder().
		OnStartFn(func(ctx context.Context, info *callbacks.RunInfo, input callbacks.CallbackInput) context.Context {
			meta := wpTraceMeta{
				traceID: extractTraceID(input),
				startAt: time.Now(),
			}
			recordGraphCallbackInput(ctx, recorder, input)
			if info != nil && info.Name != "" {
				slog.Info("wp_graph_start",
					slog.String("trace", meta.traceID),
					slog.String("node", info.Name),
				)
			}
			return context.WithValue(ctx, wpTraceKey{}, meta)
		}).
		OnEndFn(func(ctx context.Context, info *callbacks.RunInfo, output callbacks.CallbackOutput) context.Context {
			meta, _ := ctx.Value(wpTraceKey{}).(wpTraceMeta)
			recordGraphCallbackOutput(ctx, recorder, output)
			if info != nil && info.Name != "" {
				slog.Info("wp_graph_end",
					slog.String("trace", fallbackTraceID(meta.traceID)),
					slog.String("node", info.Name),
					slog.Int64("ms", time.Since(meta.startAt).Milliseconds()),
				)
			}
			return ctx
		}).
		OnErrorFn(func(ctx context.Context, info *callbacks.RunInfo, err error) context.Context {
			meta, _ := ctx.Value(wpTraceKey{}).(wpTraceMeta)
			nodeName := ""
			if info != nil {
				nodeName = info.Name
			}
			slog.Error("wp_graph_error",
				slog.String("trace", fallbackTraceID(meta.traceID)),
				slog.String("node", nodeName),
				slog.String("err", compactTraceError(err)),
			)
			return ctx
		}).
		Build()
}

func extractTraceID(input callbacks.CallbackInput) string {
	state, ok := input.(*WpGraphState)
	if !ok || state == nil || strings.TrimSpace(state.SessionID) == "" {
		return "unknown"
	}
	return strings.TrimSpace(state.SessionID)
}

func fallbackTraceID(traceID string) string {
	if strings.TrimSpace(traceID) == "" {
		return "unknown"
	}
	return traceID
}

func compactTraceError(err error) string {
	if err == nil {
		return ""
	}
	return strings.Join(strings.Fields(err.Error()), " ")
}

func recordGraphCallbackInput(ctx context.Context, recorder *sessionrecorder.Recorder, input callbacks.CallbackInput) {
	if recorder == nil {
		return
	}

	sessionID, ok := sessionrecorder.SessionIDFromContext(ctx)
	if !ok {
		return
	}

	if modelInput := model.ConvCallbackInput(input); modelInput != nil {
		for _, message := range modelInput.Messages {
			recordSchemaMessage(sessionID, recorder, message)
		}
		return
	}

	if toolInput := einotool.ConvCallbackInput(input); toolInput != nil {
		recordTranscriptEvent(sessionID, recorder, "tool_call", toolInput.ArgumentsInJSON)
	}
}

func recordGraphCallbackOutput(ctx context.Context, recorder *sessionrecorder.Recorder, output callbacks.CallbackOutput) {
	if recorder == nil {
		return
	}

	sessionID, ok := sessionrecorder.SessionIDFromContext(ctx)
	if !ok {
		return
	}

	if modelOutput := model.ConvCallbackOutput(output); modelOutput != nil && modelOutput.Message != nil {
		recordSchemaMessage(sessionID, recorder, modelOutput.Message)
		return
	}

	if toolOutput := einotool.ConvCallbackOutput(output); toolOutput != nil {
		if toolOutput.Response != "" {
			recordTranscriptEvent(sessionID, recorder, "tool_output", toolOutput.Response)
			return
		}
		if toolOutput.ToolOutput != nil {
			raw, err := json.Marshal(toolOutput.ToolOutput)
			if err != nil {
				recordTranscriptEvent(sessionID, recorder, "tool_output", fmt.Sprintf("%v", toolOutput.ToolOutput))
				return
			}
			recordTranscriptEvent(sessionID, recorder, "tool_output", string(raw))
		}
	}
}

func recordSchemaMessageFromRecorder(ctx context.Context, recorder *sessionrecorder.Recorder, message *schema.Message) {
	if recorder == nil {
		return
	}
	sessionID, ok := sessionrecorder.SessionIDFromContext(ctx)
	if !ok {
		return
	}
	recordSchemaMessage(sessionID, recorder, message)
}

func recordSchemaMessage(sessionID string, recorder *sessionrecorder.Recorder, message *schema.Message) {
	if message == nil {
		return
	}

	switch message.Role {
	case schema.System:
		recordTranscriptEvent(sessionID, recorder, "system_message", message.Content)
	case schema.User:
		recordTranscriptEvent(sessionID, recorder, "user_message", message.Content)
	case schema.Assistant:
		recordTranscriptEvent(sessionID, recorder, "assistant_message", message.Content)
		for _, toolCall := range message.ToolCalls {
			raw, err := json.Marshal(toolCall)
			if err != nil {
				recordTranscriptEvent(sessionID, recorder, "tool_call", fmt.Sprintf("%+v", toolCall))
				continue
			}
			recordTranscriptEvent(sessionID, recorder, "tool_call", string(raw))
		}
	case schema.Tool:
		recordTranscriptEvent(sessionID, recorder, "tool_output", message.Content)
	}
}

func recordTranscriptEventFromRecorder(ctx context.Context, recorder *sessionrecorder.Recorder, role, content string) {
	if recorder == nil {
		return
	}
	sessionID, ok := sessionrecorder.SessionIDFromContext(ctx)
	if !ok {
		return
	}
	recordTranscriptEvent(sessionID, recorder, role, content)
}

func recordTranscriptEvent(sessionID string, recorder *sessionrecorder.Recorder, role, content string) {
	if recorder == nil || strings.TrimSpace(sessionID) == "" {
		return
	}

	ctx := sessionrecorder.WithSessionID(context.Background(), sessionID)
	if err := recorder.AppendEvent(ctx, sessionrecorder.Event{
		Role:    role,
		Content: content,
	}); err != nil {
		slog.Error("wp_graph_record_event_failed",
			slog.String("session_id", sessionID),
			slog.String("role", role),
			slog.String("err", err.Error()),
		)
	}
}
