package studio

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"vowfilm/server/internal/domain"
	"vowfilm/server/internal/wedding"
)

type weddingMixSettings struct {
	VoiceFile string  `json:"voiceFile"`
	MusicFile string  `json:"musicFile"`
	VoiceSHA  string  `json:"voiceSha256"`
	MusicSHA  string  `json:"musicSha256"`
	Gain      float64 `json:"gain"`
}

func fileDigest(data []byte) string { sum := sha256.Sum256(data); return hex.EncodeToString(sum[:]) }
func ffmpegDecode(ctx context.Context, path string) error {
	return ffmpeg(ctx, "-i", path, "-f", "null", "-")
}

func (a *App) weddingMix(ctx context.Context, p *Project, settings weddingMixSettings, start, length float64, name string) error {
	if filepath.Base(settings.VoiceFile) != settings.VoiceFile || filepath.Base(settings.MusicFile) != settings.MusicFile {
		return errors.New("音轨路径无效")
	}
	dir := filepath.Join(a.cfg.DataDir, p.ID)
	for _, entry := range []struct{ File, SHA string }{{settings.VoiceFile, settings.VoiceSHA}, {settings.MusicFile, settings.MusicSHA}} {
		data, err := os.ReadFile(filepath.Join(dir, entry.File))
		if err != nil {
			return err
		}
		if fileDigest(data) != entry.SHA {
			return errors.New("已试听音轨发生变化，请重新确认")
		}
	}
	musicSeconds, err := probeAudio(ctx, filepath.Join(dir, settings.MusicFile))
	if err != nil {
		return err
	}
	if musicSeconds+0.05 < float64(p.Duration) {
		return errors.New("音乐不足以覆盖全片，请补充完整音乐源")
	}
	if math.IsNaN(settings.Gain) || settings.Gain <= 0 || settings.Gain > 1 {
		return errors.New("混音音量无效")
	}
	graph := fmt.Sprintf("[0:a]aresample=48000,apad=whole_dur=%d,atrim=0:%d,asplit=2[vo][side];[1:a]aresample=48000,atrim=0:%d,asetpts=PTS-STARTPTS,volume=%.4f[bg];[bg][side]sidechaincompress=threshold=0.025:ratio=6:attack=20:release=350[duck];[vo][duck]amix=inputs=2:duration=first:normalize=0,alimiter=limit=0.89:level=0,afade=t=out:st=%.4f:d=2,atrim=start=%.4f:duration=%.4f,asetpts=PTS-STARTPTS[a]", p.Duration, p.Duration, p.Duration, settings.Gain, float64(p.Duration)-2, start, length)
	return ffmpeg(ctx, "-i", filepath.Join(dir, settings.VoiceFile), "-i", filepath.Join(dir, settings.MusicFile), "-filter_complex", graph, "-map", "[a]", "-ar", "48000", "-ac", "2", "-c:a", "pcm_s16le", filepath.Join(dir, name))
}

