package studio

import (
	"archive/zip"
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math"
	"net/http"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"vowfilm/server/internal/domain"
	"vowfilm/server/internal/wedding"
)

func isWeddingTemplate(p *Project) bool {
	return p != nil && p.CreationMode == "template" && p.TemplateID == wedding.TemplateID
}

func weddingInit(p *Project) {
	if !isWeddingTemplate(p) {
		return
	}
	if p.Wedding == nil {
		p.Wedding = wedding.New()
	}
	p.PromptPolicy = wedding.Version()
	p.GenerationMode = "wedding-story-guided-v1"
}

func weddingLayout(p *Project) error {
	if p.Wedding == nil || len(p.Wedding.Cues) == 0 {
		return errors.New("请先登记实际旁白和语义时间点")
	}
	cursor := 0
	facts, cues := map[string]bool{}, map[string]bool{}
	for _, f := range p.Wedding.Facts {
		facts[f.ID] = true
	}
	for _, c := range p.Wedding.Cues {
		cues[c.ID] = true
	}
	for i := range p.Shots {
		s := &p.Shots[i]
		frames := int(math.Round(s.EditSeconds * 24))
		if int(math.Round(s.TimelineStart*24)) != cursor || frames < 24 || frames > 12*24 {
			return fmt.Errorf("%s 的时间轴不连续或镜头时长超出1–12秒", s.ID)
		}
		if len(s.FactIDs) == 0 || len(s.CueIDs) == 0 {
			return fmt.Errorf("%s 缺少事实或旁白来源", s.ID)
		}
		for _, id := range s.FactIDs {
			if !facts[id] {
				return fmt.Errorf("%s 引用了不存在的事实 %s", s.ID, id)
			}
		}
		for _, id := range s.CueIDs {
			if !cues[id] {
				return fmt.Errorf("%s 引用了不存在的旁白 %s", s.ID, id)
			}
		}
		if s.Transition != "cut" && s.Transition != "match" && s.Transition != "" {
			return errors.New("故事向导先采用自然切接，转场需纳入确认时间表")
		}
		s.Transition = "cut"
		s.EditFrames = frames
		s.EditSeconds = float64(frames) / 24
		s.TimelineStart = float64(cursor) / 24
		s.Duration = max(4, int(math.Ceil(s.EditSeconds+0.75)))
		cursor += frames
	}
	if cursor != p.Duration*24 {
		return fmt.Errorf("分镜总长 %.2f 秒与目标 %d 秒不一致", float64(cursor)/24, p.Duration)
	}
	return nil
}

func (a *App) checkWeddingArtifacts(p *Project) error {
	if p.Wedding == nil {
		return errors.New("婚礼模板状态尚未初始化")
	}
	for _, step := range p.Wedding.Steps {
		if step.Status != "confirmed" {
			continue
		}
		for _, art := range step.Artifacts {
			if err := a.checkWeddingArtifact(p, art); err != nil {
				return err
			}
		}
	}
	return nil
}
func (a *App) checkWeddingArtifact(p *Project, art domain.WeddingArtifact) error {
	if filepath.Base(art.File) != art.File || art.File == "" {
		return errors.New("素材路径无效")
	}
	b, err := os.ReadFile(filepath.Join(a.cfg.DataDir, p.ID, art.File))
	if err != nil {
		return fmt.Errorf("缺少素材：%s", art.Name)
	}
	sum := sha256.Sum256(b)
	if hex.EncodeToString(sum[:]) != art.SHA256 {
		return fmt.Errorf("素材已变化，请先返工并重新审核：%s", art.Name)
	}
	return nil
}

func (a *App) validateWeddingAction(p *Project, action string) error {
	if !isWeddingTemplate(p) {
		return nil
	}
	if p.Wedding != nil && p.Wedding.Automatic && action == "generate" {
		if strings.TrimSpace(p.Brief) == "" {
			return errors.New("请先填写故事资料")
		}
		return a.checkWeddingArtifacts(p)
	}
	if err := a.checkWeddingArtifacts(p); err != nil {
		return err
	}
	n := p.Wedding.CurrentStep
	allowed := action == "plan" && (n == 2 || n == 6) || (action == "generate" || action == "shot") && n == 9 || action == "render" && n >= 11 && n <= 14
	if !allowed {
		return fmt.Errorf("当前第 %d/14 步：%s，请在故事向导完成本步材料和确认", n, wedding.Names[n-1])
	}
	if action == "generate" || action == "shot" {
		for _, s := range p.Shots {
			if s.FirstFrameFile == "" {
				return fmt.Errorf("%s 缺少已选定首帧", s.ID)
			}
		}
	}
	return nil
}

