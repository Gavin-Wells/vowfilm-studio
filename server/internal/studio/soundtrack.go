package studio

import (
	"bytes"
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"
)

func (a *App) mediaSignature(path, expires string) string {
	h := hmac.New(sha256.New, []byte(a.cfg.Token))
	_, _ = h.Write([]byte(path + "\n" + expires))
	return hex.EncodeToString(h.Sum(nil))
}
func (a *App) validMediaShare(r *http.Request) bool {
	if !strings.HasPrefix(r.URL.Path, "/api/media/") || (r.Method != "GET" && r.Method != "HEAD") {
		return false
	}
	expires := r.URL.Query().Get("expires")
	stamp, err := strconv.ParseInt(expires, 10, 64)
	if err != nil || stamp < time.Now().Unix() || stamp > time.Now().Add(2*time.Hour).Unix() {
		return false
	}
	return hmac.Equal([]byte(r.URL.Query().Get("signature")), []byte(a.mediaSignature(r.URL.Path, expires)))
}
func (a *App) sharedMediaURL(id, name string) (string, error) {
	base, err := url.Parse(a.cfg.PublicMediaURL)
	if err != nil || base.Scheme != "https" || base.Host == "" {
		return "", errors.New("AI 配乐需要配置可访问的 HTTPS 素材服务地址")
	}
	base.Path = "/api/media/" + id + "/" + name
	expires := strconv.FormatInt(time.Now().Add(time.Hour).Unix(), 10)
	q := url.Values{"expires": {expires}, "signature": {a.mediaSignature(base.Path, expires)}}
	base.RawQuery = q.Encode()
	return base.String(), nil
}
func (p *Provider) SubmitMusic(ctx context.Context, project *Project, mediaURL string) (string, error) {
	var body bytes.Buffer
	w := multipart.NewWriter(&body)
	profile := profileForProject(project)
	direction := profile.Music
	if project.Treatment != nil {
		direction = project.Treatment.MusicDirection
	}
	prompt := fmt.Sprintf("Compose one original instrumental video soundtrack, exactly %d seconds. Authoritative creative music direction: %s. Evolving lead melody, contrasting harmony and five differentiated sections: 0-13%% opening anticipation; 13-40%% melodic development; 40-67%% contrasting bridge and breathing space; 67-87%% celebratory climax; 87-100%% resolved coda. Keep a %d BPM pulse. Follow the creative direction for instruments and genre. No unchanging loop, vocals or speech. Match the picture edit and chapter transitions. Screening context: %s", project.Duration, direction, targetBPM(project), occasionDirection(project))
	if endHold(project) > 0 {
		prompt += fmt.Sprintf(" End the final musical resolution by %.1f seconds, then leave quiet space for the host.", float64(project.Duration)-endHold(project))
	}
	for k, v := range map[string]string{"video_url": mediaURL, "mode": "async", "output_format": "mp3", "variants_num": "1", "preserve_speech": "false", "ducking": "false", "prompt_influence": "1", "prompt": prompt} {
		_ = w.WriteField(k, v)
	}
	_ = w.Close()
	req, err := http.NewRequestWithContext(ctx, "POST", strings.TrimRight(p.config.BaseURL, "/")+"/v1/video-to-music", &body)
	if err != nil {
		return "", err
	}
	req.Header.Set("Content-Type", w.FormDataContentType())
	req.Header.Set("Authorization", "Bearer "+p.config.APIKey)
	req.Header.Set("Idempotency-Key", fmt.Sprintf("vowfilm:%s:r%d:score-v2", project.ID, project.Revision))
	resp, err := p.client.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	raw, err := io.ReadAll(io.LimitReader(resp.Body, 2<<20))
	if err != nil {
		return "", err
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return "", fmt.Errorf("AI 配乐服务返回 HTTP %d", resp.StatusCode)
	}
	var data map[string]any
	if json.Unmarshal(raw, &data) != nil {
		return "", errors.New("配乐服务响应无效")
	}
	id := resp.Header.Get("X-Gateway-Task-ID")
	if id == "" {
		id, _ = data["task_id"].(string)
	}
	if id == "" {
		return "", errors.New("配乐服务未返回任务 ID")
	}
	return id, nil
}
func (a *App) generatedSoundtrack(ctx context.Context, p *Project) (string, error) {
	dir := filepath.Join(a.cfg.DataDir, p.ID)
	if p.MusicFile != "" {
		if s, err := os.Stat(filepath.Join(dir, p.MusicFile)); err == nil && s.Size() > 1000 {
			return filepath.Join(dir, p.MusicFile), nil
		}
	}
	if p.MusicTaskID == "" {
		mediaURL, err := a.sharedMediaURL(p.ID, "picture-edit.mp4")
		if err != nil {
			return "", err
		}
		_ = a.store.Update(p.ID, func(q *Project) error {
			q.Progress = 91
			event(q, "正在按画面与五段情绪结构生成 AI 配乐")
			return nil
		})
		id, err := a.provider.SubmitMusic(ctx, p, mediaURL)
		if err != nil {
			return "", err
		}
		if err = a.store.Update(p.ID, func(q *Project) error { q.MusicTaskID = id; return nil }); err != nil {
			return "", err
		}
		p.MusicTaskID = id
	}
	failures := 0
	for {
		raw, err := a.provider.request(ctx, "GET", "/v1/tasks/"+p.MusicTaskID, nil, "")
		if err != nil {
			failures++
			if failures >= 5 {
				return "", fmt.Errorf("配乐任务暂时无法查询，继续时将恢复原任务：%w", err)
			}
		} else {
			failures = 0
			status, _ := raw["status"].(string)
			if status == "failed" || status == "cancelled" || status == "expired" {
				return "", errors.New("AI 配乐任务未完成，请在素材库上传配乐后重新合成")
			}
			if status == "succeeded" || status == "completed" {
				u := audioURL(raw)
				if u == "" {
					if id := archivedAssetID(raw); id != "" {
						u = "https://cdn.embervale.cn/assets/" + id + "/source.mp3"
					}
				}
				if u == "" {
					return "", errors.New("配乐任务完成但没有返回音频")
				}
				name := fmt.Sprintf("soundtrack-r%d.mp3", p.Revision)
				downloaded := filepath.Join(dir, fmt.Sprintf("score-source-r%d.mp3", p.Revision))
				if err := downloadSound(ctx, u, downloaded); err != nil {
					return "", err
				}
				if err := ffmpeg(ctx, "-i", downloaded, "-af", "loudnorm=I=-18:TP=-2:LRA=11", "-ar", "48000", "-ac", "2", "-c:a", "libmp3lame", "-q:a", "2", filepath.Join(dir, name)); err != nil {
					return "", err
				}
				if err := a.store.Update(p.ID, func(q *Project) error {
					q.MusicFile = name
					q.MusicSource = "sonilo"
					event(q, "AI 配乐已生成，正在完成最终混音")
					return nil
				}); err != nil {
					return "", err
				}
				return filepath.Join(dir, name), nil
			}
		}
		select {
		case <-ctx.Done():
			return "", ctx.Err()
		case <-time.After(8 * time.Second):
		}
	}
}
func audioURL(v any) string {
	switch x := v.(type) {
	case map[string]any:
		for _, k := range []string{"audio_url", "music_url", "url"} {
			if s, ok := x[k].(string); ok && strings.HasPrefix(s, "https://") {
				return s
			}
		}
		for _, k := range []string{"results", "outputs", "variants", "result", "output", "data", "audio", "tracks", "music"} {
			if s := audioURL(x[k]); s != "" {
				return s
			}
		}
	case []any:
		for _, item := range x {
			if s := audioURL(item); s != "" {
				return s
			}
		}
	}
	return ""
}

