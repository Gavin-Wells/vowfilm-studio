package studio

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"
	"vowfilm/server/internal/platform"
	"vowfilm/server/internal/storage"
)

type App struct {
	accounts  *platform.Service
	database  *storage.SQLRepository
	billingMu sync.Mutex
	cfg       Config
	cfgMu     sync.RWMutex
	store     *Store
	provider  *Provider
	mu        sync.Mutex
	running   map[string]context.CancelFunc
	slots     chan struct{}
}

func New(cfg Config) (*App, error) {
	if cfg.Concurrency < 1 {
		cfg.Concurrency = 2
	}
	if err := os.MkdirAll(cfg.DataDir, 0700); err != nil {
		return nil, err
	}
	driver := cfg.DatabaseDriver
	if driver == "" {
		driver = "sqlite"
	}
	dsn := cfg.DatabaseURL
	if driver == "sqlite" && dsn == "" {
		dsn = filepath.Join(cfg.DataDir, "platform.sqlite")
	}
	database, err := storage.Open(driver, dsn)
	if err != nil {
		return nil, err
	}
	s, err := NewRepositoryStore(database)
	if err != nil {
		_ = database.Close()
		return nil, err
	}
	// One-time import preserves local projects; SQLite/PG hold all new metadata.
	if len(s.List()) == 0 {
		legacy, err := NewStore(cfg.DataDir)
		if err != nil {
			_ = database.Close()
			return nil, err
		}
		for _, p := range legacy.List() {
			if err = s.Put(p); err != nil {
				_ = database.Close()
				return nil, err
			}
		}
	}
	if cfg.SetupToken == "" {
		path := filepath.Join(cfg.DataDir, "admin-setup.txt")
		raw, err := os.ReadFile(path)
		if os.IsNotExist(err) {
			raw = []byte(platform.Token())
			err = os.WriteFile(path, raw, 0600)
		}
		if err != nil {
			_ = database.Close()
			return nil, err
		}
		cfg.SetupToken = strings.TrimSpace(string(raw))
	}
	cfg = loadProviderOverrides(cfg.DataDir, cfg)
	accounts := platform.New(database, cfg.SetupToken)
	if err = database.ReleaseInterrupted(); err != nil {
		_ = database.Close()
		return nil, err
	}
	a := &App{cfg: cfg, store: s, provider: newProvider(cfg), running: map[string]context.CancelFunc{}, slots: make(chan struct{}, cfg.Concurrency), accounts: accounts, database: database}
	if len(s.List()) == 0 {
		p := &Project{ID: "film_demo", Title: "与你，快乐加倍", Brief: "制作60秒欢快婚礼短片，阳光海边花园、统一成年虚构新人、自然笑容、牵手起跑、转圈、朋友抛花瓣与碰杯庆祝，最后拥抱收尾。用动作和构图衔接镜头，配合有旋律和段落变化的欢快音乐。", Duration: 60, Style: "joyful", Ratio: "16:9", Status: "draft", Demo: true, Shots: []Shot{}, Assets: []Asset{}, Events: []Event{}, CreatedAt: now(), UpdatedAt: now(), Revision: 1, GenerationBudget: 180, OutputResolution: "720P", LLMModel: cfg.LLMModel, VideoModel: cfg.VideoModel}
		event(p, "演示工程已创建，使用虚构人物与原创配乐")
		if err = s.Put(p); err != nil {
			return nil, err
		}
	}
	for _, p := range s.List() {
		if isRunning(p.Status) {
			if err = s.Update(p.ID, func(q *Project) error {
				q.Status = "cancelled"
				event(q, "服务恢复：未完成的冻结额度已释放，请确认新报价后继续")
				return nil
			}); err != nil {
				return nil, err
			}
		}
	}
	return a, nil
}
func (a *App) Shutdown() {
	a.mu.Lock()
	defer a.mu.Unlock()
	for _, cancel := range a.running {
		cancel()
	}
}
func (a *App) cancel(id string) {
	a.mu.Lock()
	cancel := a.running[id]
	a.mu.Unlock()
	if cancel != nil {
		cancel()
	}
	_ = a.store.Update(id, func(p *Project) error {
		if isRunning(p.Status) {
			p.Status = "cancelled"
			event(p, "已暂停等待；已提交的云端镜头保留，继续时查询原任务")
		}
		return nil
	})
}
func (a *App) start(id, mode, shotID string, chargeIDs ...string) error {
	a.cfgMu.RLock()
	apiKey := a.cfg.APIKey
	a.cfgMu.RUnlock()
	current := a.store.Get(id)
	localTemplatePlan := mode == "plan" && current != nil && current.CreationMode == "template"
	if apiKey == "" && mode != "render" && !localTemplatePlan {
		return errors.New("请先配置创作 API Key")
	}
	a.mu.Lock()
	defer a.mu.Unlock()
	if _, ok := a.running[id]; ok {
		return errors.New("该项目已有任务在执行")
	}
	p := a.store.Get(id)
	if p == nil {
		return errors.New("项目不存在")
	}
	if err := validateTemplateProject(p); err != nil {
		return err
	}
	if err := validateCommerceAction(p, mode); err != nil {
		return err
	}
	if mode == "generate" || mode == "shot" {
		for _, asset := range p.Assets {
			if (asset.Role == "bride" || asset.Role == "groom" || asset.Role == "person") && asset.ProviderAssetID == "" {
				return errors.New("人物照片需要先在素材库中绑定已完成本人授权的 asset:// 素材")
			}
		}
	}
	if mode == "review" && len(p.Shots) == 0 {
		return errors.New("请先编排分镜再审阅")
	}
	if mode == "render" {
		if len(p.Shots) == 0 {
			return errors.New("请先生成镜头")
		}
		for _, s := range p.Shots {
			if s.Status != "completed" {
				return errors.New("所有镜头完成后才能合成影片")
			}
		}
	}
	err := a.store.Update(id, func(q *Project) error {
		if mode == "generate" && q.CreationMode == "template" {
			// Re-apply the catalog schedule before retrying or resuming a
			// template. This upgrades source durations for short net cuts while
			// preserving completed media and transition choices.
			if err := layout(q); err != nil {
				return err
			}
		}
		if mode == "shot" {
			found := false
			for i := range q.Shots {
				if q.Shots[i].ID == shotID {
					found = true
					s := &q.Shots[i]
					if s.Status == "completed" || s.Status == "failed" {
						if s.Attempt >= 3 {
							return errors.New("这个镜头已达到三次生成上限")
						}
						resetShot(s)
						s.Reserved = false
					}
				}
			}
			if !found {
				return errors.New("镜头不存在")
			}
			q.FilmURL = ""
		}
		if mode == "generate" {
			for i := range q.Shots {
				s := &q.Shots[i]
				if s.Status == "failed" && s.TaskID != "" {
					if s.Attempt >= 3 {
						return fmt.Errorf("镜头 %s 已达到三次生成上限", s.Title)
					}
					resetShot(s)
					s.Reserved = false
				}
			}
		}
		q.Status = "generating"
		if mode == "plan" || len(q.Shots) == 0 {
			q.Status = "planning"
		}
		if mode == "render" {
			q.Status = "rendering"
		}
		event(q, "已启动制作任务")
		return nil
	})
	if err != nil {
		return err
	}
	chargeID := ""
	if len(chargeIDs) > 0 {
		chargeID = chargeIDs[0]
	}
	ctx, cancel := context.WithTimeout(context.Background(), 90*time.Minute)
	a.running[id] = cancel
	go func() {
		defer cancel()
		defer func() { a.mu.Lock(); delete(a.running, id); a.mu.Unlock() }()
		err := a.run(ctx, id, mode, shotID)
		if chargeID != "" {
			if settleErr := a.accounts.Repo.Settle(chargeID, err == nil); settleErr != nil {
				log.Printf("settlement %s: %v", chargeID, settleErr)
			}
		}
		if err != nil {
			log.Printf("project %s: %v", id, err)
			_ = a.store.Update(id, func(q *Project) error {
				if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
					q.Status = "cancelled"
					event(q, "制作已暂停，已提交任务可继续恢复")
				} else {
					q.Status = "failed"
					event(q, err.Error())
				}
				return nil
			})
		}
	}()
	return nil
}
func (a *App) run(ctx context.Context, id, mode string, selectedShots ...string) error {
	p := a.store.Get(id)
	if p == nil {
		return errors.New("项目不存在")
	}
	if err := validateCommerceAction(p, mode); err != nil {
		return err
	}
	if mode == "review" && len(p.Shots) > 0 {
		if err := a.store.Update(id, func(q *Project) error {
			q.Status = "planning"
			event(q, "GPT-6 正在审阅现有分镜与字幕")
			return nil
		}); err != nil {
			return err
		}
		synopsis, captions, err := a.provider.Review(ctx, p)
		if err != nil {
			return err
		}
		if err = a.store.Update(id, func(q *Project) error {
			q.Synopsis = synopsis
			q.LLMModel = a.cfg.LLMModel
			for i := range q.Shots {
				q.Shots[i].Caption = captions[q.Shots[i].ID]
			}
			event(q, "GPT-6 已完成导演审阅与字幕修订，保留已生成镜头")
			return nil
		}); err != nil {
			return err
		}
		p = a.store.Get(id)
	}
	if mode == "plan" || len(p.Shots) == 0 {
		if err := a.store.Update(id, func(q *Project) error {
			q.Status = "planning"
			q.Progress = 6
			event(q, "导演正在编排故事、镜头与转场")
			return nil
		}); err != nil {
			return err
		}
		if p.CreationMode != "template" && p.Treatment == nil && sceneID(p) != "commerce" {
			_ = a.store.Update(id, func(q *Project) error {
				if sceneID(q) == "commerce" {
					event(q, "导演正在理解商品资料，规划广告结构与素材职责")
				} else {
					event(q, "GPT-6 正在理解自定义要求，规划现场叙事与分章造型")
				}
				return nil
			})
			treatment, err := a.provider.Develop(ctx, p)
			if err != nil {
				return fmt.Errorf("导演方案未完成：%w", err)
			}
			if err = a.store.Update(id, func(q *Project) error {
				q.Treatment = treatment
				if sceneID(q) == "commerce" {
					event(q, "广告方案已保存，正在编排镜头动作与首尾状态")
				} else {
					event(q, "故事与造型方案已保存，正在编排动作与换装衔接")
				}
				return nil
			}); err != nil {
				return err
			}
			p = a.store.Get(id)
		}
		var synopsis string
		var shots []Shot
		var err error
		if p.CreationMode == "template" {
			synopsis, shots, err = templatePlan(p)
		} else {
			synopsis, shots, err = a.provider.Plan(ctx, p)
		}
		if err != nil {
			return fmt.Errorf("分镜编排未完成：%w", err)
		}
		if err = a.store.Update(id, func(q *Project) error {
			q.Synopsis = synopsis
			q.Shots = shots
			q.MusicSections = scoreSections(q)
			if sceneID(q) == "commerce" {
				q.GenerationMode = commerceDirectMode
				q.Treatment = nil
				q.MusicSections = nil
			}
			q.MusicTaskID = ""
			q.MusicFile = ""
			q.MusicSource = ""
			q.Revision++
			q.FilmURL = ""
			q.LLMModel = a.cfg.LLMModel
			q.VideoModel = a.cfg.VideoModel
			q.PromptPolicy = advertisingVersion(q)
			if q.CreationMode == "template" {
				q.GenerationMode = "template-fixed"
				q.PromptPolicy = q.TemplateID + ":" + q.TemplateVersion
			}
			if err := layout(q); err != nil {
				return err
			}
			q.Status = "planned"
			q.Progress = 16
			if isDirectCommerce(q) {
				event(q, "v3 整条广告指令已就绪：一次直出15秒，保留原生声音")
			} else if q.CreationMode == "template" {
				event(q, fmt.Sprintf("模板已载入：%d 个固定镜头，等待上传素材或生成", len(shots)))
			} else {
				event(q, fmt.Sprintf("星网导演已完成 %d 个镜头与转场编排", len(shots)))
			}
			return nil
		}); err != nil {
			return err
		}
		if mode == "plan" {
			return nil
		}
	}
	if mode == "review" {
		return a.store.Update(id, func(q *Project) error { q.Status = "planned"; return nil })
	}
	if mode != "render" {
		if p.Treatment == nil && p.Style == "garden" && strings.Contains(p.VideoModel, "fast") {
			a.importProbe(id)
		}
		p = a.store.Get(id)
		refs, guide, err := a.references(p)
		if err != nil {
			return err
		}
		if err = a.store.Update(id, func(q *Project) error {
			q.Status = "generating"
			event(q, "开始生成镜头，已完成的片段会自动保存")
			return nil
		}); err != nil {
			return err
		}
		var wg sync.WaitGroup
		errs := make(chan error, len(p.Shots))
		for _, s := range p.Shots {
			if mode == "shot" && len(selectedShots) > 0 && s.ID != selectedShots[0] {
				continue
			}
			if s.Status == "completed" && s.VideoFile != "" {
				continue
			}
			sid := s.ID
			wg.Add(1)
			go func() {
				defer wg.Done()
				select {
				case a.slots <- struct{}{}:
					defer func() { <-a.slots }()
				case <-ctx.Done():
					errs <- ctx.Err()
					return
				}
				if err := a.generateShot(ctx, id, sid, refs, guide); err != nil {
					errs <- err
				}
			}()
		}
		wg.Wait()
		close(errs)
		var all []string
		for err := range errs {
			if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
				return err
			}
			all = append(all, err.Error())
		}
		if len(all) > 0 {
			return errors.New(strings.Join(all, "；"))
		}
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	if mode == "shot" {
		return a.store.Update(id, func(q *Project) error {
			q.Status = "planned"
			event(q, "局部镜头已完成，可单独合成影片")
			return nil
		})
	}
	if err := a.store.Update(id, func(q *Project) error {
		q.Status = "rendering"
		q.Progress = 86
		if isDirectCommerce(q) {
			event(q, "整条广告已生成，正在校验15秒画面与原生音轨")
		} else {
			event(q, "镜头已就绪，正在剪辑、排字幕与配乐")
		}
		return nil
	}); err != nil {
		return err
	}
	p = a.store.Get(id)
	film, err := a.render(ctx, p)
	if err != nil {
		return fmt.Errorf("影片合成未完成：%w", err)
	}
	return a.store.Update(id, func(q *Project) error {
		q.FilmURL = mediaURL(id, film)
		q.Status = "completed"
		q.Progress = 100
		event(q, fmt.Sprintf("%d 秒影片已完成，分辨率 720P，可下载成片与工程", q.Duration))
		return nil
	})
}
func (a *App) importProbe(id string) {
	p := a.store.Get(id)
	if id != "film_demo" || len(p.Shots) == 0 || p.Shots[0].TaskID != "" || p.Shots[0].VideoFile != "" || p.GeneratedSeconds > 0 {
		return
	}
	var probe struct {
		Response struct {
			TaskID string `json:"task_id"`
		} `json:"response"`
		Request struct {
			Content []struct {
				Text string `json:"text"`
			} `json:"content"`
		} `json:"request"`
	}
	raw, err := os.ReadFile(filepath.Join(a.cfg.DataDir, "seedance-probe.json"))
	if err != nil || json.Unmarshal(raw, &probe) != nil || probe.Response.TaskID == "" {
		return
	}
	_ = a.store.Update(id, func(q *Project) error {
		q.Shots[0].TaskID = probe.Response.TaskID
		q.Shots[0].Status = "running"
		q.Shots[0].Reserved = true
		q.GeneratedSeconds += 9
		if len(probe.Request.Content) > 0 {
			q.Shots[0].Prompt = probe.Request.Content[0].Text
		}
		event(q, "复用已提交的 Seedance 花园开场镜头")
		return nil
	})
}
func (a *App) references(p *Project) ([]map[string]any, string, error) {
	refs := []map[string]any{}
	guides := []string{}
	for _, asset := range p.Assets {
		if asset.Role == "music" {
			continue
		}
		if len(refs) >= 9 {
			return nil, "", errors.New("当前模型最多使用 9 张参考图片，请减少素材")
		}
		u := ""
		if asset.ProviderAssetID != "" {
			u = "asset://" + asset.ProviderAssetID
		} else {
			if asset.Role == "bride" || asset.Role == "groom" || asset.Role == "person" {
				return nil, "", errors.New("人物素材尚未授权")
			}
			raw, err := os.ReadFile(filepath.Join(a.cfg.DataDir, p.ID, asset.File))
			if err != nil {
				return nil, "", err
			}
			if len(raw) > 8<<20 {
				return nil, "", errors.New("场景参考图片请压缩至 8 MB 以内后上传")
			}
			u = "data:" + asset.MIME + ";base64," + base64.StdEncoding.EncodeToString(raw)
		}
		refs = append(refs, map[string]any{"type": "image_url", "image_url": map[string]string{"url": u}, "role": "reference_image"})
		role := map[string]string{"bride": "新娘的人物身份和面貌；服装按本镜造型要求", "groom": "新郎的人物身份和面貌；服装按本镜造型要求", "person": "指定人物身份和面貌，服装按本镜要求", "product": "商品外观、包装、颜色和比例，不添加未知功效", "reference": "环境和视觉风格，不替换人物身份"}[asset.Role]
		guides = append(guides, fmt.Sprintf("图片%d参考%s。", len(refs), role))
	}
	return refs, strings.Join(guides, ""), nil
}
func (a *App) generateShot(ctx context.Context, id, sid string, refs []map[string]any, guide string) error {
	p := a.store.Get(id)
	var s Shot
	for _, v := range p.Shots {
		if v.ID == sid {
			s = v
		}
	}
	if s.ID == "" {
		return errors.New("镜头不存在")
	}
	if s.TaskID == "" {
		err := a.store.Update(id, func(q *Project) error {
			for i := range q.Shots {
				if q.Shots[i].ID == sid {
					v := &q.Shots[i]
					if !v.Reserved {
						if q.GeneratedSeconds+v.Duration > q.GenerationBudget {
							return errors.New("本项目已达到生成预算上限，请保留当前成果")
						}
						q.GeneratedSeconds += v.Duration
						v.Reserved = true
					}
					v.Status = "submitting"
					event(q, "提交镜头："+v.Title)
					return nil
				}
			}
			return errors.New("镜头不存在")
		})
		if err != nil {
			return err
		}
		s.Prompt = guide + s.Prompt
		task, err := a.provider.Submit(ctx, p, s, refs)
		if err != nil {
			return fmt.Errorf("%s 提交未完成（再次继续使用同一幂等键）：%w", s.Title, err)
		}
		s.TaskID = task
		if err = a.store.Update(id, func(q *Project) error {
			for i := range q.Shots {
				if q.Shots[i].ID == sid {
					q.Shots[i].TaskID = task
					q.Shots[i].Status = "running"
				}
			}
			return nil
		}); err != nil {
			return err
		}
	}
	failures := 0
	var source string
	for {
		status, u, err := a.provider.Poll(ctx, s.TaskID)
		if err != nil {
			if ctx.Err() != nil {
				return ctx.Err()
			}
			if status == "failed" || status == "cancelled" || status == "expired" {
				_ = a.store.Update(id, func(q *Project) error {
					for i := range q.Shots {
						if q.Shots[i].ID == sid {
							q.Shots[i].Status = "failed"
							q.Shots[i].Error = err.Error()
						}
					}
					event(q, "镜头未完成："+s.Title)
					return nil
				})
				return fmt.Errorf("%s：%w", s.Title, err)
			}
			failures++
			if failures >= 5 {
				return fmt.Errorf("%s 状态查询暂时中断，可继续查询原任务", s.Title)
			}
		} else {
			failures = 0
		}
		if status == "succeeded" && err == nil && u != "" {
			source = u
			break
		}
		select {
		case <-time.After(8 * time.Second):
		case <-ctx.Done():
			return ctx.Err()
		}
	}
	dir := filepath.Join(a.cfg.DataDir, id)
	if err := os.MkdirAll(dir, 0700); err != nil {
		return err
	}
	file := fmt.Sprintf("%s-r%d-a%d.mp4", sid, p.Revision, s.Attempt)
	path := filepath.Join(dir, file)
	if err := download(ctx, source, path); err != nil {
		return fmt.Errorf("%s 下载未完成：%w", s.Title, err)
	}
	info, err := probe(ctx, path)
	mediaErr := err
	if mediaErr == nil {
		if isDirectCommerce(p) {
			mediaErr = validateDirectMedia(p, info)
		} else if p.CreationMode == "template" && info.Duration+0.08 < s.EditSeconds {
			mediaErr = errors.New("模板片段画面时长不足")
		} else if p.CreationMode != "template" && info.Duration+0.08 < s.EditSeconds+0.25 {
			mediaErr = errors.New("生成片段时长不足")
		}
	}
	if mediaErr != nil {
		_ = a.store.Update(id, func(q *Project) error {
			for i := range q.Shots {
				if q.Shots[i].ID == sid {
					q.Shots[i].Status = "failed"
					q.Shots[i].Error = mediaErr.Error()
				}
			}
			return nil
		})
		return fmt.Errorf("%s 的媒体检查未通过：%w", s.Title, mediaErr)
	}
	thumb := strings.TrimSuffix(file, ".mp4") + ".jpg"
	if err = thumbnail(ctx, path, filepath.Join(dir, thumb)); err != nil {
		return err
	}
	return a.store.Update(id, func(q *Project) error {
		for i := range q.Shots {
			if q.Shots[i].ID == sid {
				v := &q.Shots[i]
				v.Status = "completed"
				v.Error = ""
				v.VideoURL = mediaURL(id, file)
				v.VideoFile = file
				v.ThumbnailURL = mediaURL(id, thumb)
			}
		}
		done := 0
		for _, v := range q.Shots {
			if v.Status == "completed" {
				done++
			}
		}
		q.Progress = 18 + done*65/len(q.Shots)
		if q.PosterURL == "" || sid == "S04" {
			q.PosterURL = mediaURL(id, thumb)
		}
		event(q, fmt.Sprintf("镜头 %s 已完成并通过媒体检查（%d/%d）", s.Title, done, len(q.Shots)))
		return nil
	})
}
func download(ctx context.Context, raw, path string) error {
	u, err := url.Parse(raw)
	if err != nil || u.Scheme != "https" {
		return errors.New("视频地址无效")
	}
	host := strings.ToLower(u.Hostname())
	allowed := false
	for _, domain := range []string{"embervale.cn", "embervale.ai", "volces.com", "byteimg.com", "bytepluscdn.com"} {
		if host == domain || strings.HasSuffix(host, "."+domain) {
			allowed = true
		}
	}
	if !allowed {
		return errors.New("视频返回了未受信任的下载域名")
	}
	client := &http.Client{Timeout: 3 * time.Minute, CheckRedirect: func(_ *http.Request, _ []*http.Request) error { return http.ErrUseLastResponse }}
	req, _ := http.NewRequestWithContext(ctx, "GET", raw, nil)
	resp, err := client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		return fmt.Errorf("视频下载 HTTP %d", resp.StatusCode)
	}
	file, err := os.OpenFile(path+".part", os.O_CREATE|os.O_TRUNC|os.O_WRONLY, 0600)
	if err != nil {
		return err
	}
	n, err := io.Copy(file, io.LimitReader(resp.Body, 301<<20))
	closeErr := file.Close()
	if err != nil {
		_ = os.Remove(path + ".part")
		return err
	}
	if closeErr != nil {
		return closeErr
	}
	if n > 300<<20 {
		_ = os.Remove(path + ".part")
		return errors.New("视频文件超出大小限制")
	}
	return os.Rename(path+".part", path)
}
