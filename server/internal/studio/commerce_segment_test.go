package studio

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"vowfilm/server/internal/advertising"
)

func TestCommerceV3StoryboardPlanAndPack(t *testing.T) {
	project := commerceFixture()
	project.Duration = 30
	calls := map[string]int{}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls["plan"]++
		var body map[string]any
		_ = json.NewDecoder(r.Body).Decode(&body)
		messages := body["messages"].([]any)
		system := messages[0].(map[string]any)["content"].(string)
		if !strings.Contains(system, "direct-storyboard") && !strings.Contains(system, "电商广告分镜") {
			t.Error("expected storyboard director contract")
		}
		raw, _ := json.Marshal(map[string]any{
			"synopsis": "口播带货",
			"title":    "三十秒广告",
			"shots": []any{
				map[string]any{"id": "S01", "title": "开场", "duration_seconds": 8, "prompt": "镜头1口播"},
				map[string]any{"id": "S02", "title": "商品", "duration_seconds": 7, "prompt": "镜头2特写"},
				map[string]any{"id": "S03", "title": "收尾", "duration_seconds": 8, "prompt": "镜头3引导"},
				map[string]any{"id": "S04", "title": "cta", "duration_seconds": 7, "prompt": "镜头4cta"},
			},
		})
		_ = json.NewEncoder(w).Encode(map[string]any{"choices": []any{map[string]any{"message": map[string]string{"content": string(raw)}}}})
	}))
	defer srv.Close()
	provider := newProvider(Config{BaseURL: srv.URL, APIKey: "test", LLMModel: "test", VideoModel: "starnet/minimax-h3"})
	_, shots, err := provider.Plan(context.Background(), project)
	if err != nil {
		t.Fatal(err)
	}
	if len(shots) != 4 || commerceGenerateUnitCount(shots) != 2 {
		t.Fatal("storyboard pack", len(shots), commerceGenerateUnitCount(shots))
	}
	project.Shots = shots
	project.GenerationMode = commerceDirectModeFor(30)
	project.PromptPolicy = advertising.Version()
	project.VideoModel = "starnet/minimax-h3"
	if err = layout(project); err != nil {
		t.Fatal(err)
	}
	if calls["plan"] != 1 {
		t.Fatal("unexpected calls", calls)
	}
}