func (a *App) weddingArtifact(p *Project, name, kind, shot string, data []byte, mime string) (domain.WeddingArtifact, error) {
	dir := filepath.Join(a.cfg.DataDir, p.ID)
	if err := os.MkdirAll(dir, 0700); err != nil {
		return domain.WeddingArtifact{}, err
	}
	id := newID("wa_")
	ext := strings.ToLower(filepath.Ext(name))
	file := id + ext
	if err := os.WriteFile(filepath.Join(dir, file), data, 0600); err != nil {
		return domain.WeddingArtifact{}, err
	}
	sum := sha256.Sum256(data)
	art := domain.WeddingArtifact{ID: id, Kind: kind, Name: filepath.Base(name), File: file, URL: mediaURL(p.ID, file), MIME: mime, SHA256: hex.EncodeToString(sum[:]), Bytes: int64(len(data)), ShotID: shot}
	if strings.HasPrefix(mime, "text/") || mime == "application/json" {
		art.Text = string(data)
	}
	return art, nil
}

func (a *App) weddingHTTP(w http.ResponseWriter, r *http.Request, p *Project, parts []string) {
	if !isWeddingTemplate(p) {
		fail(w, 400, errors.New("该工程不是婚礼故事向导模板"))
		return
	}
	if len(parts) == 0 && r.Method == "GET" {
		respond(w, 200, p)
		return
	}
	if len(parts) == 1 && r.Method == "GET" && parts[0] == "sources" {
		data, err := wedding.SourcesZIP()
		if err != nil {
			fail(w, 500, err)
			return
		}
		w.Header().Set("Content-Type", "application/zip")
		w.Header().Set("Content-Disposition", `attachment; filename="wedding-wizard-original.zip"`)
		_, _ = w.Write(data)
		return
	}
	if len(parts) == 1 && r.Method == "GET" && parts[0] == "pack" {
		a.weddingPackHTTP(w, r, p)
		return
	}
	if len(parts) != 1 || r.Method != "POST" {
		fail(w, 404, errors.New("向导接口不存在"))
		return
	}
	if isRunning(p.Status) {
		fail(w, 409, errors.New("请等待本次任务结束"))
		return
	}
	switch parts[0] {
	case "automatic":
		if err := checkWeddingAlignment(); err != nil {
			fail(w, 409, err)
			return
		}
		err := a.store.Update(p.ID, func(q *Project) error {
			if !q.Wedding.Automatic && q.Wedding.CurrentStep > 2 {
				return errors.New("当前逐步制作已有确认成果，请新建一键模板工程以保留原版本")
			}
			q.Wedding.Automatic = true
			for _, art := range q.Wedding.Steps[0].Artifacts {
				if art.Kind == "facts" {
					q.Brief = art.Text
				}
			}
			event(q, "已选择一键生成：自动完成文案、旁白、分镜、生图、视频、配乐与字幕合成")
			return nil
		})
		if err != nil {
			fail(w, 409, err)
			return
		}
	case "prepare":
		a.weddingPrepareHTTP(w, r, p)
		return
	case "approve":
		var in struct {
			Step     int    `json:"step"`
			Version  string `json:"version"`
			By       string `json:"by"`
			Evidence string `json:"evidence"`
		}
		if err := decode(w, r, &in); err != nil {
			fail(w, 400, err)
			return
		}
		err := a.store.Update(p.ID, func(q *Project) error {
			if err := a.checkWeddingArtifacts(q); err != nil {
				return err
			}
			if in.Step < 1 || in.Step > 14 {
				return errors.New("步骤无效")
			}
			for _, art := range q.Wedding.Steps[in.Step-1].Artifacts {
				if err := a.checkWeddingArtifact(q, art); err != nil {
					return err
				}
			}
			if err := a.bindWeddingStep(r.Context(), q, in.Step); err != nil {
				return err
			}
			if err := wedding.Approve(q.Wedding, in.Step, in.Version, in.By, currentUser(r).ID, in.Evidence); err != nil {
				return err
			}
			if in.Step == 2 && q.Wedding.CurrentStep == 3 {
				var scripts []domain.WeddingArtifact
				for _, art := range q.Wedding.Steps[1].Artifacts {
					if art.Kind == "script" {
						scripts = append(scripts, art)
					}
				}
				if err := wedding.Prepare(q.Wedding, 3, scripts); err != nil {
					return err
				}
			}
			event(q, fmt.Sprintf("第%d步已记录%s确认；当前第%d/14步", in.Step, in.By, q.Wedding.CurrentStep))
			return nil
		})
		if err != nil {
			fail(w, 409, err)
			return
		}
	case "reopen":
		var in struct {
			Step   int    `json:"step"`
			Reason string `json:"reason"`
		}
		if err := decode(w, r, &in); err != nil {
			fail(w, 400, err)
			return
		}
		err := a.store.Update(p.ID, func(q *Project) error {
			if in.Step > q.Wedding.CurrentStep {
				return errors.New("不能返工尚未进行的步骤")
			}
			if err := wedding.Reopen(q.Wedding, in.Step, nil, in.Reason); err != nil {
				return err
			}
			q.FilmURL = ""
			if q.Wedding.Automatic {
				q.Revision++
				if in.Step <= 2 {
					q.VoiceoverScript = ""
				}
				if in.Step <= 5 {
					q.Wedding.NarrationFile = ""
					q.Wedding.Cues = nil
					delete(q.Wedding.AutoTasks, "narration")
					delete(q.Wedding.AutoFiles, "narration")
				}
				if in.Step <= 6 {
					q.Shots = nil
				}
				if in.Step <= 8 {
					for key := range q.Wedding.AutoTasks {
						if strings.HasPrefix(key, "frame-") {
							delete(q.Wedding.AutoTasks, key)
							delete(q.Wedding.AutoFiles, key)
						}
					}
					for i := range q.Shots {
						q.Shots[i].FirstFrameFile = ""
					}
				}
				if in.Step <= 9 {
					for i := range q.Shots {
						resetShot(&q.Shots[i])
					}
				}
				if in.Step <= 10 {
					delete(q.Wedding.AutoTasks, "music")
					delete(q.Wedding.AutoFiles, "music")
				}
				q.Wedding.MixFile = ""
			}
			q.Status = "draft"
			event(q, "返工："+in.Reason)
			return nil
		})
		if err != nil {
			fail(w, 409, err)
			return
		}
	default:
		fail(w, 404, errors.New("向导动作不存在"))
		return
	}
	respond(w, 200, a.store.Get(p.ID))
}

