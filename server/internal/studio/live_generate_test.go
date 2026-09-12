package studio

import (
	"context"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
	"vowfilm/server/internal/config"
	"vowfilm/server/internal/platform"
)

// TestLiveGenerateTemplateV3 runs one full template v3 wedding generate against
// the configured data directory and StarNet credentials. Set
// VOWFILM_LIVE_GENERATE=1; optional VOWFILM_LIVE_OWNER=admin user id.
func TestLiveGenerateTemplateV3(t *testing.T) {
	if os.Getenv("VOWFILM_LIVE_GENERATE") == "" {
		t.Skip("set VOWFILM_LIVE_GENERATE=1 to run a paid end-to-end template generate")
	}
	envFile := os.Getenv("VOWFILM_ENV_FILE")
	if envFile == "" {
		envFile = filepath.Join("..", "..", ".env")
	}
	_ = config.LoadEnv(envFile)
	dataDir := os.Getenv("VOWFILM_DATA_DIR")
	if dataDir == "" {
		dataDir = filepath.Join("..", "..", "data")
	}
	token := os.Getenv("GO_BACKEND_TOKEN")
	if len(token) < 24 {
		t.Fatal("GO_BACKEND_TOKEN must be set in the environment")
	}
	cfg := Config{
		DataDir:        dataDir,
		DatabaseDriver: valEnv("DATABASE_DRIVER", "sqlite"),
		DatabaseURL:    os.Getenv("DATABASE_URL"),
		Token:          token,
		BaseURL:        valEnv("STARNET_BASE_URL", valEnv("OPENAI_BASE_URL", "https://open.embervale.cn")),
		APIKey:         valEnv("STARNET_API_KEY", os.Getenv("OPENAI_API_KEY")),
		LLMModel:       valEnv("STARNET_LLM_MODEL", valEnv("OPENAI_MODEL", "openai/gpt-6-astra")),
		VideoModel:     valEnv("STARNET_VIDEO_MODEL", "volcengine/doubao-seedance-2-0-mini-260615"),
		AudioModel:     valEnv("STARNET_AUDIO_MODEL", defaultAudioModel),
		Concurrency:    1,
	}
	if cfg.APIKey == "" {
		t.Fatal("STARNET_API_KEY or provider.json must be configured")
	}
	a, err := New(cfg)
	if err != nil {
		t.Fatal(err)
	}
	defer a.database.Close()

	ownerID := os.Getenv("VOWFILM_LIVE_OWNER")
	if ownerID == "" {
		ownerID = "usr_d565e17826e8b04c0971"
	}
	if err = a.accounts.Repo.Credit(ownerID, ownerID, 500000, "live-template-generate", "integration"); err != nil {
		t.Fatal(err)
	}
	owner := &platform.User{ID: ownerID}
	p := &Project{
		ID:               "film_" + newID(""),
		OwnerID:          ownerID,
		CreationMode:     "template",
		TemplateID:       "wedding-timeless",
		TemplateVersion:  "3",
		Title:            "Seed Audio 分段配乐验证",
		Brief:            "两位虚构成年新人，六幕十二镜头婚庆预告，用于验证章节配乐与 Seed Audio 合成。",
		Duration:         60,
		Style:            "joyful",
		Ratio:            "16:9",
		Occasion:         "opening",
		WardrobeMode:     "auto",
		Status:           "draft",
		Revision:         1,
		GenerationBudget: 180,
		OutputResolution: "720P",
		Shots:            []Shot{},
		Assets:           []Asset{},
		Events:           []Event{},
		CreatedAt:        now(),
		UpdatedAt:        now(),
	}
	applyProjectDefaults(p)
	if err = validateProject(p); err != nil {
		t.Fatal(err)
	}
	if err = a.store.Put(p); err != nil {
		t.Fatal(err)
	}
	t.Log("project", p.ID)

	q, err := a.quote(ownerID, a.store.Get(p.ID), "generate", "")
	if err != nil {
		t.Fatal(err)
	}
	req := withUser(withQuote(httptestNewRequest("POST", "/api/projects/"+p.ID+"/generate", nil), q.ID), owner)
	if err = a.startBilled(req, p.ID, "generate", ""); err != nil {
		t.Fatal(err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 110*time.Minute)
	defer cancel()
	deadline := time.Now().Add(105 * time.Minute)
	for {
		if ctx.Err() != nil {
			t.Fatal(ctx.Err())
		}
		cur := a.store.Get(p.ID)
		if cur == nil {
			t.Fatal("project missing")
		}
		if cur.Status == "completed" && cur.FilmURL != "" {
			t.Logf("completed film=%s music=%s/%s sections=%d", cur.FilmURL, cur.MusicSource, cur.MusicFile, len(cur.MusicSections))
			return
		}
		if cur.Status == "failed" || cur.Status == "cancelled" {
			t.Fatalf("status=%s message=%s", cur.Status, cur.Message)
		}
		if len(cur.Events) > 0 {
			t.Log(cur.Events[len(cur.Events)-1].Message)
		}
		if time.Now().After(deadline) {
			t.Fatalf("timeout status=%s progress=%d", cur.Status, cur.Progress)
		}
		time.Sleep(15 * time.Second)
	}
}

func valEnv(key, def string) string {
	if v := strings.TrimSpace(os.Getenv(key)); v != "" {
		return v
	}
	return def
}

func httptestNewRequest(method, target string, body any) *http.Request {
	// Avoid importing net/http/httptest in production code paths; live test only.
	r, _ := http.NewRequest(method, target, nil)
	r.Header.Set("X-Vowfilm-Token", os.Getenv("GO_BACKEND_TOKEN"))
	r.Header.Set("X-Vowfilm-CSRF", "1")
	r.Header.Set("Idempotency-Key", "live-"+newID(""))
	return r
}

func withQuote(r *http.Request, quoteID string) *http.Request {
	r.Header.Set("X-Vowfilm-Quote", quoteID)
	return r
}
