package main

import (
	"bufio"
	"context"
	"flag"
	"log"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"time"

	"beleader/runtime/api"
	"beleader/runtime/tools"
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

	port := flag.String("port", "", "listen port (default: PORT env or 8081)")
	dataDir := flag.String("data-dir", "", "data directory (default: DATA_DIR env or ~/.beleader/runtime)")
	_ = flag.Bool("headless", true, "run browser in headless mode")
	gatewayURL := flag.String("gateway-url", "", "Gateway URL for auto-registration (default: GATEWAY_URL env)")
	gatewayToken := flag.String("gateway-token", "", "Registration token for gateway auth (default: GATEWAY_TOKEN env)")
	runtimeURL := flag.String("runtime-url", "", "Public URL of this runtime for Gateway to reach it (default: RUNTIME_URL env or http://127.0.0.1:{port})")
	runtimeName := flag.String("runtime-name", "", "Name for this runtime instance (default: RUNTIME_NAME env or hostname)")
	restrictWorkspace := flag.Bool("restrict-workspace", false, "Restrict file operations to workspace (default: RESTRICT_WORKSPACE env or false)")
	flag.Parse()

	if *port == "" {
		if p := os.Getenv("PORT"); p != "" {
			*port = p
		} else {
			*port = "8081"
		}
	}

	if *dataDir == "" {
		if d := os.Getenv("DATA_DIR"); d != "" {
			*dataDir = d
		} else {
			home, _ := os.UserHomeDir()
			*dataDir = filepath.Join(home, ".beleader", "runtime")
		}
	}

	if r := os.Getenv("RESTRICT_WORKSPACE"); r != "" {
		if v, err := strconv.ParseBool(r); err == nil {
			*restrictWorkspace = v
		}
	}

	if *gatewayURL == "" {
		if u := os.Getenv("GATEWAY_URL"); u != "" {
			*gatewayURL = u
		}
	}
	if *gatewayToken == "" {
		if t := os.Getenv("GATEWAY_TOKEN"); t != "" {
			*gatewayToken = t
		}
	}
	if *runtimeURL == "" {
		if u := os.Getenv("RUNTIME_URL"); u != "" {
			*runtimeURL = u
		} else {
			*runtimeURL = "http://127.0.0.1:" + *port
		}
	}
	if *runtimeName == "" {
		if n := os.Getenv("RUNTIME_NAME"); n != "" {
			*runtimeName = n
		} else {
			host, _ := os.Hostname()
			*runtimeName = host
		}
	}

	os.MkdirAll(*dataDir, 0755)

	srv := api.NewServer(*dataDir)

	httpServer := &http.Server{Addr: ":" + *port, Handler: srv}

	go func() {
		log.Printf("Runtime starting on :%s (data=%s)", *port, *dataDir)
		if err := httpServer.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Fatal(err)
		}
	}()

	// Configure management tools to call back to the Gateway.
	tools.SetGatewayURL(*gatewayURL)

	// Start auto-registration if gateway URL is configured.
	var stopReg chan struct{}
	if *gatewayURL != "" {
		stopReg = api.StartRegistration(*gatewayURL, *gatewayToken, *runtimeName, *runtimeURL, *restrictWorkspace)
	}

	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)
	<-sigCh
	log.Println("Shutting down...")
	if stopReg != nil {
		close(stopReg)
	}

	// Graceful HTTP shutdown — stops accepting new requests, waits for in-flight.
	shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	httpServer.Shutdown(shutdownCtx)

	// Kill all background processes started by this Runtime.
	tools.Cleanup()
	log.Println("Shutdown complete.")
}
