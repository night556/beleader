package main

import (
	"embed"
	"flag"
	"fmt"
	"io/fs"
	"os"

	"iamhuman/backend/api"
	"iamhuman/backend/config"
	"iamhuman/backend/db"
	"iamhuman/backend/llm"
	"iamhuman/backend/server"
	"iamhuman/backend/tools"
)

//go:embed web/*
var staticFiles embed.FS

func main() {
	extractAgent()
	port := flag.Int("port", 0, "HTTP server port (0=random, default: PORT env or 8080)")
	logDir := flag.String("log-dir", "", "Log directory for rotating file logs (default: stdout)")
	flag.Parse()

	home, _ := os.UserHomeDir()
	cfgPath := os.Getenv("IAMHUMAN_CONFIG")
	if cfgPath == "" {
		cfgPath = home + "/.iamhuman/config.yaml"
	}

	os.MkdirAll(home+"/.iamhuman", 0755)
	os.MkdirAll(home+"/.iamhuman/projects", 0755)

	cfg, err := config.Load(cfgPath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Failed to load config: %v\n", err)
		os.Exit(1)
	}

	database, err := db.Open(cfg.DBPath())
	if err != nil {
		fmt.Fprintf(os.Stderr, "Failed to open database: %v\n", err)
		os.Exit(1)
	}
	defer database.Close()

	if err := database.CleanupZombieSessions(); err != nil {
		fmt.Fprintf(os.Stderr, "Failed to cleanup zombie sessions: %v\n", err)
	}

	activeModel := cfg.ActiveModel()
	var llmClient *llm.Client
	if activeModel == nil {
		fmt.Fprintf(os.Stderr, "⚠ No model configured — chat will be unavailable until a model is added in Settings.\n")
		llmClient = llm.New("", "", "")
	} else {
		llmClient = llm.New(activeModel.BaseURL, activeModel.APIKey, activeModel.Model)
	}

	server.OnHandlerCreated = func(h *api.Handler) {
		webFS, _ := fs.Sub(staticFiles, "web")
		h.SetStaticFS(webFS)
		tools.SetContentNotifier(func(eventType string, data map[string]any) {
			h.Notify(api.SessionEvent{Type: eventType, Data: data})
		})
		tools.RegisterHTMLTools(h.SessionMgr)
	}

	if *port == 0 {
		server.RunAutoPort(cfg, database, llmClient, *logDir)
	} else {
		server.RunWithPort(cfg, database, llmClient, *port)
	}
}
