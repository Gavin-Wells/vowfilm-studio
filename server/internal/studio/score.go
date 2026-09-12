package studio

import (
	"context"
	"crypto/sha256"
	"encoding/binary"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"time"
	"vowfilm/server/internal/templates"
)

// The score is designed before any audio is requested: every section follows
// a picture unit (template chapter or treatment act), carries its own
// instrumentation and energy, and names the event at its first beat plus the
// hand-over into the next section. The audio model receives that whole cue
// sheet in one prompt per cue, so passages are composed as a single piece
// instead of being stitched from unrelated loops.

// maxCueSeconds keeps each audio request under the model's 120-second limit
// with headroom for the model's own tail.
const maxCueSeconds = 110

// maxAudioPromptRunes is the upstream text_prompt limit.
const maxAudioPromptRunes = 2000

func scoreSections(p *Project) []MusicSection {
	if sceneID(p) == "commerce" {
		return nil
	}
	if p.CreationMode == "template" {
		if sections := templateSections(p); len(sections) > 0 {
			return sections
		}
	}
	if p.Treatment != nil && len(p.Treatment.Acts) >= 2 {
		if sections := treatmentSections(p); len(sections) > 0 {
			return sections
		}
	}
	return legacySections(p)
}

func snapSeconds(v float64) float64 { return math.Round(v*24) / 24 }

// templateSections derives chapter boundaries from the catalog's canonical
// editFrames, so the score design exists before shots are laid out and stays
// identical across plan/render retries.
func templateSections(p *Project) []MusicSection {
	t, ok := templates.FindVersion(p.TemplateID, p.TemplateVersion)
	if !ok || len(t.Chapters) == 0 || len(t.Shots) == 0 {
		return nil
	}
	frames := make([]int, len(t.Chapters))
	total := 0
	for _, s := range t.Shots {
		if s.Chapter < 0 || s.Chapter >= len(t.Chapters) || s.EditFrames <= 0 {
			return nil
		}
		frames[s.Chapter] += s.EditFrames
		total += s.EditFrames
	}
	if total != t.Duration*24 {
		return nil
	}
	defaults := []struct {
		instruments string
		energy      int
	}{{"主题动机引入", 45}, {"主旋律陈述 / 节奏进入", 65}, {"完整鼓组 / 第一次高潮", 90}, {"对比律动 / 桥段", 70}, {"主题回归 / 推进", 80}, {"终章高潮 / 收束长音", 100}}
	result := []MusicSection{}
	cursor := 0
	for i, ch := range t.Chapters {
		if frames[i] == 0 {
			continue
		}
		start := float64(cursor) / 24
		cursor += frames[i]
		end := float64(cursor) / 24
		s := MusicSection{Name: ch.Title, Chapter: ch.Title, Start: snapSeconds(start), End: snapSeconds(end)}
		d := defaults[min(i, len(defaults)-1)]
		s.Instruments, s.Energy = d.instruments, d.energy
		if ch.Music != nil {
			s.Prompt, s.Accent, s.Join = ch.Music.Prompt, ch.Music.Accent, ch.Music.Join
			if ch.Music.Instruments != "" {
				s.Instruments = ch.Music.Instruments
			}
			if ch.Music.Energy > 0 {
				s.Energy = ch.Music.Energy
			}
		} else {
			s.Prompt = ch.Beat
		}
		result = append(result, s)
	}
	return result
}