func (a *App) weddingPrepareHTTP(w http.ResponseWriter, r *http.Request, p *Project) {
	r.Body = http.MaxBytesReader(w, r.Body, 201<<20)
	if err := r.ParseMultipartForm(1 << 20); err != nil {
		fail(w, 400, errors.New("素材需小于200 MB"))
		return
	}
	defer r.MultipartForm.RemoveAll()
	step, err := strconv.Atoi(r.FormValue("step"))
	if err != nil || step != p.Wedding.CurrentStep {
		fail(w, 409, errors.New("当前步骤已变化"))
		return
	}
	kind := r.FormValue("kind")
	shot := r.FormValue("shotId")
	allowed := map[int]string{1: "facts photo", 2: "script writing factmap", 3: "script", 4: "voice selection", 5: "narration timing", 6: "storyboard", 7: "image", 8: "image", 9: "video", 10: "music selection", 11: "music mix settings", 12: "mix", 13: "preview", 14: "final subtitles edit"}
	if !strings.Contains(" "+allowed[step]+" ", " "+kind+" ") || kind == "" {
		fail(w, 400, errors.New("材料类型不匹配"))
		return
	}
	if kind == "image" || kind == "video" {
		found := false
		for _, s := range p.Shots {
			if s.ID == shot {
				found = true
			}
		}
		if !found {
			fail(w, 400, errors.New("请指定有效镜号"))
			return
		}
	}
	var data []byte
	var name, mime string
	if value := r.FormValue("text"); value != "" {
		if kind != "facts" && kind != "script" && kind != "writing" && kind != "factmap" && kind != "timing" && kind != "storyboard" && kind != "selection" && kind != "settings" && kind != "edit" && kind != "subtitles" {
			fail(w, 400, errors.New("此材料需要原文件"))
			return
		}
		if len(value) > 200000 {
			fail(w, 400, errors.New("文字材料过长"))
			return
		}
		data = []byte(value)
		name = kind + ".txt"
		mime = "text/plain"
		if kind == "timing" || kind == "storyboard" || kind == "factmap" || kind == "selection" || kind == "settings" || kind == "edit" {
			if !json.Valid(data) {
				fail(w, 400, errors.New("请填写有效 JSON"))
				return
			}
			name = kind + ".json"
			mime = "application/json"
		}
	} else {
		f, h, e := r.FormFile("file")
		if e != nil {
			fail(w, 400, errors.New("请填写内容或上传原文件"))
			return
		}
		defer f.Close()
		data, err = io.ReadAll(io.LimitReader(f, 200<<20+1))
		if err != nil || len(data) > 200<<20 {
			fail(w, 400, errors.New("文件读取失败或过大"))
			return
		}
		name = h.Filename
		mime = http.DetectContentType(data)
		if kind == "timing" || kind == "storyboard" || kind == "factmap" || kind == "selection" || kind == "settings" || kind == "edit" {
			if !json.Valid(data) {
				fail(w, 400, errors.New("JSON 文件无效"))
				return
			}
			mime = "application/json"
		}
	}
	if (kind == "image" || kind == "photo") && mime != "image/jpeg" && mime != "image/png" && mime != "image/webp" {
		fail(w, 400, errors.New("请上传真实 JPG/PNG/WebP 首帧"))
		return
	}
	if kind == "video" || kind == "preview" || kind == "final" {
		if mime != "video/mp4" {
			fail(w, 400, errors.New("请上传 MP4 视频"))
			return
		}
	}
	if kind == "voice" || kind == "narration" || kind == "music" || kind == "mix" {
		if !strings.HasPrefix(mime, "audio/") && mime != "application/ogg" && mime != "video/mp4" {
			fail(w, 400, errors.New("请上传有效音频"))
			return
		}
	}
	art, err := a.weddingArtifact(p, name, kind, shot, data, mime)
	if err != nil {
		fail(w, 500, err)
		return
	}
	path := filepath.Join(a.cfg.DataDir, p.ID, art.File)
	if kind == "voice" || kind == "narration" || kind == "music" || kind == "mix" {
		_, err = probeAudio(r.Context(), path)
	} else if kind == "video" || kind == "preview" || kind == "final" {
		_, err = probe(r.Context(), path)
	} else if kind == "image" || kind == "photo" {
		err = ffmpeg(r.Context(), "-i", path, "-frames:v", "1", "-f", "null", "-")
	}
	if err != nil {
		_ = os.Remove(filepath.Join(a.cfg.DataDir, p.ID, art.File))
		fail(w, 400, errors.New("媒体无法解码"))
		return
	}
	err = a.store.Update(p.ID, func(q *Project) error {
		if q.Wedding.CurrentStep != step || isRunning(q.Status) {
			return errors.New("状态已变化，请刷新")
		}
		var arts []domain.WeddingArtifact
		for _, old := range q.Wedding.Steps[step-1].Artifacts {
			if old.Kind == kind && ((shot != "" && old.ShotID == shot) || (shot == "" && kind != "voice" && kind != "music" && kind != "mix" && kind != "photo")) {
				continue
			}
			arts = append(arts, old)
		}
		arts = append(arts, art)
		return wedding.Prepare(q.Wedding, step, arts)
	})
	if err != nil {
		_ = os.Remove(filepath.Join(a.cfg.DataDir, p.ID, art.File))
		fail(w, 409, err)
		return
	}
	respond(w, 201, a.store.Get(p.ID))
}

