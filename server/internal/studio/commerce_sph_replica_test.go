package studio

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"
)

func liveCommerceConcurrency() int {
	if v := os.Getenv("VOWFILM_LIVE_CONCURRENCY"); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n > 0 {
			return n
		}
	}
	return 4
}

func newLiveCommerceApp(cfg Config, store *Store) *App {
	if cfg.Concurrency < 1 {
		cfg.Concurrency = liveCommerceConcurrency()
	}
	return &App{
		cfg:      cfg,
		store:    store,
		provider: newProvider(cfg),
		slots:    make(chan struct{}, cfg.Concurrency),
		running:  map[string]context.CancelFunc{},
	}
}

// 复刻微信视频号 AXAiTdLYJ5：户外口播开场 + 快语速 + 底部口播字幕 + 中段酒瓶 + CTA（虚构样片）。
func sphWineReplicaProject() *Project {
	return &Project{
		ID:       "film_ad_sph_wine_park_v3",
		Scene:    "commerce",
		Occasion: "product",
		Title:    "喜欢喝酒的朋友注意了（视频号复刻）",
		Brief:    "虚构验收样片，结构参考 https://weixin.qq.com/sph/AXAiTdLYJ5 。商品：虚构「老仓醇」透明圆瓶、红旋盖、米黄简标。样片台词含「三到五折起买酒」仅为复刻句式，非真实活动。禁止饮酒画面。",
		CustomPrompt: "15秒9:16微信视频号实拍：镜头1–2户外公园绿地中近景，灰发戴眼镜、白拉链外套蓝T、领夹麦，浅景深，阴天自然光，直视镜头快口播约5–5.5字/秒；" +
			"画面底部居中粗体白字黑描边口播字幕，随说话内容同步显示（如「这平时爱喝酒的朋友」），左侧保留竖排小字免责：优惠券存在时效性商品领券金额以实际为准。" +
			"镜头3–4硬切举虚构酒瓶标签特写再回到人脸；逐字口播链：{喜欢喝酒的朋友注意了，教你一招，三到五折起买酒！}接{选酒看标签，别乱花冤枉钱。}末镜{想了解的点开看看。}画内与商品特写画外同一男声，句间停顿极短。",
		Duration:     15,
		Ratio:        "9:16",
		Style:        "editorial",
		WardrobeMode: "fixed",
	}
}

