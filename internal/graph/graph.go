package graph

import (
	"context"

	"github.com/cloudwego/eino/compose"
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

type WpTemplate struct {
	ID      int
	Name    string
	Content interface{}
}

type FilledWp struct {
	TemplateID int
	Name       string
	Content    interface{}
}

type WpGraphState struct {
	CurrentNode      string
	SiteID           int
	UserID           int
	Area             AreaInfo
	StartTime        string
	EndTime          string
	Description      string
	Equipments       []EquipmentInfo
	WpCompany        WpCompanyInfo
	IntentWpTypes    []string
	WpTemplates      []WpTemplate
	FilledWps        []FilledWp
	ValidationStatus string
	ErrorMsg         string
	RetryCount       int
}

// LeaveGraphState keeps compatibility with the existing type name in this package.
type LeaveGraphState = WpGraphState

func askWpInfoNode(ctx context.Context, state *WpGraphState) (*WpGraphState, error) {
	// TODO: Extract or ask for work area, start/end time, description, equipment, and company.
	state.CurrentNode = nodeAskWpInfo
	return state, nil
}

func validateWpInfoNode(ctx context.Context, state *WpGraphState) (*WpGraphState, error) {
	// TODO: Validate that required work permit base information is complete and normalized.
	state.CurrentNode = nodeValidateWpInfo
	state.ValidationStatus = validationValid
	return state, nil
}

func intentRecognitionNode(ctx context.Context, state *WpGraphState) (*WpGraphState, error) {
	// TODO: Recognize required work permit types from the work description.
	state.CurrentNode = nodeIntentRecognition
	return state, nil
}

func searchWpTemplateNode(ctx context.Context, state *WpGraphState) (*WpGraphState, error) {
	// TODO: Query matched work permit templates by recognized permit types and site context.
	state.CurrentNode = nodeSearchWpTemplate
	return state, nil
}

func fillWpsNode(ctx context.Context, state *WpGraphState) (*WpGraphState, error) {
	// TODO: Fill each selected work permit template, using parallel execution if needed.
	state.CurrentNode = nodeFillWps
	return state, nil
}

func validateFilledWpsNode(ctx context.Context, state *WpGraphState) (*WpGraphState, error) {
	// TODO: Validate generated work permits before submission.
	state.CurrentNode = nodeValidateFilledWps
	state.ValidationStatus = validationValid
	return state, nil
}

func submitWpsNode(ctx context.Context, state *WpGraphState) (*WpGraphState, error) {
	// TODO: Submit generated work permits to the downstream API.
	state.CurrentNode = nodeSubmitWps
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

// BuildWpGraph builds the intelligent work permit filling graph.
func BuildWpGraph() (compose.Runnable[*WpGraphState, *WpGraphState], error) {
	graph := compose.NewGraph[*WpGraphState, *WpGraphState]()

	graph.AddLambdaNode(nodeAskWpInfo, compose.InvokableLambda(askWpInfoNode))
	graph.AddLambdaNode(nodeValidateWpInfo, compose.InvokableLambda(validateWpInfoNode))
	graph.AddLambdaNode(nodeIntentRecognition, compose.InvokableLambda(intentRecognitionNode))
	graph.AddLambdaNode(nodeSearchWpTemplate, compose.InvokableLambda(searchWpTemplateNode))
	graph.AddLambdaNode(nodeFillWps, compose.InvokableLambda(fillWpsNode))
	graph.AddLambdaNode(nodeValidateFilledWps, compose.InvokableLambda(validateFilledWpsNode))
	graph.AddLambdaNode(nodeSubmitWps, compose.InvokableLambda(submitWpsNode))

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

	return graph.Compile(context.Background(), compose.WithMaxRunSteps(20))
}

// BuildLeaveGraph keeps compatibility with callers that still use the old leave graph name.
func BuildLeaveGraph() (compose.Runnable[*WpGraphState, *WpGraphState], error) {
	return BuildWpGraph()
}

/* 智能填充作业票的 Graph 结构

```
START
  -> askWpInfo (提取作业区域、起止时间、作业描述)
  -> validateWpInfo (验证基础信息是否完整)
  |-- valid -> intentRecognition
  `-- invalid -> askWpInfo (重新询问)
  -> intentRecognition (根据作业描述，判断需要哪些作业票)
  -> searchWpTemplate (查询作业票模板)
  -> fillWps (填充作业票，可并行实现)
  -> validateFilledWps (验证填充后的作业票信息是否完整)
  |-- valid -> submitWps
  `-- invalid -> fillWps (重新填充)
  -> submitWps (提交)
  -> END
```

*/