// treatmentSections maps GPT-6's acts to score passages. Energy rises across
// acts, the last act carries the climax and coda, and each join is written
// from the act's picture bridge so the musical hand-over matches the cut.
func treatmentSections(p *Project) []MusicSection {
	t := p.Treatment
	n := len(t.Acts)
	result := make([]MusicSection, 0, n)
	for i, act := range t.Acts {
		start, end := act.Start, act.End
		if end <= start {
			start = snapSeconds(float64(i) * float64(p.Duration) / float64(n))
			end = snapSeconds(float64(i+1) * float64(p.Duration) / float64(n))
		}
		s := MusicSection{Name: act.Title, Chapter: act.Title, Start: start, End: end}
		s.Energy = actEnergy(i, n)
		s.Accent = []string{"intro", "build", "lift", "climax"}[min(i, 3)]
		if i == n-1 {
			s.Accent = "climax"
		}
		s.Instruments = []string{"主题动机引入", "第一主题展开 / 节奏进入", "对比主题 / 和声推进", "主旋律变奏 / 情绪高潮"}[min(i, 3)]
		if i == n-1 {
			s.Instruments = "主旋律变奏 / 高潮后收束"
		}
		if strings.TrimSpace(act.Music) != "" {
			s.Prompt = strings.TrimSpace(act.Music)
		} else {
			s.Prompt = fmt.Sprintf("%s。本章画面：%s", s.Instruments, act.StoryBeat)
		}
		if i < n-1 {
			at := fmt.Sprintf("第%.0f秒", end)
			switch act.Bridge {
			case "veil":
				s.Join = at + "换装遮挡处：弦乐上行与镲声渐强穿过遮挡，在新章第一拍落下重音。"
			case "spin":
				s.Join = at + "转身换装处：一记鼓点重音落在转身完成的强拍，新章配器立刻进入。"
			case "prop":
				s.Join = at + "道具特写处：旋律留白半拍，仅保留低音与拨弦，新章第一拍进入。"
			default:
				s.Join = at + "强拍直切进入下一章，不渐弱、不留空拍。"
			}
		} else {
			hold := endHold(p)
			if hold > 0 {
				s.Join = fmt.Sprintf("第%.0f秒在强拍完成收束，最后%.0f秒只保留温暖长音渐弱，不再有鼓点。", float64(p.Duration)-hold, hold)
			} else {
				s.Join = "结尾在强拍上完整收束，留一个可循环的自然终止。"
			}
		}
		result = append(result, s)
	}
	return result
}

func actEnergy(i, n int) int {
	switch n {
	case 2:
		return []int{60, 100}[i]
	case 3:
		return []int{50, 75, 100}[i]
	default:
		return []int{45, 65, 80, 100}[min(i, 3)]
	}
}

func legacySections(p *Project) []MusicSection {
	profiles := []struct {
		name, instruments string
		ratio             float64
		energy            int
	}{
		{"开场 · 点亮", "拨弦 / 钢琴动机", 0, 25},
		{"推进 · 相遇", "主旋律 / 贝斯 / 轻打击", 8.0 / 60, 60},
		{"转折 · 心动", "新和声 / 对答旋律 / 节奏留白", 24.0 / 60, 40},
		{"高潮 · 庆祝", "完整鼓组 / 主旋律变奏 / 和弦铺底", 40.0 / 60, 100},
		{"收尾 · 余韵", "旋律回归 / 渐弱终止", 52.0 / 60, 35},
	}
	joins := []string{"以一记轻重音进入推进段。", "新和声直接接入，不留空拍。", "鼓组填充后落在高潮第一拍。", "旋律回归，渐弱进入收尾。", ""}
	result := []MusicSection{}
	for i, s := range profiles {
		if sceneID(p) != "wedding" {
			s.name = []string{"开场 · 主题", "推进 · 展开", "转折 · 对比", "高潮 · 重点", "收尾 · 回响"}[i]
		}
		end := float64(p.Duration)
		if i+1 < len(profiles) {
			end = snapSeconds(profiles[i+1].ratio * float64(p.Duration))
		}
		result = append(result, MusicSection{Name: s.name, Start: snapSeconds(s.ratio * float64(p.Duration)), End: end, Instruments: s.instruments, Energy: s.energy, Prompt: s.instruments, Accent: []string{"intro", "build", "break", "climax", "coda"}[i], Join: joins[i]})
	}
	return result
}

