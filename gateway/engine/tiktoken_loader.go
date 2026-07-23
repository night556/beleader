package engine

import (
	_ "embed"
	"encoding/base64"
	"strconv"
	"strings"

	"github.com/pkoukk/tiktoken-go"
)

//go:embed data/cl100k_base.tiktoken
var cl100kData []byte

func init() {
	tiktoken.SetBpeLoader(&localBpeLoader{})
}

type localBpeLoader struct{}

func (l *localBpeLoader) LoadTiktokenBpe(tiktokenBpeFile string) (map[string]int, error) {
	if strings.Contains(tiktokenBpeFile, "cl100k_base") {
		return parseBpeRanks(cl100kData)
	}
	return tiktoken.NewDefaultBpeLoader().LoadTiktokenBpe(tiktokenBpeFile)
}

func parseBpeRanks(data []byte) (map[string]int, error) {
	ranks := make(map[string]int)
	for _, line := range strings.Split(string(data), "\n") {
		if line == "" {
			continue
		}
		parts := strings.Split(line, " ")
		token, err := base64.StdEncoding.DecodeString(parts[0])
		if err != nil {
			return nil, err
		}
		rank, err := strconv.Atoi(parts[1])
		if err != nil {
			return nil, err
		}
		ranks[string(token)] = rank
	}
	return ranks, nil
}
