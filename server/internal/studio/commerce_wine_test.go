package studio

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
	"vowfilm/server/internal/advertising"
)

func warehouseWineProject() *Project {
	return &Project{
		ID:           "film_wine_warehouse_test",
		Scene:        "commerce",
		Occasion:     "product",
		Title:        "仓库里老爷子荐酒",
		Brief:        "虚构验收样片「老仓醇」玻璃陈列瓶：透明圆瓶、红旋盖、米黄纸质简标、浅琥珀色模型填充液；仅描述包装外观与粮食发酵品类陈列故事（入口绵、粮香明显为用户提供的文案用语，非功效承诺）。纯虚构、非真实商品销售，无价格、活动、认证，不鼓励饮酒。",
		CustomPrompt: "15秒9:16仓库实拍口播：约65岁精神矍铄老年男性在库房讲解虚构包装，语速偏快、节奏紧凑像老邻居赶话，举瓶展示标签与瓶身。句间停顿短，禁止饮酒/倒酒/聚餐畅饮与夸张叫卖；末镜一句短CTA。",
		Duration:     15,
		Ratio:        "9:16",
		Style:        "editorial",
		WardrobeMode: "fixed",
	}
}

func TestLiveCommerceWarehouseWinePlan(t *testing.T) {
	project := warehouseWineProject()
	project.Assets = []Asset{
		{Role: "product", Name: "老仓醇酒瓶.png"},
		{Role: "reference", Name: "老年男性人物卡.png"},
	}
	if os.Getenv("VOWFILM_LIVE_WINE_PLAN") != "1" {
		t.Skip("set VOWFILM_LIVE_WINE_PLAN=1 to call the configured LLM for a warehouse wine prompt")
	}
	dir := os.Getenv("VOWFILM_LIVE_PROVIDER_DIR")
	if dir == "" {
		dir = "data"
	}
	cfg := loadProviderOverrides(dir, Config{})
	if cfg.APIKey == "" || cfg.BaseURL == "" || cfg.LLMModel == "" {
		t.Fatal("configure provider in VOWFILM_LIVE_PROVIDER_DIR (e.g. data/provider.json)")
	}
	provider := newProvider(cfg)
	synopsis, shots, err := provider.Plan(context.Background(), project)
	if err != nil {
		t.Fatal(err)
	}
	if len(shots) != 1 || shots[0].Duration != 15 {
		t.Fatalf("expected one 15s shot, got %d shots", len(shots))
	}
	prompt := shots[0].Prompt
	if err := advertising.ValidateReferences(prompt, 2); err != nil {
		t.Fatal(err)
	}
	for _, need := range []string{"镜头1[", "老仓醇", "<图片1>", "<图片2>"} {
		if !strings.Contains(prompt, need) {
			t.Fatalf("prompt missing %q", need)
		}
	}
	t.Logf("synopsis: %s", synopsis)
	t.Logf("title: %s", shots[0].Title)
	t.Logf("prompt:\n%s", prompt)
}