func (a *App) bindWeddingStep(ctx context.Context, p *Project, n int) error {
	state := p.Wedding
	arts := state.Steps[n-1].Artifacts
	find := func(kind string) *domain.WeddingArtifact {
		for i := len(arts) - 1; i >= 0; i-- {
			if arts[i].Kind == kind {
				return &arts[i]
			}
		}
		return nil
	}
	require := func(kinds ...string) error {
		for _, k := range kinds {
			if find(k) == nil {
				return fmt.Errorf("本步缺少 %s 材料", k)
			}
		}
		return nil
	}
	switch n {
	case 1:
		if err := require("facts"); err != nil {
			return err
		}
		if len([]rune(find("facts").Text)) < 20 {
			return errors.New("请提供完整故事采集资料")
		}
		p.Brief = find("facts").Text
		p.Demo = false
	case 2, 3:
		if err := require("script"); err != nil {
			return err
		}
		if n == 2 {
			if err := require("writing"); err != nil {
				return err
			}
			if art := find("factmap"); art != nil {
				if err := json.Unmarshal([]byte(art.Text), &state.Facts); err != nil {
					return errors.New("事实表应为包含 id、text、source 的 JSON 数组")
				}
			}
			if len(state.Facts) == 0 {
				return errors.New("缺少事实来源表，请上传 factmap 或使用写作生成器")
			}
		} else {
			matched := false
			for _, art := range state.Steps[1].Artifacts {
				if art.Kind == "script" && art.SHA256 == find("script").SHA256 {
					matched = true
				}
			}
			if !matched {
				return errors.New("新人待确认文案与制作方审稿版本不同，请先返工第2步")
			}
		}
		p.VoiceoverScript = strings.TrimSpace(find("script").Text)
		if p.VoiceoverScript == "" {
			return errors.New("旁白正文为空")
		}
	case 4:
		if err := require("voice", "selection"); err != nil {
			return err
		}
		count := 0
		for _, art := range arts {
			if art.Kind == "voice" {
				count++
			}
		}
		if count < 3 {
			return errors.New("请先上传至少三个同文音色试听")
		}
		selection, err := selectedWeddingArtifact(state.Steps[3], "voice")
		if err != nil {
			return err
		}
		_ = selection
		var setting struct {
			ScriptSHA256 string  `json:"scriptSha256"`
			VoiceID      string  `json:"voiceId"`
			Direction    string  `json:"direction"`
			Rate         float64 `json:"rate"`
		}
		if err = json.Unmarshal([]byte(find("selection").Text), &setting); err != nil {
			return err
		}
		if setting.ScriptSHA256 != fileDigest([]byte(p.VoiceoverScript)) || setting.VoiceID == "" || setting.Direction == "" || setting.Rate <= 0 {
			return errors.New("音色选择表需记录当前文案 scriptSha256、真实 voiceId、direction 和正数 rate")
		}
	case 5:
		if err := require("narration", "timing"); err != nil {
			return err
		}
		var doc struct {
			Cues []domain.SubtitleCue `json:"cues"`
		}
		if err := json.Unmarshal([]byte(find("timing").Text), &doc); err != nil {
			return err
		}
		seconds, err := probeAudio(ctx, filepath.Join(a.cfg.DataDir, p.ID, find("narration").File))
		if err != nil {
			return err
		}
		if seconds > float64(p.Duration)+0.05 {
			return errors.New("旁白长于目标片长，请调整文案或语速后重新试听")
		}
		if err = validateWeddingCues(doc.Cues, seconds); err != nil {
			return err
		}
		state.NarrationFile = find("narration").File
		state.NarrationURL = find("narration").URL
		state.NarrationSeconds = seconds
		state.Cues = doc.Cues
	case 6:
		if err := require("storyboard"); err != nil {
			return err
		}
		var doc struct {
			Synopsis string `json:"synopsis"`
			Shots    []Shot `json:"shots"`
		}
		if err := json.Unmarshal([]byte(find("storyboard").Text), &doc); err != nil {
			return err
		}
		if len(doc.Shots) == 0 {
			return errors.New("分镜为空")
		}
		seen := map[string]bool{}
		for i := range doc.Shots {
			s := &doc.Shots[i]
			if !validID.MatchString(s.ID) || seen[s.ID] || s.ImagePrompt == "" || s.Prompt == "" {
				return errors.New("分镜缺少有效镜号、生图词或视频动作词")
			}
			seen[s.ID] = true
			s.Status = "pending"
			s.Attempt = 1
			s.TaskID = ""
			s.VideoFile = ""
			s.VideoURL = ""
			s.FirstFrameFile = ""
			s.Reserved = false
		}
		p.Shots = doc.Shots
		p.Synopsis = doc.Synopsis
		return weddingLayout(p)
	case 7, 8:
		if err := require("image"); err != nil {
			return err
		}
		if n == 7 && len(arts) < min(3, len(p.Shots)) {
			return errors.New("请先回传三个重点分镜试图")
		}
		for _, art := range arts {
			if art.Kind == "image" {
				for i := range p.Shots {
					if p.Shots[i].ID == art.ShotID {
						p.Shots[i].FirstFrameFile = art.File
					}
				}
			}
		}
		if n == 8 {
			for _, s := range p.Shots {
				if s.FirstFrameFile == "" {
					return fmt.Errorf("%s 缺少首帧", s.ID)
				}
			}
		}
	case 9:
		for _, s := range p.Shots {
			found := false
			for _, art := range arts {
				if art.Kind == "video" && art.ShotID == s.ID {
					found = true
				}
			}
			if !found {
				return fmt.Errorf("%s 视频尚未返回", s.ID)
			}
		}
		for i := range p.Shots {
			for _, art := range arts {
				if art.Kind == "video" && art.ShotID == p.Shots[i].ID {
					info, err := probe(ctx, filepath.Join(a.cfg.DataDir, p.ID, art.File))
					if err != nil {
						return err
					}
					if info.Duration+0.04 < p.Shots[i].EditSeconds {
						return fmt.Errorf("%s 视频时长不足", art.ShotID)
					}
					p.Shots[i].VideoFile = art.File
					p.Shots[i].VideoURL = art.URL
					p.Shots[i].Status = "completed"
				}
			}
		}
	case 10:
		if err := require("music", "selection"); err != nil {
			return err
		}
		count := 0
		for _, art := range arts {
			if art.Kind == "music" {
				count++
			}
		}
		if count < 3 {
			return errors.New("请先上传至少三个真实音乐试听")
		}
		_, err := selectedWeddingArtifact(state.Steps[9], "music")
		return err
	case 11:
		return require("mix", "settings")
	case 12:
		if err := require("mix"); err != nil {
			return err
		}
		duration, err := probeAudio(ctx, filepath.Join(a.cfg.DataDir, p.ID, find("mix").File))
		if err != nil {
			return err
		}
		if math.Abs(duration-float64(p.Duration)) > 0.1 {
			return errors.New("请登记覆盖全片的完整混音")
		}
		state.MixFile = find("mix").File
	case 13:
		return require("preview")
	case 14:
		return require("final", "subtitles", "edit")
	}
	return nil
}

