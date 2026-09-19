package studio

import (
	"context"
	_ "embed"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"sync"
	"time"
	"vowfilm/server/internal/domain"
	"vowfilm/server/internal/wedding"
)

//go:embed wedding_align.py
var weddingAlignScript []byte

func checkWeddingAlignment() error {
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	if err := exec.CommandContext(ctx, "python3", "-c", "import faster_whisper").Run(); err != nil {
		return errors.New("一键生成需在服务端安装 faster-whisper 以对齐真实旁白时间点")
	}
	return nil
}

func (a *App) autoWeddingStep(id string, n int, arts []domain.WeddingArtifact, done bool) error {
	return a.store.Update(id, func(p *Project) error {
		p.Wedding.CurrentStep = n
		p.Status = "generating"
		if arts != nil {
			s := &p.Wedding.Steps[n-1]
			s.Artifacts = arts
			s.Version = wedding.Digest(arts)
			s.Approvals = []domain.WeddingApproval{}
			s.Status = "automated"
		}
		if done {
			p.Wedding.CurrentStep = min(14, n+1)
		}
		event(p, fmt.Sprintf("一键生成 %d/14：%s", n, wedding.Names[n-1]))
		return nil
	})
}

// Tasks and downloaded results are saved independently, so retry never submits
// another billable task for an already submitted request.
func (a *App) autoWeddingMedia(ctx context.Context, id, key, endpoint, name string, body map[string]any) (string, error) {
	p := a.store.Get(id)
	dir := filepath.Join(a.cfg.DataDir, id)
	_ = os.MkdirAll(dir, 0700)
	if file := p.Wedding.AutoFiles[key]; file != "" {
		if _, err := os.Stat(filepath.Join(dir, file)); err == nil {
			return file, nil
		}
	}
	task := p.Wedding.AutoTasks[key]
	if task == "" {
		out, err := a.provider.request(ctx, "POST", endpoint, body, fmt.Sprintf("vowfilm:%s:auto:%s:r%d", id, key, p.Revision))
		if err != nil {
			return "", err
		}
		task, _ = out["task_id"].(string)
		if task == "" {
			task, _ = out["id"].(string)
		}
		if task == "" {
			return "", errors.New("媒体服务没有返回任务 ID")
		}
		if err = a.store.Update(id, func(q *Project) error {
			if q.Wedding.AutoTasks == nil {
				q.Wedding.AutoTasks = map[string]string{}
			}
			q.Wedding.AutoTasks[key] = task
			return nil
		}); err != nil {
			return "", err
		}
	}
	poll := "/v1/tasks/" + task
	if endpoint == "/v1/images/tasks" {
		poll = "/v1/images/tasks/" + task
	}
	var source string
	for {
		out, err := a.provider.request(ctx, "GET", poll, nil, "")
		if err != nil {
			return "", err
		}
		status, _ := out["status"].(string)
		if status == "failed" || status == "cancelled" || status == "expired" {
			return "", fmt.Errorf("%s 任务未完成，任务记录已保留", key)
		}
		if status == "succeeded" || status == "completed" {
			results, _ := out["results"].([]any)
			for _, result := range results {
				m, _ := result.(map[string]any)
				source, _ = m["url"].(string)
				if source != "" {
					break
				}
			}
			if source == "" {
				return "", errors.New("媒体结果没有文件地址")
			}
			break
		}
		select {
		case <-ctx.Done():
			return "", ctx.Err()
		case <-time.After(6 * time.Second):
		}
	}
	if err := download(ctx, source, filepath.Join(dir, name)); err != nil {
		return "", err
	}
	err := a.store.Update(id, func(q *Project) error {
		if q.Wedding.AutoFiles == nil {
			q.Wedding.AutoFiles = map[string]string{}
		}
		q.Wedding.AutoFiles[key] = name
		return nil
	})
	return name, err
}

func (a *App) autoWeddingFile(p *Project, file, kind, shot string) (domain.WeddingArtifact, error) {
	b, err := os.ReadFile(filepath.Join(a.cfg.DataDir, p.ID, file))
	if err != nil {
		return domain.WeddingArtifact{}, err
	}
	return a.weddingArtifact(p, file, kind, shot, b, http.DetectContentType(b))
}