func TestLiveCommerceWarehouseWineVideo(t *testing.T) {
	if os.Getenv("VOWFILM_LIVE_COMMERCE") != "1" {
		t.Skip("set VOWFILM_LIVE_COMMERCE=1 for a paid upstream acceptance run")
	}
	dir := os.Getenv("VOWFILM_LIVE_OUTPUT_DIR")
	providerDir := os.Getenv("VOWFILM_LIVE_PROVIDER_DIR")
	productImage := os.Getenv("VOWFILM_LIVE_PRODUCT_IMAGE")
	characterImage := os.Getenv("VOWFILM_LIVE_CHARACTER_IMAGE")
	if dir == "" || providerDir == "" || productImage == "" || characterImage == "" {
		t.Fatal("require VOWFILM_LIVE_OUTPUT_DIR, VOWFILM_LIVE_PROVIDER_DIR, VOWFILM_LIVE_PRODUCT_IMAGE, VOWFILM_LIVE_CHARACTER_IMAGE")
	}
	cfg := loadProviderOverrides(providerDir, Config{DataDir: dir, Concurrency: 2})
	if cfg.APIKey == "" || cfg.BaseURL == "" || cfg.VideoModel == "" || cfg.LLMModel == "" {
		t.Fatal("configure generation credentials and models first")
	}
	store, err := NewStore(dir)
	if err != nil {
		t.Fatal(err)
	}
	a := &App{cfg: cfg, store: store, provider: newProvider(cfg), slots: make(chan struct{}, 2), running: map[string]context.CancelFunc{}}
	const id = "film_ad_wine_warehouse_live"
	p := store.Get(id)
	if p == nil {
		base := warehouseWineProject()
		base.ID = id
		base.Status = "draft"
		base.Demo = true
		base.Revision = 1
		base.GenerationBudget = 15
		base.OutputResolution = "720P"
		base.Shots = []Shot{}
		base.Events = []Event{}
		base.CreatedAt = now()
		base.UpdatedAt = now()
		base.LLMModel = cfg.LLMModel
		base.VideoModel = cfg.VideoModel
		p = base
		projectDir := filepath.Join(dir, id)
		if err = os.MkdirAll(projectDir, 0700); err != nil {
			t.Fatal(err)
		}
		for _, item := range []struct {
			src, file, name string
			role            string
		}{
			{productImage, "fictional-wine-product.png", "虚构老仓醇酒瓶.png", "product"},
			{characterImage, "fictional-wine-host.png", "虚构仓库主播人物卡.png", "reference"},
		} {
			raw, readErr := os.ReadFile(item.src)
			if readErr != nil {
				t.Fatal(readErr)
			}
			if err = os.WriteFile(filepath.Join(projectDir, item.file), raw, 0600); err != nil {
				t.Fatal(err)
			}
			p.Assets = append(p.Assets, Asset{
				ID:   "asset_" + item.role,
				Name: item.name,
				Role: item.role,
				File: item.file,
				MIME: "image/png",
				URL:  mediaURL(id, item.file),
			})
		}
		if err = store.Put(p); err != nil {
			t.Fatal(err)
		}
	}
	ctx, cancel := context.WithTimeout(context.Background(), 25*time.Minute)
	defer cancel()
	var runErr error
	if p.Status != "completed" {
		done := make(chan error, 1)
		go func() { done <- a.run(ctx, id, "generate") }()
		ticker := time.NewTicker(10 * time.Second)
		defer ticker.Stop()
		previous := ""
	run:
		for {
			select {
			case runErr = <-done:
				break run
			case <-ticker.C:
				q := store.Get(id)
				states := []string{q.Status, q.Message}
				for _, s := range q.Shots {
					states = append(states, s.ID+":"+s.Status)
				}
				state := strings.Join(states, " | ")
				if state != previous {
					t.Log(state)
					previous = state
				}
			}
		}
	}
	p = store.Get(id)
	raw, _ := json.MarshalIndent(p, "", "  ")
	_ = os.WriteFile(filepath.Join(dir, "project.json"), raw, 0600)
	if runErr != nil {
		t.Fatal(runErr)
	}
	if p.Status != "completed" || p.FilmURL == "" {
		t.Fatal("no completed film")
	}
	if len(p.Shots) != 1 || p.GeneratedSeconds != 15 || !isDirectCommerce(p) {
		t.Fatal("not one direct 15-second commerce task")
	}
	source := filepath.Join(dir, id, filepath.Base(p.FilmURL))
	video, err := os.ReadFile(source)
	if err != nil {
		t.Fatal(err)
	}
	target := filepath.Join(dir, "commerce-wine-warehouse-15s.mp4")
	if err = os.WriteFile(target, video, 0600); err != nil {
		t.Fatal(err)
	}
	_ = os.WriteFile(filepath.Join(dir, "prompt.txt"), []byte(p.Shots[0].Prompt), 0600)
	manifest := map[string]any{
		"video":             filepath.Base(target),
		"duration_seconds":  15,
		"ratio":             "9:16",
		"fictional_product": true,
		"references":        []string{"fictional-wine-product.png", "fictional-wine-host.png"},
		"video_model":       p.VideoModel,
		"llm_model":         p.LLMModel,
		"prompt_policy":     p.PromptPolicy,
		"generation_mode":   p.GenerationMode,
		"video_tasks":       len(p.Shots),
		"billing":           "isolated acceptance run; no platform wallet adjustment",
	}
	raw, _ = json.MarshalIndent(manifest, "", "  ")
	_ = os.WriteFile(filepath.Join(dir, "manifest.json"), raw, 0600)
	t.Log("Saved actual AI test video:", target)
}

func teaWarehouseProject() *Project {
	return &Project{
		ID:           "film_ad_tea_warehouse_live",
		Scene:        "commerce",
		Occasion:     "product",
		Title:        "仓库里老爷子荐茶",
		Brief:        "虚构商品「老仓春」礼盒装绿茶：浅绿纸盒、透明罐可见茶叶；卖点仅为清香、回甘、适合日常待客，无价格活动。纯虚构验收样片。",
		CustomPrompt: "15秒9:16仓库实拍：约65岁老年男性在库房口播介绍虚构茶叶礼盒，举盒展示，语气朴实，末镜轻CTA。",
		Duration:     15,
		Ratio:        "9:16",
		Style:        "editorial",
		WardrobeMode: "fixed",
	}
}

