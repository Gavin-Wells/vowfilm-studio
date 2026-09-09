package storage

import (
	"database/sql"
	"encoding/json"
	"errors"
	"time"
	"vowfilm/server/internal/platform"
)

func (r *SQLRepository) Pricing() (*platform.Pricing, error) {
	var body string
	err := r.row("SELECT body FROM pricing ORDER BY version DESC LIMIT 1").Scan(&body)
	if err != nil {
		return nil, err
	}
	var p platform.Pricing
	err = json.Unmarshal([]byte(body), &p)
	return &p, err
}
func (r *SQLRepository) PublishPricing(p platform.Pricing, actor string) error {
	if err := platform.ValidatePricing(p); err != nil {
		return err
	}
	t, err := r.begin()
	if err != nil {
		return err
	}
	defer t.tx.Rollback()
	u, err := scanUser(t.row("SELECT "+userColumns+" FROM users WHERE id=?", actor))
	if err != nil || !u.Can("pricing:manage") {
		return errors.New("没有调价权限")
	}
	var latest int64
	if err = t.row("SELECT MAX(version) FROM pricing").Scan(&latest); err != nil {
		return err
	}
	if p.Version != latest {
		return errors.New("价格已被更新，请刷新后重试")
	}
	p.Version++
	if _, err = t.exec("INSERT INTO pricing VALUES(?,?,?,?)", p.Version, platform.JSON(p), actor, platform.Now()); err != nil {
		return err
	}
	if err = t.audit(actor, "pricing.publish", "pricing", platform.JSON(p)); err != nil {
		return err
	}
	return t.tx.Commit()
}
func (r *SQLRepository) SaveQuote(q platform.Quote) error {
	_, err := r.exec("INSERT INTO quotes VALUES(?,?,?,?)", q.ID, q.UserID, platform.JSON(q), q.Expires)
	return err
}
func (r *SQLRepository) Quote(id string) (*platform.Quote, error) {
	var raw string
	if err := r.row("SELECT body FROM quotes WHERE id=?", id).Scan(&raw); err != nil {
		return nil, platform.ErrNotFound
	}
	var q platform.Quote
	err := json.Unmarshal([]byte(raw), &q)
	return &q, err
}

const chargeColumns = "id,user_id,quote_id,request_key,project_id,amount,state,created_at,updated_at"

