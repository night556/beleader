package main

import (
	"embed"
	"fmt"
	"io/fs"
	"log"
	"net/http"
	"net/http/httputil"
	"net/url"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"runtime"
	"strings"
	"syscall"

	"beleader/gateway/api"
	"beleader/gateway/config"
	"beleader/gateway/db"
	"beleader/gateway/llm"

	toolapi "beleader/tool-agent/api"
	"beleader/tool-agent/tools"

	"github.com/gin-gonic/gin"
)

//go:embed all:webdist
var webDist embed.FS

func main() {
	// Data directory: ~/.beleader or next to the exe
	dataDir := os.Getenv("BELEADER_DATA")
	if dataDir == "" {
		home, _ := os.UserHomeDir()
		if home != "" {
			dataDir = filepath.Join(home, ".beleader")
		} else {
			exe, _ := os.Executable()
			dataDir = filepath.Join(filepath.Dir(exe), "data")
		}
	}
	os.MkdirAll(dataDir, 0755)

	log.SetFlags(log.Ltime)
	log.Printf("BeLeader Desktop starting (data=%s)", dataDir)

	// ── Gateway ──
	cfg := config.DefaultConfig()
	dbCfg := db.DBConfig{
		Driver: "sqlite",
		Path:   filepath.Join(dataDir, "beleader.db"),
	}
	database, err := db.Open(dbCfg)
	if err != nil {
		log.Fatalf("Failed to open database: %v", err)
	}
	defer database.Close()

	llmClient := llm.New("", "", "")
	h := api.NewHandler(database, llmClient, cfg)

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

	gatewayPort := "8082"
	go func() {
		log.Printf("Gateway listening on http://127.0.0.1:%s", gatewayPort)
		if err := r.Run(":" + gatewayPort); err != nil {
			log.Fatalf("Gateway error: %v", err)
		}
	}()

	// ── Tool Agent ──
	toolPort := "8083"
	restrict := false
	if r := os.Getenv("RESTRICT_WORKSPACE"); r == "true" || r == "1" {
		restrict = true
	}
	srv := toolapi.NewServer(dataDir, restrict)

	// Initialize MCP manager
	mcpMgr := tools.NewMCPManager()
	tools.SetMCPManager(mcpMgr)

	toolSrv := &http.Server{Addr: ":" + toolPort, Handler: srv}
	go func() {
		log.Printf("Tool Agent listening on http://127.0.0.1:%s", toolPort)
		if err := toolSrv.ListenAndServe(); err != http.ErrServerClosed {
			log.Fatalf("Tool Agent error: %v", err)
		}
	}()

	// ── Register tool-agent with gateway ──
	myURL := fmt.Sprintf("http://127.0.0.1:%s", toolPort)
	env := map[string]string{
		"shell":      detectShell(),
		"platform":   fmt.Sprintf("%s/%s", runtime.GOOS, runtime.GOARCH),
		"go_version": runtime.Version(),
	}
	toolDefs := tools.AllToolDefs()
	stopReg := toolapi.StartRegistration(
		fmt.Sprintf("http://127.0.0.1:%s", gatewayPort),
		cfg.RegToken,
		"desktop",
		myURL,
		dataDir,
		restrict,
		env,
		toolDefs,
		mcpMgr,
	)
	defer close(stopReg)

	// ── Web Frontend ──
	webFS, err := fs.Sub(webDist, "webdist")
	if err != nil {
		log.Fatalf("Failed to serve web frontend: %v", err)
	}
	webHandler := http.FileServer(http.FS(webFS))

	// Proxy /api requests to gateway
	gatewayURL, _ := url.Parse(fmt.Sprintf("http://127.0.0.1:%s", gatewayPort))
	proxy := httputil.NewSingleHostReverseProxy(gatewayURL)

	webPort := "8080"
	webMux := http.NewServeMux()
	webMux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		if len(r.URL.Path) >= 4 && r.URL.Path[:4] == "/api" {
			proxy.ServeHTTP(w, r)
			return
		}
		// SPA fallback: serve index.html for non-file paths.
		// fs.FS paths must not have a leading slash.
		checkPath := strings.TrimPrefix(r.URL.Path, "/")
		f, err := webFS.Open(checkPath)
		if err != nil {
			r.URL.Path = "/"
		} else {
			f.Close()
		}
		webHandler.ServeHTTP(w, r)
	})

	webSrv := &http.Server{Addr: ":" + webPort, Handler: webMux}
	go func() {
		log.Printf("Web UI listening on http://127.0.0.1:%s", webPort)
		if err := webSrv.ListenAndServe(); err != http.ErrServerClosed {
			log.Fatalf("Web server error: %v", err)
		}
	}()

	// ── Open Browser ──
	openBrowser(fmt.Sprintf("http://127.0.0.1:%s", webPort))

	// ── Wait for shutdown ──
	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)
	<-sigCh
	log.Println("Shutting down...")
	toolSrv.Close()
	webSrv.Close()
}

func openBrowser(url string) {
	var cmd *exec.Cmd
	switch runtime.GOOS {
	case "windows":
		cmd = exec.Command("rundll32", "url.dll,FileProtocolHandler", url)
	case "darwin":
		cmd = exec.Command("open", url)
	default:
		cmd = exec.Command("xdg-open", url)
	}
	if cmd != nil {
		cmd.Start()
	}
}

func detectShell() string {
	if runtime.GOOS == "windows" {
		return "powershell"
	}
	for _, sh := range []string{"/bin/bash", "/usr/bin/bash", "/bin/zsh", "/bin/sh"} {
		if _, err := os.Stat(sh); err == nil {
			return sh
		}
	}
	return "sh"
}