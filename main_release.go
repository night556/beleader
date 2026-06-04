//go:build release

package main

import (
	_ "embed"
	"os"
	"path/filepath"
	"runtime"
)

//go:embed bin/iamhuman-agent-release
var agentBinary []byte

func agentExeName() string {
	if runtime.GOOS == "windows" {
		return "iamhuman-agent.exe"
	}
	return "iamhuman-agent"
}

func extractAgent() {
	exe, err := os.Executable()
	if err != nil {
		return
	}
	dir := filepath.Dir(exe)
	agentPath := filepath.Join(dir, agentExeName())
	if info, err := os.Stat(agentPath); err == nil {
		if info.Size() == int64(len(agentBinary)) {
			return
		}
	}
	os.WriteFile(agentPath, agentBinary, 0755)
}
