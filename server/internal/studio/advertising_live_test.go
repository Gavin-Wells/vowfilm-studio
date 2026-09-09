package studio

// Explicitly opt-in live acceptance run. Never invoked by normal go test.
// Uses isolated local project storage and the production creative/video/render pipeline.
import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestLiveCommerceVideo(t *testing.T) {
	if os.Getenv("VOWFILM_LIVE_COMMERCE") != "1" {
		t.Skip("set VOWFILM_LIVE_COMMERCE=1 for a paid upstream acceptance run")
	}
	dir := os.Getenv("VOWFILM_LIVE_OUTPUT_DIR")
	providerDir := os.Getenv("VOWFILM_LIVE_PROVIDER_DIR")
	reference := os.Getenv("VOWFILM_LIVE_PRODUCT_IMAGE")
	if dir == "" || providerDir == "" || reference == "" {
		t.Fatal("explicit isolated output, provider directory and fictional product image required")
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
	const id = "film_ad_prompt_v3_sample"
	p := store.Get(id)
	if p == nil {
		p = &Project{ID: id, Scene: "commerce", Occasion: "product", Title: "出门前的小麻烦 · v3广告测试", Brief: "虚构教学商品：一只米白奶油色哑光塑料桌面收纳盒，圆角长方形，恰好三个等宽并排隔层、两块隔板，一个透明铰链盖。参考图为本次AI生成的虚构商品，不代表任何真实品牌。卖点仅为分格收纳。无价格、优惠、认证、尺寸参数和购买口号。卖点只有将小物按类别分格放置后便于找取。", CustomPrompt: "一次直出15秒完整9:16真人生活感广告，带原生声音，允许切镜。以<图片1>只绑定商品外观结构，场景为明亮卧室梳妆台。只出现一位成年女性的双手与浅米色袖口，不露脸；手是演示者，不是摆拍道具。前三秒在桌面少量发圈、发夹中翻找，动作有出门前的小焦急；中段通过俯拍和手部特写切镜，明确展示三格分别放发圈、发夹、耳钉；结尾手从收纳盒中轻松拿出一根发圈离开画面，留下整洁桌面。全片只一只参考盒，三格两隔板、透明铰链盖固定打开约65度，不拆盖不关盖；把小物件拿起再放下，不凭空消失。同一位年轻成年女性作为独立画外旁白，口语轻松，逐字保留以下台词并在15秒内说完、留停顿：‘出门前，又找不到发圈？发圈、发夹、耳钉，分格放好。要用的时候，一眼就找到。’不安排画面人物对嘴；旁白跨切镜连续，清晰高于轻柔背景音乐，保留翻找和放下物件的轻响。无字幕、无品牌字卡、水印、额外Logo或购买口号。自然收尾，不要三个静态产品展示镜头。", Duration: 15, Style: "editorial", Ratio: "9:16", WardrobeMode: "fixed", Status: "draft", Demo: true, Revision: 1, GenerationBudget: 15, OutputResolution: "720P", Shots: []Shot{}, Assets: []Asset{}, Events: []Event{}, CreatedAt: now(), UpdatedAt: now(), LLMModel: cfg.LLMModel, VideoModel: cfg.VideoModel}
		projectDir := filepath.Join(dir, id)
		if err = os.MkdirAll(projectDir, 0700); err != nil {
			t.Fatal(err)
		}
		raw, err := os.ReadFile(reference)
		if err != nil {
			t.Fatal(err)
		}
		if err = os.WriteFile(filepath.Join(projectDir, "fictional-product.png"), raw, 0600); err != nil {
			t.Fatal(err)
		}
		p.Assets = []Asset{{ID: "asset_test_product", Name: "虚构收纳盒参考图.png", Role: "product", File: "fictional-product.png", MIME: "image/png", URL: mediaURL(id, "fictional-product.png")}}
		if err = store.Put(p); err != nil {
			t.Fatal(err)
		}
	}
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Minute)
	defer cancel()
	if p.Status != "completed" {
		done := make(chan error, 1)
		go func() { done <- a.run(ctx, id, "generate") }()
		ticker := time.NewTicker(10 * time.Second)
		defer ticker.Stop()
		previous := ""
	run:
		for {
			select {
			case err = <-done:
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
	if err != nil {
		t.Fatal(err)
	}
	if p.Status != "completed" || p.FilmURL == "" {
		t.Fatal("no completed film")
	}
	if len(p.Shots) != 1 || p.GeneratedSeconds != 15 || !isDirectCommerce(p) {
		t.Fatal("not one direct 15-second task")
	}
	source := filepath.Join(dir, id, filepath.Base(p.FilmURL))
	raw, err = os.ReadFile(source)
	if err != nil {
		t.Fatal(err)
	}
	target := filepath.Join(dir, "commerce-v3-test-15s.mp4")
	if err = os.WriteFile(target, raw, 0600); err != nil {
		t.Fatal(err)
	}
	manifest := map[string]any{"video": filepath.Base(target), "duration_seconds": 15, "ratio": "9:16", "fictional_product": true, "reference": "fictional-product.png (AI-generated)", "video_model": p.VideoModel, "llm_model": p.LLMModel, "prompt_policy": p.PromptPolicy, "generated_seconds": p.GeneratedSeconds, "audio": "native video-model audio, no post-production replacement", "generation_mode": p.GenerationMode, "video_tasks": len(p.Shots), "billing": "isolated acceptance run; upstream API usage; no platform wallet adjustment"}
	raw, _ = json.MarshalIndent(manifest, "", "  ")
	_ = os.WriteFile(filepath.Join(dir, "manifest.json"), raw, 0600)
	t.Log("Saved actual AI test video:", target)
}
