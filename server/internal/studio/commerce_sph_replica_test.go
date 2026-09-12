package studio

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// 复刻微信视频号 AXAiTdLYJ5：户外口播开场 + 快语速 + 底部口播字幕 + 中段酒瓶 + CTA（虚构样片）。
func sphWineReplicaProject() *Project {
	return &Project{
		ID:       "film_ad_sph_wine_park_v2",
		Scene:    "commerce",
		Occasion: "product",
		Title:    "喜欢喝酒的朋友注意了（视频号复刻）",
		Brief: "虚构验收样片，结构参考 https://weixin.qq.com/sph/AXAiTdLYJ5 。商品：虚构「老仓醇」透明圆瓶、红旋盖、米黄简标。样片台词含「三到五折起买酒」仅为复刻句式，非真实活动。禁止饮酒画面。",
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
	cfg := loadProviderOverrides(providerDir, Config{DataDir: dir, Concurrency: 2})
	if cfg.APIKey == "" || cfg.BaseURL == "" || cfg.VideoModel == "" || cfg.LLMModel == "" {
		t.Fatal("configure generation credentials and models first")
	}
	store, err := NewStore(dir)
	if err != nil {
		t.Fatal(err)
	}
	a := &App{cfg: cfg, store: store, provider: newProvider(cfg), slots: make(chan struct{}, 2), running: map[string]context.CancelFunc{}}
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
	if err = os.WriteFile(target, video, 0600); err != nil {
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
