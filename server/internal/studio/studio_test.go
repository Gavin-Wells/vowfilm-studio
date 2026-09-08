package studio

import (
	"context"
	"encoding/json"
	"io"
	"math"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"
)

func TestTimelineExactAcrossDurationsAndTransitions(t *testing.T) {
	for _, seconds := range []int{60, 120, 180, 240} {
		for _, kind := range []string{"cut", "dissolve", "mixed"} {
			p := &Project{Duration: seconds, Shots: make([]Shot, seconds/60*8)}
			for i := range p.Shots {
				p.Shots[i] = Shot{ID: "shot", Transition: kind}
				if kind == "mixed" {
					p.Shots[i].Transition = "dissolve"
					if i%3 != 0 {
						p.Shots[i].Transition = "cut"
					}
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
func TestRhythmicStylesAndTransitionHandles(t *testing.T) {
	for _, style := range filmStyles {
		for _, duration := range []int{60, 120, 180, 240} {
			p := &Project{Style: style.ID, Duration: duration, Shots: make([]Shot, duration/60*style.ShotsPerMinute)}
			for i := range p.Shots {
				p.Shots[i].Transition = "match"
			}
			p.Shots[3].Transition = "dipwhite"
			if err := layout(p); err != nil {
				t.Fatal(err)
			}
			frames := 0
			for i, s := range p.Shots {
				frames += s.EditFrames
				if i == 3 {
					frames -= 3
				}
				beatFrames := 1440.0 / float64(style.BPM)
				if math.Abs(s.TimelineStart*24-math.Round(s.TimelineStart*24/beatFrames)*beatFrames) > .51 {
					t.Fatal("cut does not land on beat grid")
				}
				if s.EditSeconds+.25 > float64(s.Duration) {
					t.Fatal("insufficient source handles")
				}
			}
			if frames != duration*24 {
				t.Fatal("wrong final frame count")
			}
			if p.Shots[0].EditFrames == p.Shots[6].EditFrames {
				t.Fatal("style lost variable pacing")
			}
			sections := scoreSections(p)
			if len(sections) != 5 || sections[4].End != float64(duration) {
				t.Fatal("score structure does not cover film")
			}
		}
	}
}
func TestMediaShareIsScopedAndExpires(t *testing.T) {
	a := &App{cfg: Config{Token: strings.Repeat("k", 32)}}
	path := "/api/media/film_demo/picture-edit.mp4"
	expires := strconv.FormatInt(time.Now().Add(time.Minute).Unix(), 10)
	r := httptest.NewRequest("GET", path+"?expires="+expires+"&signature="+a.mediaSignature(path, expires), nil)
	if !a.validMediaShare(r) {
		t.Fatal("signed clip unavailable")
	}
	r.URL.Path = "/api/media/another/movie.mp4"
	if a.validMediaShare(r) {
		t.Fatal("signature accepted for other clip")
	}
	r.URL.Path = path
	r.Method = "POST"
	if a.validMediaShare(r) {
		t.Fatal("read signature grants write")
	}
	r.Method = "GET"
	expired := strconv.FormatInt(time.Now().Add(-time.Minute).Unix(), 10)
	r.URL.RawQuery = "expires=" + expired + "&signature=" + a.mediaSignature(path, expired)
	if a.validMediaShare(r) {
		t.Fatal("expired link accepted")
	}
}
func TestNativeAudioArchiveResponse(t *testing.T) {
	var raw any
	_ = json.Unmarshal([]byte(`{"status":"succeeded","audio":[{"url":{"asset_id":"ast_example123","source_replaced":"remote_url"}}]}`), &raw)
	if archivedAssetID(raw) != "ast_example123" {
		t.Fatal("native audio archive lost")
	}
	_ = json.Unmarshal([]byte(`{"audio":[{"url":{"asset_id":"ast_../../secret"}}]}`), &raw)
	if archivedAssetID(raw) != "" {
		t.Fatal("invalid archive path accepted")
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
	if err := a.store.Update(p.ID, func(q *Project) error {
		q.MusicTaskID, q.MusicFile, q.MusicSource = "old-score", "old.mp3", "sonilo"
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	resp = request("PATCH", "/api/projects/"+p.ID, `{"title":"Wedding","brief":"celebrate","duration":60,"style":"joyful","ratio":"16:9"}`)
	resp.Body.Close()
	updated := a.store.Get(p.ID)
	if resp.StatusCode != 200 || updated.MusicTaskID != "" || updated.MusicFile != "" || updated.MusicSource != "" || len(updated.MusicSections) != 5 {
		t.Fatal("style change retained obsolete music")
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