func (a *App) renderWeddingStage(ctx context.Context, p *Project) error {
	n := p.Wedding.CurrentStep
	var arts []domain.WeddingArtifact
	dir := filepath.Join(a.cfg.DataDir, p.ID)
	add := func(file, name, kind, mime string) error {
		b, err := os.ReadFile(filepath.Join(dir, file))
		if err != nil {
			return err
		}
		art, err := a.weddingArtifact(p, name, kind, "", b, mime)
		if err == nil {
			arts = append(arts, art)
		}
		return err
	}
	if n == 11 || n == 12 {
		var settings weddingMixSettings
		if n == 11 {
			music, err := selectedWeddingArtifact(p.Wedding.Steps[9], "music")
			if err != nil {
				return err
			}
			for i := range p.Wedding.Steps[10].Artifacts {
				art := &p.Wedding.Steps[10].Artifacts[i]
				if art.Kind == "music" {
					music = art
					arts = append(arts, *art)
				}
			}
			if music == nil {
				return errors.New("请先确认音乐源")
			}
			voice, err := os.ReadFile(filepath.Join(dir, p.Wedding.NarrationFile))
			if err != nil {
				return err
			}
			settings = weddingMixSettings{VoiceFile: p.Wedding.NarrationFile, MusicFile: music.File, VoiceSHA: fileDigest(voice), MusicSHA: music.SHA256, Gain: 0.16}
		} else {
			for _, art := range p.Wedding.Steps[10].Artifacts {
				if art.Kind == "settings" {
					if err := json.Unmarshal([]byte(art.Text), &settings); err != nil {
						return err
					}
				}
			}
			if settings.VoiceFile == "" {
				return errors.New("缺少已确认的试听混音参数")
			}
		}
		starts := []float64{0}
		length := float64(p.Duration)
		if n == 11 {
			starts = []float64{0, float64(p.Duration)/2 - 7, float64(p.Duration) - 15}
			length = 15
		}
		for i, start := range starts {
			name := fmt.Sprintf("wedding-mix-%d-%s.wav", i, newID(""))
			if err := a.weddingMix(ctx, p, settings, start, length, name); err != nil {
				return err
			}
			if err := add(name, fmt.Sprintf("mix-%02d.wav", i+1), "mix", "audio/wav"); err != nil {
				return err
			}
		}
		b, _ := json.MarshalIndent(settings, "", "  ")
		art, err := a.weddingArtifact(p, "MIX_SETTINGS.json", "settings", "", b, "application/json")
		if err != nil {
			return err
		}
		arts = append(arts, art)
	} else {
		file, err := a.renderWedding(ctx, p)
		if err != nil {
			return err
		}
		kind := "preview"
		if n == 14 {
			kind = "final"
		}
		if err := add(file, kind+".mp4", kind, "video/mp4"); err != nil {
			return err
		}
		if n == 14 {
			if err := add(file+".srt", "captions.srt", "subtitles", "text/plain"); err != nil {
				return err
			}
			if err := add(file+".edit.json", "EDIT_PLAN.json", "edit", "application/json"); err != nil {
				return err
			}
		}
	}
	return a.store.Update(p.ID, func(q *Project) error {
		if err := wedding.Prepare(q.Wedding, n, arts); err != nil {
			return err
		}
		q.Status = "planned"
		if n >= 13 {
			q.FilmURL = arts[0].URL
		}
		event(q, fmt.Sprintf("第%d步已完成处理，请完整听看后确认", n))
		return nil
	})
}

func (a *App) renderWedding(ctx context.Context, p *Project) (string, error) {
	if err := weddingLayout(p); err != nil {
		return "", err
	}
	if p.Wedding.MixFile == "" || filepath.Base(p.Wedding.MixFile) != p.Wedding.MixFile {
		return "", errors.New("缺少已确认的完整混音")
	}
	dir := filepath.Join(a.cfg.DataDir, p.ID)
	width, height := 1280, 720
	if p.Ratio == "9:16" {
		width, height = 720, 1280
	}
	job := newID("wedding-")
	var concat strings.Builder
	for i, s := range p.Shots {
		if s.Status != "completed" || s.VideoFile == "" || filepath.Base(s.VideoFile) != s.VideoFile {
			return "", fmt.Errorf("%s 视频尚未就绪", s.ID)
		}
		src := filepath.Join(dir, s.VideoFile)
		info, err := probe(ctx, src)
		if err != nil {
			return "", err
		}
		if info.Duration+0.04 < s.EditSeconds {
			return "", fmt.Errorf("%s 视频时长不足", s.ID)
		}
		part := fmt.Sprintf("%s-%02d.mp4", job, i)
		vf := fmt.Sprintf("fps=24,scale=%d:%d:force_original_aspect_ratio=decrease,pad=%d:%d:(ow-iw)/2:(oh-ih)/2,setsar=1,setpts=PTS-STARTPTS", width, height, width, height)
		if err := ffmpeg(ctx, "-i", src, "-an", "-vf", vf, "-frames:v", strconv.Itoa(s.EditFrames), "-c:v", "libx264", "-preset", "fast", "-crf", "18", "-pix_fmt", "yuv420p", "-threads", "2", filepath.Join(dir, part)); err != nil {
			return "", err
		}
		fmt.Fprintf(&concat, "file '%s'\n", part)
	}
	list := filepath.Join(dir, job+".concat.txt")
	if err := os.WriteFile(list, []byte(concat.String()), 0600); err != nil {
		return "", err
	}
	ass, srt := weddingCaptions(p, width, height)
	assFile := filepath.Join(dir, job+".ass")
	if err := os.WriteFile(assFile, []byte(ass), 0600); err != nil {
		return "", err
	}
	name := job + ".mp4"
	dest := filepath.Join(dir, name)
	if err := ffmpeg(ctx, "-f", "concat", "-safe", "1", "-i", list, "-i", filepath.Join(dir, p.Wedding.MixFile), "-vf", "subtitles=filename='"+strings.ReplaceAll(assFile, "'", "\\'")+"'", "-map", "0:v:0", "-map", "1:a:0", "-frames:v", strconv.Itoa(p.Duration*24), "-t", strconv.Itoa(p.Duration), "-c:v", "libx264", "-preset", "fast", "-crf", "18", "-pix_fmt", "yuv420p", "-threads", "2", "-c:a", "aac", "-b:a", "192k", "-ar", "48000", "-ac", "2", "-movflags", "+faststart", dest); err != nil {
		return "", err
	}
	info, err := probe(ctx, dest)
	if err != nil {
		return "", err
	}
	if math.Abs(info.Duration-float64(p.Duration)) > 0.06 || info.Width != width || info.Height != height {
		return "", errors.New("成片技术规格不匹配")
	}
	if err := ffmpegDecode(ctx, dest); err != nil {
		return "", err
	}
	if err := os.WriteFile(dest+".srt", []byte(srt), 0600); err != nil {
		return "", err
	}
	edl, _ := json.MarshalIndent(map[string]any{"policy": p.PromptPolicy, "fps": 24, "duration": p.Duration, "shots": p.Shots, "cues": p.Wedding.Cues, "mix": p.Wedding.MixFile}, "", "  ")
	if err := os.WriteFile(dest+".edit.json", edl, 0600); err != nil {
		return "", err
	}
	report, _ := json.MarshalIndent(map[string]any{"technical_validation": "passed", "duration_seconds": info.Duration, "frames": p.Duration * 24, "width": width, "height": height, "narration": "separate recorded audio with actual timing", "human_watch": "pending", "couple_acceptance": "pending", "source_policy": p.PromptPolicy}, "", "  ")
	_ = os.WriteFile(dest+".qc.json", report, 0600)
	return name, nil
}

