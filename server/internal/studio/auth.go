package studio

import (
	"context"
	"errors"
	"net/http"
	"strings"
	"time"
	"vowfilm/server/internal/platform"
)

type userContextKey struct{}

func currentUser(r *http.Request) *platform.User {
	u, _ := r.Context().Value(userContextKey{}).(*platform.User)
	return u
}
func canRead(u *platform.User, p *Project) bool {
	return u != nil && (u.Can("project:all") || (u.Can("project:read") && p.OwnerID == u.ID))
}
func withUser(r *http.Request, u *platform.User) *http.Request {
	return r.WithContext(context.WithValue(r.Context(), userContextKey{}, u))
}
func cookie(w http.ResponseWriter, r *http.Request, value string, age int) {
	http.SetCookie(w, &http.Cookie{Name: "vowfilm_session", Value: value, Path: "/", HttpOnly: true, Secure: r.Header.Get("X-Vowfilm-Proto") == "https", SameSite: http.SameSiteStrictMode, MaxAge: age})
}
func (a *App) sessionUser(r *http.Request) *platform.User {
	c, err := r.Cookie("vowfilm_session")
	if err != nil {
		return nil
	}
	u, err := a.accounts.Repo.SessionUser(platform.Digest(c.Value), time.Now().Unix())
	if err != nil {
		return nil
	}
	return u
}
func (a *App) authHTTP(w http.ResponseWriter, r *http.Request) {
	path := strings.TrimPrefix(r.URL.Path, "/api/auth/")
	if path == "status" && r.Method == "GET" {
		n, err := a.accounts.Repo.CountUsers()
		if err != nil {
			fail(w, 500, err)
			return
		}
		respond(w, 200, map[string]bool{"setupRequired": n == 0})
		return
	}
	if (path == "login" || path == "register" || path == "recover") && r.Method == "POST" {
		var in struct {
			Email        string `json:"email"`
			Name         string `json:"name"`
			Password     string `json:"password"`
			SetupToken   string `json:"setupToken"`
			RecoveryCode string `json:"recoveryCode"`
		}
		if err := decode(w, r, &in); err != nil {
			fail(w, 400, err)
			return
		}
		if path == "recover" {
			code, err := a.accounts.Recover(in.Email, in.RecoveryCode, in.Password)
			if err != nil {
				fail(w, 400, err)
				return
			}
			cookie(w, r, "", -1)
			respond(w, 200, map[string]string{"recoveryCode": code})
			return
		}
		if path == "register" {
			u, code, err := a.accounts.Register(in.Email, in.Name, in.Password, in.SetupToken)
			if err != nil {
				fail(w, 400, err)
				return
			}
			if u.Role == "admin" {
				for _, p := range a.store.List() {
					if p.OwnerID == "" {
						if err = a.store.Update(p.ID, func(q *Project) error { q.OwnerID = u.ID; return nil }); err != nil {
							fail(w, 500, err)
							return
						}
					}
				}
			}
			session, err := a.accounts.NewSession(u, r.UserAgent())
			if err != nil {
				fail(w, 500, err)
				return
			}
			cookie(w, r, session, 604800)
			respond(w, 201, map[string]any{"user": u, "recoveryCode": code})
			return
		}
		u, session, err := a.accounts.Login(in.Email, in.Password, r.UserAgent())
		if err != nil {
			fail(w, 401, err)
			return
		}
		cookie(w, r, session, 604800)
		respond(w, 200, map[string]any{"user": u})
		return
	}
	u := a.sessionUser(r)
	if u == nil {
		fail(w, 401, errors.New("请先登录"))
		return
	}
	if path == "me" && r.Method == "GET" {
		respond(w, 200, map[string]any{"user": u, "permissions": platform.Roles[u.Role]})
		return
	}
	if (path == "logout" || path == "logout-all") && r.Method == "POST" {
		hash := ""
		if path == "logout" {
			c, _ := r.Cookie("vowfilm_session")
			hash = platform.Digest(c.Value)
		}
		if err := a.accounts.Repo.RevokeSessions(u.ID, hash); err != nil {
			fail(w, 500, err)
			return
		}
		cookie(w, r, "", -1)
		respond(w, 200, map[string]bool{"ok": true})
		return
	}
	if path == "password" && r.Method == "POST" {
		var in struct {
			CurrentPassword string `json:"currentPassword"`
			Password        string `json:"password"`
		}
		if err := decode(w, r, &in); err != nil {
			fail(w, 400, err)
			return
		}
		code, err := a.accounts.ChangePassword(u, in.CurrentPassword, in.Password)
		if err != nil {
			fail(w, 400, err)
			return
		}
		cookie(w, r, "", -1)
		respond(w, 200, map[string]string{"recoveryCode": code})
		return
	}
	if path == "sessions" && r.Method == "GET" {
		list, err := a.accounts.Repo.Sessions(u.ID, time.Now().Unix())
		if err != nil {
			fail(w, 500, err)
			return
		}
		c, _ := r.Cookie("vowfilm_session")
		for i := range list {
			list[i].Current = list[i].Hash == platform.Digest(c.Value)
		}
		respond(w, 200, list)
		return
	}
	fail(w, 404, errors.New("账号接口不存在"))
}
