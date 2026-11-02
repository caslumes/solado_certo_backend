package postgres

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/caslumes/solado_certo_backend/internal/app"
	"github.com/caslumes/solado_certo_backend/internal/features/auth"
	"github.com/caslumes/solado_certo_backend/internal/testutil"
	"github.com/jackc/pgx/v5/pgxpool"
)

func createReset(t *testing.T, r *postgresRepository, userID, hash string) *auth.PasswordResetEntity {
	t.Helper()
	reset := &auth.PasswordResetEntity{UserID: userID, CodeHash: hash, ExpiresAt: time.Now().Add(auth.ResetCodeLifetime)}
	if err := r.CreatePasswordReset(context.Background(), reset); err != nil {
		t.Fatalf("CreatePasswordReset: %v", err)
	}
	return reset
}

func resetState(t *testing.T, pool *pgxpool.Pool, id string) (attempts int, used bool, active bool) {
	t.Helper()
	err := pool.QueryRow(
		context.Background(),
		"SELECT attempts, used_at IS NOT NULL, expires_at > now() FROM password_reset_tokens WHERE id = $1",
		id,
	).Scan(&attempts, &used, &active)
	if err != nil {
		t.Fatalf("read reset %s: %v", id, err)
	}
	return attempts, used, active
}

func TestLastPasswordResetAt(t *testing.T) {
	r, pool := newRepository(t)
	ctx := context.Background()
	user := testutil.InsertUser(t, pool)

	last, err := r.LastPasswordResetAt(ctx, user.ID)
	if err != nil || last != nil {
		t.Fatalf("no code yet: last = %v, err = %v", last, err)
	}

	reset := createReset(t, r, user.ID, "hash-1")
	last, err = r.LastPasswordResetAt(ctx, user.ID)
	if err != nil {
		t.Fatalf("LastPasswordResetAt: %v", err)
	}
	if last == nil || !last.Equal(reset.CreatedAt.Truncate(time.Microsecond)) {
		t.Fatalf("last = %v, want %v", last, reset.CreatedAt)
	}
}

func TestCreatePasswordResetExpiresPreviousCode(t *testing.T) {
	r, pool := newRepository(t)
	user := testutil.InsertUser(t, pool)
	other := testutil.InsertUser(t, pool)
	first := createReset(t, r, user.ID, "hash-1")
	otherReset := createReset(t, r, other.ID, "hash-other")

	second := createReset(t, r, user.ID, "hash-2")

	if second.ID == "" || second.ID == first.ID {
		t.Fatalf("second id = %q", second.ID)
	}
	if _, _, active := resetState(t, pool, first.ID); active {
		t.Fatal("the previous code is still active")
	}
	if _, _, active := resetState(t, pool, second.ID); !active {
		t.Fatal("the new code is not active")
	}
	if _, _, active := resetState(t, pool, otherReset.ID); !active {
		t.Fatal("another user's code was expired")
	}
}

func TestClaimPasswordResetAttemptCountsTries(t *testing.T) {
	r, pool := newRepository(t)
	ctx := context.Background()
	user := testutil.InsertUser(t, pool)
	reset := createReset(t, r, user.ID, "hash-1")

	for attempt := 1; attempt <= 2; attempt++ {
		claimed, err := r.ClaimPasswordResetAttempt(ctx, user.ID, 2)
		if err != nil {
			t.Fatalf("attempt %d: %v", attempt, err)
		}
		if claimed.ID != reset.ID || claimed.CodeHash != "hash-1" || claimed.Attempts != attempt {
			t.Fatalf("attempt %d: claimed = %+v", attempt, claimed)
		}
	}

	if _, err := r.ClaimPasswordResetAttempt(ctx, user.ID, 2); !errors.Is(err, app.ErrResetAttemptsExceeded) {
		t.Fatalf("after the limit: err = %v, want %v", err, app.ErrResetAttemptsExceeded)
	}
	if attempts, _, _ := resetState(t, pool, reset.ID); attempts != 2 {
		t.Fatalf("attempts = %d, want 2", attempts)
	}
}