// planCues groups sections into audio requests. Films within the model limit
// become one cue so the whole score is composed together; longer films split
// only at section boundaries so every join stays on a designed hand-over.
func planCues(p *Project, sections []MusicSection) []MusicCue {
	if len(sections) == 0 {
		return nil
	}
	cues := []MusicCue{}
	first := 0
	for i := range sections {
		last := i == len(sections)-1
		if !last && sections[i+1].End-sections[first].Start <= maxCueSeconds {
			continue
		}
		cues = append(cues, MusicCue{ID: fmt.Sprintf("C%d", len(cues)+1), Start: sections[first].Start, End: sections[i].End})
		first = i + 1
	}
	for i := range cues {
		cues[i].Prompt = scorePrompt(p, sections, cues[i], i, len(cues))
	}
	return cues
}

// scorePrompt writes the cue sheet for one audio request. Times are relative
// to the cue so the model can place accents; the global header keeps key,
// tempo and theme identical across cues of the same film.
func scorePrompt(p *Project, sections []MusicSection, cue MusicCue, index, total int) string {
	direction := profileForProject(p).Music
	if p.Treatment != nil && strings.TrimSpace(p.Treatment.MusicDirection) != "" {
		direction = p.Treatment.MusicDirection
	}
	length := cue.End - cue.Start
	var b strings.Builder
	fmt.Fprintf(&b, "纯器乐影视配乐，无人声、无歌词、无念白、无环境音效。总时长精确%.0f秒，速度%d BPM，全曲统一调性和一个可辨认的主题动机，不做无变化的循环。", length, targetBPM(p))
	fmt.Fprintf(&b, "音乐方向：%s。", strings.TrimRight(clampRunes(direction, 300), "。．. "))
	if total > 1 {
		fmt.Fprintf(&b, "整部影片%d秒，本段是第%d/%d段，覆盖第%.0f–%.0f秒。", p.Duration, index+1, total, cue.Start, cue.End)
		if index > 0 {
			b.WriteString("本段开头不做引子，第一拍直接进入并延续前一段的主题、配器与能量。")
		}
		if index < total-1 {
			b.WriteString("本段结尾不收束、不渐弱，保持能量并结束在强拍上，供下一段直接接入。")
		}
	}
	b.WriteString("分段时间表（本段内时间）：")
	n := 0
	for _, s := range sections {
		if s.End <= cue.Start || s.Start >= cue.End {
			continue
		}
		n++
		start, end := s.Start-cue.Start, s.End-cue.Start
		fmt.Fprintf(&b, "%d) %.0f–%.0f秒「%s」能量%d/100，%s：%s", n, start, end, s.Name, s.Energy, accentText(s.Accent), clampRunes(s.Prompt, 160))
		if s.Join != "" {
			fmt.Fprintf(&b, " 衔接：%s", clampRunes(s.Join, 90))
		}
		b.WriteString(" ")
	}
	if hold := endHold(p); hold > 0 && cue.End >= float64(p.Duration) {
		fmt.Fprintf(&b, "片尾：第%.0f秒完成最后的音乐终止，最后%.0f秒只保留安静长音，交给现场主持人。", length-hold, hold)
	}
	if occasion(p) == "warmup" && cue.End >= float64(p.Duration) {
		b.WriteString("结尾与开头能量接近，便于循环播放。")
	}
	return clampRunes(strings.TrimSpace(b.String()), maxAudioPromptRunes)
}

func accentText(accent string) string {
	switch accent {
	case "hit":
		return "第一拍即起、无引子"
	case "groove":
		return "节奏组完整进入"
	case "drop":
		return "强落点"
	case "break":
		return "对比桥段、换律动"
	case "return":
		return "主题回归"
	case "climax":
		return "终章高潮"
	case "build":
		return "推进"
	case "lift":
		return "抬升"
	case "coda":
		return "收束"
	default:
		return "开场"
	}
}

func clampRunes(s string, limit int) string {
	r := []rune(strings.TrimSpace(s))
	if len(r) <= limit {
		return string(r)
	}
	return string(r[:limit])
}

