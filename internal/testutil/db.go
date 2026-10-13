package testutil

import (
	"context"
	"fmt"
	"os"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

const lockKey = 20261008

var keptTables = map[string]bool{"onboarding_steps": true, "roles": true}

var sequence atomic.Int64

func DB(t testing.TB) *pgxpool.Pool {
	t.Helper()

	url := os.Getenv("TEST_DATABASE_URL")
	if url == "" {
		t.Skip("TEST_DATABASE_URL not set")
	}

	cfg, err := pgxpool.ParseConfig(url)
	if err != nil {
		t.Fatalf("parse TEST_DATABASE_URL: %v", err)
	}
	if !strings.HasSuffix(cfg.ConnConfig.Database, "_test") {
		t.Fatalf("TEST_DATABASE_URL must point to a database ending in _test, got %q", cfg.ConnConfig.Database)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	pool, err := pgxpool.NewWithConfig(ctx, cfg)
	if err != nil {
		t.Skipf("test database unavailable: %v", err)
	}
	if err := pool.Ping(ctx); err != nil {
		pool.Close()
		t.Skipf("test database unavailable: %v", err)
	}

	lock, err := pool.Acquire(context.Background())
	if err != nil {
		pool.Close()
		t.Fatalf("acquire lock connection: %v", err)
	}
	if _, err := lock.Exec(context.Background(), "SELECT pg_advisory_lock($1)", lockKey); err != nil {
		lock.Release()
		pool.Close()
		t.Fatalf("advisory lock: %v", err)
	}

	t.Cleanup(func() {
		_, _ = lock.Exec(context.Background(), "SELECT pg_advisory_unlock($1)", lockKey)
		lock.Release()
		pool.Close()
	})

	Truncate(t, pool)

	return pool
}

func Truncate(t testing.TB, pool *pgxpool.Pool) {
	t.Helper()

	ctx := context.Background()
	rows, err := pool.Query(ctx, "SELECT tablename FROM pg_tables WHERE schemaname = 'public'")
	if err != nil {
		t.Fatalf("list tables: %v", err)
	}
	names, err := pgx.CollectRows(rows, pgx.RowTo[string])
	if err != nil {
		t.Fatalf("list tables: %v", err)
	}

	var tables []string
	for _, name := range names {
		if !keptTables[name] {
			tables = append(tables, pgx.Identifier{name}.Sanitize())
		}
	}
	if len(tables) == 0 {
		return
	}

	if _, err := pool.Exec(ctx, "TRUNCATE "+strings.Join(tables, ", ")+" RESTART IDENTITY CASCADE"); err != nil {
		t.Fatalf("truncate: %v", err)
	}
}

type User struct {
	ID    string
	Name  string
	Email string
	Phone string
}

func InsertUser(t testing.TB, pool *pgxpool.Pool) User {
	t.Helper()

	n := sequence.Add(1)
	u := User{
		Name:  fmt.Sprintf("Usuário %d", n),
		Email: fmt.Sprintf("user%d@example.com", n),
		Phone: fmt.Sprintf("119%08d", n),
	}

	now := time.Now().UTC()
	err := pool.QueryRow(
		context.Background(),
		"INSERT INTO users (name, email, phone, password_hash, status, created_at, updated_at) "+
			"VALUES ($1, $2, $3, $4, 'active', $5, $5) RETURNING id",
		u.Name, u.Email, u.Phone, "not-a-real-hash", now,
	).Scan(&u.ID)
	if err != nil {
		t.Fatalf("insert user: %v", err)
	}

	return u
}

type Category struct {
	ID       string
	ParentID *string
	Name     string
	Slug     string
}

func InsertCategory(t testing.TB, pool *pgxpool.Pool, parentID *string) Category {
	t.Helper()

	n := sequence.Add(1)
	c := Category{
		ParentID: parentID,
		Name:     fmt.Sprintf("Categoria %d", n),
		Slug:     fmt.Sprintf("categoria-%d", n),
	}

	err := pool.QueryRow(
		context.Background(),
		"INSERT INTO categories (parent_category_id, name, slug) VALUES ($1, $2, $3) RETURNING id",
		c.ParentID, c.Name, c.Slug,
	).Scan(&c.ID)
	if err != nil {
		t.Fatalf("insert category: %v", err)
	}

	return c
}

type Ad struct {
	ID                   string
	SellerID             string
	CategoryID           string
	Title                string
	Description          string
	Price                string
	Stock                int
	Status               string
	PrescriptionRequired bool
	CustomMadeAllowed    bool
	Side                 *string
	Size                 *string
	Material             *string
	Rigidity             *string
	UsageIndication      *string
	CreatedAt            time.Time
}

func InsertAd(t testing.TB, pool *pgxpool.Pool, ad Ad) Ad {
	t.Helper()

	n := sequence.Add(1)
	if ad.Title == "" {
		ad.Title = fmt.Sprintf("Palmilha %d", n)
	}
	if ad.Description == "" {
		ad.Description = "Descrição do anúncio"
	}
	if ad.Price == "" {
		ad.Price = "100.00"
	}
	if ad.Status == "" {
		ad.Status = "published"
	}
	if ad.CreatedAt.IsZero() {
		ad.CreatedAt = time.Now().UTC()
	}

	err := pool.QueryRow(
		context.Background(),
		"INSERT INTO advertisements (seller_id, category_id, title, description, price, stock, status, "+
			"prescription_required, custom_made_allowed, side, size, material, rigidity, usage_indication, created_at, updated_at) "+
			"VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14, $15, $15) RETURNING id",
		ad.SellerID, ad.CategoryID, ad.Title, ad.Description, ad.Price, ad.Stock, ad.Status,
		ad.PrescriptionRequired, ad.CustomMadeAllowed, ad.Side, ad.Size, ad.Material, ad.Rigidity, ad.UsageIndication, ad.CreatedAt,
	).Scan(&ad.ID)
	if err != nil {
		t.Fatalf("insert ad: %v", err)
	}

	return ad
}
