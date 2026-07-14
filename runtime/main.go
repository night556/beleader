package main

import (
	"bufio"
	"flag"
	"log"
	"os"
	"path/filepath"
	"strconv"
	"strings"

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
	headless := flag.Bool("headless", true, "run browser in headless mode")
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

	if h := os.Getenv("HEADLESS"); h != "" {
		if v, err := strconv.ParseBool(h); err == nil {
			*headless = v
		}
	}
	tools.BrowserHeadless = *headless

	os.MkdirAll(*dataDir, 0755)

	srv := api.NewServer(*dataDir)
	log.Printf("Runtime starting on :%s (data=%s)", *port, *dataDir)
	if err := srv.ListenAndServe(":" + *port); err != nil {
		log.Fatal(err)
	}
}
