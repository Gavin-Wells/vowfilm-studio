package studio

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
	"vowfilm/server/internal/platform"
)

func testSession(t *testing.T, a *App) string {
	t.Helper()
	u, _, err := a.accounts.Register(platform.ID("test")+"@example.test", "Tester", "test-password-12345", a.cfg.SetupToken)
	if err != nil {
		t.Fatal(err)
	}
	session, err := a.accounts.NewSession(u, "test")
	if err != nil {
		t.Fatal(err)
	}
	return session
}
func TestAccountsOwnershipBillingAndScenes(t *testing.T) {
	a, err := New(Config{DataDir: t.TempDir(), Token: strings.Repeat("x", 32)})
	if err != nil {
		t.Fatal(err)
	}
	defer a.database.Close()
	call := func(session, method, path string, body any) *httptest.ResponseRecorder {
		t.Helper()
		var b bytes.Buffer
		if body != nil {
			_ = json.NewEncoder(&b).Encode(body)
		}
		r := httptest.NewRequest(method, path, &b)
		r.Header.Set("X-Vowfilm-Token", a.cfg.Token)
		r.Header.Set("X-Vowfilm-CSRF", "1")
		if session != "" {
			r.AddCookie(&http.Cookie{Name: "vowfilm_session", Value: session})
		}
		w := httptest.NewRecorder()
		a.Handler().ServeHTTP(w, r)
		return w
	}
	if w := call("", "POST", "/api/auth/register", map[string]string{"email": "owner@example.test", "name": "Owner", "password": "owner-password-123"}); w.Code != 400 {
		t.Fatal("bootstrap token bypass", w.Code)
	}
	w := call("", "POST", "/api/auth/register", map[string]string{"email": "owner@example.test", "name": "Owner", "password": "owner-password-123", "setupToken": a.cfg.SetupToken})
	if w.Code != 201 {
		t.Fatal(w.Body.String())
	}
	cookies := w.Result().Cookies()
	if len(cookies) != 1 || !cookies[0].HttpOnly || cookies[0].SameSite != http.SameSiteStrictMode {
		t.Fatal("session cookie protections")
	}
	adminSession := cookies[0].Value
	var result struct {
		User         platform.User `json:"user"`
		RecoveryCode string        `json:"recoveryCode"`
	}
	if err = json.Unmarshal(w.Body.Bytes(), &result); err != nil {
		t.Fatal(err)
	}
	admin := result.User
	if bytes.Contains(w.Body.Bytes(), []byte("owner-password-123")) || bytes.Contains(w.Body.Bytes(), []byte("recovery\"")) {
		t.Fatal("credential leaked")
	}
	u, _, err := a.accounts.Register("creator@example.test", "Creator", "creator-password-123", a.cfg.SetupToken)
	if err != nil {
		t.Fatal(err)
	}
	session, err := a.accounts.NewSession(u, "test")
	if err != nil {
		t.Fatal(err)
	}
	if w = call("", "GET", "/api/projects", nil); w.Code != 401 {
		t.Fatal("unauthenticated access")
	}
	w = call(session, "GET", "/api/projects", nil)
	if strings.TrimSpace(w.Body.String()) != "[]" {
		t.Fatal("legacy project leaked", w.Body.String())
	}
	w = call(session, "PATCH", "/api/config", map[string]string{})
	if w.Code != 403 {
		t.Fatal("creator configured provider")
	}
	for _, s := range scenes {
		duration := 60
		if s.ID == "commerce" {
			duration = 15
		}
		w = call(session, "POST", "/api/projects", map[string]any{"title": "Test " + s.Name, "scene": s.ID, "occasion": s.Occasions[0], "duration": duration, "style": "editorial", "ratio": "9:16"})
		if w.Code != 201 {
			t.Fatal(s.ID, w.Body.String())
		}
		var p Project
		_ = json.Unmarshal(w.Body.Bytes(), &p)
		if p.OwnerID != u.ID || p.Scene != s.ID {
			t.Fatal("ownership missing")
		}
		outsider, _, e := a.accounts.Register(platform.ID("outside")+"@example.test", "Other", "outside-password-123", "")
		if e != nil {
			t.Fatal(e)
		}
		outsideSession, e := a.accounts.NewSession(outsider, "test")
		if e != nil {
			t.Fatal(e)
		}
		for _, path := range []string{"/api/projects/" + p.ID, "/api/projects/" + p.ID + "/export", "/api/media/" + p.ID + "/test.mp4"} {
			if w = call(outsideSession, "GET", path, nil); w.Code != 404 {
				t.Fatal("cross-account read", path, w.Code)
			}
		}
		w = call(session, "POST", "/api/billing/quote", map[string]string{"projectId": p.ID, "action": "generate"})
		if w.Code != 201 {
			t.Fatal(w.Body.String())
		}
		var q platform.Quote
		_ = json.Unmarshal(w.Body.Bytes(), &q)
		request := httptest.NewRequest("POST", "/api/projects/"+p.ID+"/generate", strings.NewReader("{}"))
		request.Header.Set("X-Vowfilm-Token", a.cfg.Token)
		request.Header.Set("X-Vowfilm-CSRF", "1")
		request.Header.Set("X-Vowfilm-Quote", q.ID)
		request.Header.Set("Idempotency-Key", q.ID)
		request.AddCookie(&http.Cookie{Name: "vowfilm_session", Value: session})
		rr := httptest.NewRecorder()
		a.Handler().ServeHTTP(rr, request)
		if rr.Code != 409 {
			t.Fatal("insufficient credits accepted", rr.Code)
		}
		if e = a.accounts.Repo.Credit(admin.ID, u.ID, 20000, platform.ID("receipt_"), "test"); e != nil {
			t.Fatal(e)
		}
		// A start failure (missing provider key) must release its authorization.
		rr = httptest.NewRecorder()
		request.Body = http.NoBody
		a.Handler().ServeHTTP(rr, request)
		if rr.Code != 409 {
			t.Fatal("missing provider key accepted")
		}
		wallet, e := a.accounts.Repo.Wallet(u.ID)
		if e != nil || wallet.User.Held != 0 {
			t.Fatal("failed start stranded hold")
		}
		if s.ID != "wedding" && endHold(&Project{Scene: s.ID, Treatment: &Treatment{}}) != 0 {
			t.Fatal("wedding end hold leaked")
		}
	}
	if err = a.accounts.Repo.UpdateUser(admin.ID, u.ID, "viewer", false); err != nil {
		t.Fatal(err)
	}
	if w = call(session, "GET", "/api/auth/me", nil); w.Code != 401 {
		t.Fatal("role change session valid")
	}
	u, _ = a.accounts.Repo.User(u.ID)
	session, _ = a.accounts.NewSession(u, "test")
	if w = call(session, "POST", "/api/projects", map[string]string{}); w.Code != 403 {
		t.Fatal("viewer can write")
	}
	recovery := result.RecoveryCode
	w = call("", "POST", "/api/auth/recover", map[string]string{"email": admin.Email, "password": "new-password-12345", "recoveryCode": recovery})
	if w.Code != 200 {
		t.Fatal(w.Body.String())
	}
	if w = call(adminSession, "GET", "/api/auth/me", nil); w.Code != 401 {
		t.Fatal("password reset did not revoke session")
	}
	w = call("", "POST", "/api/auth/recover", map[string]string{"email": admin.Email, "password": "another-password-123", "recoveryCode": recovery})
	if w.Code == 200 {
		t.Fatal("recovery code reused")
	}
}

