package main

import (
	"flag"
	"fmt"
	"os"
	"os/signal"
	"syscall"

	"beleader/gateway/api"
	"beleader/gateway/config"
	"beleader/gateway/db"
	"beleader/gateway/llm"
	"beleader/gateway/mcp"

	"github.com/gin-gonic/gin"
	"gopkg.in/natefinch/lumberjack.v2"
)

func main() {
	port := flag.Int("port", 0, "HTTP server port (0=random, default: PORT env or 8080)")
	runtimeURL := flag.String("runtime-url", "http://127.0.0.1:8081", "Runtime service URL")
	logDir := flag.String("log-dir", "", "Log directory for rotating file logs (default: stdout)")
	flag.Parse()

	os.MkdirAll(config.ConfigDir(), 0755)

	cfg := config.DefaultConfig()
	if *runtimeURL != "" {
		cfg.RuntimeURL = *runtimeURL
	}

	database, err := db.Open(config.DBPath())
	if err != nil {
		fmt.Fprintf(os.Stderr, "Failed to open database: %v\n", err)
		os.Exit(1)
	}
	defer database.Close()

	// Check if a model is configured
	var llmClient *llm.Client
	if _, err := database.ActiveModel(); err != nil {
		fmt.Fprintf(os.Stderr, "Warning: No active model configured — chat will be unavailable until a model is added in Settings.\n")
		llmClient = llm.New("", "", "")
	} else {
		// LLM client is created per-request from DB model config
		llmClient = llm.New("", "", "")
	}

	if *logDir != "" {
		os.MkdirAll(*logDir, 0755)
		lj := &lumberjack.Logger{
			Filename:   *logDir + "/beleader-gateway.log",
			MaxSize:    10,
			MaxBackups: 5,
			MaxAge:     30,
			Compress:   true,
		}
		gin.DefaultWriter = lj
		gin.DefaultErrorWriter = lj
		llm.LogWriter = lj
	}

	h := api.NewHandler(database, llmClient, cfg)

	mcpMgr := mcp.NewManager(database)
	mcpMgr.Start()
	h.MCPMgr = mcpMgr

	gin.SetMode(gin.ReleaseMode)
	r := gin.New()
	r.Use(gin.Logger(), gin.Recovery())

	r.Use(func(c *gin.Context) {
		c.Header("Access-Control-Allow-Origin", "*")
		c.Header("Access-Control-Allow-Methods", "GET, POST, PUT, DELETE, OPTIONS")
		c.Header("Access-Control-Allow-Headers", "Content-Type, Authorization")
		if c.Request.Method == "OPTIONS" {
			c.AbortWithStatus(204)
			return
		}
		c.Next()
	})

	h.RegisterRoutes(r)

	listenPort := *port
	if listenPort == 0 {
		envPort := os.Getenv("PORT")
		if envPort != "" {
			fmt.Sscanf(envPort, "%d", &listenPort)
		}
	}
	if listenPort == 0 {
		listenPort = 8080
	}

	go func() {
		fmt.Printf("Gateway listening on http://127.0.0.1:%d\n", listenPort)
		fmt.Printf("Runtime URL: %s\n", cfg.RuntimeURL)
		if err := r.Run(fmt.Sprintf(":%d", listenPort)); err != nil {
			fmt.Fprintf(os.Stderr, "Server error: %v\n", err)
		}
	}()

	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)
	<-sigCh
	fmt.Println("\nShutting down...")
	mcpMgr.Stop()
}
