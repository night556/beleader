package tools

import (
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"sync"

	"github.com/go-rod/rod"
	"github.com/go-rod/rod/lib/launcher"
)

var bingUA = "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/131.0.0.0 Safari/537.36"

var (
	BrowserHeadless   = true
	BrowserProfileDir = ""

	bmu    sync.Mutex
	bState *browserState
)

type browserState struct {
	browser  *rod.Browser
	pid      int
	launcher *launcher.Launcher
}

func getOrCreateBrowser() (*browserState, error) {
	bmu.Lock()
	defer bmu.Unlock()

	if bState != nil {
		return bState, nil
	}

	if bs := reconnect(); bs != nil {
		bState = bs
		return bState, nil
	}

	if BrowserProfileDir != "" {
		os.MkdirAll(BrowserProfileDir, 0755)
	}

	l := launcher.New().
		Leakless(false).
		Headless(BrowserHeadless).
		Bin(findBrowser()).
		Set("user-agent", bingUA).
		Set("disable-blink-features", "AutomationControlled").
		Set("no-sandbox", "true").
		Set("window-size", fmt.Sprintf("%d,%d", 1920, 1080)).
		Set("disable-gpu", "true").
		Set("disable-features", "IsolateOrigins,site-per-process,OptimizationHints").
		Set("disable-site-isolation-trials", "true").
		Set("no-default-browser-check", "true").
		Set("no-first-run", "true")

	if BrowserProfileDir != "" {
		l = l.Set("user-data-dir", BrowserProfileDir)
	}

	url, err := l.Launch()
	if err != nil {
		if bs := reconnect(); bs != nil {
			bState = bs
			return bState, nil
		}
		return nil, fmt.Errorf("launch browser: %w", err)
	}

	browser := rod.New().ControlURL(url)
	if err := browser.Connect(); err != nil {
		return nil, fmt.Errorf("connect browser: %w", err)
	}
	bState = &browserState{
		browser:  browser,
		pid:      l.PID(),
		launcher: l,
	}
	return bState, nil
}

func resolveDebugURL(port string) (string, error) {
	resp, err := http.Get(fmt.Sprintf("http://127.0.0.1:%s/json/version", port))
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	var info struct {
		WebSocketDebuggerURL string `json:"webSocketDebuggerUrl"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&info); err != nil {
		return "", err
	}
	return info.WebSocketDebuggerURL, nil
}

func reconnect() *browserState {
	url := readDevToolsURL()
	if url == "" {
		return nil
	}
	browser := rod.New().ControlURL(url)
	if err := browser.Connect(); err != nil {
		return nil
	}
	return &browserState{
		browser: browser,
		pid:     findBrowserPID(),
	}
}

func readDevToolsURL() string {
	if BrowserProfileDir == "" {
		return ""
	}
	data, err := os.ReadFile(filepath.Join(BrowserProfileDir, "DevToolsActivePort"))
	if err != nil {
		return ""
	}
	port := strings.TrimSpace(strings.Split(string(data), "\n")[0])
	if port == "" {
		return ""
	}
	url, err := resolveDebugURL(port)
	if err != nil {
		return ""
	}
	return url
}

func findBrowserPID() int {
	if BrowserProfileDir == "" || runtime.GOOS != "windows" {
		return 0
	}
	ps := fmt.Sprintf(
		`Get-CimInstance Win32_Process -Filter "Name='msedge.exe' OR Name='chrome.exe'" | Where-Object { $_.CommandLine -like '*%s*' } | Select-Object -ExpandProperty ProcessId -First 1`,
		BrowserProfileDir,
	)
	out, err := exec.Command("powershell", "-NoProfile", "-Command", ps).Output()
	if err != nil {
		return 0
	}
	pid, err := strconv.Atoi(strings.TrimSpace(string(out)))
	if err != nil {
		return 0
	}
	return pid
}

func killBrowserLocked() {
	if bState == nil || bState.browser == nil {
		return
	}
	_ = bState.browser.Close()
	if bState.launcher != nil {
		bState.launcher.Kill()
		bState.launcher.Cleanup()
	} else if bState.pid != 0 {
		exec.Command("taskkill", "/f", "/t", "/pid", strconv.Itoa(bState.pid)).Run()
	}
}

func findBrowser() string {
	paths := []string{
		"C:\\Program Files (x86)\\Microsoft\\Edge\\Application\\msedge.exe",
		"C:\\Program Files\\Microsoft\\Edge\\Application\\msedge.exe",
		"C:\\Program Files\\Google\\Chrome\\Application\\chrome.exe",
		"C:\\Program Files (x86)\\Google\\Chrome\\Application\\chrome.exe",
	}
	for _, p := range paths {
		if _, err := os.Stat(p); err == nil {
			return p
		}
	}
	return ""
}

// Cleanup kills the browser process.
func Cleanup() {
	bgMu.Lock()
	for id, sess := range bgSessions {
		select {
		case <-sess.done:
		default:
			if sess.cmd.Process != nil {
				sess.cmd.Process.Kill()
			}
		}
		_ = id
	}
	bgMu.Unlock()

	bmu.Lock()
	if bState != nil {
		killBrowserLocked()
		bState = nil
	}
	bmu.Unlock()
}
