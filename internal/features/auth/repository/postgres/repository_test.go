package postgres

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/caslumes/solado_certo_backend/internal/app"
	"github.com/caslumes/solado_certo_backend/internal/app/enum"
	"github.com/caslumes/solado_certo_backend/internal/features/auth"
	"github.com/caslumes/solado_certo_backend/internal/testutil"
	"github.com/jackc/pgx/v5/pgxpool"
)

func newRepository(t *testing.T) (*postgresRepository, *pgxpool.Pool) {
	t.Helper()
	pool := testutil.DB(t)
	return NewPostgresRepository(pool), pool
}

func newUser() *auth.UserEntity {
	return &auth.UserEntity{
		Name:         "Maria Silva",
		Email:        "maria@example.com",
		Phone:        "11912345678",
		PasswordHash: "hash",
	}
}

func TestCreateUserStoresPendingUser(t *testing.T) {
	r, _ := newRepository(t)
	ctx := context.Background()

	if err := r.CreateUser(ctx, newUser()); err != nil {
		t.Fatalf("CreateUser: %v", err)
	}

	user, err := r.GetUserByEmail(ctx, "maria@example.com")
	if err != nil {
		t.Fatalf("GetUserByEmail: %v", err)
	}
	if user.ID == "" || user.Name != "Maria Silva" || user.Phone != "11912345678" || user.PasswordHash != "hash" {
		t.Fatalf("user = %+v", user)
	}
	if user.Status != string(enum.ProfileStatusPending) {
		t.Fatalf("status = %q, want %q", user.Status, enum.ProfileStatusPending)
	}
	if user.CreatedAt == nil || user.UpdatedAt == nil {
		t.Fatalf("timestamps were not set: %+v", user)
	}
}

func TestCreateUserRejectsEmailOrPhoneInUse(t *testing.T) {
	tests := map[string]func(*auth.UserEntity){
		"e-mail": func(u *auth.UserEntity) { u.Phone = "11900000000" },
		"phone":  func(u *auth.UserEntity) { u.Email = "outra@example.com" },
	}

	for name, mutate := range tests {
		t.Run(name, func(t *testing.T) {
			r, _ := newRepository(t)
			if err := r.CreateUser(context.Background(), newUser()); err != nil {
				t.Fatalf("CreateUser: %v", err)
			}
			duplicate := newUser()
			mutate(duplicate)

			err := r.CreateUser(context.Background(), duplicate)

			if !errors.Is(err, app.ErrUserAlreadyExists) {
				t.Fatalf("err = %v, want %v", err, app.ErrUserAlreadyExists)
			}
		})
	}
}

func TestGetUserByEmailReturnsNotFound(t *testing.T) {
	r, _ := newRepository(t)

	_, err := r.GetUserByEmail(context.Background(), "ninguem@example.com")

	if !errors.Is(err, app.ErrUserNotFound) {
		t.Fatalf("err = %v, want %v", err, app.ErrUserNotFound)
	}
}

func newToken(userID, hash string) *auth.RefreshTokenEntity {
	expiresAt := time.Now().Add(time.Hour).UTC().Truncate(time.Microsecond)
	return &auth.RefreshTokenEntity{UserID: userID, TokenHash: hash, DeviceName: "Pixel 8", ExpiresAt: &expiresAt}
}

func getToken(t *testing.T, r *postgresRepository, hash string) *auth.RefreshTokenEntity {
	t.Helper()
	token, err := r.GetTokenByHash(context.Background(), hash)
	if err != nil {
		t.Fatalf("GetTokenByHash(%s): %v", hash, err)
	}
	return token
}

func TestSaveRefreshTokenCanBeReadByHash(t *testing.T) {
	r, pool := newRepository(t)
	user := testutil.InsertUser(t, pool)
	token := newToken(user.ID, "hash-1")

	if err := r.SaveRefreshToken(context.Background(), token); err != nil {
		t.Fatalf("SaveRefreshToken: %v", err)
	}

	saved := getToken(t, r, "hash-1")
	if saved.ID == "" || saved.UserID != user.ID || saved.DeviceName != "Pixel 8" || saved.IsRevoked() {
		t.Fatalf("token = %+v", saved)
	}
	if saved.ExpiresAt == nil || !saved.ExpiresAt.Equal(*token.ExpiresAt) {
		t.Fatalf("expires at = %v, want %v", saved.ExpiresAt, token.ExpiresAt)
	}
}

