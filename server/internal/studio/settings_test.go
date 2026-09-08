package studio

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestProviderSettingsRoundTrip(t *testing.T) {
	dir := t.TempDir()
	base := Config{
		DataDir:    dir,
		BaseURL:    "https://default.example",
		APIKey:     "default-key",
		LLMModel:   "default-llm",
		VideoModel: "default-video",
		Token:      strings.Repeat("x", 32),
		Concurrency: 1,
	}
	a, err := New(base)
	if err != nil {
		t.Fatal(err)
	}
	srv := httptest.NewServer(a.Handler())
	defer srv.Close()
	request := func(method, path, body string) *http.Response {
		r, _ := http.NewRequest(method, srv.URL+path, strings.NewReader(body))
		r.Header.Set("X-Vowfilm-Token", base.Token)
		r.Header.Set("Content-Type", "application/json")
		resp, e := http.DefaultClient.Do(r)
		if e != nil {
			t.Fatal(e)
		}
		return resp
	}
	resp := request("GET", "/api/config", "")
	var view map[string]any
	if err := json.NewDecoder(resp.Body).Decode(&view); err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	if view["baseUrl"] != "https://default.example" || view["connected"] != true {
		t.Fatal("config view mismatch")
	}
	resp = request("PATCH", "/api/config", `{"baseUrl":"https://custom.example","llmModel":"gpt-test","videoModel":"video-test","apiKey":"secret-key"}`)
	if err := json.NewDecoder(resp.Body).Decode(&view); err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	if resp.StatusCode != 200 || view["baseUrl"] != "https://custom.example" || view["llmModel"] != "gpt-test" {
		t.Fatal("patch did not apply")
	}
	reloaded, err := New(Config{DataDir: dir, Token: base.Token, Concurrency: 1, APIKey: "ignored-on-load"})
	if err != nil {
		t.Fatal(err)
	}
	reloaded.cfgMu.RLock()
	got := reloaded.cfg
	reloaded.cfgMu.RUnlock()
	if got.BaseURL != "https://custom.example" || got.APIKey != "secret-key" || got.LLMModel != "gpt-test" {
		t.Fatal("provider.json not loaded on restart")
	}
	resp = request("PATCH", "/api/config", `{"baseUrl":"https://custom.example","llmModel":"gpt-test","videoModel":"video-test"}`)
	resp.Body.Close()
	if resp.StatusCode != 200 {
		t.Fatal("patch without apiKey should keep existing key")
	}
	reloaded.cfgMu.RLock()
	got = reloaded.cfg
	reloaded.cfgMu.RUnlock()
	if got.APIKey != "secret-key" {
		t.Fatal("api key changed without update")
	}
}
