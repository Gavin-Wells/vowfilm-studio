package storage

import (
	"os"
	"path/filepath"
	"testing"
	"vowfilm/server/internal/domain"
	"vowfilm/server/internal/platform"
)

func TestDatabaseTransferPreservesRecordsAndRefusesOverwrite(t *testing.T) {
	source, e := Open("sqlite", filepath.Join(t.TempDir(), "source.sqlite"))
	if e != nil {
		t.Fatal(e)
	}
	defer source.Close()
	driver, dsn := "sqlite", filepath.Join(t.TempDir(), "target.sqlite")
	if pg := os.Getenv("TEST_POSTGRES_MIGRATION_URL"); pg != "" {
		driver, dsn = "postgres", pg
	}
	target, e := Open(driver, dsn)
	if e != nil {
		t.Fatal(e)
	}
	defer target.Close()
	u := &platform.User{ID: "owner", Email: "owner@example.test", Name: "Owner", Password: "password-hash", Recovery: "recovery-hash", CreatedAt: platform.Now()}
	if e = source.Register(u, true); e != nil {
		t.Fatal(e)
	}
	if e = source.Credit(u.ID, u.ID, 12345, "receipt-transfer", "verified"); e != nil {
		t.Fatal(e)
	}
	if e = source.SaveProjects(map[string]*domain.Project{"film_1": {ID: "film_1", OwnerID: u.ID, Scene: "family", Title: "Family"}}); e != nil {
		t.Fatal(e)
	}
	if e = source.TransferTo(target); e != nil {
		t.Fatal(e)
	}
	w, e := target.Wallet(u.ID)
	if e != nil || w.User.Balance != 12345 || len(w.Entries) != 1 || w.User.Password != "password-hash" {
		t.Fatal("records changed during transfer")
	}
	projects, e := target.LoadProjects()
	if e != nil || projects["film_1"].OwnerID != u.ID {
		t.Fatal("project ownership lost")
	}
	if e = source.TransferTo(target); e == nil {
		t.Fatal("overwrote a nonempty target")
	}
	w, e = target.Wallet(u.ID)
	if e != nil || w.User.Balance != 12345 || len(w.Entries) != 1 {
		t.Fatal("rejected migration changed balances")
	}
}
