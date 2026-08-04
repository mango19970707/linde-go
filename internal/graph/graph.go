package graph

import (
	"context"
	"fmt"
	"log/slog"
	"strings"
	"time"

	"github.com/cloudwego/eino/callbacks"
	einotool "github.com/cloudwego/eino/components/tool"
	"github.com/cloudwego/eino/compose"
	"github.com/cloudwego/eino/schema"
)

const (
	nodeAskWpInfo         = "askWpInfo"
	nodeValidateWpInfo    = "validateWpInfo"
	nodeIntentRecognition = "intentRecognition"
	nodeSearchWpTemplate  = "searchWpTemplate"
	nodeFillWps           = "fillWps"
	nodeValidateFilledWps = "validateFilledWps"
	nodeSubmitWps         = "submitWps"

	validationValid   = "valid"
	validationInvalid = "invalid"
)

type AreaInfo struct {
	ID   int
	Name string
}

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
	TemplateID     int
	Name           string
	Content        interface{}
	FilledFields   []FilledField
	UnfilledFields []UnfilledField
}

type FilledField struct {
	Name   string
	Value  any
	Source string
}

type UnfilledField struct {
	Name string
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

type AreaResolver interface {
	Resolve(ctx context.Context, siteID int, userInput, extractedName string) (AreaInfo, error)
}

type WpTypeRecognizer interface {
	Recognize(ctx context.Context, description string, templates []WpMetaInfo) ([]int, error)
}

type StructuredMemoryEntry struct {
	WpTypeID   int
	AreaID     int
	FieldName  string
	FieldValue any
	Source     string
	UpdatedAt  int64
}

type StructuredMemoryStore interface {
	Lookup(ctx context.Context, wpTypeID, areaID int) ([]StructuredMemoryEntry, error)
}

type SubmitWpsApproval struct {
	Approved bool             `json:"approved"`
	Payloads []map[string]any `json:"payloads"`
}

type WpInfoSupplement struct {
	Input string `json:"input"`
}

type WpGraphState struct {
	SessionID        string
	CurrentNode      string
	UserInput        string
	SiteID           int
	UserID           int
	Area             AreaInfo
	StartTime        string
	EndTime          string
	Description      string
	Equipments       []EquipmentInfo
	WpCompany        WpCompanyInfo
	IntentWpTypes    []WpMetaInfo
	WpTemplates      []WpTemplate
	FilledWps        []FilledWp
	ValidationStatus string
	ErrorMsg         string
	RetryCount       int
}

func init() {
	schema.RegisterName[*WpGraphState]("work_permit_graph_state")
}

func newAskWpInfoNode(extractor WpInfoExtractor, areaResolver AreaResolver) func(context.Context, *WpGraphState) (*WpGraphState, error) {
	return func(ctx context.Context, state *WpGraphState) (*WpGraphState, error) {
		if extractor == nil {
			return nil, fmt.Errorf("wp info extractor is nil")
		}
		if areaResolver == nil {
			return nil, fmt.Errorf("area resolver is nil")
		}

		state.CurrentNode = nodeAskWpInfo
		if strings.TrimSpace(state.UserInput) == "" {
			return state, nil
		}

		extracted, err := extractor.Extract(ctx, state.UserInput)
		if err != nil {
			return nil, fmt.Errorf("extract work permit info: %w", err)
		}

		if value := strings.TrimSpace(extracted.AreaName); value != "" {
			area, err := areaResolver.Resolve(ctx, state.SiteID, state.UserInput, value)
			if err != nil {
				return nil, fmt.Errorf("resolve area %q: %w", value, err)
			}
			if area.ID != 0 || strings.TrimSpace(area.Name) != "" {
				state.Area = area
			}
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
		if equipments := mapEquipmentNames(extracted.EquipmentNames); len(equipments) > 0 {
			state.Equipments = equipments
		}
		if value := strings.TrimSpace(extracted.CompanyName); value != "" {
			state.WpCompany.Name = value
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
		state.UserInput = strings.TrimSpace(supplement.Input)
		state.ValidationStatus = validationInvalid
		state.ErrorMsg = ""
		return state, nil
	}

	missingFields := make([]string, 0, 6)
	if strings.TrimSpace(state.Area.Name) == "" {
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

func newIntentRecognitionNode(templateTool einotool.InvokableTool, recognizer WpTypeRecognizer) func(context.Context, *WpGraphState) (*WpGraphState, error) {
	return func(ctx context.Context, state *WpGraphState) (*WpGraphState, error) {
		if templateTool == nil {
			return nil, fmt.Errorf("work permit template tool is nil")
		}
		if recognizer == nil {
			return nil, fmt.Errorf("work permit type recognizer is nil")
		}

		raw, err := templateTool.InvokableRun(ctx, fmt.Sprintf(`{"siteid":%d}`, state.SiteID))
		if err != nil {
			return nil, fmt.Errorf("query work permit templates: %w", err)
		}

		templates, err := decodeWpMetaInfoResponse(raw)
		if err != nil {
			return nil, fmt.Errorf("decode work permit templates: %w", err)
		}

		selectedIDs, err := recognizer.Recognize(ctx, state.Description, templates)
		if err != nil {
			return nil, fmt.Errorf("recognize work permit types: %w", err)
		}

		state.IntentWpTypes, err = selectWpMetaInfos(templates, selectedIDs)
		if err != nil {
			return nil, err
		}
		state.CurrentNode = nodeIntentRecognition
		return state, nil
	}
}

func validateFilledWpsNode(ctx context.Context, state *WpGraphState) (*WpGraphState, error) {
	// TODO: Validate generated work permits before submission.
	state.CurrentNode = nodeValidateFilledWps
	state.ValidationStatus = validationValid
	return state, nil
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

func mapEquipmentNames(names []string) []EquipmentInfo {
	equipments := make([]EquipmentInfo, 0, len(names))
	for _, name := range names {
		name = strings.TrimSpace(name)
		if name == "" {
			continue
		}
		equipments = append(equipments, EquipmentInfo{Name: name})
	}
	return equipments
}

// BuildWpGraph builds the intelligent work permit filling graph.
func BuildWpGraph(extractor WpInfoExtractor, areaResolver AreaResolver, templateTool einotool.InvokableTool, recognizer WpTypeRecognizer, templateDetailTool einotool.InvokableTool, memoryStore StructuredMemoryStore, updateWpInfoTool einotool.InvokableTool, linkWpInfoTool einotool.InvokableTool, graphOptions ...compose.GraphCompileOption) (compose.Runnable[*WpGraphState, *WpGraphState], error) {
	if extractor == nil {
		return nil, fmt.Errorf("wp info extractor is nil")
	}
	if areaResolver == nil {
		return nil, fmt.Errorf("area resolver is nil")
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

	graph.AddLambdaNode(nodeAskWpInfo, compose.InvokableLambda(newAskWpInfoNode(extractor, areaResolver)))
	graph.AddLambdaNode(nodeValidateWpInfo, compose.InvokableLambda(validateWpInfoNode))
	graph.AddLambdaNode(nodeIntentRecognition, compose.InvokableLambda(newIntentRecognitionNode(templateTool, recognizer)))
	graph.AddLambdaNode(nodeSearchWpTemplate, compose.InvokableLambda(newSearchWpTemplateNode(templateDetailTool)))
	graph.AddLambdaNode(nodeFillWps, compose.InvokableLambda(newFillWpsNode(memoryStore)))
	graph.AddLambdaNode(nodeValidateFilledWps, compose.InvokableLambda(validateFilledWpsNode))
	graph.AddLambdaNode(nodeSubmitWps, compose.InvokableLambda(newSubmitWpsNode(updateWpInfoTool, linkWpInfoTool)))

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
		handler: newWpGraphTraceHandler(),
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

func newWpGraphTraceHandler() callbacks.Handler {
	return callbacks.NewHandlerBuilder().
		OnStartFn(func(ctx context.Context, info *callbacks.RunInfo, input callbacks.CallbackInput) context.Context {
			meta := wpTraceMeta{
				traceID: extractTraceID(input),
				startAt: time.Now(),
			}
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