func TestSuccessfulTaskSettlesOnce(t *testing.T) {
	model := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var request struct{ Messages []struct{ Content string } }
		if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
			t.Error(err)
			return
		}
		var input struct {
			Count int `json:"shot_count"`
		}
		if err := json.Unmarshal([]byte(request.Messages[1].Content), &input); err != nil {
			t.Error(err)
			return
		}
		shots := make([]Shot, input.Count)
		for i := range shots {
			shots[i] = Shot{Title: "镜头", Prompt: "自然互动", Transition: "cut", EntryAction: "牵手", ExitAction: "微笑"}
		}
		_ = json.NewEncoder(w).Encode(map[string]any{"choices": []any{map[string]any{"message": map[string]string{"content": platform.JSON(map[string]any{"synopsis": "故事", "shots": shots})}}}})
	}))
	defer model.Close()
	a, err := New(Config{DataDir: t.TempDir(), Token: strings.Repeat("t", 32), APIKey: "test-only", BaseURL: model.URL})
	if err != nil {
		t.Fatal(err)
	}
	defer a.database.Close()
	u := &platform.User{ID: "admin", Email: "admin@example.test", Name: "Admin", Password: "test-hash", Recovery: "test-recovery", CreatedAt: now()}
	if err = a.accounts.Repo.Register(u, true); err != nil {
		t.Fatal(err)
	}
	if err = a.accounts.Repo.Credit(u.ID, u.ID, 5000, "task-success", "test"); err != nil {
		t.Fatal(err)
	}
	p := &Project{ID: "film_success", OwnerID: u.ID, Title: "Test", Duration: 60, Ratio: "16:9", Style: "joyful", WardrobeMode: "auto", Revision: 1, Status: "draft", GenerationBudget: 180}
	treatment := fixtureTreatment()
	if err = normalizeTreatment(p, &treatment); err != nil {
		t.Fatal(err)
	}
	p.Treatment = &treatment
	if err = a.store.Put(p); err != nil {
		t.Fatal(err)
	}
	q, err := a.quote(u.ID, a.store.Get(p.ID), "plan", "")
	if err != nil {
		t.Fatal(err)
	}
	req := withUser(httptest.NewRequest("POST", "/api/projects/film_success/plan", nil), u)
	req.Header.Set("X-Vowfilm-Quote", q.ID)
	req.Header.Set("Idempotency-Key", q.ID)
	if err = a.startBilled(req, p.ID, "plan", ""); err != nil {
		t.Fatal(err)
	}
	deadline := time.Now().Add(10 * time.Second)
	for {
		a.mu.Lock()
		active := len(a.running) > 0
		a.mu.Unlock()
		if !active {
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("task timed out")
		}
		time.Sleep(10 * time.Millisecond)
	}
	wallet, err := a.accounts.Repo.Wallet(u.ID)
	if err != nil {
		t.Fatal(err)
	}
	if wallet.User.Balance != 4000 || wallet.User.Held != 0 || len(wallet.Charges) != 1 || wallet.Charges[0].State != "settled" {
		t.Fatalf("successful task not settled: %+v", wallet)
	}
	if err = a.startBilled(req, p.ID, "plan", ""); err != nil {
		t.Fatal(err)
	}
	again, err := a.accounts.Repo.Wallet(u.ID)
	if err != nil || again.User.Balance != 4000 || len(again.Entries) != 3 {
		t.Fatal("retry charged again")
	}
}
