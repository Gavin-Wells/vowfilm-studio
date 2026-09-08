package studio

import (
	"context"
	"encoding/binary"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
)

type MediaInfo struct {
	Duration      float64
	Width, Height int
}

func probe(ctx context.Context, path string) (MediaInfo, error) {
	raw, err := exec.CommandContext(ctx, "ffprobe", "-v", "error", "-select_streams", "v:0", "-show_entries", "stream=width,height:format=duration", "-of", "json", path).Output()
	if err != nil {
		return MediaInfo{}, err
	}
	var result struct {
		Streams []struct {
			Width  int `json:"width"`
			Height int `json:"height"`
		} `json:"streams"`
		Format struct {
			Duration string `json:"duration"`
		} `json:"format"`
	}
	if err = json.Unmarshal(raw, &result); err != nil || len(result.Streams) == 0 {
		return MediaInfo{}, errors.New("视频没有有效画面")
	}
	duration, err := strconv.ParseFloat(result.Format.Duration, 64)
	return MediaInfo{duration, result.Streams[0].Width, result.Streams[0].Height}, err
}
func ffmpeg(ctx context.Context, args ...string) error {
	cmd := exec.CommandContext(ctx, "ffmpeg", append([]string{"-nostdin", "-hide_banner", "-loglevel", "error", "-y"}, args...)...)
	out, err := cmd.CombinedOutput()
	if err != nil {
		if ctx.Err() != nil {
			return ctx.Err()
		}
		text := string(out)
		if len(text) > 1800 {
			text = text[len(text)-1800:]
		}
		return fmt.Errorf("媒体处理失败: %s", text)
	}
	return nil
}
func thumbnail(ctx context.Context, src, dst string) error {
	return ffmpeg(ctx, "-ss", "2", "-i", src, "-frames:v", "1", "-vf", "scale=640:-2", "-q:v", "3", dst)
}
func sourceTrimStart(p *Project, index int, duration float64) float64 {
	if p.Treatment != nil {
		if index == len(p.Shots)-1 && endHold(p) > 0 {
			active := math.Max(1.0/24, p.Shots[index].EditSeconds-endHold(p))
			return math.Max(0, duration-active-1.0/24)
		}
		if p.Shots[index].ChangeToLookID != "" {
			// Preserve the outgoing occlusion/turn at the actual end of the source.
			return math.Max(0, duration-p.Shots[index].EditSeconds-1.0/24)
		}
		if index > 0 && p.Shots[index-1].ChangeToLookID != "" {
			// The incoming clip starts in the same occlusion before revealing the look.
			return 0
		}
	}
	return .25
}
func (a *App) render(ctx context.Context, p *Project) (string, error) {
	if len(p.Shots) == 0 {
		return "", errors.New("缺少镜头")
	}
	if err := layout(p); err != nil {
		return "", err
	}
	dir := filepath.Join(a.cfg.DataDir, p.ID)
	if err := os.MkdirAll(dir, 0700); err != nil {
		return "", err
	}
	width, height := 1280, 720
	if p.Ratio == "9:16" {
		width, height = 720, 1280
	}
	normalized := []string{}
	for i, s := range p.Shots {
		if s.Status != "completed" || s.VideoFile == "" {
			return "", fmt.Errorf("镜头 %s 尚未完成", s.Title)
		}
		dest := filepath.Join(dir, fmt.Sprintf("edit-%02d.mp4", i))
		source := filepath.Join(dir, s.VideoFile)
		info, err := probe(ctx, source)
		if err != nil {
			return "", err
		}
		trim := sourceTrimStart(p, i, info.Duration)
		filter := fmt.Sprintf("fps=24,scale=%d:%d:force_original_aspect_ratio=decrease,pad=%d:%d:(ow-iw)/2:(oh-ih)/2,setsar=1,setpts=PTS-STARTPTS", width, height, width, height)
		if hold := endHold(p); i == len(p.Shots)-1 && hold > 0 {
			activeFrames := s.EditFrames - int(hold*24)
			if activeFrames < 1 {
				return "", errors.New("片尾镜头不足以留画，请重新编排")
			}
			filter += fmt.Sprintf(",trim=end_frame=%d,setpts=PTS-STARTPTS,tpad=stop_mode=clone:stop_duration=%.2f", activeFrames, hold)
		}
		if err := ffmpeg(ctx, "-ss", fmt.Sprintf("%.6f", trim), "-i", source, "-an", "-vf", filter, "-frames:v", strconv.Itoa(s.EditFrames), "-c:v", "libx264", "-preset", "fast", "-crf", "19", "-pix_fmt", "yuv420p", "-threads", "2", dest); err != nil {
			return "", err
		}
		normalized = append(normalized, dest)
	}
	args := []string{}
	filters := []string{}
	for i, f := range normalized {
		args = append(args, "-i", f)
		filters = append(filters, fmt.Sprintf("[%d:v]settb=AVTB,setpts=PTS-STARTPTS[v%d]", i, i))
	}
	label := "v0"
	cursor := p.Shots[0].EditSeconds
	for i := 1; i < len(p.Shots); i++ {
		out := fmt.Sprintf("joined%d", i)
		if overlapFrames(p.Shots[i-1].Transition) == 0 {
			filters = append(filters, fmt.Sprintf("[%s][v%d]concat=n=2:v=1:a=0,settb=AVTB[%s]", label, i, out))
			cursor += p.Shots[i].EditSeconds
		} else {
			fade := float64(overlapFrames(p.Shots[i-1].Transition)) / 24
			effect := "fade"
			if p.Shots[i-1].Transition == "dipwhite" {
				effect = "fadewhite"
			}
			filters = append(filters, fmt.Sprintf("[%s][v%d]xfade=transition=%s:duration=%.6f:offset=%.6f,settb=AVTB[%s]", label, i, effect, fade, cursor-fade, out))
			cursor += p.Shots[i].EditSeconds - fade
		}
		label = out
	}
	editPath := filepath.Join(dir, "picture-edit.mp4")
	args = append(args, "-filter_complex_threads", "1", "-filter_complex", strings.Join(filters, ";"), "-map", "["+label+"]", "-an", "-frames:v", strconv.Itoa(p.Duration*24), "-r", "24", "-c:v", "libx264", "-preset", "fast", "-crf", "19", "-pix_fmt", "yuv420p", "-threads", "2", editPath)
	if err := ffmpeg(ctx, args...); err != nil {
		return "", err
	}
	music := filepath.Join(dir, "original-score.wav")
	custom := false
	for _, asset := range p.Assets {
		if asset.Role == "music" {
			music = filepath.Join(dir, asset.File)
			custom = true
			break
		}
	}
	if !custom && rhythmicStyle(p.Style) {
		generated, err := a.generatedSoundtrack(ctx, p)
		if err != nil {
			return "", err
		}
		music = generated
	} else if !custom {
		if err := composeMusic(music, p.Duration); err != nil {
			return "", err
		}
	}
	subtitles := filepath.Join(dir, "captions.ass")
	if err := os.WriteFile(subtitles, []byte(makeASS(p, width, height)), 0600); err != nil {
		return "", err
	}
	name := fmt.Sprintf("vowfilm-r%d-%s.mp4", p.Revision, newID("")[:8])
	dest := filepath.Join(dir, name)
	audio := fmt.Sprintf("[1:a]aresample=48000,loudnorm=I=-18:TP=-1.5:LRA=9,afade=t=in:d=2,afade=t=out:st=%d:d=3[a]", p.Duration-3)
	subtitleFilter := fmt.Sprintf("subtitles=filename='%s',fade=t=in:d=0.8,fade=t=out:st=%d:d=1.8", strings.ReplaceAll(subtitles, "'", "\\'"), p.Duration-2)
	if rhythmicStyle(p.Style) {
		subtitleFilter = fmt.Sprintf("subtitles=filename='%s',fade=t=in:d=0.2,fade=t=out:st=%.2f:d=0.6", strings.ReplaceAll(subtitles, "'", "\\'"), float64(p.Duration)-0.6)
	}
	if hold := endHold(p); hold > 0 {
		end := float64(p.Duration) - hold
		subtitleFilter = fmt.Sprintf("trim=end_frame=%d,setpts=PTS-STARTPTS,tpad=stop_mode=clone:stop_duration=%.2f,subtitles=filename='%s',fade=t=in:d=0.2", int(end*24), hold, strings.ReplaceAll(subtitles, "'", "\\'"))
		audio = fmt.Sprintf("[1:a]aresample=48000,loudnorm=I=-18:TP=-1.5:LRA=9,afade=t=in:d=0.8,afade=t=out:st=%.2f:d=2[a]", end-2)
	}
	comment := "Created with Vowfilm; AI-generated video"
	if p.Demo {
		comment += "; fictional wedding demo"
	}
	if err := ffmpeg(ctx, "-i", editPath, "-stream_loop", "-1", "-i", music, "-filter_complex_threads", "1", "-filter_complex", audio, "-vf", subtitleFilter, "-map", "0:v:0", "-map", "[a]", "-t", strconv.Itoa(p.Duration), "-r", "24", "-c:v", "libx264", "-preset", "fast", "-crf", "19", "-pix_fmt", "yuv420p", "-threads", "2", "-c:a", "aac", "-ar", "48000", "-ac", "2", "-b:a", "192k", "-movflags", "+faststart", "-metadata", "title="+p.Title, "-metadata", "comment="+comment, dest); err != nil {
		return "", err
	}
	info, err := probe(ctx, dest)
	if err != nil {
		return "", err
	}
	if math.Abs(info.Duration-float64(p.Duration)) > 1.0/24+0.01 || info.Width != width || info.Height != height {
		return "", errors.New("成片时长或尺寸验收未通过")
	}
	qa, _ := json.MarshalIndent(map[string]any{"duration_seconds": info.Duration, "width": width, "height": height, "fps": 24, "technical_validation": "passed", "identity_validation": "not_automated", "fictional_demo": p.Demo, "shots": len(p.Shots), "music": map[bool]string{true: "user_supplied", false: map[bool]string{true: "sonilo_generated", false: "original_procedural_piano"}[rhythmicStyle(p.Style) && a.cfg.PublicMediaURL != ""]}[custom]}, "", "  ")
	_ = os.WriteFile(filepath.Join(dir, "quality-report.json"), qa, 0600)
	return name, nil
}
func assTime(s float64) string {
	cs := int(math.Round(s * 100))
	return fmt.Sprintf("%d:%02d:%02d.%02d", cs/360000, (cs/6000)%60, (cs/100)%60, cs%100)
}
func assEscape(s string) string {
	s = strings.ReplaceAll(s, "\\", "")
	s = strings.ReplaceAll(s, "{", "")
	s = strings.ReplaceAll(s, "}", "")
	s = strings.ReplaceAll(s, "\n", " ")
	s = strings.ReplaceAll(s, "\r", "")
	return s
}
func makeASS(p *Project, width, height int) string {
	var b strings.Builder
	fmt.Fprintf(&b, "[Script Info]\nScriptType: v4.00+\nPlayResX: %d\nPlayResY: %d\nWrapStyle: 0\n\n[V4+ Styles]\nFormat: Name, Fontname, Fontsize, PrimaryColour, SecondaryColour, OutlineColour, BackColour, Bold, Italic, Underline, StrikeOut, ScaleX, ScaleY, Spacing, Angle, BorderStyle, Outline, Shadow, Alignment, MarginL, MarginR, MarginV, Encoding\n", width, height)
	b.WriteString("Style: Caption,Noto Serif CJK SC,27,&H00F4F6EF,&H00FFFFFF,&H800B100D,&H80000000,0,0,0,0,100,100,3,0,1,1,0,2,65,65,58,1\nStyle: Title,Noto Serif CJK SC,48,&H00F8F8F1,&H00FFFFFF,&H700B100D,&H80000000,0,0,0,0,100,100,5,0,1,1,0,5,55,55,30,1\nStyle: Small,Noto Sans CJK SC,16,&H00E0E7DD,&H00FFFFFF,&H700B100D,&H80000000,0,0,0,0,100,100,4,0,1,1,0,2,50,50,35,1\n\n[Events]\nFormat: Layer, Start, End, Style, Name, MarginL, MarginR, MarginV, Effect, Text\n")
	line := func(start, end float64, style, text string) {
		if end > start {
			fade := 450
			if rhythmicStyle(p.Style) {
				fade = 130
			}
			fmt.Fprintf(&b, "Dialogue: 0,%s,%s,%s,,0,0,0,,{\\fad(%d,%d)}%s\n", assTime(start), assTime(end), style, fade, fade, assEscape(text))
		}
	}
	if p.Treatment != nil {
		header := b.String()
		b.Reset()
		header = strings.ReplaceAll(header, "Noto Serif CJK SC", "Noto Sans CJK SC")
		captionSize, titleSize := 38, 58
		if width < height {
			captionSize, titleSize = 32, 44
		}
		header = strings.Replace(header, "CJK SC,27,", fmt.Sprintf("CJK SC,%d,", captionSize), 1)
		header = strings.Replace(header, "CJK SC,48,", fmt.Sprintf("CJK SC,%d,", titleSize), 1)
		// Bottom-center title, inside the screen-safe area; the final hold stays visible.
		header = strings.Replace(header, "100,100,5,0,1,1,0,5,55,55,30,1", "100,100,1,0,1,2,1,2,80,80,90,1", 1)
		b.WriteString(header)
		line(.6, 3.7, "Title", p.Title)
		for i, s := range p.Shots {
			if i%4 == 2 && s.TimelineStart+s.EditSeconds < float64(p.Duration)-5 {
				line(s.TimelineStart+.2, s.TimelineStart+s.EditSeconds-.15, "Caption", s.Caption)
			}
		}
		// A near-zero fade at the end preserves the invitation on the last frame.
		fmt.Fprintf(&b, "Dialogue: 1,%s,%s,Title,,0,0,0,,{\\fad(180,0)}%s\n", assTime(float64(p.Duration)-5), assTime(float64(p.Duration)), assEscape(p.Treatment.ClosingLine))
		if p.Demo {
			line(float64(p.Duration)-5, float64(p.Duration), "Small", "AI 创作演示 · 虚构人物与场景")
		}
		return b.String()
	}
	if rhythmicStyle(p.Style) {
		header := b.String()
		b.Reset()
		header = strings.ReplaceAll(header, "Noto Serif CJK SC", "Noto Sans CJK SC")
		header = strings.Replace(header, "100,100,5,0,1,1,0,5,55,55,30,1", "100,100,2,0,1,2,1,1,55,55,65,1", 1)
		b.WriteString(header)
		line(.45, 2.7, "Title", p.Title)
		for i, s := range p.Shots {
			if i%3 == 1 && i < len(p.Shots)-1 {
				line(s.TimelineStart+.3, s.TimelineStart+s.EditSeconds-.25, "Caption", s.Caption)
			}
		}
		closing := map[string]string{"joyful": "所有快乐，都想和你", "romantic": "每一次心动，都有你", "vintage": "下一帧，还是你", "epic": "从此，同行每一程", "travel": "下一站，一起出发", "editorial": "我们，自成一派"}[p.Style]
		line(float64(p.Duration)-3.3, float64(p.Duration)-.7, "Title", closing)
		if p.Demo {
			line(float64(p.Duration)-3.3, float64(p.Duration)-.7, "Small", "AI 创作演示 · 虚构人物与场景")
		}
		return b.String()
	}
	line(1, 5.8, "Title", p.Title)
	line(1.4, 5.8, "Small", "A PROMISE IN LIGHT")
	for i, s := range p.Shots {
		if i == 0 || i == len(p.Shots)-1 {
			continue
		}
		start := s.TimelineStart + 1
		end := math.Min(s.TimelineStart+s.EditSeconds-1, float64(p.Duration)-6)
		line(start, end, "Caption", s.Caption)
	}
	line(float64(p.Duration)-6.5, float64(p.Duration)-1.9, "Title", "把每一天，写成我们")
	if p.Demo {
		line(float64(p.Duration)-6.2, float64(p.Duration)-1.9, "Small", "AI 创作演示 · 虚构人物与场景")
	}
	return b.String()
}
func composeMusic(path string, seconds int) error {
	const rate = 24000
	count := rate * seconds
	data := make([]byte, 44+count*2)
	copy(data, "RIFF")
	binary.LittleEndian.PutUint32(data[4:], uint32(len(data)-8))
	copy(data[8:], "WAVEfmt ")
	binary.LittleEndian.PutUint32(data[16:], 16)
	binary.LittleEndian.PutUint16(data[20:], 1)
	binary.LittleEndian.PutUint16(data[22:], 1)
	binary.LittleEndian.PutUint32(data[24:], rate)
	binary.LittleEndian.PutUint32(data[28:], rate*2)
	binary.LittleEndian.PutUint16(data[32:], 2)
	binary.LittleEndian.PutUint16(data[34:], 16)
	copy(data[36:], "data")
	binary.LittleEndian.PutUint32(data[40:], uint32(count*2))
	chords := [][]int{{48, 55, 60, 64}, {43, 50, 59, 62}, {45, 52, 60, 64}, {41, 48, 57, 60}}
	arp := []int{0, 2, 1, 3, 2, 1, 3, 2}
	beat := 60.0 / 72
	step := beat / 2
	noteHz := func(m int) float64 { return 440 * math.Pow(2, float64(m-69)/12) }
	for i := 0; i < count; i++ {
		t := float64(i) / rate
		v := 0.0
		idx := int(t / step)
		for k := 0; k < 7; k++ {
			n := idx - k
			if n < 0 {
				continue
			}
			age := t - float64(n)*step
			if age > 3.0 {
				continue
			}
			ch := chords[(n/16)%len(chords)]
			m := ch[arp[n%len(arp)]]
			f := noteHz(m)
			attack := 1 - math.Exp(-age*110)
			decay := math.Exp(-age * 2.2)
			tone := math.Sin(2*math.Pi*f*age) + .34*math.Exp(-age*.9)*math.Sin(4*math.Pi*f*age) + .12*math.Exp(-age*2)*math.Sin(6*math.Pi*f*age)
			v += .15 * attack * decay * tone
		}
		bar := int(t / (beat * 4))
		ch := chords[bar%4]
		barAge := math.Mod(t, beat*4)
		env := math.Sin(math.Pi * barAge / (beat * 4))
		for _, m := range ch {
			f := noteHz(m - 12)
			v += .017 * env * (math.Sin(2*math.Pi*f*t) + .2*math.Sin(2*math.Pi*f*1.002*t))
		}
		swell := .7 + .25*math.Sin(math.Pi*math.Min(1, t/float64(seconds)))
		fade := math.Min(1, t/2) * math.Min(1, (float64(seconds)-t)/4)
		v = math.Tanh(v*1.15) * swell * fade
		binary.LittleEndian.PutUint16(data[44+i*2:], uint16(int16(v*27000)))
	}
	return os.WriteFile(path, data, 0600)
}