// soundtrack returns a normalized, exact-length score for the film. It plans
// cues from the designed sections, submits each to the audio model once (task
// IDs are persisted so a resumed render never pays twice), downloads and
// aligns the passages, and reports which source actually produced the music.
func (a *App) soundtrack(ctx context.Context, p *Project) (string, string, error) {
	dir := filepath.Join(a.cfg.DataDir, p.ID)
	if p.MusicFile != "" {
		if s, err := os.Stat(filepath.Join(dir, p.MusicFile)); err == nil && s.Size() > 1000 {
			source := p.MusicSource
			if source == "" {
				source = "seed-audio"
			}
			return filepath.Join(dir, p.MusicFile), source, nil
		}
	}
	if a.cfg.APIKey == "" {
		return "", "", errors.New("未配置创作 API Key")
	}
	// Always design from the current picture structure: projects planned
	// before chapter-based scoring still carry the old ratio sections.
	sections := scoreSections(p)
	if len(sections) == 0 {
		sections = p.MusicSections
	}
	if len(sections) == 0 {
		return "", "", errors.New("没有可用的配乐设计")
	}
	cues := p.MusicCues
	if len(cues) == 0 {
		cues = planCues(p, sections)
		if err := a.store.Update(p.ID, func(q *Project) error {
			q.MusicSections = sections
			q.MusicCues = cues
			return nil
		}); err != nil {
			return "", "", err
		}
	}
	for i := range cues {
		cue := &cues[i]
		if cue.File != "" {
			if s, err := os.Stat(filepath.Join(dir, cue.File)); err == nil && s.Size() > 1000 {
				continue
			}
			cue.File = ""
		}
		if cue.TaskID == "" {
			_ = a.store.Update(p.ID, func(q *Project) error {
				q.Progress = 90 + i*4/len(cues)
				event(q, fmt.Sprintf("正在按分段设计生成配乐 %d/%d（%.0f–%.0f秒）", i+1, len(cues), cue.Start, cue.End))
				return nil
			})
			// The key covers the prompt too: a redesigned cue sheet within the
			// same revision must not be served the previous composition.
			id, err := a.provider.SubmitAudio(ctx, cue.Prompt, fmt.Sprintf("vowfilm:%s:r%d:score-v3:%s:%s", p.ID, p.Revision, cue.ID, promptDigest(cue.Prompt)))
			if err != nil {
				return "", "", err
			}
			cue.TaskID, cue.Status = id, "running"
			if err = a.store.Update(p.ID, func(q *Project) error {
				setCue(q, *cue)
				q.MusicTaskID = id
				return nil
			}); err != nil {
				return "", "", err
			}
		}
		u, assetID, err := a.awaitAudio(ctx, cue.TaskID)
		if err != nil {
			return "", "", err
		}
		name := fmt.Sprintf("score-%s-r%d.wav", strings.ToLower(cue.ID), p.Revision)
		if err := downloadSound(ctx, u, filepath.Join(dir, name)); err != nil {
			return "", "", err
		}
		info, err := probeAudio(ctx, filepath.Join(dir, name))
		if err != nil {
			return "", "", err
		}
		if info < (cue.End-cue.Start)*0.6 {
			return "", "", fmt.Errorf("配乐片段 %s 只有 %.1f 秒，短于设计时长", cue.ID, info)
		}
		cue.File, cue.AssetID, cue.Status = name, assetID, "completed"
		if err = a.store.Update(p.ID, func(q *Project) error { setCue(q, *cue); return nil }); err != nil {
			return "", "", err
		}
	}
	out := fmt.Sprintf("soundtrack-r%d.mp3", p.Revision)
	if err := assembleScore(ctx, dir, cues, p.Duration, filepath.Join(dir, out)); err != nil {
		return "", "", err
	}
	if err := a.store.Update(p.ID, func(q *Project) error {
		q.MusicFile = out
		q.MusicSource = "seed-audio"
		event(q, "分段配乐已生成并对齐章节，正在完成最终混音")
		return nil
	}); err != nil {
		return "", "", err
	}
	return filepath.Join(dir, out), "seed-audio", nil
}

