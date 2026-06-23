package tools

import (
	"context"
	"encoding/json"
	"fmt"
	"testing"
	"time"
)

func TestQuickSearch(t *testing.T) {
	query := `鸿蒙 用户 吐槽 缺少 软件 习惯 用 的 app 回不去 苹果`
	args, _ := json.Marshal(map[string]string{"query": query})

	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()

	result := webSearchHandler(ctx, string(args))
	if result.Error != "" {
		t.Fatalf("ERROR: %s", result.Error)
	}

	fmt.Println(result.Content)
}
