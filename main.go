package main

import (
	"context"
	"fmt"
	"log"
	"net/http"
	"os"
	"time"

	"linde-go-v2/internal/chatmodel"
	appconfig "linde-go-v2/internal/config"
	"linde-go-v2/internal/graph"
	"linde-go-v2/internal/httpapi"
	"linde-go-v2/internal/tools"

	"github.com/cloudwego/eino/compose"
)

func main() {
	if err := run(); err != nil {
		log.Fatal(err)
	}
}

func run() error {
	configPath := os.Getenv("LINDE_CONFIG")
	if configPath == "" {
		configPath = "configs/config.yaml"
	}

	cfg, err := appconfig.Load(configPath)
	if err != nil {
		return err
	}

	ctx := context.Background()
	model, err := chatmodel.NewChatModelFromAppConfig(ctx, cfg, "")
	if err != nil {
		return fmt.Errorf("create chat model: %w", err)
	}

	extractor, err := graph.NewLLMWpInfoExtractor(model)
	if err != nil {
		return err
	}
	recognizer, err := graph.NewLLMWpTypeRecognizer(model)
	if err != nil {
		return err
	}

	toolConfig := tools.ToolConfigFromAppConfig(cfg)
	templateListTool, err := tools.NewQueryWptempletsNoContentTool(toolConfig)
	if err != nil {
		return fmt.Errorf("create template list tool: %w", err)
	}
	templateDetailTool, err := tools.NewQueryWptempletTool(toolConfig)
	if err != nil {
		return fmt.Errorf("create template detail tool: %w", err)
	}
	updateTool, err := tools.NewUpdateWpInfoTool(toolConfig)
	if err != nil {
		return fmt.Errorf("create update work permit tool: %w", err)
	}
	linkTool, err := tools.NewLinkexistWpinfoTool(toolConfig)
	if err != nil {
		return fmt.Errorf("create link work permit tool: %w", err)
	}

	checkpointStore := httpapi.NewMemoryCheckPointStore()
	runner, err := graph.BuildWpGraph(
		extractor,
		templateListTool,
		recognizer,
		templateDetailTool,
		httpapi.EmptyStructuredMemoryStore{},
		updateTool,
		linkTool,
		compose.WithCheckPointStore(checkpointStore),
		compose.WithGraphName("work-permit"),
	)
	if err != nil {
		return fmt.Errorf("build work permit graph: %w", err)
	}

	server := httpapi.NewWorkPermitServer(runner, cfg.WorkPermitAPI.SiteID, cfg.WorkPermitAPI.UserID)
	port := os.Getenv("PORT")
	if port == "" {
		port = "8080"
	}

	httpServer := &http.Server{
		Addr:              ":" + port,
		Handler:           server.Router(),
		ReadHeaderTimeout: 5 * time.Second,
	}

	log.Printf("work permit API listening on %s", httpServer.Addr)
	return httpServer.ListenAndServe()
}
