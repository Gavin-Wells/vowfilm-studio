package studio

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"strconv"
	"strings"
	"time"
)

// Signed media links let an external service read exactly one file for a
// bounded time. They are kept for integrations that need to fetch a clip; the
// score itself is generated from the designed cue sheet and does not upload
// the picture edit anywhere.
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

// SubmitAudio sends one designed cue to the audio model through the gateway's
// unified task endpoint. Only the prompt and output format are sent; the
// gateway archives the result and returns a task ID that the workflow
// persists before polling.
func (p *Provider) SubmitAudio(ctx context.Context, prompt, idempotency string) (string, error) {
	if strings.TrimSpace(prompt) == "" {
		return "", errors.New("配乐提示词为空")
	}
	model := p.config.AudioModel
	if model == "" {
		model = defaultAudioModel
	}
	out, err := p.request(ctx, "POST", "/v1/audio/generate", map[string]any{"model": model, "prompt": prompt, "audio_config": map[string]any{"format": "wav", "sample_rate": 48000}}, idempotency)
	if err != nil {
		return "", fmt.Errorf("配乐提交未完成：%w", err)
	}
	id, _ := out["task_id"].(string)
	if id == "" {
		if data, ok := out["data"].(map[string]any); ok {
			id, _ = data["task_id"].(string)
		}
	}
	if id == "" {
		return "", errors.New("配乐服务未返回任务 ID")
	}
	return id, nil
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
	for _, host := range []string{"embervale.cn", "embervale.ai", "volces.com", "byteimg.com"} {
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