// Gateway native-task responses replace provider URLs with archive IDs. The
// gateway's CDN exposes the archived source at this stable media path.
func archivedAssetID(v any) string {
	switch x := v.(type) {
	case map[string]any:
		if id, ok := x["asset_id"].(string); ok && strings.HasPrefix(id, "ast_") && validID.MatchString(id) {
			return id
		}
		for _, k := range []string{"results", "audio", "output", "outputs", "data", "url", "variants", "music"} {
			if id := archivedAssetID(x[k]); id != "" {
				return id
			}
		}
	case []any:
		for _, item := range x {
			if id := archivedAssetID(item); id != "" {
				return id
			}
		}
	}
	return ""
}
func downloadSound(ctx context.Context, source, dest string) error {
	u, err := url.Parse(source)
	if err != nil || u.Scheme != "https" {
		return errors.New("配乐下载地址无效")
	}
	allowed := false
	for _, host := range []string{"embervale.cn", "embervale.ai", "sonilo.com", "sonilo.ai", "amazonaws.com", "cloudfront.net", "a04dd7c600d37fd3409c2689a0c2f467.r2.cloudflarestorage.com"} {
		if u.Hostname() == host || strings.HasSuffix(u.Hostname(), "."+host) {
			allowed = true
		}
	}
	if !allowed {
		return fmt.Errorf("配乐地址域名尚未配置：%s", u.Hostname())
	}
	req, err := http.NewRequestWithContext(ctx, "GET", source, nil)
	if err != nil {
		return err
	}
	client := http.Client{Timeout: 3 * time.Minute, CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}
	resp, err := client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		return fmt.Errorf("下载配乐 HTTP %d", resp.StatusCode)
	}
	f, err := os.OpenFile(dest+".tmp", os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0600)
	if err != nil {
		return err
	}
	n, err := io.Copy(f, io.LimitReader(resp.Body, 50<<20+1))
	closeErr := f.Close()
	if err != nil {
		return err
	}
	if closeErr != nil {
		return closeErr
	}
	if n > 50<<20 {
		return errors.New("配乐文件过大")
	}
	return os.Rename(dest+".tmp", dest)
}
