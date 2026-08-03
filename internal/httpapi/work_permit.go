package httpapi

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net/http"
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
	SessionID string           `json:"session_id"`
	Input     string           `json:"input"`
	Approved  *bool            `json:"approved,omitempty"`
	Payloads  []map[string]any `json:"payloads,omitempty"`
}

type workPermitResponse struct {
	SessionID     string           `json:"session_id"`
	Status        string           `json:"status"`
	MissingFields []string         `json:"missing_fields,omitempty"`
	Question      string           `json:"question,omitempty"`
	FilledWps     []graph.FilledWp `json:"filled_wps,omitempty"`
	Message       string           `json:"message,omitempty"`
}

type sessionState struct {
	CheckpointID string
	InterruptID  string
	Operation    string
}

type WorkPermitServer struct {
	runner WorkPermitRunner
	siteID int
	userID int

	mu       sync.Mutex
	sessions map[string]sessionState
}

func NewWorkPermitServer(runner WorkPermitRunner, siteID, userID int) *WorkPermitServer {
	return &WorkPermitServer{
		runner:   runner,
		siteID:   siteID,
		userID:   userID,
		sessions: make(map[string]sessionState),
	}
}

func (s *WorkPermitServer) Router() http.Handler {
	router := gin.New()
	router.Use(gin.Recovery())
	router.POST("/api/work-permits", s.handleCreateWorkPermit)
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
	if strings.TrimSpace(request.SessionID) == "" {
		s.handleInitialRequest(c, request)
		return
	}
	s.handleResumeRequest(c, request)
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
		UserInput: request.Input,
		SiteID:    s.siteID,
		UserID:    s.userID,
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
	default:
		c.JSON(http.StatusConflict, gin.H{"error": "unsupported session operation"})
		return
	}

	resumeContext := compose.ResumeWithData(c.Request.Context(), session.InterruptID, resumeData)
	state := &graph.WpGraphState{
		UserInput: request.Input,
		SiteID:    s.siteID,
		UserID:    s.userID,
	}
	output, err := s.runner.Invoke(resumeContext, state, compose.WithCheckPointID(session.CheckpointID))
	s.writeRunResult(c, request.SessionID, session.CheckpointID, output, err)
}

func (s *WorkPermitServer) writeRunResult(c *gin.Context, sessionID, checkpointID string, output *graph.WpGraphState, runErr error) {
	if runErr != nil {
		if info, ok := compose.ExtractInterruptInfo(runErr); ok && len(info.InterruptContexts) > 0 {
			interrupt := info.InterruptContexts[0]
			operation, missingFields, question, filledWps := interruptDetails(interrupt.Info)
			s.setSession(sessionID, sessionState{
				CheckpointID: checkpointID,
				InterruptID:  interrupt.ID,
				Operation:    operation,
			})

			response := workPermitResponse{
				SessionID:     sessionID,
				MissingFields: missingFields,
				Question:      question,
				FilledWps:     filledWps,
			}
			if operation == "collect_wp_info" {
				response.Status = "need_more_info"
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
func interruptDetails(info any) (operation string, missingFields []string, question string, filledWps []graph.FilledWp) {
	payload, ok := info.(map[string]any)
	if !ok {
		return "", nil, "", nil
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
	return operation, missingFields, question, filledWps
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