func (a *App) runWeddingAutomatic(ctx context.Context, id string) error {
	if err := checkWeddingAlignment(); err != nil {
		return err
	}
	p := a.store.Get(id)
	dir := filepath.Join(a.cfg.DataDir, id)
	_ = os.MkdirAll(dir, 0700)
	if p.VoiceoverScript == "" {
		if err := a.autoWeddingStep(id, 2, nil, false); err != nil {
			return err
		}
		p = a.store.Get(id)
		if err := a.planWedding(ctx, p); err != nil {
			return err
		}
		if err := a.store.Update(id, func(q *Project) error { return a.bindWeddingStep(ctx, q, 2) }); err != nil {
			return err
		}
		p = a.store.Get(id)
		if err := a.autoWeddingStep(id, 2, p.Wedding.Steps[1].Artifacts, true); err != nil {
			return err
		}
	}
	p = a.store.Get(id)
	if p.Wedding.NarrationFile == "" {
		if err := a.autoWeddingStep(id, 5, nil, false); err != nil {
			return err
		}
		prompt := fmt.Sprintf("生成纯中文婚礼故事旁白。自然温暖的成年女声、自然讲述、适度停顿、避免播音腔。全文在约%d秒内完整读完，不赶字不漏字。不唱歌，无配乐，无环境音。严格逐字朗读下面正文，不读标题，不添加任何文字：\n%s", p.Duration-5, p.VoiceoverScript)
		file, err := a.autoWeddingMedia(ctx, id, "narration", "/v1/audio/generate", "auto-narration.wav", map[string]any{"model": defaultAudioModel, "prompt": prompt, "audio_config": map[string]any{"format": "wav", "sample_rate": 48000, "enable_subtitle": true}})
		if err != nil {
			return err
		}
		seconds, err := probeAudio(ctx, filepath.Join(dir, file))
		if err != nil {
			return err
		}
		if seconds > float64(p.Duration) {
			return errors.New("实际旁白长于目标片长，请缩短原稿后重试")
		}
		alignPath := filepath.Join(dir, "align-narration.py")
		scriptPath := filepath.Join(dir, "auto-script.txt")
		timingPath := filepath.Join(dir, "auto-timing.json")
		if err = os.WriteFile(alignPath, weddingAlignScript, 0600); err != nil {
			return err
		}
		if err = os.WriteFile(scriptPath, []byte(p.VoiceoverScript), 0600); err != nil {
			return err
		}
		if out, e := exec.CommandContext(ctx, "python3", alignPath, filepath.Join(dir, file), scriptPath, timingPath).CombinedOutput(); e != nil {
			message := string(out)
			if len(message) > 500 {
				message = message[len(message)-500:]
			}
			return fmt.Errorf("旁白实际时间点对齐失败：%s", message)
		}
		b, err := os.ReadFile(timingPath)
		if err != nil {
			return err
		}
		var timing struct {
			Cues []domain.SubtitleCue `json:"cues"`
		}
		if err = json.Unmarshal(b, &timing); err != nil {
			return err
		}
		if err = validateWeddingCues(timing.Cues, seconds); err != nil {
			return err
		}
		if err = a.store.Update(id, func(q *Project) error {
			q.Wedding.NarrationFile = file
			q.Wedding.NarrationURL = mediaURL(id, file)
			q.Wedding.NarrationSeconds = seconds
			q.Wedding.Cues = timing.Cues
			return nil
		}); err != nil {
			return err
		}
		narration, err := a.autoWeddingFile(p, file, "narration", "")
		if err != nil {
			return err
		}
		times, err := a.weddingArtifact(p, "NARRATION_TIMING.json", "timing", "", b, "application/json")
		if err != nil {
			return err
		}
		if err = a.autoWeddingStep(id, 5, []domain.WeddingArtifact{narration, times}, true); err != nil {
			return err
		}
	}
	p = a.store.Get(id)
	if len(p.Shots) == 0 {
		if err := a.autoWeddingStep(id, 6, nil, false); err != nil {
			return err
		}
		if err := a.planWedding(ctx, a.store.Get(id)); err != nil {
			return err
		}
		if err := a.store.Update(id, func(q *Project) error { return a.bindWeddingStep(ctx, q, 6) }); err != nil {
			return err
		}
		p = a.store.Get(id)
		if err := a.autoWeddingStep(id, 6, p.Wedding.Steps[5].Artifacts, true); err != nil {
			return err
		}
	}
	p = a.store.Get(id)
	if err := a.autoWeddingStep(id, 8, nil, false); err != nil {
		return err
	}
	var identityRefs []map[string]string
	for _, art := range p.Wedding.Steps[0].Artifacts {
		if art.Kind == "photo" && len(identityRefs) < 4 {
			b, err := os.ReadFile(filepath.Join(dir, art.File))
			if err != nil {
				return err
			}
			identityRefs = append(identityRefs, map[string]string{"url": "data:" + art.MIME + ";base64," + base64.StdEncoding.EncodeToString(b)})
		}
	}
	pictures := make([]domain.WeddingArtifact, len(p.Shots))
	buildFrame := func(i int, s Shot) error {
		file := s.FirstFrameFile
		if file == "" {
			body := map[string]any{"model": "aws/gpt-image-2", "prompt": s.ImagePrompt, "resolution": "1K", "aspect_ratio": p.Ratio, "quality": "medium", "output_format": "png", "n": 1}
			if len(identityRefs) > 0 {
				body["reference_images"] = identityRefs
			}
			var err error
			file, err = a.autoWeddingMedia(ctx, id, "frame-"+s.ID, "/v1/images/tasks", "auto-"+s.ID+".png", body)
			if err != nil {
				return err
			}
			if err = ffmpeg(ctx, "-i", filepath.Join(dir, file), "-frames:v", "1", "-f", "null", "-"); err != nil {
				return err
			}
			if err = a.store.Update(id, func(q *Project) error { q.Shots[i].FirstFrameFile = file; return nil }); err != nil {
				return err
			}
		}
		art, err := a.autoWeddingFile(p, file, "image", s.ID)
		if err != nil {
			return err
		}
		pictures[i] = art
		return nil
	}
	if err := buildFrame(0, p.Shots[0]); err != nil {
		return err
	}
	first := a.store.Get(id).Shots[0].FirstFrameFile
	firstBytes, err := os.ReadFile(filepath.Join(dir, first))
	if err != nil {
		return err
	}
	identityRefs = append(identityRefs, map[string]string{"url": "data:image/png;base64," + base64.StdEncoding.EncodeToString(firstBytes)})
	var imageWG sync.WaitGroup
	var imageMu sync.Mutex
	var imageErrors []error
	imageSlots := make(chan struct{}, max(1, a.cfg.Concurrency))
	for i, s := range p.Shots {
		if i == 0 {
			continue
		}
		imageWG.Add(1)
		go func(i int, s Shot) {
			defer imageWG.Done()
			imageSlots <- struct{}{}
			defer func() { <-imageSlots }()
			if err := buildFrame(i, s); err != nil {
				imageMu.Lock()
				imageErrors = append(imageErrors, err)
				imageMu.Unlock()
			}
		}(i, s)
	}
	imageWG.Wait()
	if len(imageErrors) > 0 {
		return imageErrors[0]
	}
	if err := a.autoWeddingStep(id, 8, pictures, true); err != nil {
		return err
	}
	p = a.store.Get(id)
	if err := a.autoWeddingStep(id, 9, nil, false); err != nil {
		return err
	}
	var wg sync.WaitGroup
	var mu sync.Mutex
	var failures []error
	sem := make(chan struct{}, max(1, a.cfg.Concurrency))
	for _, s := range p.Shots {
		if s.Status == "completed" {
			continue
		}
		wg.Add(1)
		go func(s Shot) {
			defer wg.Done()
			sem <- struct{}{}
			defer func() { <-sem }()
			if err := a.generateShot(ctx, id, s.ID, nil, ""); err != nil {
				mu.Lock()
				failures = append(failures, err)
				mu.Unlock()
			}
		}(s)
	}
	wg.Wait()
	if len(failures) > 0 {
		return failures[0]
	}
	p = a.store.Get(id)
	var videos []domain.WeddingArtifact
	for _, s := range p.Shots {
		art, err := a.autoWeddingFile(p, s.VideoFile, "video", s.ID)
		if err != nil {
			return err
		}
		videos = append(videos, art)
	}
	if err := a.autoWeddingStep(id, 9, videos, true); err != nil {
		return err
	}
	if err := a.autoWeddingStep(id, 10, nil, false); err != nil {
		return err
	}
	musicPrompt := fmt.Sprintf("创作完整连续%d秒纯器乐婚礼故事配乐。%s。温暖自然的felt piano与轻弦乐，76 BPM，稀疏旋律，为中文旁白留白。开场轻柔、日常发展、牵手转折、结尾温柔收束，情绪变化循序渐进。不要人声、哼唱、对白、歌词、重鼓。不可循环片段或尾部补静音。", p.Duration+5, p.Synopsis)
	music, err := a.autoWeddingMedia(ctx, id, "music", "/v1/audio/generate", "auto-music.wav", map[string]any{"model": defaultAudioModel, "prompt": musicPrompt, "audio_config": map[string]any{"format": "wav", "sample_rate": 48000}})
	if err != nil {
		return err
	}
	voice, _ := os.ReadFile(filepath.Join(dir, p.Wedding.NarrationFile))
	bg, _ := os.ReadFile(filepath.Join(dir, music))
	settings := weddingMixSettings{VoiceFile: p.Wedding.NarrationFile, MusicFile: music, VoiceSHA: fileDigest(voice), MusicSHA: fileDigest(bg), Gain: 0.16}
	if err = a.autoWeddingStep(id, 11, nil, false); err != nil {
		return err
	}
	var samples []domain.WeddingArtifact
	for i, start := range []float64{0, float64(p.Duration)/2 - 7, float64(p.Duration) - 15} {
		name := fmt.Sprintf("auto-mix-sample-%d.wav", i)
		if err = a.weddingMix(ctx, p, settings, start, 15, name); err != nil {
			return err
		}
		art, e := a.autoWeddingFile(p, name, "mix", "")
		if e != nil {
			return e
		}
		samples = append(samples, art)
	}
	settingsJSON, _ := json.MarshalIndent(settings, "", "  ")
	settingArtifact, err := a.weddingArtifact(p, "MIX_SETTINGS.json", "settings", "", settingsJSON, "application/json")
	if err != nil {
		return err
	}
	samples = append(samples, settingArtifact)
	if err = a.autoWeddingStep(id, 11, samples, true); err != nil {
		return err
	}
	if err = a.weddingMix(ctx, p, settings, 0, float64(p.Duration), "auto-full-mix.wav"); err != nil {
		return err
	}
	p.Wedding.MixFile = "auto-full-mix.wav"
	if err = a.autoWeddingStep(id, 13, nil, false); err != nil {
		return err
	}
	file, err := a.renderWedding(ctx, p)
	if err != nil {
		return err
	}
	var finals []domain.WeddingArtifact
	for _, entry := range []struct{ File, Name, Kind, MIME string }{{file, "final.mp4", "final", "video/mp4"}, {file + ".srt", "captions.srt", "subtitles", "text/plain"}, {file + ".edit.json", "EDIT_PLAN.json", "edit", "application/json"}} {
		b, e := os.ReadFile(filepath.Join(dir, entry.File))
		if e != nil {
			return e
		}
		art, e := a.weddingArtifact(p, entry.Name, entry.Kind, "", b, entry.MIME)
		if e != nil {
			return e
		}
		finals = append(finals, art)
	}
	return a.store.Update(id, func(q *Project) error {
		q.Wedding.CurrentStep = 14
		q.Wedding.MixFile = p.Wedding.MixFile
		for i := 0; i < 13; i++ {
			q.Wedding.Steps[i].Status = "automated"
		}
		q.Wedding.Steps[13].Artifacts = finals
		q.Wedding.Steps[13].Version = wedding.Digest(finals)
		q.Wedding.Steps[13].Status = "awaiting_confirmation"
		q.FilmURL = finals[0].URL
		q.Wedding.SubtitleURL = finals[1].URL
		q.Status = "completed"
		q.Progress = 100
		event(q, "一键成片已完成：完整视频、旁白、配乐和字幕已输出；可下载后审阅")
		return nil
	})
}
