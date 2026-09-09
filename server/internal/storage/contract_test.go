package storage

import (
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"
	"vowfilm/server/internal/domain"
	"vowfilm/server/internal/platform"
)

func repositories(t *testing.T, fn func(*testing.T, *SQLRepository)) {
	t.Run("sqlite", func(t *testing.T) {
		r, e := Open("sqlite", filepath.Join(t.TempDir(), "test.sqlite"))
		if e != nil {
			t.Fatal(e)
		}
		defer r.Close()
		fn(t, r)
	})
	t.Run("postgres", func(t *testing.T) {
		dsn := os.Getenv("TEST_POSTGRES_URL")
		if dsn == "" {
			t.Skip("TEST_POSTGRES_URL is unset")
		}
		r, e := Open("postgres", dsn)
		if e != nil {
			t.Fatal(e)
		}
		defer r.Close()
		fn(t, r)
	})
}
func TestRepositoryContract(t *testing.T) {
	repositories(t, func(t *testing.T, r *SQLRepository) {
		suffix := platform.ID("")
		admin := &platform.User{ID: platform.ID("usr_"), Email: suffix + "admin@test.invalid", Name: "Admin", Password: "hash", Recovery: "recovery", CreatedAt: platform.Now()}
		count, e := r.CountUsers()
		if e != nil {
			t.Fatal(e)
		}
		if e = r.Register(admin, true); e != nil {
			t.Fatal(e)
		}
		if count == 0 && admin.Role != "admin" {
			t.Fatal("bootstrap role")
		}
		// Each test DB is isolated. Additional fixtures require an existing administrator.
		if admin.Role != "admin" {
			t.Fatal("contract database must be empty")
		}
		u := &platform.User{ID: platform.ID("usr_"), Email: suffix + "user@test.invalid", Name: "User", Password: "hash", Recovery: "recovery", CreatedAt: platform.Now()}
		if e = r.Register(u, true); e != nil {
			t.Fatal(e)
		}
		if u.Role != "creator" {
			t.Fatal("registration elevated role")
		}
		if e = r.Credit(u.ID, u.ID, 1000, "bad-ref", "gift"); e == nil {
			t.Fatal("unauthorized credit")
		}
		if e = r.Credit(admin.ID, u.ID, 10000, "receipt-1", "verified"); e != nil {
			t.Fatal(e)
		}
		if e = r.Credit(admin.ID, u.ID, 10000, "receipt-1", "verified"); e != nil {
			t.Fatal(e)
		}
		if e = r.Credit(admin.ID, u.ID, 10001, "receipt-1", "verified"); e == nil {
			t.Fatal("conflicting receipt accepted")
		}
		q := platform.Quote{ID: platform.ID("quote_"), UserID: u.ID, ProjectID: "film_test", Action: "generate", Amount: 6000, Expires: time.Now().Add(time.Minute).Unix(), Version: 1, Quantity: 1, Rule: platform.Rule{Action: "generate", Unit: "task", Rate: 6000}, Factor: 10000}
		if e = r.SaveQuote(q); e != nil {
			t.Fatal(e)
		}
		c, reused, e := r.Reserve(q, "request-0000000001")
		if e != nil || reused {
			t.Fatalf("reserve %v", e)
		}
		retry, reused, e := r.Reserve(q, "request-0000000001")
		if e != nil || !reused || retry.ID != c.ID {
			t.Fatal("idempotent reservation failed")
		}
		if _, _, e = r.Reserve(q, "request-0000000002"); e == nil {
			t.Fatal("same quote reused with new key")
		}
		q2 := q
		q2.ID = platform.ID("quote_")
		if e = r.SaveQuote(q2); e != nil {
			t.Fatal(e)
		}
		if _, _, e = r.Reserve(q2, "request-0000000003"); e == nil {
			t.Fatal("overspend accepted")
		}
		w, e := r.Wallet(u.ID)
		if e != nil || w.User.Balance != 10000 || w.User.Held != 6000 || len(w.Entries) != 2 {
			t.Fatalf("bad wallet after hold: %+v %v", w, e)
		}
		if e = r.Settle(c.ID, true); e != nil {
			t.Fatal(e)
		}
		if e = r.Settle(c.ID, true); e != nil {
			t.Fatal(e)
		}
		if e = r.Settle(c.ID, false); e != nil {
			t.Fatal(e)
		}
		w, e = r.Wallet(u.ID)
		if e != nil || w.User.Balance != 4000 || w.User.Held != 0 || len(w.Entries) != 3 {
			t.Fatal("settlement duplicated")
		}
		p, e := r.Pricing()
		if e != nil {
			t.Fatal(e)
		}
		p.Rules[2].Rate = 99000
		if e = r.PublishPricing(*p, admin.ID); e != nil {
			t.Fatal(e)
		}
		if e = r.PublishPricing(*p, admin.ID); e == nil {
			t.Fatal("stale price edit accepted")
		}
		original, e := r.Quote(q.ID)
		if e != nil || original.Amount != 6000 || original.Version != 1 {
			t.Fatal("history repriced")
		}
		if e = r.Credit(admin.ID, u.ID, 6000, "receipt-2", "verified"); e != nil {
			t.Fatal(e)
		}
		var wg sync.WaitGroup
		var mu sync.Mutex
		success := 0
		for i := 0; i < 8; i++ {
			wg.Add(1)
			go func() {
				defer wg.Done()
				next := q
				next.ID = platform.ID("quote_")
				if e := r.SaveQuote(next); e != nil {
					t.Error(e)
					return
				}
				_, _, e := r.Reserve(next, platform.Token())
				if e == nil {
					mu.Lock()
					success++
					mu.Unlock()
				}
			}()
		}
		wg.Wait()
		if success != 1 {
			t.Fatalf("concurrent holds = %d", success)
		}
		if e = r.ReleaseInterrupted(); e != nil {
			t.Fatal(e)
		}
		w, e = r.Wallet(u.ID)
		if e != nil || w.User.Balance != 10000 || w.User.Held != 0 {
			t.Fatal("recovery didn't release")
		}
		var b, h int64
		for _, e := range w.Entries {
			b += e.Delta
			h += e.HeldDelta
		}
		if b != w.User.Balance || h != w.User.Held {
			t.Fatal("journal and wallet diverged")
		}
		sess := platform.Session{Hash: "test-session", UserID: u.ID, CreatedAt: platform.Now(), Expires: time.Now().Add(time.Hour).Unix(), Agent: "contract"}
		if e = r.CreateSession(sess, "hash"); e != nil {
			t.Fatal(e)
		}
		if _, e = r.SessionUser(sess.Hash, time.Now().Unix()); e != nil {
			t.Fatal(e)
		}
		if e = r.UpdateUser(admin.ID, u.ID, "viewer", false); e != nil {
			t.Fatal(e)
		}
		if _, e = r.SessionUser(sess.Hash, time.Now().Unix()); e == nil {
			t.Fatal("role change kept session")
		}
		if e = r.UpdateUser(admin.ID, admin.ID, "creator", false); e == nil {
			t.Fatal("last admin demoted")
		}
		projects := map[string]*domain.Project{"film_test": {ID: "film_test", OwnerID: u.ID, Scene: "commerce", Title: "Product"}}
		if e = r.SaveProjects(projects); e != nil {
			t.Fatal(e)
		}
		loaded, e := r.LoadProjects()
		if e != nil || loaded["film_test"].OwnerID != u.ID {
			t.Fatal("project persistence failed")
		}
	})
}
func TestFailedJournalRollsBack(t *testing.T) {
	r, e := Open("sqlite", filepath.Join(t.TempDir(), "test.sqlite"))
	if e != nil {
		t.Fatal(e)
	}
	defer r.Close()
	u := &platform.User{ID: "u", Email: "u@test.invalid", Name: "u", Password: "hash", Recovery: "r", CreatedAt: platform.Now()}
	if e = r.Register(u, true); e != nil {
		t.Fatal(e)
	}
	if _, e = r.exec("CREATE TRIGGER fail_ledger BEFORE INSERT ON ledger BEGIN SELECT RAISE(ABORT, 'disk failure'); END"); e != nil {
		t.Fatal(e)
	}
	if e = r.Credit(u.ID, u.ID, 1000, "rollback-test", "test"); e == nil {
		t.Fatal("expected transaction failure")
	}
	got, e := r.User(u.ID)
	if e != nil || got.Balance != 0 || got.Held != 0 {
		t.Fatal("wallet changed without journal")
	}
}