func selectedWeddingArtifact(step domain.WeddingStep, kind string) (*domain.WeddingArtifact, error) {
	var choice struct {
		ArtifactID string `json:"artifactId"`
	}
	for _, art := range step.Artifacts {
		if art.Kind == "selection" {
			if err := json.Unmarshal([]byte(art.Text), &choice); err != nil {
				return nil, err
			}
		}
	}
	for i := range step.Artifacts {
		art := &step.Artifacts[i]
		if art.Kind == kind && art.ID == choice.ArtifactID {
			return art, nil
		}
	}
	return nil, errors.New("选择表 artifactId 必须对应本步实际试听文件")
}

func validateWeddingCues(cues []domain.SubtitleCue, seconds float64) error {
	if len(cues) == 0 {
		return errors.New("需要实际音频时间点，不用字数估算时间")
	}
	previous := 0.0
	seen := map[string]bool{}
	for _, c := range cues {
		if c.ID == "" || seen[c.ID] || math.IsNaN(c.Start) || math.IsNaN(c.End) || math.IsInf(c.Start, 0) || math.IsInf(c.End, 0) || c.Start < previous || c.End <= c.Start || c.End > seconds+0.05 || strings.TrimSpace(c.Text) == "" {
			return errors.New("旁白时间点重叠、越界或无效")
		}
		previous = c.End
		seen[c.ID] = true
	}
	return nil
}

