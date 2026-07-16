package api

import (
	"bytes"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"time"
)

const heartbeatInterval = 30 * time.Second

type registerRequest struct {
	Name  string `json:"name"`
	URL   string `json:"url"`
	Token string `json:"token"`
}

type registerResponse struct {
	ID     int64  `json:"id"`
	Name   string `json:"name"`
	URL    string `json:"url"`
	Status string `json:"status"`
}

type heartbeatRequest struct {
	ID     int64  `json:"id"`
	Status string `json:"status"`
}

// StartRegistration registers this runtime with the gateway, then sends periodic
// heartbeats. Returns a channel that the caller closes to trigger deregistration.
func StartRegistration(gatewayURL, token, name, runtimeURL string) chan struct{} {
	client := &http.Client{Timeout: 10 * time.Second}

	register := func() (*registerResponse, error) {
		body, _ := json.Marshal(registerRequest{Name: name, URL: runtimeURL, Token: token})
		resp, err := client.Post(gatewayURL+"/api/runtimes/register", "application/json", bytes.NewReader(body))
		if err != nil {
			return nil, err
		}
		defer resp.Body.Close()
		if resp.StatusCode != 200 {
			return nil, fmt.Errorf("register failed: %s", resp.Status)
		}
		var regResp registerResponse
		json.NewDecoder(resp.Body).Decode(&regResp)
		return &regResp, nil
	}

	sendHeartbeat := func(id int64, status string) error {
		body, _ := json.Marshal(heartbeatRequest{ID: id, Status: status})
		req, _ := http.NewRequest("POST", gatewayURL+"/api/runtimes/heartbeat", bytes.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
		resp, err := client.Do(req)
		if err != nil {
			return err
		}
		resp.Body.Close()
		return nil
	}

	done := make(chan struct{})

	go func() {
		// Initial registration.
		regResp, err := register()
		if err != nil {
			log.Printf("[registration] initial registration failed: %v", err)
			return
		}
		runtimeID := regResp.ID
		log.Printf("[registration] registered as %q (id=%d) at %s", name, runtimeID, runtimeURL)

		ticker := time.NewTicker(heartbeatInterval)
		defer ticker.Stop()

		for {
			select {
			case <-ticker.C:
				if err := sendHeartbeat(runtimeID, "active"); err != nil {
					log.Printf("[registration] heartbeat failed: %v", err)
				}
			case <-done:
				log.Println("[registration] deregistering...")
				sendHeartbeat(runtimeID, "inactive")
				return
			}
		}
	}()

	return done
}
