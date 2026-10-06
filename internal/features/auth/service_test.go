package auth

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/caslumes/solado_certo_backend/internal/app"
	"github.com/golang-jwt/jwt/v5"
)

const testSecretKey = "test-secret-key"

type fakeRepository struct {
	tokens map[string]*RefreshTokenEntity
}

func newFakeRepository() *fakeRepository {
	return &fakeRepository{tokens: map[string]*RefreshTokenEntity{}}
}

func (f *fakeRepository) CreateUser(context.Context, *UserEntity) error {
	return nil
}

func (f *fakeRepository) GetUserByEmail(context.Context, string) (*UserEntity, error) {
	return nil, nil
}

func (f *fakeRepository) SaveRefreshToken(_ context.Context, rt *RefreshTokenEntity) error {
	f.tokens[rt.TokenHash] = rt
	return nil
}

func (f *fakeRepository) GetTokenByHash(_ context.Context, tokenHash string) (*RefreshTokenEntity, error) {
	return f.tokens[tokenHash], nil
}

func (f *fakeRepository) RevokeToken(_ context.Context, tokenHash string) error {
	if rt, ok := f.tokens[tokenHash]; ok {
		now := time.Now()
		rt.RevokedAt = &now
	}
	return nil
}

func newTestService(t *testing.T) (*service, TokenPair) {
	t.Helper()

	s := NewService(newFakeRepository(), testSecretKey)
	tokens, err := s.createTokens("user-1")
	if err != nil {
		t.Fatalf("createTokens: %v", err)
	}

	return s, tokens
}

func signToken(t *testing.T, method jwt.SigningMethod, key any, claims *Claims) string {
	t.Helper()

	token, err := jwt.NewWithClaims(method, claims).SignedString(key)
	if err != nil {
		t.Fatalf("SignedString: %v", err)
	}

	return token
}

func validClaims(tokenType string) *Claims {
	return &Claims{
		UserID:    "user-1",
		TokenType: tokenType,
		RegisteredClaims: jwt.RegisteredClaims{
			ExpiresAt: jwt.NewNumericDate(time.Now().Add(time.Minute)),
		},
	}
}

func TestParseTokenAcceptsAccessToken(t *testing.T) {
	s, tokens := newTestService(t)

	userID, err := s.ParseToken(tokens.AccessToken)
	if err != nil {
		t.Fatalf("ParseToken: %v", err)
	}
	if userID != "user-1" {
		t.Fatalf("userID = %q, want %q", userID, "user-1")
	}
}

func TestParseTokenRejectsRefreshToken(t *testing.T) {
	s, tokens := newTestService(t)

	if _, err := s.ParseToken(tokens.RefreshToken); err == nil {
		t.Fatal("refresh token was accepted as an access token")
	}
}

func TestRefreshTokenRejectsAccessToken(t *testing.T) {
	s, tokens := newTestService(t)

	if _, err := s.RefreshToken(context.Background(), tokens.AccessToken); err == nil {
		t.Fatal("access token was accepted by RefreshToken")
	}
}

func TestLogoutRejectsAccessToken(t *testing.T) {
	s, tokens := newTestService(t)

	if err := s.Logout(context.Background(), tokens.AccessToken); err == nil {
		t.Fatal("access token was accepted by Logout")
	}
}

func TestRefreshTokenAcceptsRefreshToken(t *testing.T) {
	s, tokens := newTestService(t)

	if _, err := s.RefreshToken(context.Background(), tokens.RefreshToken); err != nil {
		t.Fatalf("RefreshToken: %v", err)
	}
}

func TestParseTokenRejectsInvalidTokens(t *testing.T) {
	s := NewService(newFakeRepository(), testSecretKey)

	noExpiry := validClaims(accessTokenType)
	noExpiry.ExpiresAt = nil

	expired := validClaims(accessTokenType)
	expired.ExpiresAt = jwt.NewNumericDate(time.Now().Add(-time.Minute))

	noType := validClaims("")

	tests := map[string]string{
		"alg none":      signToken(t, jwt.SigningMethodNone, jwt.UnsafeAllowNoneSignatureType, validClaims(accessTokenType)),
		"HS512":         signToken(t, jwt.SigningMethodHS512, []byte(testSecretKey), validClaims(accessTokenType)),
		"wrong key":     signToken(t, jwt.SigningMethodHS256, []byte("other-key"), validClaims(accessTokenType)),
		"no expiry":     signToken(t, jwt.SigningMethodHS256, []byte(testSecretKey), noExpiry),
		"expired":       signToken(t, jwt.SigningMethodHS256, []byte(testSecretKey), expired),
		"no token type": signToken(t, jwt.SigningMethodHS256, []byte(testSecretKey), noType),
		"malformed":     "not-a-jwt",
	}

	for name, token := range tests {
		t.Run(name, func(t *testing.T) {
			if _, err := s.ParseToken(token); err == nil {
				t.Fatal("token was accepted")
			}
		})
	}
}

func refreshTokenID(t *testing.T, s *service, refreshToken string) string {
	t.Helper()

	claims, err := s.parseToken(refreshToken, refreshTokenType)
	if err != nil {
		t.Fatalf("parseToken: %v", err)
	}

	return claims.ID
}

func TestRefreshTokenIDIsRandom(t *testing.T) {
	s := NewService(newFakeRepository(), testSecretKey)

	first, err := s.createTokens("user-1")
	if err != nil {
		t.Fatalf("createTokens: %v", err)
	}
	second, err := s.createTokens("user-1")
	if err != nil {
		t.Fatalf("createTokens: %v", err)
	}

	firstID := refreshTokenID(t, s, first.RefreshToken)
	secondID := refreshTokenID(t, s, second.RefreshToken)

	if firstID == secondID {
		t.Fatal("two refresh tokens share the same ID")
	}
	if strings.Contains(firstID, "user-1") {
		t.Fatalf("token ID %q contains the user ID", firstID)
	}
}

func TestRefreshTokenStoresHashOfID(t *testing.T) {
	repository := newFakeRepository()
	s := NewService(repository, testSecretKey)

	tokens, err := s.createTokens("user-1")
	if err != nil {
		t.Fatalf("createTokens: %v", err)
	}
	tokenID := refreshTokenID(t, s, tokens.RefreshToken)

	if _, ok := repository.tokens[tokenID]; ok {
		t.Fatal("database stores the raw token ID")
	}
	if _, ok := repository.tokens[hashTokenID(tokenID)]; !ok {
		t.Fatal("database does not store the token ID hash")
	}
}

func TestLogoutRevokesRefreshToken(t *testing.T) {
	s, tokens := newTestService(t)

	if err := s.Logout(context.Background(), tokens.RefreshToken); err != nil {
		t.Fatalf("Logout: %v", err)
	}

	_, err := s.RefreshToken(context.Background(), tokens.RefreshToken)
	if !errors.Is(err, app.ErrTokenRevoked) {
		t.Fatalf("err = %v, want %v", err, app.ErrTokenRevoked)
	}
}
