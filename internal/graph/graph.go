package graph

import (
	"context"
	"fmt"
	"strings"

	einotool "github.com/cloudwego/eino/components/tool"
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

type WpGraphState struct {
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

func newAskWpInfoNode(extractor WpInfoExtractor) func(context.Context, *WpGraphState) (*WpGraphState, error) {
	return func(ctx context.Context, state *WpGraphState) (*WpGraphState, error) {
		if extractor == nil {
			return nil, fmt.Errorf("wp info extractor is nil")
		}

		state.CurrentNode = nodeAskWpInfo
		if strings.TrimSpace(state.UserInput) == "" {
			return state, nil
		}

		extracted, err := extractor.Extract(ctx, state.UserInput)
		if err != nil {
			return nil, fmt.Errorf("extract work permit info: %w", err)
		}

		state.Area.Name = strings.TrimSpace(extracted.AreaName)
		state.StartTime = strings.TrimSpace(extracted.StartTime)
		state.EndTime = strings.TrimSpace(extracted.EndTime)
		state.Description = strings.TrimSpace(extracted.Description)
		state.Equipments = mapEquipmentNames(extracted.EquipmentNames)
		state.WpCompany.Name = strings.TrimSpace(extracted.CompanyName)
		return state, nil
	}
}

func validateWpInfoNode(ctx context.Context, state *WpGraphState) (*WpGraphState, error) {
	state.CurrentNode = nodeValidateWpInfo
	missingFields := make([]string, 0, 6)
	if strings.TrimSpace(state.Area.Name) == "" {
		missingFields = append(missingFields, "area")
	}
	if strings.TrimSpace(state.StartTime) == "" {
		missingFields = append(missingFields, "start time")
	}
	if strings.TrimSpace(state.EndTime) == "" {
		missingFields = append(missingFields, "end time")
	}
	if strings.TrimSpace(state.Description) == "" {
		missingFields = append(missingFields, "description")
	}
	if len(missingFields) > 0 {
		state.ValidationStatus = validationInvalid
		state.ErrorMsg = fmt.Sprintf("missing required work permit info: %s", strings.Join(missingFields, ", "))
		return state, nil
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
func BuildWpGraph(extractor WpInfoExtractor, templateTool einotool.InvokableTool, recognizer WpTypeRecognizer, templateDetailTool einotool.InvokableTool, memoryStore StructuredMemoryStore, updateWpInfoTool einotool.InvokableTool, linkWpInfoTool einotool.InvokableTool) (compose.Runnable[*WpGraphState, *WpGraphState], error) {
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

	return graph.Compile(context.Background(), compose.WithMaxRunSteps(20))
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
  -> fillWps (填充作业票，可并行填充)
  -> validateFilledWps (验证填充后的作业票信息是否完整)
  |-- valid -> submitWps
  `-- invalid -> fillWps (重新填充)
  -> submitWps (提交)
  -> END
```

*/
