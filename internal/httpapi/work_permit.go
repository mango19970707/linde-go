package httpapi

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"sync"

	"github.com/cloudwego/eino/compose"
	"github.com/gin-gonic/gin"
	"linde-go-v2/internal/graph"
)

type WorkPermitRunner interface {
	Invoke(ctx context.Context, input *graph.WpGraphState, opts ...compose.Option) (*graph.WpGraphState, error)
}

type workPermitRequest struct {
	SessionID     string             `json:"session_id"`
	Input         string             `json:"input"`
	SiteID        int                `json:"site_id"`
	UserID        int                `json:"user_id"`
	Approved      *bool              `json:"approved,omitempty"`
	Confirmed     *bool              `json:"confirmed,omitempty"`
	IntentWpTypes []graph.WpMetaInfo `json:"intent_wp_types,omitempty"`
	Payloads      []map[string]any   `json:"payloads,omitempty"`
}

type structuredMemoryImportRequest struct {
	Source   string           `json:"source"`
	Payloads []map[string]any `json:"payloads"`
}

type workPermitResponse struct {
	SessionID     string             `json:"session_id"`
	Status        string             `json:"status"`
	MissingFields []string           `json:"missing_fields,omitempty"`
	Question      string             `json:"question,omitempty"`
	IntentWpTypes []graph.WpMetaInfo `json:"intent_wp_types,omitempty"`
	FilledWps     []graph.FilledWp   `json:"filled_wps,omitempty"`
	Message       string             `json:"message,omitempty"`
}

type sessionState struct {
	CheckpointID string
	InterruptID  string
	Operation    string
}

type WorkPermitServer struct {
	runner      WorkPermitRunner
	promptStore graph.PromptStore
	memoryStore graph.StructuredMemoryStore

	mu       sync.Mutex
	sessions map[string]sessionState
}

func NewWorkPermitServer(runner WorkPermitRunner, args ...any) *WorkPermitServer {
	var promptStore graph.PromptStore
	var memoryStore graph.StructuredMemoryStore

	switch len(args) {
	case 1:
		promptStore, _ = args[0].(graph.PromptStore)
	case 2:
		promptStore, _ = args[0].(graph.PromptStore)
		memoryStore, _ = args[1].(graph.StructuredMemoryStore)
	case 3:
		promptStore, _ = args[0].(graph.PromptStore)
		memoryStore, _ = args[1].(graph.StructuredMemoryStore)
	}

	return &WorkPermitServer{
		runner:      runner,
		promptStore: promptStore,
		memoryStore: memoryStore,
		sessions:    make(map[string]sessionState),
	}
}

func (s *WorkPermitServer) Router() http.Handler {
	router := gin.New()
	router.Use(gin.Recovery())
	router.POST("/api/work-permits", s.handleCreateWorkPermit)
	router.POST("/api/structured-memory/import", s.handleImportStructuredMemory)
	router.GET("/api/structured-memory/export", s.handleExportStructuredMemory)
	router.GET("/api/prompts", s.handleListPrompts)
	router.GET("/api/prompts/:key", s.handleGetPrompt)
	router.PUT("/api/prompts/:key", s.handleUpdatePrompt)
	router.POST("/api/prompts/:key/reset", s.handleResetPrompt)
	return router
}

func (s *WorkPermitServer) handleCreateWorkPermit(c *gin.Context) {
	if s.runner == nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "work permit graph is nil"})
		return
	}

	var request workPermitRequest
	if err := c.ShouldBindJSON(&request); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid request body"})
		return
	}

	request.Input = strings.TrimSpace(request.Input)
	if request.SiteID <= 0 || request.UserID <= 0 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "site_id and user_id are required"})
		return
	}
	if strings.TrimSpace(request.SessionID) == "" {
		s.handleInitialRequest(c, request)
		return
	}
	s.handleResumeRequest(c, request)
}

func (s *WorkPermitServer) handleImportStructuredMemory(c *gin.Context) {
	if s.memoryStore == nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "structured memory store is nil"})
		return
	}

	var request structuredMemoryImportRequest
	if err := c.ShouldBindJSON(&request); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid request body"})
		return
	}
	if len(request.Payloads) == 0 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "payloads are required"})
		return
	}

	importedRows, err := graph.ImportStructuredMemoryPayloads(c.Request.Context(), s.memoryStore, request.Payloads, strings.TrimSpace(request.Source))
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"status":        "completed",
		"imported_wps":  len(request.Payloads),
		"imported_rows": importedRows,
	})
}

func (s *WorkPermitServer) handleExportStructuredMemory(c *gin.Context) {
	if s.memoryStore == nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "structured memory store is nil"})
		return
	}

	wpTypeIDValue := strings.TrimSpace(c.Query("wp_type_id"))
	area := strings.TrimSpace(c.Query("area"))
	if wpTypeIDValue == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "wp_type_id is required"})
		return
	}
	if area == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "area is required"})
		return
	}

	wpTypeID, err := strconv.Atoi(wpTypeIDValue)
	if err != nil || wpTypeID <= 0 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "wp_type_id must be a positive integer"})
		return
	}

	payloads, err := graph.ExportStructuredMemoryPayloads(c.Request.Context(), s.memoryStore, wpTypeID, area)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"status":   "completed",
		"payloads": payloads,
	})
}

