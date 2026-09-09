package studio

import (
	"errors"
	"net/http"
	"strings"
	"time"
	"vowfilm/server/internal/platform"
)

func (a *App) billingHTTP(w http.ResponseWriter, r *http.Request) {
	u := currentUser(r)
	if r.URL.Path == "/api/billing" && r.Method == "GET" {
		v, err := a.accounts.Repo.Wallet(u.ID)
		if err != nil {
			fail(w, 500, err)
			return
		}
		respond(w, 200, v)
		return
	}
	if r.URL.Path == "/api/billing/pricing" && r.Method == "GET" {
		p, err := a.accounts.Repo.Pricing()
		if err != nil {
			fail(w, 500, err)
			return
		}
		respond(w, 200, p)
		return
	}
	if r.URL.Path == "/api/billing/quote" && r.Method == "POST" {
		if !u.Can("project:write") {
			fail(w, 403, errors.New("当前角色不能执行创作任务"))
			return
		}
		var in struct {
			ProjectID string `json:"projectId"`
			Action    string `json:"action"`
			ShotID    string `json:"shotId"`
		}
		if err := decode(w, r, &in); err != nil {
			fail(w, 400, err)
			return
		}
		p := a.store.Get(in.ProjectID)
		if p == nil || !canRead(u, p) {
			fail(w, 404, errors.New("项目不存在"))
			return
		}
		q, err := a.quote(u.ID, p, in.Action, in.ShotID)
		if err != nil {
			fail(w, 400, err)
			return
		}
		respond(w, 201, q)
		return
	}
	fail(w, 404, errors.New("计费接口不存在"))
}
func (a *App) quote(uid string, p *Project, action, shotID string) (*platform.Quote, error) {
	if err := validateCommerceAction(p, action); err != nil {
		return nil, err
	}
	prices, err := a.accounts.Repo.Pricing()
	if err != nil {
		return nil, err
	}
	rule, err := platform.RuleFor(*prices, action)
	if err != nil {
		return nil, err
	}
	seconds, shots := int64(p.Duration), int64(len(p.Shots))
	if sceneID(p) == "commerce" && action == "plan" {
		shots = 1
	}
	if shots == 0 {
		shots = int64(shotCount(p))
	}
	if action == "shot" {
		found := false
		for _, s := range p.Shots {
			if s.ID == shotID {
				seconds = int64(s.Duration)
				shots = 1
				found = true
			}
		}
		if !found {
			return nil, errors.New("镜头不存在")
		}
	} else if shotID != "" {
		return nil, errors.New("该动作不接受镜头标识")
	}
	quantity := int64(1)
	switch rule.Unit {
	case "second":
		quantity = seconds
	case "shot":
		quantity = shots
	}
	scene := sceneID(p)
	factor, ok := prices.SceneFactors[scene]
	if !ok {
		return nil, errors.New("该场景未配置价格")
	}
	amount, err := platform.Price(rule, quantity, factor)
	if err != nil {
		return nil, err
	}
	q := &platform.Quote{ID: platform.ID("quote_"), UserID: uid, ProjectID: p.ID, Fingerprint: platform.Digest(platform.JSON(p)), Action: action, ShotID: shotID, Scene: scene, Rule: rule, Version: prices.Version, Quantity: quantity, Factor: factor, Amount: amount, Expires: time.Now().Add(10 * time.Minute).Unix()}
	if err = a.accounts.Repo.SaveQuote(*q); err != nil {
		return nil, err
	}
	return q, nil
}
func (a *App) startBilled(r *http.Request, id, action, shotID string) error {
	u := currentUser(r)
	q, err := a.accounts.Repo.Quote(r.Header.Get("X-Vowfilm-Quote"))
	if err != nil {
		return errors.New("请先获取并确认任务报价")
	}
	if q.UserID != u.ID || q.ProjectID != id || q.Action != action || q.ShotID != shotID {
		return errors.New("报价与任务不匹配")
	}
	// Repository handles retries before checking expiry; never create another charge for the same key.
	existing, err := a.accounts.Repo.ChargeByKey(u.ID, r.Header.Get("Idempotency-Key"))
	if err == nil {
		if existing.State == "released" {
			return errors.New("这次任务未完成且额度已释放，请重新获取报价")
		}
		if existing.QuoteID != q.ID {
			return errors.New("幂等键已用于另一份报价")
		}
		return nil
	}
	if !errors.Is(err, platform.ErrNotFound) {
		return err
	}
	p := a.store.Get(id)
	if p == nil || platform.Digest(platform.JSON(p)) != q.Fingerprint {
		return errors.New("工程内容已变化，请重新获取报价")
	}
	if isRunning(p.Status) {
		return errors.New("该项目已有任务在执行")
	}
	if err := validateCommerceAction(p, action); err != nil {
		return err
	}
	c, reused, err := a.accounts.Repo.Reserve(*q, r.Header.Get("Idempotency-Key"))
	if err != nil {
		return err
	}
	if reused {
		return nil
	}
	err = a.start(id, action, shotID, c.ID)
	if err != nil {
		if releaseErr := a.accounts.Repo.Settle(c.ID, false); releaseErr != nil {
			return errors.New("任务未启动，额度释放待恢复处理")
		}
	}
	return err
}
func (a *App) adminHTTP(w http.ResponseWriter, r *http.Request) {
	u := currentUser(r)
	path := strings.TrimPrefix(r.URL.Path, "/api/admin/")
	if path == "simulate" && r.Method == "POST" && u.Can("pricing:manage") {
		var in struct {
			Pricing platform.Pricing `json:"pricing"`
			Action  string           `json:"action"`
			Scene   string           `json:"scene"`
			Seconds int64            `json:"seconds"`
			Shots   int64            `json:"shots"`
		}
		if err := decode(w, r, &in); err != nil {
			fail(w, 400, err)
			return
		}
		if err := platform.ValidatePricing(in.Pricing); err != nil {
			fail(w, 400, err)
			return
		}
		rule, err := platform.RuleFor(in.Pricing, in.Action)
		if err != nil {
			fail(w, 400, err)
			return
		}
		factor, ok := in.Pricing.SceneFactors[in.Scene]
		if !ok || in.Seconds < 1 || in.Seconds > 240 || in.Shots < 1 || in.Shots > 100 {
			fail(w, 400, errors.New("试算参数无效"))
			return
		}
		quantity := int64(1)
		if rule.Unit == "second" {
			quantity = in.Seconds
		}
		if rule.Unit == "shot" {
			quantity = in.Shots
		}
		amount, err := platform.Price(rule, quantity, factor)
		if err != nil {
			fail(w, 400, err)
			return
		}
		respond(w, 200, map[string]int64{"amount": amount, "quantity": quantity})
		return
	}

	if path == "users" && r.Method == "GET" && (u.Can("users:manage") || u.Can("billing:manage")) {
		v, err := a.accounts.Repo.Users()
		if err != nil {
			fail(w, 500, err)
			return
		}
		respond(w, 200, v)
		return
	}
	if strings.HasPrefix(path, "users/") && r.Method == "PATCH" && u.Can("users:manage") {
		var in struct {
			Role     string `json:"role"`
			Disabled bool   `json:"disabled"`
		}
		if err := decode(w, r, &in); err != nil {
			fail(w, 400, err)
			return
		}
		id := strings.TrimPrefix(path, "users/")
		if err := a.accounts.Repo.UpdateUser(u.ID, id, in.Role, in.Disabled); err != nil {
			fail(w, 400, err)
			return
		}
		if in.Disabled || in.Role != "creator" && in.Role != "admin" {
			for _, p := range a.store.List() {
				if p.OwnerID == id && isRunning(p.Status) {
					a.cancel(p.ID)
				}
			}
		}
		respond(w, 200, map[string]bool{"ok": true})
		return
	}
	if path == "pricing" && r.Method == "POST" && u.Can("pricing:manage") {
		var p platform.Pricing
		if err := decode(w, r, &p); err != nil {
			fail(w, 400, err)
			return
		}
		if err := a.accounts.Repo.PublishPricing(p, u.ID); err != nil {
			fail(w, 400, err)
			return
		}
		v, err := a.accounts.Repo.Pricing()
		if err != nil {
			fail(w, 500, err)
			return
		}
		respond(w, 200, v)
		return
	}
	if path == "credit" && r.Method == "POST" && u.Can("billing:manage") {
		var in struct {
			UserID    string `json:"userId"`
			Amount    int64  `json:"amount"`
			Reference string `json:"reference"`
			Note      string `json:"note"`
		}
		if err := decode(w, r, &in); err != nil {
			fail(w, 400, err)
			return
		}
		if err := a.accounts.Repo.Credit(u.ID, in.UserID, in.Amount, strings.TrimSpace(in.Reference), strings.TrimSpace(in.Note)); err != nil {
			fail(w, 400, err)
			return
		}
		respond(w, 200, map[string]bool{"ok": true})
		return
	}
	if path == "audit" && r.Method == "GET" && u.Can("users:manage") {
		v, err := a.accounts.Repo.AuditLog()
		if err != nil {
			fail(w, 500, err)
			return
		}
		respond(w, 200, v)
		return
	}
	if path == "roles" && r.Method == "GET" {
		respond(w, 200, platform.Roles)
		return
	}
	fail(w, 403, errors.New("没有此管理操作的权限"))
}
