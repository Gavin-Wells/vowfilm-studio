package storage

import (
	"database/sql"
	_ "embed"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	_ "github.com/jackc/pgx/v5/stdlib"
	_ "modernc.org/sqlite"
	"vowfilm/server/internal/platform"
)

//go:embed migrations/001_initial.sql
var initialSchema string

type SQLRepository struct {
	db     *sql.DB
	driver string
}

func Open(driver, dsn string) (*SQLRepository, error) {
	if driver == "" {
		driver = "sqlite"
	}
	if driver != "sqlite" && driver != "postgres" {
		return nil, errors.New("DATABASE_DRIVER 仅支持 sqlite 或 postgres")
	}
	name := driver
	if driver == "postgres" {
		name = "pgx"
		if dsn == "" {
			return nil, errors.New("PostgreSQL 需要 DATABASE_URL")
		}
	} else {
		if err := os.MkdirAll(filepath.Dir(dsn), 0700); err != nil {
			return nil, err
		}
		f, err := os.OpenFile(dsn, os.O_CREATE|os.O_RDWR, 0600)
		if err != nil {
			return nil, err
		}
		_ = f.Close()
	}
	db, err := sql.Open(name, dsn)
	if err != nil {
		return nil, err
	}
	db.SetMaxOpenConns(1)
	r := &SQLRepository{db, driver}
	if err = db.Ping(); err != nil {
		_ = db.Close()
		return nil, errors.New("无法连接数据库，请检查配置")
	}
	if driver == "sqlite" {
		if _, err = db.Exec("PRAGMA foreign_keys=ON; PRAGMA journal_mode=WAL; PRAGMA synchronous=FULL; PRAGMA busy_timeout=5000;"); err != nil {
			_ = db.Close()
			return nil, err
		}
	}
	if err = r.migrate(); err != nil {
		_ = db.Close()
		return nil, err
	}
	return r, nil
}
func (r *SQLRepository) Close() error { return r.db.Close() }
func (r *SQLRepository) bind(q string) string {
	if r.driver != "postgres" {
		return q
	}
	var b strings.Builder
	n := 0
	for _, c := range q {
		if c == '?' {
			n++
			fmt.Fprintf(&b, "$%d", n)
		} else {
			b.WriteRune(c)
		}
	}
	return b.String()
}
func (r *SQLRepository) exec(q string, args ...any) (sql.Result, error) {
	return r.db.Exec(r.bind(q), args...)
}
func (r *SQLRepository) row(q string, args ...any) *sql.Row { return r.db.QueryRow(r.bind(q), args...) }
func (r *SQLRepository) query(q string, args ...any) (*sql.Rows, error) {
	return r.db.Query(r.bind(q), args...)
}

type transaction struct {
	tx *sql.Tx
	r  *SQLRepository
}

func (r *SQLRepository) begin() (*transaction, error) {
	tx, err := r.db.Begin()
	if err != nil {
		return nil, err
	}
	t := &transaction{tx, r}
	if r.driver == "postgres" {
		if _, err = tx.Exec("SELECT pg_advisory_xact_lock(867530918)"); err != nil {
			_ = tx.Rollback()
			return nil, err
		}
	}
	return t, nil
}
func (t *transaction) exec(q string, args ...any) (sql.Result, error) {
	return t.tx.Exec(t.r.bind(q), args...)
}
func (t *transaction) row(q string, args ...any) *sql.Row { return t.tx.QueryRow(t.r.bind(q), args...) }
func (t *transaction) query(q string, args ...any) (*sql.Rows, error) {
	return t.tx.Query(t.r.bind(q), args...)
}
func (t *transaction) audit(actor, action, target, detail string) error {
	_, err := t.exec("INSERT INTO audit VALUES(?,?,?,?,?,?)", platform.ID("audit_"), actor, action, target, detail, platform.Now())
	return err
}
func (r *SQLRepository) migrate() error {
	t, err := r.begin()
	if err != nil {
		return err
	}
	defer t.tx.Rollback()
	if _, err = t.exec("CREATE TABLE IF NOT EXISTS schema_migrations(version BIGINT PRIMARY KEY,applied_at TEXT NOT NULL)"); err != nil {
		return err
	}
	var version int64
	if err = t.row("SELECT COALESCE(MAX(version),0) FROM schema_migrations").Scan(&version); err != nil {
		return err
	}
	if version > 1 {
		return errors.New("数据库版本高于当前程序，请升级程序")
	}
	if version == 1 {
		return t.tx.Commit()
	}
	schema := strings.Split(initialSchema, ";")

	for _, q := range schema {
		if strings.TrimSpace(q) == "" {
			continue
		}
		if _, err = t.exec(q); err != nil {
			return err
		}
	}
	_, err = t.exec("INSERT INTO pricing(version,body,actor,created_at) VALUES(1,?,'system',?) ON CONFLICT(version) DO NOTHING", platform.JSON(platform.DefaultPricing()), platform.Now())
	if err != nil {
		return err
	}
	_, err = t.exec("INSERT INTO schema_migrations VALUES(1,?) ON CONFLICT(version) DO NOTHING", platform.Now())
	if err != nil {
		return err
	}
	return t.tx.Commit()
}

var _ platform.Repository = (*SQLRepository)(nil)