// Pipeline smoke test when upstream rejects alcohol-related prompts/images.
func TestLiveCommerceWarehouseTeaVideo(t *testing.T) {
	if os.Getenv("VOWFILM_LIVE_TEA_FALLBACK") != "1" {
		t.Skip("set VOWFILM_LIVE_TEA_FALLBACK=1 to run non-alcohol pipeline smoke test")
	}
	dir := os.Getenv("VOWFILM_LIVE_OUTPUT_DIR")
	providerDir := os.Getenv("VOWFILM_LIVE_PROVIDER_DIR")
	productImage := os.Getenv("VOWFILM_LIVE_TEA_PRODUCT_IMAGE")
	characterImage := os.Getenv("VOWFILM_LIVE_CHARACTER_IMAGE")
	if dir == "" || providerDir == "" || productImage == "" || characterImage == "" {
		t.Fatal("require output dir, provider dir, tea product image, character image")
	}
	cfg := loadProviderOverrides(providerDir, Config{DataDir: dir, Concurrency: 2})
	store, err := NewStore(dir)
	if err != nil {
		t.Fatal(err)
	}
	a := &App{cfg: cfg, store: store, provider: newProvider(cfg), slots: make(chan struct{}, 2), running: map[string]context.CancelFunc{}}
	const id = "film_ad_tea_warehouse_live"
	if store.Get(id) == nil {
		base := teaWarehouseProject()
		base.Status = "draft"
		base.Demo = true
		base.Revision = 1
		base.GenerationBudget = 15
		base.OutputResolution = "720P"
		base.LLMModel = cfg.LLMModel
		base.VideoModel = cfg.VideoModel
		projectDir := filepath.Join(dir, id)
		if err = os.MkdirAll(projectDir, 0700); err != nil {
			t.Fatal(err)
		}
		for _, item := range []struct {
			src, file, name string
			role            string
		}{
			{productImage, "fictional-tea-product.png", "虚构老仓春茶叶.png", "product"},
			{characterImage, "fictional-wine-host.png", "虚构仓库主播人物卡.png", "reference"},
		} {
			raw, readErr := os.ReadFile(item.src)
			if readErr != nil {
				t.Fatal(readErr)
			}
			if err = os.WriteFile(filepath.Join(projectDir, item.file), raw, 0600); err != nil {
				t.Fatal(err)
			}
			base.Assets = append(base.Assets, Asset{
				ID: "asset_" + item.role, Name: item.name, Role: item.role,
				File: item.file, MIME: "image/png", URL: mediaURL(id, item.file),
			})
		}
		if err = store.Put(base); err != nil {
			t.Fatal(err)
		}
	}
	ctx, cancel := context.WithTimeout(context.Background(), 25*time.Minute)
	defer cancel()
	if err = a.run(ctx, id, "generate"); err != nil {
		t.Fatal(err)
	}
	p := store.Get(id)
	if p.Status != "completed" || p.FilmURL == "" {
		t.Fatal("tea smoke test did not complete")
	}
	target := filepath.Join(dir, "commerce-tea-warehouse-15s.mp4")
	raw, err := os.ReadFile(filepath.Join(dir, id, filepath.Base(p.FilmURL)))
	if err != nil {
		t.Fatal(err)
	}
	if err = os.WriteFile(target, raw, 0600); err != nil {
		t.Fatal(err)
	}
	t.Log("Saved pipeline smoke video:", target)
}

// Finish render when upstream video already downloaded but media QA failed on an older build.
func TestLiveCommerceWarehouseWineFinish(t *testing.T) {
	if os.Getenv("VOWFILM_LIVE_WINE_FINISH") != "1" {
		t.Skip("set VOWFILM_LIVE_WINE_FINISH=1 to render an existing wine shot")
	}
	dir := os.Getenv("VOWFILM_LIVE_OUTPUT_DIR")
	providerDir := os.Getenv("VOWFILM_LIVE_PROVIDER_DIR")
	if dir == "" || providerDir == "" {
		t.Fatal("require VOWFILM_LIVE_OUTPUT_DIR and VOWFILM_LIVE_PROVIDER_DIR")
	}
	cfg := loadProviderOverrides(providerDir, Config{DataDir: dir, Concurrency: 2})
	store, err := NewStore(dir)
	if err != nil {
		t.Fatal(err)
	}
	const id = "film_ad_wine_warehouse_live"
	p := store.Get(id)
	if p == nil || len(p.Shots) == 0 {
		t.Fatal("wine live project missing")
	}
	videoFile := "S01-r2-a1.mp4"
	if _, err = os.Stat(filepath.Join(dir, id, videoFile)); err != nil {
		t.Fatal(err)
	}
	if err = store.Update(id, func(q *Project) error {
		q.GenerationMode = commerceDirectMode
		q.PromptPolicy = advertising.Version()
		q.Shots[0].Status = "completed"
		q.Shots[0].VideoFile = videoFile
		q.Shots[0].Error = ""
		q.GeneratedSeconds = 15
		q.Status = "generating"
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	a := &App{cfg: cfg, store: store, provider: newProvider(cfg), slots: make(chan struct{}, 2), running: map[string]context.CancelFunc{}}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancel()
	if err = a.run(ctx, id, "render"); err != nil {
		t.Fatal(err)
	}
	p = store.Get(id)
	if p.Status != "completed" || p.FilmURL == "" {
		t.Fatal("render did not complete")
	}
	target := filepath.Join(dir, "commerce-wine-warehouse-15s.mp4")
	raw, err := os.ReadFile(filepath.Join(dir, id, filepath.Base(p.FilmURL)))
	if err != nil {
		t.Fatal(err)
	}
	if err = os.WriteFile(target, raw, 0600); err != nil {
		t.Fatal(err)
	}
	t.Log("Saved:", target, "videoModel:", p.VideoModel)
}
