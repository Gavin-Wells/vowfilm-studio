package studio

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestOptionalCreativeSettingsHTTP(t *testing.T) {
	a, err := New(Config{DataDir: t.TempDir(), Token: strings.Repeat("x", 32)})
	if err != nil {
		t.Fatal(err)
	}
	defer a.database.Close()
	session := testSession(t, a)
	call := func(method, path string, body map[string]any, status int) Project {
		t.Helper()
		raw, _ := json.Marshal(body)
		r := httptest.NewRequest(method, path, bytes.NewReader(raw))
		r.Header.Set("X-Vowfilm-Token", a.cfg.Token)
		r.Header.Set("X-Vowfilm-CSRF", "1")
		r.AddCookie(&http.Cookie{Name: "vowfilm_session", Value: session})
		w := httptest.NewRecorder()
		a.Handler().ServeHTTP(w, r)
		if w.Code != status {
			t.Fatalf("%s %s: %d %s", method, path, w.Code, w.Body.String())
		}
		var p Project
		if status < 300 {
			if err := json.Unmarshal(w.Body.Bytes(), &p); err != nil {
				t.Fatal(err)
			}
		}
		return p
	}
	for _, tc := range []struct {
		scene        string
		duration     int
		style, ratio string
	}{
		{"wedding", 60, "joyful", "16:9"}, {"family", 120, "vintage", "16:9"},
		{"anniversary", 60, "romantic", "16:9"}, {"commerce", 15, "editorial", "9:16"},
	} {
		p := call("POST", "/api/projects", map[string]any{"scene": tc.scene, "brief": "一只三格收纳盒。只使用提供的商品资料。"}, 201)
		if p.Title != "一只三格收纳盒" || !p.AutoTitle || p.Duration != tc.duration || p.Style != tc.style || p.Ratio != tc.ratio || p.GenerationBudget != tc.duration*3 {
			t.Fatalf("defaults mismatch: %+v", p)
		}
		if a.store.Get(p.ID).Title != p.Title || !a.store.Get(p.ID).AutoTitle {
			t.Fatal("automatic name not persisted")
		}
		manual := call("PATCH", "/api/projects/"+p.ID, map[string]any{"scene": tc.scene, "title": "  我的自定义片名  ", "brief": "新的素材。"}, 200)
		if manual.Title != "我的自定义片名" || manual.AutoTitle {
			t.Fatal("manual name overwritten")
		}
		auto := call("PATCH", "/api/projects/"+p.ID, map[string]any{"scene": tc.scene, "title": " \n ", "brief": "新的素材。"}, 200)
		if auto.Title != "新的素材" || !auto.AutoTitle {
			t.Fatal("empty name did not return to automatic naming")
		}
	}
	blank := call("POST", "/api/projects", map[string]any{}, 201)
	if blank.Title == "" || !blank.AutoTitle || blank.Scene != "wedding" {
		t.Fatal("empty draft has no usable defaults")
	}
	long := call("POST", "/api/projects", map[string]any{"brief": strings.Repeat("人", 500)}, 201)
	if len([]rune(long.Title)) != 25 {
		t.Fatal("automatic title was not bounded")
	}
	call("POST", "/api/projects", map[string]any{"title": strings.Repeat("名", 81)}, 400)
	call("POST", "/api/projects", map[string]any{"scene": "commerce", "duration": 30}, 400)
	call("POST", "/api/projects", map[string]any{"ratio": "1:1"}, 400)
	call("POST", "/api/projects", map[string]any{"style": "invalid"}, 400)
	call("POST", "/api/projects", map[string]any{"wardrobeMode": "custom"}, 400)
}
