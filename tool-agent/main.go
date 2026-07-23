package main

import (
	"bufio"
	"context"
	"flag"
	"fmt"
	"log"
	"net"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"syscall"
	"time"

	"beleader/tool-agent/api"
	"beleader/tool-agent/tools"
)

func loadEnvFile(path string) {
	f, err := os.Open(path)
	if err != nil {
		return
	}
	defer f.Close()
	sc := bufio.NewScanner(f)
	for sc.Scan() {
		line := strings.TrimSpace(sc.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		kv := strings.SplitN(line, "=", 2)
		if len(kv) != 2 {
			continue
		}
		k := strings.TrimSpace(kv[0])
		v := strings.TrimSpace(kv[1])
		if os.Getenv(k) == "" {
			os.Setenv(k, v)
		}
	}
}

func main() {
	loadEnvFile(".env")

	port := flag.String("port", "", "listen port (default: PORT env or 8083)")
	dataDir := flag.String("data-dir", "", "data directory (default: WORKSPACE_ROOT env or ~/.beleader)")
	gatewayURL := flag.String("gateway-url", "", "Gateway URL for auto-registration")
	gatewayToken := flag.String("gateway-token", "", "Registration token")
	poolName := flag.String("pool", "", "Pool name to join (default: POOL env or hostname)")
	restrict := flag.Bool("restrict-workspace", false, "Restrict file ops to workspace")
	flag.Parse()

	if *port == "" {
		*port = envOr("PORT", "8083")
	}
	if *dataDir == "" {
		*dataDir = envOr("WORKSPACE_ROOT", func() string {
			home, _ := os.UserHomeDir()
			return filepath.Join(home, ".beleader")
		}())
	}
	if *gatewayURL == "" {
		*gatewayURL = os.Getenv("GATEWAY_URL")
	}
	if *gatewayToken == "" {
		*gatewayToken = os.Getenv("GATEWAY_TOKEN")
	}
	if *poolName == "" {
		*poolName = envOr("POOL", func() string {
			h, _ := os.Hostname()
			return h
		}())
	}
	if r := os.Getenv("RESTRICT_WORKSPACE"); r != "" {
		if v, err := strconv.ParseBool(r); err == nil {
			*restrict = v
		}
	}

	os.MkdirAll(*dataDir, 0755)

	srv := api.NewServer(*dataDir, *restrict)

	// Build tool definitions
	toolDefs := tools.AllToolDefs()

	httpServer := &http.Server{Addr: ":" + *port, Handler: srv}

	go func() {
		log.Printf("Tool Agent starting on :%s (data=%s, pool=%s, restrict=%t)",
			*port, *dataDir, *poolName, *restrict)
		if err := httpServer.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Fatal(err)
		}
	}()

	// Auto-register with Gateway
	var stopReg chan struct{}
	if *gatewayURL != "" {
		env := map[string]string{
			"shell":      detectShell(),
			"platform":   fmt.Sprintf("%s/%s", runtime.GOOS, runtime.GOARCH),
			"go_version": runtime.Version(),
		}
		myURL := fmt.Sprintf("http://%s:%s", getMyIP(), *port)
		stopReg = api.StartRegistration(*gatewayURL, *gatewayToken, *poolName, myURL, *dataDir, *restrict, env, toolDefs)
	}

	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)
	<-sigCh
	log.Println("Shutting down...")
	if stopReg != nil {
		close(stopReg)
	}

	shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	httpServer.Shutdown(shutdownCtx)
	tools.Cleanup()
	log.Println("Shutdown complete.")
}

func envOr(key, def string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return def
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

func getMyIP() string {
	// In Docker, the hostname resolves to the container IP
	host, _ := os.Hostname()
	if host == "localhost" || host == "" {
		return "127.0.0.1"
	}
	// Try to resolve hostname
	if addrs, err := net.LookupHost(host); err == nil && len(addrs) > 0 {
		return addrs[0]
	}
	return "127.0.0.1"
}
