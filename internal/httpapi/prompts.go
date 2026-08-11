package httpapi

import (
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"
	"linde-go-v2/internal/graph"
)

type updatePromptRequest struct {
	Title       string `json:"title"`
	Description string `json:"description"`
	Content     string `json:"content"`
}

func (s *WorkPermitServer) handleListPrompts(c *gin.Context) {
	if s.promptStore == nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "prompt store is nil"})
		return
	}

	prompts, err := s.promptStore.List(c.Request.Context())
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, prompts)
}

func (s *WorkPermitServer) handleGetPrompt(c *gin.Context) {
	if s.promptStore == nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "prompt store is nil"})
		return
	}

	prompt, err := s.promptStore.Get(c.Request.Context(), strings.TrimSpace(c.Param("key")))
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, prompt)
}

func (s *WorkPermitServer) handleUpdatePrompt(c *gin.Context) {
	if s.promptStore == nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "prompt store is nil"})
		return
	}

	var request updatePromptRequest
	if err := c.ShouldBindJSON(&request); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid request body"})
		return
	}

	prompt := graph.PromptDefinition{
		Key:         strings.TrimSpace(c.Param("key")),
		Title:       request.Title,
		Description: request.Description,
		Content:     request.Content,
	}
	if err := s.promptStore.Upsert(c.Request.Context(), prompt); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	updated, err := s.promptStore.Get(c.Request.Context(), prompt.Key)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, updated)
}

func (s *WorkPermitServer) handleResetPrompt(c *gin.Context) {
	if s.promptStore == nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "prompt store is nil"})
		return
	}

	key := strings.TrimSpace(c.Param("key"))
	if err := s.promptStore.Reset(c.Request.Context(), key); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	reset, err := s.promptStore.Get(c.Request.Context(), key)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, reset)
}
