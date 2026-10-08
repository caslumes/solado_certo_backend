package testutil

import (
	"context"
	"testing"
)

func TestFixturesAndTruncate(t *testing.T) {
	pool := DB(t)

	seller := InsertUser(t, pool)
	parent := InsertCategory(t, pool, nil)
	child := InsertCategory(t, pool, &parent.ID)
	ad := InsertAd(t, pool, Ad{SellerID: seller.ID, CategoryID: child.ID})
	if ad.ID == "" {
		t.Fatal("expected ad id")
	}

	Truncate(t, pool)

	ctx := context.Background()
	for _, table := range []string{"users", "categories", "advertisements"} {
		var count int
		if err := pool.QueryRow(ctx, "SELECT COUNT(*) FROM "+table).Scan(&count); err != nil {
			t.Fatalf("count %s: %v", table, err)
		}
		if count != 0 {
			t.Fatalf("%s not truncated: %d rows", table, count)
		}
	}

	var steps int
	if err := pool.QueryRow(ctx, "SELECT COUNT(*) FROM onboarding_steps").Scan(&steps); err != nil {
		t.Fatalf("count onboarding_steps: %v", err)
	}
	if steps == 0 {
		t.Fatal("onboarding_steps should be kept")
	}
}