func TestClaimPasswordResetAttemptWithoutActiveCode(t *testing.T) {
	r, pool := newRepository(t)
	ctx := context.Background()
	user := testutil.InsertUser(t, pool)

	if _, err := r.ClaimPasswordResetAttempt(ctx, user.ID, auth.MaxResetAttempts); !errors.Is(err, app.ErrInvalidResetCode) {
		t.Fatalf("no code: err = %v, want %v", err, app.ErrInvalidResetCode)
	}

	reset := createReset(t, r, user.ID, "hash-1")
	if _, err := pool.Exec(ctx, "UPDATE password_reset_tokens SET expires_at = now() - interval '1 second' WHERE id = $1", reset.ID); err != nil {
		t.Fatalf("expire: %v", err)
	}
	if _, err := r.ClaimPasswordResetAttempt(ctx, user.ID, auth.MaxResetAttempts); !errors.Is(err, app.ErrInvalidResetCode) {
		t.Fatalf("expired code: err = %v, want %v", err, app.ErrInvalidResetCode)
	}
}

func TestClaimPasswordResetAttemptLimitHoldsUnderConcurrency(t *testing.T) {
	r, pool := newRepository(t)
	user := testutil.InsertUser(t, pool)
	reset := createReset(t, r, user.ID, "hash-1")

	const tries = 12
	var wg sync.WaitGroup
	var mu sync.Mutex
	claimed := 0
	for range tries {
		wg.Go(func() {
			if _, err := r.ClaimPasswordResetAttempt(context.Background(), user.ID, auth.MaxResetAttempts); err == nil {
				mu.Lock()
				claimed++
				mu.Unlock()
			}
		})
	}
	wg.Wait()

	if claimed != auth.MaxResetAttempts {
		t.Fatalf("claimed = %d, want %d", claimed, auth.MaxResetAttempts)
	}
	if attempts, _, _ := resetState(t, pool, reset.ID); attempts != auth.MaxResetAttempts {
		t.Fatalf("attempts = %d, want %d", attempts, auth.MaxResetAttempts)
	}
}

func TestCompletePasswordResetChangesPasswordAndRevokesTokens(t *testing.T) {
	r, pool := newRepository(t)
	ctx := context.Background()
	user := testutil.InsertUser(t, pool)
	other := testutil.InsertUser(t, pool)
	for _, token := range []*auth.RefreshTokenEntity{newToken(user.ID, "phone"), newToken(other.ID, "other")} {
		if err := r.SaveRefreshToken(ctx, token); err != nil {
			t.Fatalf("SaveRefreshToken: %v", err)
		}
	}
	reset := createReset(t, r, user.ID, "hash-1")

	if err := r.CompletePasswordReset(ctx, reset.ID, user.ID, "new-hash"); err != nil {
		t.Fatalf("CompletePasswordReset: %v", err)
	}

	var passwordHash string
	if err := pool.QueryRow(ctx, "SELECT password_hash FROM users WHERE id = $1", user.ID).Scan(&passwordHash); err != nil {
		t.Fatalf("read user: %v", err)
	}
	if passwordHash != "new-hash" {
		t.Fatalf("password hash = %q", passwordHash)
	}
	if _, used, _ := resetState(t, pool, reset.ID); !used {
		t.Fatal("the code was not marked used")
	}
	if !getToken(t, r, "phone").IsRevoked() {
		t.Fatal("the user's session is still active")
	}
	if getToken(t, r, "other").IsRevoked() {
		t.Fatal("another user's session was revoked")
	}

	var audits int
	err := pool.QueryRow(ctx, "SELECT count(*) FROM audit_logs WHERE user_id = $1 AND entity = 'user' AND entity_id = $1 AND action = 'user.password_reset'", user.ID).Scan(&audits)
	if err != nil || audits != 1 {
		t.Fatalf("audit rows = %d, err = %v", audits, err)
	}

	if err := r.CompletePasswordReset(ctx, reset.ID, user.ID, "other-hash"); !errors.Is(err, app.ErrInvalidResetCode) {
		t.Fatalf("second use: err = %v, want %v", err, app.ErrInvalidResetCode)
	}
}

func TestCompletePasswordResetRejectsAnotherUsersCode(t *testing.T) {
	r, pool := newRepository(t)
	ctx := context.Background()
	user := testutil.InsertUser(t, pool)
	other := testutil.InsertUser(t, pool)
	reset := createReset(t, r, other.ID, "hash-1")

	if err := r.CompletePasswordReset(ctx, reset.ID, user.ID, "new-hash"); !errors.Is(err, app.ErrInvalidResetCode) {
		t.Fatalf("err = %v, want %v", err, app.ErrInvalidResetCode)
	}
}
