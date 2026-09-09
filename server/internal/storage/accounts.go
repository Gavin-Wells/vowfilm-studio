package storage

import (
	"database/sql"
	"errors"
	"vowfilm/server/internal/platform"
)

const userColumns = "id,email,name,role,disabled,balance,held,created_at,password,recovery"

type scanner interface{ Scan(...any) error }

func scanUser(row scanner) (*platform.User, error) {
	var u platform.User
	var disabled int
	err := row.Scan(&u.ID, &u.Email, &u.Name, &u.Role, &disabled, &u.Balance, &u.Held, &u.CreatedAt, &u.Password, &u.Recovery)
	u.Disabled = disabled != 0
	if errors.Is(err, sql.ErrNoRows) {
		err = platform.ErrNotFound
	}
	return &u, err
}
func (r *SQLRepository) CountUsers() (int, error) {
	var n int
	err := r.row("SELECT COUNT(*) FROM users").Scan(&n)
	return n, err
}
func (r *SQLRepository) User(id string) (*platform.User, error) {
	return scanUser(r.row("SELECT "+userColumns+" FROM users WHERE id=?", id))
}
func (r *SQLRepository) UserByEmail(email string) (*platform.User, error) {
	return scanUser(r.row("SELECT "+userColumns+" FROM users WHERE email=?", email))
}
func (r *SQLRepository) Register(u *platform.User, setup bool) error {
	t, err := r.begin()
	if err != nil {
		return err
	}
	defer t.tx.Rollback()
	var n int
	if err = t.row("SELECT COUNT(*) FROM users").Scan(&n); err != nil {
		return err
	}
	u.Role = "creator"
	if n == 0 {
		if !setup {
			return errors.New("初始化管理员需要本机初始化密钥")
		}
		u.Role = "admin"
	}
	var exists int
	if err = t.row("SELECT COUNT(*) FROM users WHERE email=?", u.Email).Scan(&exists); err != nil {
		return err
	}
	if exists > 0 {
		return errors.New("该邮箱无法注册，请登录或找回密码")
	}
	_, err = t.exec("INSERT INTO users(id,email,name,password,recovery,role,created_at) VALUES(?,?,?,?,?,?,?)", u.ID, u.Email, u.Name, u.Password, u.Recovery, u.Role, u.CreatedAt)
	if err != nil {
		return err
	}
	if err = t.audit(u.ID, "account.register", u.ID, u.Role); err != nil {
		return err
	}
	return t.tx.Commit()
}
func (r *SQLRepository) CreateSession(s platform.Session, password string) error {
	t, err := r.begin()
	if err != nil {
		return err
	}
	defer t.tx.Rollback()
	var disabled int
	var latest string
	if err = t.row("SELECT disabled,password FROM users WHERE id=?", s.UserID).Scan(&disabled, &latest); err != nil {
		return err
	}
	if disabled != 0 || latest != password {
		return errors.New("账号状态已变化，请重新登录")
	}
	_, err = t.exec("DELETE FROM sessions WHERE user_id=? AND hash NOT IN (SELECT hash FROM sessions WHERE user_id=? ORDER BY created_at DESC LIMIT 9)", s.UserID, s.UserID)
	if err != nil {
		return err
	}
	_, err = t.exec("INSERT INTO sessions VALUES(?,?,?,?,?)", s.Hash, s.UserID, s.CreatedAt, s.Expires, s.Agent)
	if err != nil {
		return err
	}
	if err = t.audit(s.UserID, "account.login", s.UserID, ""); err != nil {
		return err
	}
	return t.tx.Commit()
}
func (r *SQLRepository) SessionUser(hash string, ts int64) (*platform.User, error) {
	var id string
	if err := r.row("SELECT user_id FROM sessions WHERE hash=? AND expires>?", hash, ts).Scan(&id); err != nil {
		return nil, platform.ErrNotFound
	}
	u, err := r.User(id)
	if err == nil && u.Disabled {
		return nil, platform.ErrNotFound
	}
	return u, err
}
func (r *SQLRepository) Sessions(uid string, ts int64) ([]platform.Session, error) {
	rows, err := r.query("SELECT hash,created_at,expires,agent FROM sessions WHERE user_id=? AND expires>? ORDER BY created_at DESC", uid, ts)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	list := []platform.Session{}
	for rows.Next() {
		var s platform.Session
		if err = rows.Scan(&s.Hash, &s.CreatedAt, &s.Expires, &s.Agent); err != nil {
			return nil, err
		}
		list = append(list, s)
	}
	return list, rows.Err()
}
func (r *SQLRepository) RevokeSessions(uid, hash string) error {
	q := "DELETE FROM sessions WHERE user_id=?"
	args := []any{uid}
	if hash != "" {
		q += " AND hash=?"
		args = append(args, hash)
	}
	_, err := r.exec(q, args...)
	return err
}
func (r *SQLRepository) ChangePassword(uid, field, old, password, recovery string) error {
	if field != "password" && field != "recovery" {
		return errors.New("无效凭据类型")
	}
	t, err := r.begin()
	if err != nil {
		return err
	}
	defer t.tx.Rollback()
	result, err := t.exec("UPDATE users SET password=?,recovery=? WHERE id=? AND "+field+"=? AND disabled=0", password, recovery, uid, old)
	if err != nil {
		return err
	}
	n, _ := result.RowsAffected()
	if n != 1 {
		return errors.New("凭据已失效，请重新登录")
	}
	if _, err = t.exec("DELETE FROM sessions WHERE user_id=?", uid); err != nil {
		return err
	}
	if err = t.audit(uid, "account."+field, uid, ""); err != nil {
		return err
	}
	return t.tx.Commit()
}
func (r *SQLRepository) Limit(key string, max int, ts int64) (bool, error) {
	t, err := r.begin()
	if err != nil {
		return false, err
	}
	defer t.tx.Rollback()
	if _, err = t.exec("DELETE FROM auth_limits WHERE until_time<?", ts); err != nil {
		return false, err
	}
	if _, err = t.exec("INSERT INTO auth_limits VALUES(?,1,?) ON CONFLICT(key) DO UPDATE SET count=auth_limits.count+1", key, ts+900); err != nil {
		return false, err
	}
	var n int
	if err = t.row("SELECT count FROM auth_limits WHERE key=?", key).Scan(&n); err != nil {
		return false, err
	}
	return n <= max, t.tx.Commit()
}
func (r *SQLRepository) Users() ([]platform.User, error) {
	rows, err := r.query("SELECT " + userColumns + " FROM users ORDER BY created_at DESC LIMIT 500")
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	list := []platform.User{}
	for rows.Next() {
		u, e := scanUser(rows)
		if e != nil {
			return nil, e
		}
		list = append(list, *u)
	}
	return list, rows.Err()
}
func (r *SQLRepository) UpdateUser(actor, id, role string, disabled bool) error {
	if _, ok := platform.Roles[role]; !ok {
		return errors.New("角色不存在")
	}
	if actor == id && (role != "admin" || disabled) {
		return errors.New("不能停用或降级当前管理员")
	}
	t, err := r.begin()
	if err != nil {
		return err
	}
	defer t.tx.Rollback()
	a, err := scanUser(t.row("SELECT "+userColumns+" FROM users WHERE id=?", actor))
	if err != nil || !a.Can("users:manage") {
		return errors.New("没有账号管理权限")
	}
	old, err := scanUser(t.row("SELECT "+userColumns+" FROM users WHERE id=?", id))
	if err != nil {
		return err
	}
	if old.Role == "admin" && (role != "admin" || disabled) {
		var count int
		if err = t.row("SELECT COUNT(*) FROM users WHERE role='admin' AND disabled=0").Scan(&count); err != nil {
			return err
		}
		if count <= 1 {
			return errors.New("至少保留一位启用的管理员")
		}
	}
	d := 0
	if disabled {
		d = 1
	}
	if _, err = t.exec("UPDATE users SET role=?,disabled=? WHERE id=?", role, d, id); err != nil {
		return err
	}
	if _, err = t.exec("DELETE FROM sessions WHERE user_id=?", id); err != nil {
		return err
	}
	if err = t.audit(actor, "account.permissions", id, platform.JSON(map[string]any{"role": role, "disabled": disabled})); err != nil {
		return err
	}
	return t.tx.Commit()
}
func (r *SQLRepository) AuditLog() ([]platform.Audit, error) {
	rows, err := r.query("SELECT actor,action,target,detail,created_at FROM audit ORDER BY created_at DESC LIMIT 200")
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	list := []platform.Audit{}
	for rows.Next() {
		var a platform.Audit
		if err = rows.Scan(&a.Actor, &a.Action, &a.Target, &a.Detail, &a.CreatedAt); err != nil {
			return nil, err
		}
		list = append(list, a)
	}
	return list, rows.Err()
}
