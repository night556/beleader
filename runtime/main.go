package main

import (
	"flag"
	"log"
	"os"
	"path/filepath"

	"beleader/runtime/api"
	"beleader/runtime/tools"
)

func main() {
	port := flag.String("port", "8081", "listen port")
	dataDir := flag.String("data-dir", "", "data directory (default: ~/.beleader/runtime)")
	headless := flag.Bool("headless", true, "run browser in headless mode")
	flag.Parse()

	if *dataDir == "" {
		home, _ := os.UserHomeDir()
		*dataDir = filepath.Join(home, ".beleader", "runtime")
	}

	tools.BrowserHeadless = *headless

	srv := api.NewServer(*dataDir)
	log.Printf("Runtime starting on :%s (data=%s)", *port, *dataDir)
	if err := srv.ListenAndServe(":" + *port); err != nil {
		log.Fatal(err)
	}
}