func TestLiveCommerceSphWineReplica(t *testing.T) {
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
	cfg := loadProviderOverrides(providerDir, Config{DataDir: dir, Concurrency: liveCommerceConcurrency()})
	if cfg.APIKey == "" || cfg.BaseURL == "" || cfg.VideoModel == "" || cfg.LLMModel == "" {
		t.Fatal("configure generation credentials and models first")
	}
	store, err := NewStore(dir)
	if err != nil {
		t.Fatal(err)
	}
	a := newLiveCommerceApp(cfg, store)
	id := sphWineReplicaProject().ID
	p := store.Get(id)
	if p == nil {
		base := sphWineReplicaProject()
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
			{characterImage, "sph-jianxuan-host.png", "视频号鉴选官人物卡.png", "reference"},
		} {
			raw, readErr := os.ReadFile(item.src)
			if readErr != nil {
				t.Fatal(readErr)
			}
			if err = os.WriteFile(filepath.Join(projectDir, item.file), raw, 0600); err != nil {
				t.Fatal(err)
			}
			p.Assets = append(p.Assets, Asset{
				ID: "asset_" + item.role, Name: item.name, Role: item.role,
				File: item.file, MIME: "image/png", URL: mediaURL(id, item.file),
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
	source := filepath.Join(dir, id, filepath.Base(p.FilmURL))
	video, err := os.ReadFile(source)
	if err != nil {
		t.Fatal(err)
	}
	target := filepath.Join(dir, "commerce-sph-wine-replica-15s.mp4")
	alt := filepath.Join(dir, "commerce-sph-wine-replica-15s-"+p.ID+".mp4")
	if err = os.WriteFile(target, video, 0600); err != nil {
		t.Fatal(err)
	}
	if err = os.WriteFile(alt, video, 0600); err != nil {
		t.Fatal(err)
	}
	_ = os.WriteFile(filepath.Join(dir, "prompt.txt"), []byte(p.Shots[0].Prompt), 0600)
	manifest := map[string]any{
		"reference":         "https://weixin.qq.com/sph/AXAiTdLYJ5",
		"reference_title":   "喜欢喝酒的朋友注意了，教你一招3-5折起买酒！",
		"video":             filepath.Base(target),
		"duration_seconds":  15,
		"ratio":             "9:16",
		"fictional_product": true,
		"video_model":       p.VideoModel,
		"llm_model":         p.LLMModel,
		"prompt_policy":     p.PromptPolicy,
		"generation_mode":   p.GenerationMode,
	}
	raw, _ = json.MarshalIndent(manifest, "", "  ")
	_ = os.WriteFile(filepath.Join(dir, "manifest.json"), raw, 0600)
	t.Log("Saved replica video:", target)
}

func sphWine60sParkProject() *Project {
	return &Project{
		ID:       "film_ad_sph_wine_park_60s_v1",
		Scene:    "commerce",
		Occasion: "product",
		Title:    "老仓醇 · 60秒公园口播（一致性验收）",
		Brief:    "虚构验收样片「老仓醇」：透明圆瓶、红色旋盖、米黄简标。参考 https://weixin.qq.com/sph/AXAiTdLYJ5 的口播节奏，扩展为60秒。台词折扣句仅为复刻句式。禁止饮酒、开盖、倒酒。无真实价格与活动。",
		CustomPrompt: "60秒9:16微信视频号实拍质感。全片固定同一户外公园绿地：阴天柔和自然光、浅景深、背景树木草坪，禁止换场景。" +
			"全片固定同一男主讲人：灰发、戴眼镜、白色拉链外套内搭蓝色T恤、领夹麦，外貌以<图片2>人物卡为准，各镜同一人、禁止换脸或换装。" +
			"全片固定同一瓶<图片1>老仓醇，红旋盖始终关闭，标签朝向在举瓶时一致。" +
			"画内口播与标签特写画外为同一成熟男声普通话，语速偏快约5–5.5字/秒，句间停顿极短；跨生成段必须写「接上句」保持声线连续。" +
			"画面底部居中粗体白字黑描边同步口播字幕；左侧全程竖排免责：优惠券存在时效性商品领券金额以实际为准。" +
			"分镜每2–6秒一切，交替人脸中近景与酒瓶标签特写；60秒内覆盖：开场提醒→折扣句式→看标签选酒→简讲粮香口感（非功效承诺）→再次举瓶→行动引导。末段CTA仅一次。",
		Duration:     60,
		Ratio:        "9:16",
		Style:        "editorial",
		WardrobeMode: "fixed",
	}
}

// TestLiveCommerceWine60sPark runs a paid 60s wine ad with storyboard + model-limited segment concat.
func TestLiveCommerceWine60sPark(t *testing.T) {
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
	cfg := loadProviderOverrides(providerDir, Config{DataDir: dir, Concurrency: liveCommerceConcurrency()})
	if cfg.APIKey == "" || cfg.BaseURL == "" || cfg.VideoModel == "" || cfg.LLMModel == "" {
		t.Fatal("configure generation credentials and models first")
	}
	store, err := NewStore(dir)
	if err != nil {
		t.Fatal(err)
	}
	a := newLiveCommerceApp(cfg, store)
	base := sphWine60sParkProject()
	id := base.ID
	if os.Getenv("VOWFILM_LIVE_FORCE") == "1" {
		_ = os.RemoveAll(filepath.Join(dir, id))
		_ = os.Remove(filepath.Join(dir, "projects.json"))
		store, err = NewStore(dir)
		if err != nil {
			t.Fatal(err)
		}
		a.store = store
	}
	p := store.Get(id)
	if p == nil {
		base.Status = "draft"
		base.Demo = true
		base.Revision = 1
		base.GenerationBudget = 240
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
			{characterImage, "sph-jianxuan-host.png", "视频号鉴选官人物卡.png", "reference"},
		} {
			raw, readErr := os.ReadFile(item.src)
			if readErr != nil {
				t.Fatal(readErr)
			}
			if err = os.WriteFile(filepath.Join(projectDir, item.file), raw, 0600); err != nil {
				t.Fatal(err)
			}
			p.Assets = append(p.Assets, Asset{
				ID: "asset_" + item.role, Name: item.name, Role: item.role,
				File: item.file, MIME: "image/png", URL: mediaURL(id, item.file),
			})
		}
		if err = store.Put(p); err != nil {
			t.Fatal(err)
		}
	}
	t.Log("videoModel:", p.VideoModel, "maxSegment:", videoModelMaxSegmentSeconds(p.VideoModel))
	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Minute)
	defer cancel()
	var runErr error
	if p.Status != "completed" {
		done := make(chan error, 1)
		go func() { done <- a.run(ctx, id, "generate") }()
		ticker := time.NewTicker(15 * time.Second)
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
				units := 0
				for _, s := range q.Shots {
					if s.GenerateUnit {
						units++
						states = append(states, s.ID+":"+s.Status+"(unit:"+s.GenerateGroup+")")
					}
				}
				states = append(states, "units="+strconv.Itoa(units))
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
	source := filepath.Join(dir, id, filepath.Base(p.FilmURL))
	video, err := os.ReadFile(source)
	if err != nil {
		t.Fatal(err)
	}
	target := filepath.Join(dir, "commerce-sph-wine-park-60s.mp4")
	if err = os.WriteFile(target, video, 0600); err != nil {
		t.Fatal(err)
	}
	var prompts []string
	for _, s := range p.Shots {
		if s.GenerateUnit {
			prompts = append(prompts, fmt.Sprintf("=== %s (%ds) ===\n%s", s.ID, s.GenerateSeconds, s.Prompt))
		}
	}
	_ = os.WriteFile(filepath.Join(dir, "prompt.txt"), []byte(strings.Join(prompts, "\n\n")), 0600)
	manifest := map[string]any{
		"video":               filepath.Base(target),
		"duration_seconds":    60,
		"ratio":               "9:16",
		"storyboard_shots":    len(p.Shots),
		"generate_units":      commerceGenerateUnitCount(p.Shots),
		"max_segment_seconds": videoModelMaxSegmentSeconds(p.VideoModel),
		"consistency":         "same park + host card + product ref across units",
		"fictional_product":   true,
		"video_model":         p.VideoModel,
		"llm_model":           p.LLMModel,
		"prompt_policy":       p.PromptPolicy,
		"generation_mode":     p.GenerationMode,
	}
	raw, _ = json.MarshalIndent(manifest, "", "  ")
	_ = os.WriteFile(filepath.Join(dir, "manifest.json"), raw, 0600)
	t.Log("Saved 60s film:", target, "shots:", len(p.Shots), "units:", commerceGenerateUnitCount(p.Shots))
}
