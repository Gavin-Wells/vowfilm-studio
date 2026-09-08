package studio

import (
	"crypto/subtle"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"mime"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"strings"
)

var validID = regexp.MustCompile(`^[A-Za-z0-9_-]+$`)
var validMedia = regexp.MustCompile(`^[A-Za-z0-9_-]+\.(mp4|jpg|jpeg|png|webp|wav|mp3|m4a|ogg)$`)
var assetID = regexp.MustCompile(`^asset-[A-Za-z0-9_-]+$`)

func respond(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.Header().Set("Cache-Control", "no-store")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}
func fail(w http.ResponseWriter, status int, err error) {
	respond(w, status, map[string]string{"error": err.Error()})
}
func decode(w http.ResponseWriter, r *http.Request, v any) error {
	r.Body = http.MaxBytesReader(w, r.Body, 1<<20)
	d := json.NewDecoder(r.Body)
	d.DisallowUnknownFields()
	if err := d.Decode(v); err != nil {
		return errors.New("请求内容无效")
	}
	if d.Decode(new(any)) != io.EOF {
		return errors.New("请求只能包含一个 JSON 对象")
	}
	return nil
}
func (a *App) Handler() http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("X-Content-Type-Options", "nosniff")
		if r.URL.Path == "/healthz" {
			respond(w, 200, map[string]string{"status": "ok"})
			return
		}
		if subtle.ConstantTimeCompare([]byte(r.Header.Get("X-Vowfilm-Token")), []byte(a.cfg.Token)) != 1 {
			fail(w, 401, errors.New("需要有效的创作服务凭证"))
			return
		}
		switch {
		case r.URL.Path == "/api/config" && r.Method == "GET":
			respond(w, 200, map[string]any{"connected": a.cfg.APIKey != "", "llmModel": a.cfg.LLMModel, "videoModel": a.cfg.VideoModel, "maxDuration": 240, "generationConcurrency": a.cfg.Concurrency})
		case r.URL.Path == "/api/projects":
			a.projectsHTTP(w, r)
		case strings.HasPrefix(r.URL.Path, "/api/projects/"):
			a.projectHTTP(w, r)
		case strings.HasPrefix(r.URL.Path, "/api/media/") && (r.Method == "GET" || r.Method == "HEAD"):
			a.mediaHTTP(w, r)
		default:
			fail(w, 404, errors.New("接口不存在"))
		}
	})
}
func (a *App) projectsHTTP(w http.ResponseWriter, r *http.Request) {
	if r.Method == "GET" {
		respond(w, 200, a.store.List())
		return
	}
	if r.Method != "POST" {
		fail(w, 405, errors.New("不支持的请求方法"))
		return
	}
	var p Project
	var in struct {
		Title    string `json:"title"`
		Brief    string `json:"brief"`
		Duration int    `json:"duration"`
		Style    string `json:"style"`
		Ratio    string `json:"ratio"`
	}
	if err := decode(w, r, &in); err != nil {
		fail(w, 400, err)
		return
	}
	p.Title = strings.TrimSpace(in.Title)
	p.Brief = in.Brief
	p.Duration = in.Duration
	p.Style = in.Style
	p.Ratio = in.Ratio
	if err := validateProject(&p); err != nil {
		fail(w, 400, err)
		return
	}
	p.ID = newID("film_")
	p.Status = "draft"
	p.CreatedAt = now()
	p.UpdatedAt = p.CreatedAt
	p.Revision = 1
	p.GenerationBudget = p.Duration * 3
	p.Shots = []Shot{}
	p.Assets = []Asset{}
	p.Events = []Event{}
	p.OutputResolution = "720P"
	p.Demo = true
	p.LLMModel = a.cfg.LLMModel
	p.VideoModel = a.cfg.VideoModel
	event(&p, "影片工程已创建")
	if err := a.store.Put(&p); err != nil {
		fail(w, 500, err)
		return
	}
	respond(w, 201, &p)
}
func (a *App) projectHTTP(w http.ResponseWriter, r *http.Request) {
	parts := strings.Split(strings.TrimPrefix(r.URL.Path, "/api/projects/"), "/")
	if !validID.MatchString(parts[0]) {
		fail(w, 400, errors.New("项目标识无效"))
		return
	}
	id := parts[0]
	p := a.store.Get(id)
	if p == nil {
		fail(w, 404, errors.New("项目不存在"))
		return
	}
	if len(parts) == 1 {
		if r.Method == "GET" {
			respond(w, 200, p)
			return
		}
		if r.Method == "PATCH" {
			var in struct {
				Title    string `json:"title"`
				Brief    string `json:"brief"`
				Duration int    `json:"duration"`
				Style    string `json:"style"`
				Ratio    string `json:"ratio"`
			}
			if err := decode(w, r, &in); err != nil {
				fail(w, 400, err)
				return
			}
			err := a.store.Update(id, func(q *Project) error {
				if isRunning(q.Status) {
					return errors.New("请先暂停当前生成任务")
				}
				q.Title = strings.TrimSpace(in.Title)
				q.Brief = in.Brief
				q.Duration = in.Duration
				q.Style = in.Style
				q.Ratio = in.Ratio
				if err := validateProject(q); err != nil {
					return err
				}
				q.Shots = []Shot{}
				q.FilmURL = ""
				q.Status = "draft"
				q.Progress = 0
				q.Revision++
				q.GenerationBudget = q.Duration * 3
				event(q, "创作设置已更新，请重新编排分镜")
				return nil
			})
			if err != nil {
				fail(w, 400, err)
				return
			}
			respond(w, 200, a.store.Get(id))
			return
		}
	}
	if len(parts) == 2 && parts[1] == "export" && r.Method == "GET" {
		w.Header().Set("Content-Disposition", `attachment; filename="vowfilm-project.json"`)
		respond(w, 200, p)
		return
	}
	if len(parts) >= 2 && parts[1] == "assets" {
		if len(parts) == 2 && r.Method == "POST" {
			a.uploadHTTP(w, r, p)
			return
		}
		if len(parts) == 3 && r.Method == "PATCH" {
			a.bindHTTP(w, r, p, parts[2])
			return
		}
	}
	if len(parts) >= 3 && parts[1] == "shots" {
		a.shotHTTP(w, r, p, parts[2:])
		return
	}
	if len(parts) == 2 && r.Method == "POST" {
		mode := parts[1]
		if mode == "cancel" {
			a.cancel(id)
			respond(w, 200, a.store.Get(id))
			return
		}
		if mode == "plan" || mode == "generate" || mode == "render" || mode == "review" {
			if err := a.start(id, mode, ""); err != nil {
				fail(w, 409, err)
				return
			}
			respond(w, 202, a.store.Get(id))
			return
		}
	}
	fail(w, 404, errors.New("接口不存在"))
}
func (a *App) mediaHTTP(w http.ResponseWriter, r *http.Request) {
	parts := strings.Split(strings.TrimPrefix(r.URL.Path, "/api/media/"), "/")
	if len(parts) != 2 || !validID.MatchString(parts[0]) || !validMedia.MatchString(parts[1]) || a.store.Get(parts[0]) == nil {
		fail(w, 404, errors.New("文件不存在"))
		return
	}
	path := filepath.Join(a.cfg.DataDir, parts[0], parts[1])
	info, err := os.Stat(path)
	if err != nil || !info.Mode().IsRegular() {
		fail(w, 404, errors.New("文件不存在"))
		return
	}
	if r.URL.Query().Get("download") == "1" {
		w.Header().Set("Content-Disposition", mime.FormatMediaType("attachment", map[string]string{"filename": parts[1]}))
	}
	w.Header().Set("Cache-Control", "private, max-age=3600")
	http.ServeFile(w, r, path)
}
func (a *App) uploadHTTP(w http.ResponseWriter, r *http.Request, p *Project) {
	if isRunning(p.Status) {
		fail(w, 409, errors.New("请在生成结束或暂停后添加素材"))
		return
	}
	r.Body = http.MaxBytesReader(w, r.Body, 21<<20)
	if err := r.ParseMultipartForm(1 << 20); err != nil {
		fail(w, 400, errors.New("文件过大或上传格式不正确，最大 20 MB"))
		return
	}
	defer r.MultipartForm.RemoveAll()
	role := r.FormValue("role")
	if role != "bride" && role != "groom" && role != "reference" && role != "music" {
		fail(w, 400, errors.New("请选择素材用途"))
		return
	}
	if len(p.Assets) >= 12 {
		fail(w, 400, errors.New("每个项目最多添加 12 个素材"))
		return
	}
	f, h, err := r.FormFile("file")
	if err != nil {
		fail(w, 400, errors.New("没有收到文件"))
		return
	}
	defer f.Close()
	if h.Size > 20<<20 {
		fail(w, 400, errors.New("文件不能超过 20 MB"))
		return
	}
	head := make([]byte, 512)
	n, _ := io.ReadFull(f, head)
	head = head[:n]
	detected := http.DetectContentType(head)
	ext := ""
	switch detected {
	case "image/jpeg":
		ext = ".jpg"
	case "image/png":
		ext = ".png"
	case "image/webp":
		ext = ".webp"
	}
	if role == "music" {
		switch detected {
		case "audio/mpeg":
			ext = ".mp3"
		case "audio/wave", "audio/wav":
			ext = ".wav"
		case "application/ogg":
			ext = ".ogg"
		case "video/mp4":
			ext = ".m4a"
		}
		if !strings.HasPrefix(detected, "audio/") && detected != "application/ogg" && detected != "video/mp4" {
			ext = ""
		}
	}
	if ext == "" || (role != "music" && !strings.HasPrefix(detected, "image/")) {
		fail(w, 400, errors.New("请上传 JPG、PNG、WebP 图片或有效的音频文件"))
		return
	}
	id := newID("asset_")
	dir := filepath.Join(a.cfg.DataDir, p.ID)
	if err = os.MkdirAll(dir, 0700); err != nil {
		fail(w, 500, err)
		return
	}
	name := id + ext
	path := filepath.Join(dir, name)
	out, err := os.OpenFile(path, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0600)
	if err != nil {
		fail(w, 500, err)
		return
	}
	_, err = io.Copy(out, io.MultiReader(strings.NewReader(string(head)), f))
	closeErr := out.Close()
	if err == nil {
		err = closeErr
	}
	if err != nil {
		_ = os.Remove(path)
		fail(w, 500, errors.New("素材保存失败"))
		return
	}
	asset := Asset{ID: id, Name: filepath.Base(h.Filename), Role: role, URL: mediaURL(p.ID, name), File: name, MIME: detected}
	err = a.store.Update(p.ID, func(q *Project) error {
		if isRunning(q.Status) {
			return errors.New("任务已开始，请暂停后添加素材")
		}
		if len(q.Assets) >= 12 {
			return errors.New("素材数量已达上限")
		}
		q.Assets = append(q.Assets, asset)
		if role == "bride" || role == "groom" {
			q.Demo = false
		}
		event(q, "已添加素材："+asset.Name)
		return nil
	})
	if err != nil {
		_ = os.Remove(path)
		fail(w, 409, err)
		return
	}
	respond(w, 201, asset)
}
func (a *App) bindHTTP(w http.ResponseWriter, r *http.Request, p *Project, id string) {
	var in struct {
		ProviderAssetID string `json:"providerAssetId"`
	}
	if err := decode(w, r, &in); err != nil {
		fail(w, 400, err)
		return
	}
	providerID := strings.TrimPrefix(strings.TrimSpace(in.ProviderAssetID), "asset://")
	if !assetID.MatchString(providerID) {
		fail(w, 400, errors.New("请输入有效的已授权 asset://asset-… 素材 ID"))
		return
	}
	exists := false
	for _, v := range p.Assets {
		if v.ID == id {
			exists = true
		}
	}
	if !exists {
		fail(w, 404, errors.New("素材不存在"))
		return
	}
	out, err := a.provider.request(r.Context(), "POST", "/providers/volcengine/open?Action=GetAsset&Version=2024-01-01", map[string]string{"Id": providerID}, "")
	if err != nil {
		fail(w, 400, fmt.Errorf("人物素材验证失败：%w", err))
		return
	}
	result, _ := out["Result"].(map[string]any)
	if result == nil {
		result = out
	}
	status, _ := result["Status"].(string)
	if strings.ToLower(status) != "active" {
		fail(w, 400, errors.New("素材尚未处于 Active 可用状态，请先完成本人授权与素材审核"))
		return
	}
	err = a.store.Update(p.ID, func(q *Project) error {
		if isRunning(q.Status) {
			return errors.New("请暂停后绑定人物素材")
		}
		for i := range q.Assets {
			if q.Assets[i].ID == id {
				q.Assets[i].ProviderAssetID = providerID
				event(q, "人物素材已验证并绑定")
				return nil
			}
		}
		return errors.New("素材不存在")
	})
	if err != nil {
		fail(w, 409, err)
		return
	}
	respond(w, 200, a.store.Get(p.ID))
}
func resetShot(s *Shot) {
	s.Attempt++
	s.Reserved = false
	s.TaskID = ""
	s.VideoURL = ""
	s.VideoFile = ""
	s.ThumbnailURL = ""
	s.Error = ""
	s.Status = "pending"
}
func (a *App) shotHTTP(w http.ResponseWriter, r *http.Request, p *Project, parts []string) {
	id := parts[0]
	if len(parts) == 1 && r.Method == "PATCH" {
		var in struct {
			Prompt string `json:"prompt"`
		}
		if err := decode(w, r, &in); err != nil {
			fail(w, 400, err)
			return
		}
		if strings.TrimSpace(in.Prompt) == "" || len([]rune(in.Prompt)) > 6000 {
			fail(w, 400, errors.New("镜头指令需为 1–6000 字"))
			return
		}
		err := a.store.Update(p.ID, func(q *Project) error {
			if isRunning(q.Status) {
				return errors.New("请等待当前任务结束")
			}
			for i := range q.Shots {
				if q.Shots[i].ID == id {
					s := &q.Shots[i]
					if s.Prompt != in.Prompt {
						if s.TaskID != "" || s.VideoURL != "" {
							if s.Attempt >= 3 {
								return errors.New("这个镜头已达到三次生成上限")
							}
							resetShot(s)
						}
						s.Prompt = in.Prompt
						q.FilmURL = ""
						q.Status = "planned"
						event(q, "已更新镜头指令："+s.Title)
					}
					return nil
				}
			}
			return errors.New("镜头不存在")
		})
		if err != nil {
			fail(w, 409, err)
			return
		}
		respond(w, 200, a.store.Get(p.ID))
		return
	}
	if len(parts) == 2 && parts[1] == "generate" && r.Method == "POST" {
		if err := a.start(p.ID, "shot", id); err != nil {
			fail(w, 409, err)
			return
		}
		respond(w, 202, a.store.Get(p.ID))
		return
	}
	fail(w, 404, errors.New("镜头接口不存在"))
}
func mediaURL(projectID, file string) string { return "/api/media/" + projectID + "/" + file }