func scanCharge(row scanner) (*platform.Charge, error) {
	var c platform.Charge
	err := row.Scan(&c.ID, &c.UserID, &c.QuoteID, &c.RequestKey, &c.ProjectID, &c.Amount, &c.State, &c.CreatedAt, &c.UpdatedAt)
	return &c, err
}
func (t *transaction) journal(uid, kind string, delta, heldDelta int64, ref, note, actor string) error {
	result, err := t.exec("UPDATE users SET balance=balance+?,held=held+? WHERE id=? AND balance+?>=held+? AND held+?>=0 AND balance+?<=?", delta, heldDelta, uid, delta, heldDelta, heldDelta, delta, platform.MaxMoney)
	if err != nil {
		return err
	}
	n, _ := result.RowsAffected()
	if n != 1 {
		return errors.New("可用积分不足或余额超出上限")
	}
	var b, h int64
	if err = t.row("SELECT balance,held FROM users WHERE id=?", uid).Scan(&b, &h); err != nil {
		return err
	}
	_, err = t.exec("INSERT INTO ledger VALUES(?,?,?,?,?,?,?,?,?,?,?)", platform.ID("entry_"), uid, kind, delta, heldDelta, b, h, ref, note, actor, platform.Now())
	return err
}
func (r *SQLRepository) Reserve(q platform.Quote, key string) (*platform.Charge, bool, error) {
	if len(key) < 16 || len(key) > 128 {
		return nil, false, errors.New("请求需要 16–128 字符的幂等键")
	}
	t, err := r.begin()
	if err != nil {
		return nil, false, err
	}
	defer t.tx.Rollback()
	c, err := scanCharge(t.row("SELECT "+chargeColumns+" FROM charges WHERE user_id=? AND request_key=?", q.UserID, key))
	if err == nil {
		if c.QuoteID != q.ID {
			return nil, false, errors.New("幂等键已用于另一份报价")
		}
		return c, true, nil
	}
	if !errors.Is(err, sql.ErrNoRows) {
		return nil, false, err
	}
	if q.Expires < time.Now().Unix() {
		return nil, false, errors.New("报价已过期，请重新获取")
	}
	u, err := scanUser(t.row("SELECT "+userColumns+" FROM users WHERE id=?", q.UserID))
	if err != nil || !u.Can("project:write") {
		return nil, false, errors.New("没有执行权限")
	}
	c = &platform.Charge{ID: platform.ID("charge_"), UserID: q.UserID, QuoteID: q.ID, RequestKey: key, ProjectID: q.ProjectID, Amount: q.Amount, State: "held", CreatedAt: platform.Now(), UpdatedAt: platform.Now()}
	if _, err = t.exec("INSERT INTO charges VALUES(?,?,?,?,?,?,?,?,?)", c.ID, c.UserID, c.QuoteID, c.RequestKey, c.ProjectID, c.Amount, c.State, c.CreatedAt, c.UpdatedAt); err != nil {
		return nil, false, errors.New("报价已提交，请刷新任务状态")
	}
	if err = t.journal(q.UserID, "hold", 0, q.Amount, "hold:"+c.ID, q.Action, q.UserID); err != nil {
		return nil, false, err
	}
	return c, false, t.tx.Commit()
}
func (t *transaction) settle(id string, success bool) error {
	c, err := scanCharge(t.row("SELECT "+chargeColumns+" FROM charges WHERE id=?", id))
	if err != nil {
		return err
	}
	if c.State != "held" {
		return nil
	}
	state, kind, delta := "released", "release", int64(0)
	if success {
		state, kind, delta = "settled", "consume", -c.Amount
	}
	if err = t.journal(c.UserID, kind, delta, -c.Amount, "final:"+c.ID, c.ProjectID, "system"); err != nil {
		return err
	}
	_, err = t.exec("UPDATE charges SET state=?,updated_at=? WHERE id=? AND state='held'", state, platform.Now(), id)
	return err
}
func (r *SQLRepository) Settle(id string, success bool) error {
	if id == "" {
		return nil
	}
	t, err := r.begin()
	if err != nil {
		return err
	}
	defer t.tx.Rollback()
	if err = t.settle(id, success); err != nil {
		return err
	}
	return t.tx.Commit()
}
func (r *SQLRepository) ReleaseInterrupted() error {
	t, err := r.begin()
	if err != nil {
		return err
	}
	defer t.tx.Rollback()
	rows, err := t.query("SELECT id FROM charges WHERE state='held'")
	if err != nil {
		return err
	}
	ids := []string{}
	for rows.Next() {
		var id string
		if err = rows.Scan(&id); err != nil {
			_ = rows.Close()
			return err
		}
		ids = append(ids, id)
	}
	err = rows.Err()
	_ = rows.Close()
	if err != nil {
		return err
	}
	for _, id := range ids {
		if err = t.settle(id, false); err != nil {
			return err
		}
	}
	return t.tx.Commit()
}
func (r *SQLRepository) Credit(actor, uid string, amount int64, reference, note string) error {
	if amount <= 0 || amount > 100000000 || len(reference) < 4 || len(reference) > 120 || len(note) < 1 || len(note) > 500 {
		return errors.New("充值需为 0–100000 积分，并填写凭据编号和原因")
	}
	t, err := r.begin()
	if err != nil {
		return err
	}
	defer t.tx.Rollback()
	u, err := scanUser(t.row("SELECT "+userColumns+" FROM users WHERE id=?", actor))
	if err != nil || !u.Can("billing:manage") {
		return errors.New("没有充值权限")
	}
	var existingUID, oldNote string
	var oldAmount int64
	err = t.row("SELECT user_id,delta,note FROM ledger WHERE reference=?", "credit:"+reference).Scan(&existingUID, &oldAmount, &oldNote)
	if err == nil {
		if existingUID == uid && oldAmount == amount && oldNote == note {
			return nil
		}
		return errors.New("凭据已使用且内容不同")
	}
	if !errors.Is(err, sql.ErrNoRows) {
		return err
	}
	if err = t.journal(uid, "credit", amount, 0, "credit:"+reference, note, actor); err != nil {
		return err
	}
	if err = t.audit(actor, "billing.credit", uid, reference); err != nil {
		return err
	}
	return t.tx.Commit()
}
func (r *SQLRepository) Wallet(uid string) (*platform.Wallet, error) {
	t, err := r.begin()
	if err != nil {
		return nil, err
	}
	defer t.tx.Rollback()
	u, err := scanUser(t.row("SELECT "+userColumns+" FROM users WHERE id=?", uid))
	if err != nil {
		return nil, err
	}
	v := &platform.Wallet{User: u, Entries: []platform.Entry{}, Charges: []platform.Charge{}}
	rows, err := t.query("SELECT id,user_id,kind,delta,held_delta,balance_after,held_after,reference,note,actor,created_at FROM ledger WHERE user_id=? ORDER BY created_at DESC LIMIT 200", uid)
	if err != nil {
		return nil, err
	}
	for rows.Next() {
		var e platform.Entry
		if err = rows.Scan(&e.ID, &e.UserID, &e.Kind, &e.Delta, &e.HeldDelta, &e.BalanceAfter, &e.HeldAfter, &e.Reference, &e.Note, &e.Actor, &e.CreatedAt); err != nil {
			_ = rows.Close()
			return nil, err
		}
		v.Entries = append(v.Entries, e)
	}
	err = rows.Err()
	_ = rows.Close()
	if err != nil {
		return nil, err
	}
	rows, err = t.query("SELECT c.id,c.user_id,c.quote_id,c.request_key,c.project_id,c.amount,c.state,c.created_at,c.updated_at,q.body FROM charges c JOIN quotes q ON q.id=c.quote_id WHERE c.user_id=? ORDER BY c.created_at DESC LIMIT 100", uid)
	if err != nil {
		return nil, err
	}
	for rows.Next() {
		var c platform.Charge
		var body string
		if err = rows.Scan(&c.ID, &c.UserID, &c.QuoteID, &c.RequestKey, &c.ProjectID, &c.Amount, &c.State, &c.CreatedAt, &c.UpdatedAt, &body); err != nil {
			_ = rows.Close()
			return nil, err
		}
		var q platform.Quote
		if err = json.Unmarshal([]byte(body), &q); err != nil {
			_ = rows.Close()
			return nil, err
		}
		c.Quote = &q
		v.Charges = append(v.Charges, c)
	}
	err = rows.Err()
	_ = rows.Close()
	if err != nil {
		return nil, err
	}
	return v, t.tx.Commit()
}

func (r *SQLRepository) ChargeByKey(uid, key string) (*platform.Charge, error) {
	c, err := scanCharge(r.row("SELECT "+chargeColumns+" FROM charges WHERE user_id=? AND request_key=?", uid, key))
	if errors.Is(err, sql.ErrNoRows) {
		return nil, platform.ErrNotFound
	}
	return c, err
}