func promptDigest(prompt string) string {
	sum := sha256.Sum256([]byte(prompt))
	return hex.EncodeToString(sum[:6])
}

func setCue(q *Project, cue MusicCue) {
	for i := range q.MusicCues {
		if q.MusicCues[i].ID == cue.ID {
			q.MusicCues[i] = cue
			return
		}
	}
	q.MusicCues = append(q.MusicCues, cue)
}

func (a *App) awaitAudio(ctx context.Context, taskID string) (string, string, error) {
	failures := 0
	for {
		raw, err := a.provider.request(ctx, "GET", "/v1/tasks/"+taskID, nil, "")
		if err != nil {
			if ctx.Err() != nil {
				return "", "", ctx.Err()
			}
			failures++
			if failures >= 5 {
				return "", "", fmt.Errorf("配乐任务暂时无法查询，继续时将恢复原任务：%w", err)
			}
		} else {
			failures = 0
			status, _ := raw["status"].(string)
			switch status {
			case "failed", "cancelled", "expired":
				return "", "", errors.New("配乐任务未完成")
			case "succeeded", "completed":
				assetID := archivedAssetID(raw)
				u := audioURL(raw)
				if u == "" && assetID != "" {
					u = "https://cdn.embervale.cn/assets/" + assetID + "/source.wav"
				}
				if u == "" {
					return "", "", errors.New("配乐任务完成但没有返回音频")
				}
				return u, assetID, nil
			}
		}
		select {
		case <-ctx.Done():
			return "", "", ctx.Err()
		case <-time.After(6 * time.Second):
		}
	}
}

func probeAudio(ctx context.Context, path string) (float64, error) {
	raw, err := exec.CommandContext(ctx, "ffprobe", "-v", "error", "-select_streams", "a:0", "-show_entries", "stream=duration:format=duration", "-of", "json", path).Output()
	if err != nil {
		return 0, errors.New("配乐文件无法解析")
	}
	var result struct {
		Streams []struct {
			Duration string `json:"duration"`
		} `json:"streams"`
		Format struct {
			Duration string `json:"duration"`
		} `json:"format"`
	}
	if json.Unmarshal(raw, &result) != nil || len(result.Streams) == 0 {
		return 0, errors.New("配乐文件没有音频轨")
	}
	if d, err := strconv.ParseFloat(result.Streams[0].Duration, 64); err == nil && d > 0 {
		return d, nil
	}
	d, err := strconv.ParseFloat(result.Format.Duration, 64)
	if err != nil || d <= 0 {
		return 0, errors.New("配乐文件时长无效")
	}
	return d, nil
}

