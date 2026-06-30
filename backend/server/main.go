package server

import (
	"fmt"
	"log"
	"net"
	"os"
	"os/signal"
	"path/filepath"
	"strconv"
	"syscall"

	"beleader/backend/api"
	"beleader/backend/config"
	"beleader/backend/db"
	"beleader/backend/llm"
	"beleader/backend/mcp"
	"beleader/backend/session"
	"beleader/backend/tools"

	"github.com/gin-gonic/gin"
	"gopkg.in/natefinch/lumberjack.v2"
)

// OnHandlerCreated is an optional hook called after the API handler is created.
// Desktop mode uses this to inject the desktop bridge.
var OnHandlerCreated func(h *api.Handler)

// Run starts the HTTP server with the port from PORT env var (default 8080).
// Blocks until SIGINT/SIGTERM.
func Run(cfg *config.Config, database *db.DB, llmClient *llm.Client) {
	port := os.Getenv("PORT")
	if port == "" {
		port = "8080"
	}
	runServer(cfg, database, llmClient, port, true)
}

// RunWithPort starts the HTTP server on a specific port. Blocking.
func RunWithPort(cfg *config.Config, database *db.DB, llmClient *llm.Client, port int) {
	runServer(cfg, database, llmClient, strconv.Itoa(port), true)
}

// RunAutoPort listens on a random OS-assigned port, prints "PORT=<port>" to stdout
// so a parent process can read it, optionally redirects logs to a rotating file,
// then blocks until SIGINT/SIGTERM.
func RunAutoPort(cfg *config.Config, database *db.DB, llmClient *llm.Client, logDir string) {
	ln, err := net.Listen("tcp", ":0")
	if err != nil {
		fmt.Fprintf(os.Stderr, "Failed to find random port: %v\n", err)
		os.Exit(1)
	}
	port := ln.Addr().(*net.TCPAddr).Port
	ln.Close()

	fmt.Printf("PORT=%d\n", port)

	if logDir != "" {
		os.MkdirAll(logDir, 0755)
		lj := &lumberjack.Logger{
			Filename:   filepath.Join(logDir, "beleader-backend.log"),
			MaxSize:    10,
			MaxBackups: 5,
			MaxAge:     30,
			Compress:   true,
		}
		gin.DefaultWriter = lj
		gin.DefaultErrorWriter = lj
		log.SetOutput(lj)
		llm.LogWriter = lj
	}

	runServer(cfg, database, llmClient, strconv.Itoa(port), true)
}

func runServer(cfg *config.Config, database *db.DB, llmClient *llm.Client, port string, blocking bool) {
	// Register all builtin tools into the global Registry before creating the handler.
	tools.RegisterBuiltinTools()

	// Seed prompts are canonical — set them before DB seed uses them.
	db.SetCoordinatorPrompt(session.CoordinatorPrompt)

	h := api.NewHandler(database, llmClient, cfg)

	mcpMgr := mcp.NewManager(database)
	mcpMgr.Start()
	h.MCPMgr = mcpMgr

	if OnHandlerCreated != nil {
		OnHandlerCreated(h)
	}

	gin.SetMode(gin.ReleaseMode)
	r := gin.New()
	r.Use(gin.Logger(), gin.Recovery())

	// CORS
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

	go func() {
		fmt.Printf("BeLeader backend listening on http://127.0.0.1:%s\n", port)
		if err := r.Run(":" + port); err != nil {
			fmt.Fprintf(os.Stderr, "Server error: %v\n", err)
		}
	}()

	if blocking {
		sigCh := make(chan os.Signal, 1)
		signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)
		<-sigCh
		fmt.Println("\nShutting down...")
		mcpMgr.Stop()
		tools.Cleanup()
	}
}