func (s *WorkPermitServer) handleInitialRequest(c *gin.Context, request workPermitRequest) {
	if request.Input == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "input is required"})
		return
	}

	sessionID, err := newID()
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	checkpointID, err := newID()
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	state := &graph.WpGraphState{
		SessionID: sessionID,
		UserInput: request.Input,
		SiteID:    request.SiteID,
		UserID:    request.UserID,
	}
	output, err := s.runner.Invoke(c.Request.Context(), state, compose.WithCheckPointID(checkpointID))
	s.writeRunResult(c, sessionID, checkpointID, output, err)
}

func (s *WorkPermitServer) handleResumeRequest(c *gin.Context, request workPermitRequest) {
	session, ok := s.getSession(request.SessionID)
	if !ok {
		c.JSON(http.StatusNotFound, gin.H{"error": "session not found"})
		return
	}
	if session.InterruptID == "" {
		c.JSON(http.StatusConflict, gin.H{"error": "session is not waiting for input"})
		return
	}

	var resumeData any
	switch session.Operation {
	case "collect_wp_info":
		if request.Input == "" {
			c.JSON(http.StatusBadRequest, gin.H{"error": "input is required"})
			return
		}
		resumeData = graph.WpInfoSupplement{Input: request.Input}
	case "submit_work_permits":
		if request.Approved == nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": "approved is required"})
			return
		}
		resumeData = graph.SubmitWpsApproval{
			Approved: *request.Approved,
			Payloads: request.Payloads,
		}
	case "confirm_wp_types":
		if request.Confirmed == nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": "confirmed is required"})
			return
		}
		resumeData = graph.WpTypeConfirmation{
			Confirmed:     *request.Confirmed,
			IntentWpTypes: request.IntentWpTypes,
		}
	default:
		c.JSON(http.StatusConflict, gin.H{"error": "unsupported session operation"})
		return
	}

	resumeContext := compose.ResumeWithData(c.Request.Context(), session.InterruptID, resumeData)
	state := &graph.WpGraphState{
		SessionID: request.SessionID,
		UserInput: request.Input,
		SiteID:    request.SiteID,
		UserID:    request.UserID,
	}
	output, err := s.runner.Invoke(resumeContext, state, compose.WithCheckPointID(session.CheckpointID))
	s.writeRunResult(c, request.SessionID, session.CheckpointID, output, err)
}

func (s *WorkPermitServer) writeRunResult(c *gin.Context, sessionID, checkpointID string, output *graph.WpGraphState, runErr error) {
	if runErr != nil {
		if info, ok := compose.ExtractInterruptInfo(runErr); ok && len(info.InterruptContexts) > 0 {
			interrupt := info.InterruptContexts[0]
			operation, missingFields, question, filledWps, intentWpTypes := interruptDetails(interrupt.Info)
			s.setSession(sessionID, sessionState{
				CheckpointID: checkpointID,
				InterruptID:  interrupt.ID,
				Operation:    operation,
			})

			response := workPermitResponse{
				SessionID:     sessionID,
				MissingFields: missingFields,
				Question:      question,
				IntentWpTypes: intentWpTypes,
				FilledWps:     filledWps,
			}
			if operation == "collect_wp_info" {
				response.Status = "need_more_info"
			} else if operation == "confirm_wp_types" {
				response.Status = "need_wp_type_confirmation"
			} else if operation == "submit_work_permits" {
				response.Status = "need_approval"
			} else {
				response.Status = "interrupted"
			}
			c.JSON(http.StatusOK, response)
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{"error": runErr.Error()})
		return
	}

	s.clearSession(sessionID)
	response := workPermitResponse{SessionID: sessionID, Status: "completed"}
	if output != nil {
		response.FilledWps = output.FilledWps
		response.Message = output.ErrorMsg
	}
	c.JSON(http.StatusOK, response)
}

func interruptDetails(info any) (operation string, missingFields []string, question string, filledWps []graph.FilledWp, intentWpTypes []graph.WpMetaInfo) {
	payload, ok := info.(map[string]any)
	if !ok {
		return "", nil, "", nil, nil
	}
	operation, _ = payload["operation"].(string)
	question, _ = payload["question"].(string)
	if values, ok := payload["missing_fields"].([]string); ok {
		missingFields = values
	} else if values, ok := payload["missing_fields"].([]any); ok {
		for _, value := range values {
			if field, ok := value.(string); ok {
				missingFields = append(missingFields, field)
			}
		}
	}
	if values, ok := payload["filled_wps"].([]graph.FilledWp); ok {
		filledWps = values
	} else if values, ok := payload["filled_wps"]; ok {
		raw, err := json.Marshal(values)
		if err == nil {
			_ = json.Unmarshal(raw, &filledWps)
		}
	}
	if values, ok := payload["intent_wp_types"].([]graph.WpMetaInfo); ok {
		intentWpTypes = values
	} else if values, ok := payload["intent_wp_types"]; ok {
		raw, err := json.Marshal(values)
		if err == nil {
			_ = json.Unmarshal(raw, &intentWpTypes)
		}
	}
	return operation, missingFields, question, filledWps, intentWpTypes
}

func (s *WorkPermitServer) setSession(sessionID string, state sessionState) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.sessions[sessionID] = state
}

func (s *WorkPermitServer) getSession(sessionID string) (sessionState, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	state, ok := s.sessions[sessionID]
	return state, ok
}

func (s *WorkPermitServer) clearSession(sessionID string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	delete(s.sessions, sessionID)
}

func newID() (string, error) {
	buffer := make([]byte, 16)
	if _, err := rand.Read(buffer); err != nil {
		return "", fmt.Errorf("generate session id: %w", err)
	}
	return hex.EncodeToString(buffer), nil
}