func TestGetTokenByHashReturnsNotFound(t *testing.T) {
	r, _ := newRepository(t)

	_, err := r.GetTokenByHash(context.Background(), "missing")

	if !errors.Is(err, app.ErrTokenNotFound) {
		t.Fatalf("err = %v, want %v", err, app.ErrTokenNotFound)
	}
}

func TestRevokeTokenRevokesOnlyOnce(t *testing.T) {
	r, pool := newRepository(t)
	ctx := context.Background()
	user := testutil.InsertUser(t, pool)
	if err := r.SaveRefreshToken(ctx, newToken(user.ID, "hash-1")); err != nil {
		t.Fatalf("SaveRefreshToken: %v", err)
	}

	if err := r.RevokeToken(ctx, "hash-1"); err != nil {
		t.Fatalf("RevokeToken: %v", err)
	}
	if !getToken(t, r, "hash-1").IsRevoked() {
		t.Fatal("token was not revoked")
	}

	if err := r.RevokeToken(ctx, "hash-1"); !errors.Is(err, app.ErrTokenNotFound) {
		t.Fatalf("second revoke err = %v, want %v", err, app.ErrTokenNotFound)
	}
	if err := r.RevokeToken(ctx, "missing"); !errors.Is(err, app.ErrTokenNotFound) {
		t.Fatalf("unknown token err = %v, want %v", err, app.ErrTokenNotFound)
	}
}

func TestRotateRefreshTokenReplacesToken(t *testing.T) {
	r, pool := newRepository(t)
	ctx := context.Background()
	user := testutil.InsertUser(t, pool)
	if err := r.SaveRefreshToken(ctx, newToken(user.ID, "old")); err != nil {
		t.Fatalf("SaveRefreshToken: %v", err)
	}

	if err := r.RotateRefreshToken(ctx, "old", newToken(user.ID, "new")); err != nil {
		t.Fatalf("RotateRefreshToken: %v", err)
	}

	if !getToken(t, r, "old").IsRevoked() {
		t.Fatal("old token was not revoked")
	}
	if getToken(t, r, "new").IsRevoked() {
		t.Fatal("new token is revoked")
	}
}

func TestRotateRefreshTokenRejectsRevokedTokenWithoutSavingNewOne(t *testing.T) {
	r, pool := newRepository(t)
	ctx := context.Background()
	user := testutil.InsertUser(t, pool)
	if err := r.SaveRefreshToken(ctx, newToken(user.ID, "old")); err != nil {
		t.Fatalf("SaveRefreshToken: %v", err)
	}
	if err := r.RevokeToken(ctx, "old"); err != nil {
		t.Fatalf("RevokeToken: %v", err)
	}

	err := r.RotateRefreshToken(ctx, "old", newToken(user.ID, "new"))

	if !errors.Is(err, app.ErrTokenRevoked) {
		t.Fatalf("err = %v, want %v", err, app.ErrTokenRevoked)
	}
	if _, err := r.GetTokenByHash(ctx, "new"); !errors.Is(err, app.ErrTokenNotFound) {
		t.Fatalf("new token lookup err = %v, want %v", err, app.ErrTokenNotFound)
	}
}

func TestRevokeUserTokensRevokesOnlyThatUser(t *testing.T) {
	r, pool := newRepository(t)
	ctx := context.Background()
	user := testutil.InsertUser(t, pool)
	other := testutil.InsertUser(t, pool)
	for _, token := range []*auth.RefreshTokenEntity{
		newToken(user.ID, "phone"), newToken(user.ID, "tablet"), newToken(other.ID, "other"),
	} {
		if err := r.SaveRefreshToken(ctx, token); err != nil {
			t.Fatalf("SaveRefreshToken: %v", err)
		}
	}

	if err := r.RevokeUserTokens(ctx, user.ID); err != nil {
		t.Fatalf("RevokeUserTokens: %v", err)
	}

	if !getToken(t, r, "phone").IsRevoked() || !getToken(t, r, "tablet").IsRevoked() {
		t.Fatal("a token of the user is still active")
	}
	if getToken(t, r, "other").IsRevoked() {
		t.Fatal("another user's token was revoked")
	}
}
