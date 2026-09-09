package platform

import (
	"crypto/subtle"
	"errors"
	"golang.org/x/crypto/bcrypt"
	"net/mail"
	"strings"
	"time"
	"unicode/utf8"
)

type Service struct {
	Repo       Repository
	SetupToken string
	dummyHash  string
}

func New(repo Repository, setup string) *Service {
	b, _ := bcrypt.GenerateFromPassword([]byte(Token()), 12)
	return &Service{repo, setup, string(b)}
}
func PasswordHash(p string) (string, error) {
	if utf8.RuneCountInString(p) < 12 || len(p) > 72 {
		return "", errors.New("密码至少 12 个字符，最多 72 字节")
	}
	b, e := bcrypt.GenerateFromPassword([]byte(p), 12)
	return string(b), e
}
func (s *Service) Throttle(key string) error {
	for k, max := range map[string]int{key: 20, "auth:global": 200} {
		ok, err := s.Repo.Limit(Digest(k), max, time.Now().Unix())
		if err != nil {
			return err
		}
		if !ok {
			return errors.New("尝试过于频繁，请 15 分钟后重试")
		}
	}
	return nil
}
func Email(raw string) (string, error) {
	email := strings.ToLower(strings.TrimSpace(raw))
	v, e := mail.ParseAddress(email)
	if e != nil || v.Address != email || len(email) > 254 {
		return "", errors.New("请输入有效的邮箱地址")
	}
	return email, nil
}
func (s *Service) Register(email, name, password, setup string) (*User, string, error) {
	var err error
	email, err = Email(email)
	if err != nil {
		return nil, "", err
	}
	if err = s.Throttle("auth:" + email); err != nil {
		return nil, "", err
	}
	name = strings.TrimSpace(name)
	if utf8.RuneCountInString(name) < 1 || utf8.RuneCountInString(name) > 40 {
		return nil, "", errors.New("称呼需为 1–40 字")
	}
	hash, err := PasswordHash(password)
	if err != nil {
		return nil, "", err
	}
	recovery := Token()
	u := &User{ID: ID("usr_"), Email: email, Name: name, Password: hash, Recovery: Digest(recovery), Role: "creator", CreatedAt: Now()}
	isSetup := s.SetupToken != "" && subtle.ConstantTimeCompare([]byte(setup), []byte(s.SetupToken)) == 1
	if err = s.Repo.Register(u, isSetup); err != nil {
		return nil, "", err
	}
	return u, recovery, nil
}
func (s *Service) Login(email, password, agent string) (*User, string, error) {
	email, err := Email(email)
	if err != nil {
		return nil, "", err
	}
	if err = s.Throttle("auth:" + email); err != nil {
		return nil, "", err
	}
	u, err := s.Repo.UserByEmail(email)
	hash := s.dummyHash
	if err == nil {
		hash = u.Password
	}
	valid := bcrypt.CompareHashAndPassword([]byte(hash), []byte(password)) == nil
	if err != nil || !valid || u.Disabled {
		return nil, "", errors.New("邮箱、密码不正确或账号已停用")
	}
	raw, err := s.NewSession(u, agent)
	return u, raw, err
}
func (s *Service) NewSession(u *User, agent string) (string, error) {
	if len(agent) > 240 {
		agent = agent[:240]
	}
	raw := Token()
	err := s.Repo.CreateSession(Session{Hash: Digest(raw), UserID: u.ID, CreatedAt: Now(), Expires: time.Now().Add(7 * 24 * time.Hour).Unix(), Agent: agent}, u.Password)
	return raw, err
}
func (s *Service) Recover(email, code, password string) (string, error) {
	email, err := Email(email)
	if err != nil {
		return "", err
	}
	if err = s.Throttle("auth:" + email); err != nil {
		return "", err
	}
	u, err := s.Repo.UserByEmail(email)
	if err != nil || u.Disabled || code == "" || subtle.ConstantTimeCompare([]byte(Digest(code)), []byte(u.Recovery)) != 1 {
		return "", errors.New("邮箱或恢复码不正确")
	}
	hash, err := PasswordHash(password)
	if err != nil {
		return "", err
	}
	recovery := Token()
	err = s.Repo.ChangePassword(u.ID, "recovery", u.Recovery, hash, Digest(recovery))
	return recovery, err
}
func (s *Service) ChangePassword(u *User, current, next string) (string, error) {
	if err := s.Throttle("password:" + u.ID); err != nil {
		return "", err
	}
	if bcrypt.CompareHashAndPassword([]byte(u.Password), []byte(current)) != nil {
		return "", errors.New("当前密码不正确")
	}
	hash, err := PasswordHash(next)
	if err != nil {
		return "", err
	}
	recovery := Token()
	err = s.Repo.ChangePassword(u.ID, "password", u.Password, hash, Digest(recovery))
	return recovery, err
}