// assembleScore places every cue at its absolute start, trims each to its
// designed length and applies a short guard fade at hard joins, then pads or
// trims the result to the exact film length. The final mix therefore never
// loops or restarts the music inside the film.
func assembleScore(ctx context.Context, dir string, cues []MusicCue, duration int, dest string) error {
	if len(cues) == 0 {
		return errors.New("没有配乐片段")
	}
	args := []string{}
	filters := []string{}
	labels := []string{}
	const guard = 0.08
	for i, cue := range cues {
		args = append(args, "-i", filepath.Join(dir, cue.File))
		length := cue.End - cue.Start
		f := fmt.Sprintf("[%d:a]aresample=48000,aformat=channel_layouts=stereo,atrim=0:%.3f,asetpts=PTS-STARTPTS,apad=whole_dur=%.3f", i, length, length)
		if i > 0 {
			f += fmt.Sprintf(",afade=t=in:d=%.2f", guard)
		}
		if i < len(cues)-1 {
			f += fmt.Sprintf(",afade=t=out:st=%.3f:d=%.2f", length-guard, guard)
		}
		if cue.Start > 0 {
			ms := int(math.Round(cue.Start * 1000))
			f += fmt.Sprintf(",adelay=%d|%d", ms, ms)
		}
		label := fmt.Sprintf("c%d", i)
		filters = append(filters, f+"["+label+"]")
		labels = append(labels, "["+label+"]")
	}
	mixed := labels[0]
	if len(labels) > 1 {
		filters = append(filters, fmt.Sprintf("%samix=inputs=%d:normalize=0:dropout_transition=0[mix]", strings.Join(labels, ""), len(labels)))
		mixed = "[mix]"
	}
	filters = append(filters, fmt.Sprintf("%sapad=whole_dur=%d,atrim=0:%d[out]", mixed, duration, duration))
	aligned := dest + ".aligned.wav"
	defer os.Remove(aligned)
	args = append(args, "-filter_complex_threads", "1", "-filter_complex", strings.Join(filters, ";"), "-map", "[out]", "-ar", "48000", "-ac", "2", "-c:a", "pcm_s16le", aligned)
	if err := ffmpeg(ctx, args...); err != nil {
		return err
	}
	// Normalize with a single static gain. A one-pass loudnorm would ride the
	// level dynamically and erase the quiet opening, the drops and the hold
	// the cue sheet asked for; the composer's dynamics are the point.
	gain := 0.0
	if lufs, err := integratedLoudness(ctx, aligned); err == nil {
		gain = math.Max(-20, math.Min(20, -18-lufs))
	}
	return ffmpeg(ctx, "-i", aligned, "-af", fmt.Sprintf("volume=%.2fdB,alimiter=limit=0.891:level=false", gain), "-ar", "48000", "-ac", "2", "-c:a", "libmp3lame", "-q:a", "2", dest)
}

// integratedLoudness measures program loudness (LUFS) with ffmpeg's loudnorm
// analysis pass.
func integratedLoudness(ctx context.Context, path string) (float64, error) {
	cmd := exec.CommandContext(ctx, "ffmpeg", "-nostdin", "-hide_banner", "-i", path, "-af", "loudnorm=print_format=json", "-f", "null", "-")
	out, _ := cmd.CombinedOutput()
	text := string(out)
	start, end := strings.LastIndex(text, "{"), strings.LastIndex(text, "}")
	if start < 0 || end <= start {
		return 0, errors.New("响度分析无输出")
	}
	var stats struct {
		InputI string `json:"input_i"`
	}
	if err := json.Unmarshal([]byte(text[start:end+1]), &stats); err != nil {
		return 0, err
	}
	v, err := strconv.ParseFloat(stats.InputI, 64)
	if err != nil || math.IsInf(v, 0) || math.IsNaN(v) || v < -70 {
		return 0, errors.New("响度分析无效")
	}
	return v, nil
}

// measureSections reports the RMS level of each designed section in the
// delivered score. This is a technical check for the quality report, not an
// artistic judgement: it shows whether the energy curve the design asked for
// actually exists in the audio.
func measureSections(ctx context.Context, path string, sections []MusicSection) []map[string]any {
	if len(sections) == 0 {
		return nil
	}
	tmp := path + ".measure.raw"
	defer os.Remove(tmp)
	const rate = 8000
	if err := ffmpeg(ctx, "-i", path, "-vn", "-ac", "1", "-ar", fmt.Sprint(rate), "-f", "s16le", tmp); err != nil {
		return nil
	}
	raw, err := os.ReadFile(tmp)
	if err != nil {
		return nil
	}
	samples := len(raw) / 2
	report := make([]map[string]any, 0, len(sections))
	for _, s := range sections {
		from, to := int(s.Start*rate), int(s.End*rate)
		if to > samples {
			to = samples
		}
		entry := map[string]any{"name": s.Name, "start": s.Start, "end": s.End, "designed_energy": s.Energy}
		if to-from > rate/4 {
			var sum float64
			for i := from; i < to; i++ {
				v := float64(int16(binary.LittleEndian.Uint16(raw[i*2:]))) / 32768
				sum += v * v
			}
			rms := math.Sqrt(sum / float64(to-from))
			entry["measured_rms_db"] = math.Round(20*math.Log10(rms+1e-9)*10) / 10
		}
		report = append(report, entry)
	}
	return report
}
