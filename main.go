package main

import (
	"context"
	"fmt"
	"log/slog"
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
	initLogger()

	if err := run(); err != nil {
		slog.Error("server_exit", slog.String("err", err.Error()))
		os.Exit(1)
	}
}

func initLogger() {
	slog.SetDefault(slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{
		Level: slog.LevelInfo,
	})))
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

	promptStore, err := graph.LoadPromptStore(cfg.StructuredMemoryDB.Driver, cfg.StructuredMemoryDB.DSN)
	if err != nil {
		return fmt.Errorf("load prompt store: %w", err)
	}

	extractor, err := graph.NewLLMWpInfoExtractor(model, promptStore)
	if err != nil {
		return err
	}
	recognizer, err := graph.NewLLMWpTypeRecognizer(model, promptStore)
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
	structuredMemoryStore, err := graph.LoadStructuredMemoryStore(cfg.StructuredMemoryDB.Driver, cfg.StructuredMemoryDB.DSN)
	if err != nil {
		return fmt.Errorf("load structured memory: %w", err)
	}
	fillRules, err := graph.LoadWpFillRules("data/wp_fill_rules.json")
	if err != nil {
		return fmt.Errorf("load work permit fill rules: %w", err)
	}
	runner, err := graph.BuildWpGraph(
		extractor,
		templateListTool,
		recognizer,
		templateDetailTool,
		model,
		promptStore,
		structuredMemoryStore,
		fillRules,
		updateTool,
		linkTool,
		compose.WithCheckPointStore(checkpointStore),
		compose.WithGraphName("work-permit"),
	)
	if err != nil {
		return fmt.Errorf("build work permit graph: %w", err)
	}

	server := httpapi.NewWorkPermitServer(runner, promptStore, cfg.WorkPermitAPI.SiteID, cfg.WorkPermitAPI.UserID)
	port := os.Getenv("PORT")
	if port == "" {
		port = "7070"
	}

	httpServer := &http.Server{
		Addr:              ":" + port,
		Handler:           server.Router(),
		ReadHeaderTimeout: 5 * time.Second,
	}

	slog.Info("server_listen", slog.String("addr", httpServer.Addr))
	return httpServer.ListenAndServe()
}