func (a *App) weddingPackHTTP(w http.ResponseWriter, r *http.Request, p *Project) {
	if err := a.checkWeddingArtifacts(p); err != nil {
		fail(w, 409, err)
		return
	}
	var b bytes.Buffer
	z := zip.NewWriter(&b)
	var packErr error
	add := func(name string, data []byte) error {
		if packErr != nil {
			return packErr
		}
		f, err := z.Create(name)
		if err == nil {
			_, err = f.Write(data)
		}
		packErr = err
		return err
	}
	_ = add("upstream-source.json", wedding.Manifest())
	_ = add("LICENSE", wedding.Read("LICENSE"))
	_ = add("ORIGINAL_STAGE_RULES.md", []byte(wedding.Rules(p.Wedding.CurrentStep)))
	plan, _ := json.MarshalIndent(map[string]any{"shots": p.Shots, "cues": p.Wedding.Cues, "facts": p.Wedding.Facts}, "", "  ")
	_ = add("SHOT_PLAN.json", plan)
	if r.URL.Query().Get("kind") == "video" {
		if p.Wedding.CurrentStep < 9 {
			fail(w, 409, errors.New("请先确认完整分镜与首帧"))
			return
		}
		for _, s := range p.Shots {
			if s.FirstFrameFile == "" {
				fail(w, 409, errors.New("缺少选定首帧"))
				return
			}
			pic, err := os.ReadFile(filepath.Join(a.cfg.DataDir, p.ID, s.FirstFrameFile))
			if err != nil {
				fail(w, 409, err)
				return
			}
			_ = add(s.ID+"/first-frame"+filepath.Ext(s.FirstFrameFile), pic)
			_ = add(s.ID+"/video-prompt.txt", []byte(s.Prompt))
			manifest, _ := json.MarshalIndent(map[string]any{"shot": s.ID, "first_frame_sha256": fileDigest(pic), "prompt_sha256": fileDigest([]byte(s.Prompt)), "duration_seconds": s.Duration, "edit_seconds": s.EditSeconds, "ratio": p.Ratio, "role": "first_frame"}, "", "  ")
			_ = add(s.ID+"/manifest.json", manifest)
		}
		_ = add("README.txt", []byte("每镜上传对应首帧并粘贴视频词。确认模型使用首帧槽，按提示词设置时长画幅。完整观看后按镜号回传。本文件包不自动产生人工确认。"))
	} else {
		for _, step := range p.Wedding.Steps {
			for _, art := range step.Artifacts {
				if art.Kind == "photo" {
					photo, err := os.ReadFile(filepath.Join(a.cfg.DataDir, p.ID, art.File))
					if err != nil {
						fail(w, 409, err)
						return
					}
					_ = add("references/"+art.ID+"-"+art.Name, photo)
				}
				if art.Kind == "writing" || art.Kind == "script" || art.Kind == "facts" || art.Kind == "factmap" {
					_ = add(fmt.Sprintf("step-%02d/%s-%s", step.Number, art.ID, art.Name), []byte(art.Text))
				}
			}
		}
		for _, s := range p.Shots {
			_ = add(s.ID+"/image-prompt.txt", []byte(s.ImagePrompt))
			_ = add(s.ID+"/video-prompt.txt", []byte(s.Prompt))
		}
	}
	if err := z.Close(); err != nil {
		packErr = err
	}
	if packErr != nil {
		fail(w, 500, packErr)
		return
	}
	w.Header().Set("Content-Type", "application/zip")
	w.Header().Set("Content-Disposition", `attachment; filename="wedding-production-pack.zip"`)
	_, _ = w.Write(b.Bytes())
}

