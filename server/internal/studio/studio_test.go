package studio

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestTimelineExactAcrossDurationsAndTransitions(t *testing.T) {
	for _, seconds := range []int{60, 120, 180, 240} {
		for _, kind := range []string{"cut", "dissolve", "mixed"} {
			p := &Project{Duration: seconds, Shots: make([]Shot, seconds/60*8)}
			for i := range p.Shots {
				p.Shots[i] = Shot{ID: "shot", Transition: kind}
				if kind == "mixed" && i%3 != 0 {
					p.Shots[i].Transition = "cut"
				}
			}
			if err := layout(p); err != nil {
				t.Fatal(err)
			}
			frames := 0
			for i, s := range p.Shots {
				frames += s.EditFrames
				if i < len(p.Shots)-1 && s.Transition != "cut" {
					frames -= 12
				}
				if float64(s.Duration) < s.EditSeconds+.25 {
					t.Fatal("missing editing handle")
				}
			}
			if frames != seconds*24 {
				t.Fatalf("%d %s: %d frames", seconds, kind, frames)
			}
		}
	}
}
func TestStorePersistenceAndRollback(t *testing.T) {
	dir := t.TempDir()
	s, err := NewStore(dir)
	if err != nil {
		t.Fatal(err)
	}
	p := &Project{ID: "test", Title: "first"}
	if err = s.Put(p); err != nil {
		t.Fatal(err)
	}
	p.Title = "outside mutation"
	if s.Get("test").Title != "first" {
		t.Fatal("store aliases caller")
	}
	if err = s.Update("test", func(q *Project) error { q.Title = "second"; return nil }); err != nil {
		t.Fatal(err)
	}
	restored, err := NewStore(dir)
	if err != nil || restored.Get("test").Title != "second" {
		t.Fatal("failed to restore")
	}
	_ = os.Mkdir(filepath.Join(dir, "projects.json.tmp"), 0700)
	if err = s.Update("test", func(q *Project) error { q.Title = "should not persist"; return nil }); err == nil {
		t.Fatal("expected disk write failure")
	}
	if s.Get("test").Title != "second" {
		t.Fatal("failed update was not rolled back")
	}
}
func TestProviderIdempotencyAndPolling(t *testing.T) {
	var keys []string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if r.Header.Get("Authorization") != "Bearer test" {
			t.Error("missing provider authorization")
		}
		if r.Method == "POST" {
			keys = append(keys, r.Header.Get("Idempotency-Key"))
			var in map[string]any
			_ = json.NewDecoder(r.Body).Decode(&in)
			if in["model"] != "test-model" {
				t.Error("wrong model")
			}
			_, _ = io.WriteString(w, `{"task_id":"gwtsk_same"}`)
		} else {
			_, _ = io.WriteString(w, `{"status":"succeeded","results":[{"url":"https://cdn.embervale.cn/movie.mp4"}]}`)
		}
	}))
	defer srv.Close()
	provider := newProvider(Config{BaseURL: srv.URL, APIKey: "test", VideoModel: "test-model"})
	p := &Project{ID: "test", Revision: 2, Ratio: "16:9"}
	shot := Shot{ID: "s1", Attempt: 1, Duration: 8, Prompt: "test"}
	for i := 0; i < 2; i++ {
		if _, err := provider.Submit(context.Background(), p, shot, nil); err != nil {
			t.Fatal(err)
		}
	}
	if keys[0] == "" || keys[0] != keys[1] {
		t.Fatal("transport retries must reuse idempotency key")
	}
	shot.Attempt++
	_, _ = provider.Submit(context.Background(), p, shot, nil)
	if keys[0] == keys[2] {
		t.Fatal("new creative attempts need new key")
	}
	status, u, err := provider.Poll(context.Background(), "gwtsk_same")
	if err != nil || status != "succeeded" || u == "" {
		t.Fatal("could not parse gateway result")
	}
}
func TestHTTPAuthValidationAndProjectLifecycle(t *testing.T) {
	a, err := New(Config{DataDir: t.TempDir(), Token: strings.Repeat("x", 32), Concurrency: 1})
	if err != nil {
		t.Fatal(err)
	}
	srv := httptest.NewServer(a.Handler())
	defer srv.Close()
	resp, err := http.Get(srv.URL + "/api/projects")
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	if resp.StatusCode != 401 {
		t.Fatal("API must require authentication")
	}
	request := func(method, path, body string) *http.Response {
		r, _ := http.NewRequest(method, srv.URL+path, strings.NewReader(body))
		r.Header.Set("X-Vowfilm-Token", a.cfg.Token)
		r.Header.Set("Content-Type", "application/json")
		resp, e := http.DefaultClient.Do(r)
		if e != nil {
			t.Fatal(e)
		}
		return resp
	}
	resp = request("POST", "/api/projects", `{"title":"Wedding","brief":"test","duration":61,"style":"garden","ratio":"16:9"}`)
	resp.Body.Close()
	if resp.StatusCode != 400 {
		t.Fatal("invalid duration accepted")
	}
	resp = request("POST", "/api/projects", `{"title":"Wedding","brief":"test","duration":60,"style":"garden","ratio":"16:9"}`)
	var p Project
	_ = json.NewDecoder(resp.Body).Decode(&p)
	resp.Body.Close()
	if resp.StatusCode != 201 || p.ID == "" || p.GenerationBudget != 180 {
		t.Fatal("project creation failed")
	}
	resp = request("POST", "/api/projects/"+p.ID+"/generate", `{}`)
	resp.Body.Close()
	if resp.StatusCode != 409 {
		t.Fatal("unconfigured generation must fail")
	}
	resp = request("GET", "/api/media/"+p.ID+"/secret.env", "")
	resp.Body.Close()
	if resp.StatusCode != 404 {
		t.Fatal("non-media file exposed")
	}
}
func TestASSInputEscaping(t *testing.T) {
	s := assEscape("test{\\pos(0,0)}\nline")
	if strings.ContainsAny(s, "{}\\\n") {
		t.Fatal("ASS control injection")
	}
}
func TestOriginalScore(t *testing.T) {
	p := filepath.Join(t.TempDir(), "score.wav")
	if err := composeMusic(p, 1); err != nil {
		t.Fatal(err)
	}
	raw, _ := os.ReadFile(p)
	if string(raw[:4]) != "RIFF" || len(raw) != 44+24000*2 {
		t.Fatal("invalid generated score")
	}
}
