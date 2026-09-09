package platform

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"time"
)

type User struct {
	ID        string `json:"id"`
	Email     string `json:"email"`
	Name      string `json:"name"`
	Role      string `json:"role"`
	Disabled  bool   `json:"disabled"`
	Balance   int64  `json:"balance"`
	Held      int64  `json:"held"`
	CreatedAt string `json:"createdAt"`
	Password  string `json:"-"`
	Recovery  string `json:"-"`
}

var Roles = map[string][]string{
	"admin":   {"project:read", "project:write", "project:all", "users:manage", "billing:manage", "pricing:manage", "config:manage"},
	"creator": {"project:read", "project:write"}, "viewer": {"project:read"}, "finance": {"project:read", "billing:manage"},
}

func (u *User) Can(p string) bool {
	if u == nil || u.Disabled {
		return false
	}
	for _, v := range Roles[u.Role] {
		if v == p {
			return true
		}
	}
	return false
}

type Session struct {
	Hash      string `json:"-"`
	UserID    string `json:"-"`
	CreatedAt string `json:"createdAt"`
	Expires   int64  `json:"expires"`
	Agent     string `json:"agent"`
	Current   bool   `json:"current"`
}
type Entry struct {
	ID           string `json:"id"`
	UserID       string `json:"userId"`
	Kind         string `json:"kind"`
	Delta        int64  `json:"delta"`
	HeldDelta    int64  `json:"heldDelta"`
	BalanceAfter int64  `json:"balanceAfter"`
	HeldAfter    int64  `json:"heldAfter"`
	Reference    string `json:"reference"`
	Note         string `json:"note"`
	Actor        string `json:"actor"`
	CreatedAt    string `json:"createdAt"`
}
type Audit struct {
	Actor     string `json:"actor"`
	Action    string `json:"action"`
	Target    string `json:"target"`
	Detail    string `json:"detail"`
	CreatedAt string `json:"createdAt"`
}
type Rule struct {
	Action  string `json:"action"`
	Unit    string `json:"unit"`
	Base    int64  `json:"base"`
	Rate    int64  `json:"rate"`
	Minimum int64  `json:"minimum"`
}
type Pricing struct {
	Version      int64            `json:"version"`
	Rules        []Rule           `json:"rules"`
	SceneFactors map[string]int64 `json:"sceneFactors"`
}
type Quote struct {
	ID          string `json:"id"`
	UserID      string `json:"userId"`
	ProjectID   string `json:"projectId"`
	Fingerprint string `json:"fingerprint"`
	Action      string `json:"action"`
	ShotID      string `json:"shotId"`
	Scene       string `json:"scene"`
	Rule        Rule   `json:"rule"`
	Version     int64  `json:"version"`
	Quantity    int64  `json:"quantity"`
	Factor      int64  `json:"factor"`
	Amount      int64  `json:"amount"`
	Expires     int64  `json:"expires"`
}
type Charge struct {
	ID         string `json:"id"`
	UserID     string `json:"userId"`
	QuoteID    string `json:"quoteId"`
	RequestKey string `json:"requestKey"`
	ProjectID  string `json:"projectId"`
	Amount     int64  `json:"amount"`
	State      string `json:"state"`
	CreatedAt  string `json:"createdAt"`
	UpdatedAt  string `json:"updatedAt"`
	Quote      *Quote `json:"quote,omitempty"`
}
type Wallet struct {
	User    *User    `json:"user"`
	Entries []Entry  `json:"entries"`
	Charges []Charge `json:"charges"`
}

var ErrNotFound = errors.New("记录不存在")

func Token() string {
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		panic(err)
	}
	return hex.EncodeToString(b)
}
func ID(prefix string) string { return prefix + Token()[:20] }
func Digest(s string) string  { v := sha256.Sum256([]byte(s)); return hex.EncodeToString(v[:]) }
func JSON(v any) string       { b, _ := json.Marshal(v); return string(b) }
func Now() string             { return time.Now().UTC().Format(time.RFC3339Nano) }

// Repository is the application boundary. HTTP and business services never contain SQL.
// Implementations must preserve atomic wallet/journal writes and unique request references.
type Repository interface {
	CountUsers() (int, error)
	User(string) (*User, error)
	UserByEmail(string) (*User, error)
	Register(*User, bool) error
	CreateSession(Session, string) error
	SessionUser(string, int64) (*User, error)
	Sessions(string, int64) ([]Session, error)
	RevokeSessions(string, string) error
	ChangePassword(string, string, string, string, string) error
	Limit(string, int, int64) (bool, error)
	Users() ([]User, error)
	UpdateUser(string, string, string, bool) error
	AuditLog() ([]Audit, error)
	Pricing() (*Pricing, error)
	PublishPricing(Pricing, string) error
	SaveQuote(Quote) error
	Quote(string) (*Quote, error)
	ChargeByKey(string, string) (*Charge, error)
	Reserve(Quote, string) (*Charge, bool, error)
	Settle(string, bool) error
	ReleaseInterrupted() error
	Credit(string, string, int64, string, string) error
	Wallet(string) (*Wallet, error)
}
