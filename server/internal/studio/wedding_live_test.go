package studio

// Paid, opt-in demo production against the configured provider. Uses isolated
// storage, never changes platform wallets or records fictional human approvals.
import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"
)

func TestLiveWeddingGuidedDemo(t *testing.T) {
	if os.Getenv("VOWFILM_LIVE_WEDDING") != "1" {
		t.Skip("explicit paid demo opt-in required")
	}
	root, providerDir := os.Getenv("VOWFILM_WEDDING_OUTPUT"), os.Getenv("VOWFILM_WEDDING_PROVIDER")
	if root == "" || providerDir == "" {
		t.Fatal("isolated output and provider paths required")
	}
	dir := filepath.Join(root, "production")
	cfg := loadProviderOverrides(providerDir, Config{DataDir: dir, Concurrency: 2})
	if cfg.APIKey == "" || cfg.BaseURL == "" {
		t.Fatal("configure provider first")
	}
	store, err := NewStore(dir)
	if err != nil {
		t.Fatal(err)
	}
	a := &App{cfg: cfg, store: store, provider: newProvider(cfg), slots: make(chan struct{}, 2), running: map[string]context.CancelFunc{}}
	const id = "film_wedding_story_guided_demo"
	projectDir := filepath.Join(dir, id)
	if err = os.MkdirAll(projectDir, 0700); err != nil {
		t.Fatal(err)
	}
	copyFile := func(src, dst string) error {
		b, e := os.ReadFile(src)
		if e != nil {
			return e
		}
		return os.WriteFile(dst, b, 0600)
	}
	p := store.Get(id)
	if p == nil {
		b, e := os.ReadFile(filepath.Join(root, "demo-project.json"))
		if e != nil {
			t.Fatal(e)
		}
		if e = json.Unmarshal(b, &p); e != nil {
			t.Fatal(e)
		}
		p.ID = id
		p.Demo = true
		p.Revision = 1
		p.CreatedAt = now()
		p.UpdatedAt = now()
		p.GenerationBudget = 180
		p.LLMModel = cfg.LLMModel
		p.VideoModel = cfg.VideoModel
		p.Status = "draft"
		weddingInit(p)
		if e = weddingLayout(p); e != nil {
			t.Fatal(e)
		}
		if e = store.Put(p); e != nil {
			t.Fatal(e)
		}
	}
	selected := os.Getenv("VOWFILM_WEDDING_SHOTS")
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Minute)
	defer cancel()
	var wg sync.WaitGroup
	var mu sync.Mutex
	var errs []error
	sem := make(chan struct{}, 4)
	for _, s := range p.Shots {
		if s.Status == "completed" || selected != "" && !strings.Contains(","+selected+",", ","+s.ID+",") {
			continue
		}
		if err = copyFile(filepath.Join(root, s.FirstFrameFile), filepath.Join(projectDir, s.FirstFrameFile)); err != nil {
			t.Fatal(err)
		}
		wg.Add(1)
		go func(s Shot) {
			defer wg.Done()
			sem <- struct{}{}
			defer func() { <-sem }()
			t.Log("Generating", s.ID, s.Duration, "seconds")
			e := a.generateShot(ctx, id, s.ID, nil, "")
			if e != nil {
				mu.Lock()
				errs = append(errs, e)
				mu.Unlock()
			} else {
				t.Log("Completed", s.ID)
			}
		}(s)
	}
	wg.Wait()
	p = store.Get(id)
	b, _ := json.MarshalIndent(p, "", "  ")
	_ = os.WriteFile(filepath.Join(root, "production-project.json"), b, 0600)
	for _, e := range errs {
		t.Error(e)
	}
	if t.Failed() || selected != "" {
		return
	}
	for _, s := range p.Shots {
		if s.Status != "completed" {
			t.Fatal("unfinished", s.ID)
		}
	}
	for _, name := range []string{"narration-source-1.wav", "music-full-source-1.wav"} {
		if err = copyFile(filepath.Join(root, name), filepath.Join(projectDir, name)); err != nil {
			t.Fatal(err)
		}
	}
	voice, _ := os.ReadFile(filepath.Join(projectDir, "narration-source-1.wav"))
	music, _ := os.ReadFile(filepath.Join(projectDir, "music-full-source-1.wav"))
	settings := weddingMixSettings{VoiceFile: "narration-source-1.wav", MusicFile: "music-full-source-1.wav", VoiceSHA: fileDigest(voice), MusicSHA: fileDigest(music), Gain: 0.16}
	for i, start := range []float64{0, 38, 73} {
		name := []string{"mix-opening.wav", "mix-turn.wav", "mix-ending.wav"}[i]
		if err = a.weddingMix(ctx, p, settings, start, 15, name); err != nil {
			t.Fatal(err)
		}
	}
	if err = a.weddingMix(ctx, p, settings, 0, float64(p.Duration), "full-mix.wav"); err != nil {
		t.Fatal(err)
	}
	p.Wedding.MixFile = "full-mix.wav"
	p.Wedding.NarrationFile = settings.VoiceFile
	file, err := a.renderWedding(ctx, p)
	if err != nil {
		t.Fatal(err)
	}
	for _, suffix := range []string{"", ".srt", ".edit.json", ".qc.json"} {
		if err = copyFile(filepath.Join(projectDir, file+suffix), filepath.Join(root, "wedding-story-guided-90s.mp4"+suffix)); err != nil {
			t.Fatal(err)
		}
	}
	p.FilmURL = mediaURL(id, file)
	p.Status = "completed"
	p.Progress = 100
	if err = store.Put(p); err != nil {
		t.Fatal(err)
	}
	b, _ = json.MarshalIndent(p, "", "  ")
	_ = os.WriteFile(filepath.Join(root, "production-project.json"), b, 0600)
	t.Log("Saved 90-second fictional template demo; human and couple acceptance remain pending", filepath.Join(root, "wedding-story-guided-90s.mp4"))
}