func (a *App) planWedding(ctx context.Context, p *Project) error {
	n := p.Wedding.CurrentStep
	input := map[string]any{"facts": p.Brief, "custom_prompt": p.CustomPrompt, "script": p.VoiceoverScript, "duration_seconds": p.Duration, "ratio": p.Ratio, "cues": p.Wedding.Cues, "fact_list": p.Wedding.Facts}
	body, _ := json.Marshal(input)
	out, err := a.provider.request(ctx, "POST", "/v1/chat/completions", map[string]any{"model": p.LLMModel, "messages": []map[string]string{{"role": "system", "content": wedding.SystemPrompt(n)}, {"role": "user", "content": string(body)}}, "response_format": map[string]string{"type": "json_object"}, "max_tokens": 10000}, fmt.Sprintf("vowfilm:%s:wedding:%d:%s", p.ID, n, wedding.Digest(input)))
	if err != nil {
		return err
	}
	choices, _ := out["choices"].([]any)
	if len(choices) == 0 {
		return errors.New("导演没有返回结果")
	}
	choice, _ := choices[0].(map[string]any)
	message, _ := choice["message"].(map[string]any)
	text, _ := message["content"].(string)
	text = strings.TrimSpace(strings.TrimSuffix(strings.TrimPrefix(strings.TrimSpace(text), "```json"), "```"))
	var arts []domain.WeddingArtifact
	var facts []domain.WeddingFact
	if n == 2 {
		var doc struct {
			WritingPrompt string               `json:"writingPrompt"`
			Script        string               `json:"script"`
			Facts         []domain.WeddingFact `json:"facts"`
			Notes         []string             `json:"notes"`
		}
		if err = json.Unmarshal([]byte(text), &doc); err != nil {
			return err
		}
		if strings.TrimSpace(doc.Script) == "" || strings.TrimSpace(doc.WritingPrompt) == "" {
			return errors.New("资料尚不足以写文案：" + strings.Join(doc.Notes, "；"))
		}
		facts = doc.Facts
		for _, item := range []struct{ Name, Kind, Text string }{{"WRITING_PROMPT.txt", "writing", doc.WritingPrompt}, {"SCRIPT.txt", "script", doc.Script}} {
			art, err := a.weddingArtifact(p, item.Name, item.Kind, "", []byte(item.Text), "text/plain")
			if err != nil {
				return err
			}
			arts = append(arts, art)
		}
	} else {
		if !json.Valid([]byte(text)) {
			return errors.New("分镜结果不是有效 JSON")
		}
		art, err := a.weddingArtifact(p, "SHOT_PLAN.json", "storyboard", "", []byte(text), "application/json")
		if err != nil {
			return err
		}
		arts = append(arts, art)
	}
	return a.store.Update(p.ID, func(q *Project) error {
		if q.Wedding.CurrentStep != n {
			return errors.New("当前步骤已变化")
		}
		if n == 2 {
			q.Wedding.Facts = facts
		}
		if err := wedding.Prepare(q.Wedding, n, arts); err != nil {
			return err
		}
		q.Status = "planned"
		q.PromptPolicy = wedding.Version()
		event(q, fmt.Sprintf("第%d步材料已生成，请查看当前版本并确认", n))
		return nil
	})
}