func weddingCaptions(p *Project, width, height int) (string, string) {
	var ass, srt strings.Builder
	size := 36
	if height > width {
		size = 32
	}
	fmt.Fprintf(&ass, "[Script Info]\nScriptType: v4.00+\nPlayResX: %d\nPlayResY: %d\nWrapStyle: 0\n\n[V4+ Styles]\nFormat: Name, Fontname, Fontsize, PrimaryColour, SecondaryColour, OutlineColour, BackColour, Bold, Italic, Underline, StrikeOut, ScaleX, ScaleY, Spacing, Angle, BorderStyle, Outline, Shadow, Alignment, MarginL, MarginR, MarginV, Encoding\nStyle: Voice,Noto Sans CJK SC,%d,&H00F8FAFC,&H00FFFFFF,&H00151C24,&H60000000,0,0,0,0,100,100,1,0,1,2,1,2,80,80,58,1\nStyle: Title,Noto Serif CJK SC,46,&H00F8FAFC,&H00FFFFFF,&H00151C24,&H60000000,0,0,0,0,100,100,2,0,1,2,1,8,80,80,68,1\n\n[Events]\nFormat: Layer, Start, End, Style, Name, MarginL, MarginR, MarginV, Effect, Text\n", width, height, size)
	for i, c := range p.Wedding.Cues {
		text := strings.TrimSpace(c.Text)
		r := []rune(text)
		if len(r) > 20 && len(r) <= 40 {
			text = string(r[:20]) + "\n" + string(r[20:])
		}
		fmt.Fprintf(&ass, "Dialogue: 0,%s,%s,Voice,,0,0,0,,%s\n", assTime(c.Start), assTime(c.End), assEscape(text))
		fmt.Fprintf(&srt, "%d\n%s --> %s\n%s\n\n", i+1, srtTime(c.Start), srtTime(c.End), text)
	}
	if p.EndingText != "" {
		fmt.Fprintf(&ass, "Dialogue: 1,%s,%s,Title,,0,0,0,,{\\fad(400,0)}%s\n", assTime(float64(p.Duration)-4), assTime(float64(p.Duration)), assEscape(p.EndingText))
	}
	if p.Demo {
		fmt.Fprintf(&ass, "Dialogue: 1,%s,%s,Voice,,0,0,0,,{\\fs18\\an7\\pos(44,32)}AI 模板演示 · 虚构人物与故事\n", assTime(0), assTime(5))
	}
	return ass.String(), srt.String()
}
func srtTime(seconds float64) string {
	ms := int(math.Round(seconds * 1000))
	return fmt.Sprintf("%02d:%02d:%02d,%03d", ms/3600000, ms/60000%60, ms/1000%60, ms%1000)
}
