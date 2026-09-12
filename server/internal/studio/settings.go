package studio

import (
	"encoding/json"
	"errors"
	"net/url"
	"os"
	"path/filepath"
	"strings"
)

type providerFile struct {
	BaseURL    string `json:"baseUrl"`
	APIKey     string `json:"apiKey,omitempty"`
	LLMModel   string `json:"llmModel"`
	VideoModel string `json:"videoModel"`
	AudioModel string `json:"audioModel,omitempty"`
}

func providerPath(dataDir string) string {
	return filepath.Join(dataDir, "provider.json")
}

func loadProviderOverrides(dataDir string, base Config) Config {
	raw, err := os.ReadFile(providerPath(dataDir))
	if err != nil {
		return base
	}
	var file providerFile
	if json.Unmarshal(raw, &file) != nil {
		return base
	}
	if v := strings.TrimSpace(file.BaseURL); v != "" {
		base.BaseURL = v
	}
	if v := strings.TrimSpace(file.APIKey); v != "" {
		base.APIKey = v
	}
	if v := strings.TrimSpace(file.LLMModel); v != "" {
		base.LLMModel = v
	}
	if v := strings.TrimSpace(file.VideoModel); v != "" {
		base.VideoModel = v
	}
	if v := strings.TrimSpace(file.AudioModel); v != "" {
		base.AudioModel = v
	}
	return base
}

func saveProviderOverrides(dataDir string, cfg Config) error {
	file := providerFile{
		BaseURL:    strings.TrimRight(strings.TrimSpace(cfg.BaseURL), "/"),
		APIKey:     strings.TrimSpace(cfg.APIKey),
		LLMModel:   strings.TrimSpace(cfg.LLMModel),
		VideoModel: strings.TrimSpace(cfg.VideoModel),
		AudioModel: strings.TrimSpace(cfg.AudioModel),
	}
	raw, err := json.MarshalIndent(file, "", "  ")
	if err != nil {
		return err
	}
	tmp := providerPath(dataDir) + ".tmp"
	if err = os.WriteFile(tmp, raw, 0600); err != nil {
		return err
	}
	return os.Rename(tmp, providerPath(dataDir))
}

func validateProviderBaseURL(raw string) error {
	u, err := url.Parse(strings.TrimSpace(raw))
	if err != nil || u.Scheme == "" || u.Host == "" {
		return errors.New("API 地址需为有效的 http 或 https URL")
	}
	if u.Scheme != "http" && u.Scheme != "https" {
		return errors.New("API 地址仅支持 http 或 https")
	}
	return nil
}

func apiKeyHint(key string) string {
	key = strings.TrimSpace(key)
	if key == "" {
		return ""
	}
	if len(key) <= 4 {
		return "****"
	}
	return "…" + key[len(key)-4:]
}

// audioModel must be called with cfgMu held.
func (a *App) audioModel() string {
	if a.cfg.AudioModel == "" {
		return defaultAudioModel
	}
	return a.cfg.AudioModel
}

func (a *App) configView() map[string]any {
	a.cfgMu.RLock()
	defer a.cfgMu.RUnlock()
	return map[string]any{
		"connected":             a.cfg.APIKey != "",
		"baseUrl":               a.cfg.BaseURL,
		"llmModel":              a.cfg.LLMModel,
		"videoModel":            a.cfg.VideoModel,
		"audioModel":            a.audioModel(),
		"apiKeySet":             a.cfg.APIKey != "",
		"apiKeyHint":            apiKeyHint(a.cfg.APIKey),
		"maxDuration":           240,
		"styles":                filmStyles,
		"generationConcurrency": a.cfg.Concurrency,
	}
}

func (a *App) updateProviderSettings(in providerFile, updateKey bool) error {
	a.mu.Lock()
	defer a.mu.Unlock()
	if len(a.running) > 0 {
		return errors.New("请等待所有创作任务结束后修改引擎配置")
	}
	baseURL := strings.TrimSpace(in.BaseURL)
	llmModel := strings.TrimSpace(in.LLMModel)
	videoModel := strings.TrimSpace(in.VideoModel)
	audioModel := strings.TrimSpace(in.AudioModel)
	if audioModel == "" {
		audioModel = defaultAudioModel
	}
	if baseURL == "" {
		return errors.New("请填写 API 地址")
	}
	if err := validateProviderBaseURL(baseURL); err != nil {
		return err
	}
	if llmModel == "" {
		return errors.New("请填写 LLM 模型标识")
	}
	if videoModel == "" {
		return errors.New("请填写视频模型标识")
	}
	a.cfgMu.Lock()
	defer a.cfgMu.Unlock()
	next := a.cfg
	next.BaseURL = strings.TrimRight(baseURL, "/")
	next.LLMModel = llmModel
	next.VideoModel = videoModel
	next.AudioModel = audioModel
	if updateKey {
		next.APIKey = strings.TrimSpace(in.APIKey)
	}
	if next.APIKey == "" {
		return errors.New("请填写 API Key")
	}
	if err := saveProviderOverrides(next.DataDir, next); err != nil {
		return errors.New("保存创作引擎配置失败")
	}
	a.cfg = next
	a.provider = newProvider(next)
	return nil
}