func (a *App) runWedding(ctx context.Context, id, mode, shotID string) error {
	p := a.store.Get(id)
	if p.Wedding.Automatic && mode == "generate" {
		return a.runWeddingAutomatic(ctx, id)
	}
	if err := a.validateWeddingAction(p, mode); err != nil {
		return err
	}
	if mode == "plan" {
		return a.planWedding(ctx, p)
	}
	if mode == "render" {
		return a.renderWeddingStage(ctx, p)
	}
	for _, s := range p.Shots {
		if shotID != "" && s.ID != shotID {
			continue
		}
		if s.Status == "completed" && s.VideoFile != "" {
			continue
		}
		if err := a.generateShot(ctx, id, s.ID, nil, ""); err != nil {
			return err
		}
	}
	p = a.store.Get(id)
	arts := []domain.WeddingArtifact{}
	for _, s := range p.Shots {
		if s.Status != "completed" {
			continue
		}
		b, err := os.ReadFile(filepath.Join(a.cfg.DataDir, p.ID, s.VideoFile))
		if err != nil {
			return err
		}
		sum := sha256.Sum256(b)
		arts = append(arts, domain.WeddingArtifact{ID: newID("wa_"), Kind: "video", Name: s.ID + ".mp4", File: s.VideoFile, URL: s.VideoURL, MIME: "video/mp4", SHA256: hex.EncodeToString(sum[:]), Bytes: int64(len(b)), ShotID: s.ID})
	}
	sort.Slice(arts, func(i, j int) bool { return arts[i].ShotID < arts[j].ShotID })
	return a.store.Update(id, func(q *Project) error {
		if err := wedding.Prepare(q.Wedding, 9, arts); err != nil {
			return err
		}
		q.Status = "planned"
		event(q, "视频已返回，请逐镜观看核心动作与衔接后确认")
		return nil
	})
}
