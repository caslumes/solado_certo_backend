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
	users  map[string]*UserEntity
}

func newFakeRepository() *fakeRepository {
	return &fakeRepository{
		tokens: map[string]*RefreshTokenEntity{},
		users:  map[string]*UserEntity{},
	}
}

func (f *fakeRepository) CreateUser(_ context.Context, ue *UserEntity) error {
	ue.ID = "user-" + ue.Email
	f.users[ue.Email] = ue
	return nil
}

func (f *fakeRepository) GetUserByEmail(_ context.Context, email string) (*UserEntity, error) {
	ue, ok := f.users[email]
	if !ok {
		return nil, app.ErrUserNotFound
	}
	return ue, nil
}

func (f *fakeRepository) SaveRefreshToken(_ context.Context, rt *RefreshTokenEntity) error {
	f.tokens[rt.TokenHash] = rt
	return nil
}

func (f *fakeRepository) GetTokenByHash(_ context.Context, tokenHash string) (*RefreshTokenEntity, error) {
	rt, ok := f.tokens[tokenHash]
	if !ok {
		return nil, app.ErrTokenNotFound
	}
	return rt, nil
}

func (f *fakeRepository) RevokeToken(_ context.Context, tokenHash string) error {
	rt, ok := f.tokens[tokenHash]
	if !ok || rt.IsRevoked() {
		return app.ErrTokenNotFound
	}
	now := time.Now()
	rt.RevokedAt = &now
	return nil
}

func (f *fakeRepository) RotateRefreshToken(_ context.Context, oldTokenHash string, newToken *RefreshTokenEntity) error {
	rt, ok := f.tokens[oldTokenHash]
	if !ok || rt.IsRevoked() {
		return app.ErrTokenRevoked
	}
	now := time.Now()
	rt.RevokedAt = &now
	f.tokens[newToken.TokenHash] = newToken
	return nil
}

func (f *fakeRepository) RevokeUserTokens(_ context.Context, userID string) error {
	now := time.Now()
	for _, rt := range f.tokens {
		if rt.UserID == userID && !rt.IsRevoked() {
			rt.RevokedAt = &now
		}
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

func TestRefreshTokenRotatesRefreshToken(t *testing.T) {
	s, tokens := newTestService(t)

	rotated, err := s.RefreshToken(context.Background(), tokens.RefreshToken)
	if err != nil {
		t.Fatalf("RefreshToken: %v", err)
	}
	if rotated.RefreshToken == "" || rotated.RefreshToken == tokens.RefreshToken {
		t.Fatal("refresh did not return a new refresh token")
	}
	if _, err := s.ParseToken(rotated.AccessToken); err != nil {
		t.Fatalf("new access token is invalid: %v", err)
	}

	if _, err := s.RefreshToken(context.Background(), rotated.RefreshToken); err != nil {
		t.Fatalf("rotated refresh token was rejected: %v", err)
	}
}

func TestRefreshTokenReuseRevokesAllUserTokens(t *testing.T) {
	s, tokens := newTestService(t)

	rotated, err := s.RefreshToken(context.Background(), tokens.RefreshToken)
	if err != nil {
		t.Fatalf("RefreshToken: %v", err)
	}

	_, err = s.RefreshToken(context.Background(), tokens.RefreshToken)
	if !errors.Is(err, app.ErrTokenRevoked) {
		t.Fatalf("reused token: err = %v, want %v", err, app.ErrTokenRevoked)
	}

	_, err = s.RefreshToken(context.Background(), rotated.RefreshToken)
	if !errors.Is(err, app.ErrTokenRevoked) {
		t.Fatalf("token issued before reuse: err = %v, want %v", err, app.ErrTokenRevoked)
	}
}

func TestRefreshTokenRejectsExpiredJWT(t *testing.T) {
	s := NewService(newFakeRepository(), testSecretKey)

	claims := validClaims(refreshTokenType)
	claims.ID = "token-id"
	claims.ExpiresAt = jwt.NewNumericDate(time.Now().Add(-time.Minute))
	token := signToken(t, jwt.SigningMethodHS256, []byte(testSecretKey), claims)

	_, err := s.RefreshToken(context.Background(), token)
	if !errors.Is(err, app.ErrTokenExpired) {
		t.Fatalf("err = %v, want %v", err, app.ErrTokenExpired)
	}
}

func TestRefreshTokenRejectsTokenExpiredInDatabase(t *testing.T) {
	repository := newFakeRepository()
	s := NewService(repository, testSecretKey)

	tokens, err := s.createTokens("user-1")
	if err != nil {
		t.Fatalf("createTokens: %v", err)
	}
	expiredAt := time.Now().Add(-time.Minute)
	repository.tokens[hashTokenID(refreshTokenID(t, s, tokens.RefreshToken))].ExpiresAt = &expiredAt

	_, err = s.RefreshToken(context.Background(), tokens.RefreshToken)
	if !errors.Is(err, app.ErrTokenExpired) {
		t.Fatalf("err = %v, want %v", err, app.ErrTokenExpired)
	}
}

func TestRefreshTokenRejectsUnknownToken(t *testing.T) {
	s := NewService(newFakeRepository(), testSecretKey)

	claims := validClaims(refreshTokenType)
	claims.ID = "unknown-token-id"
	token := signToken(t, jwt.SigningMethodHS256, []byte(testSecretKey), claims)

	_, err := s.RefreshToken(context.Background(), token)
	if !errors.Is(err, app.ErrTokenNotFound) {
		t.Fatalf("err = %v, want %v", err, app.ErrTokenNotFound)
	}
}

func TestRegisterStoresNormalizedFields(t *testing.T) {
	repository := newFakeRepository()
	s := NewService(repository, testSecretKey)

	err := s.Register(context.Background(), &RegisterInput{
		Name:     "  Ana Maria ",
		Email:    " Ana@Example.COM ",
		Phone:    "+55 (19) 91234-5678",
		Password: "password",
	})
	if err != nil {
		t.Fatalf("Register: %v", err)
	}

	ue, ok := repository.users["ana@example.com"]
	if !ok {
		t.Fatalf("user not stored under the normalized email: %v", repository.users)
	}
	if ue.Name != "Ana Maria" || ue.Phone != "19912345678" {
		t.Fatalf("stored user = %+v", ue)
	}
}

func TestRegisterRejectsInvalidInput(t *testing.T) {
	valid := RegisterInput{Name: "Ana", Email: "ana@example.com", Phone: "19912345678", Password: "password"}

	tests := map[string]func(*RegisterInput){
		"blank name":     func(in *RegisterInput) { in.Name = "  " },
		"invalid email":  func(in *RegisterInput) { in.Email = "ana" },
		"invalid phone":  func(in *RegisterInput) { in.Phone = "123" },
		"short password": func(in *RegisterInput) { in.Password = "1234567" },
		"long password":  func(in *RegisterInput) { in.Password = strings.Repeat("a", 73) },
	}

	for name, mutate := range tests {
		t.Run(name, func(t *testing.T) {
			input := valid
			mutate(&input)

			err := NewService(newFakeRepository(), testSecretKey).Register(context.Background(), &input)
			if !errors.Is(err, app.ErrInvalidInput) {
				t.Fatalf("err = %v, want %v", err, app.ErrInvalidInput)
			}
		})
	}
}

func TestLoginNormalizesEmail(t *testing.T) {
	s := NewService(newFakeRepository(), testSecretKey)
	err := s.Register(context.Background(), &RegisterInput{
		Name: "Ana", Email: "ana@example.com", Phone: "19912345678", Password: "password",
	})
	if err != nil {
		t.Fatalf("Register: %v", err)
	}

	if _, err := s.Login(context.Background(), &LoginInput{Email: " ANA@example.com", Password: "password"}); err != nil {
		t.Fatalf("Login: %v", err)
	}
}

func TestLoginUnknownEmailReturnsInvalidCredentials(t *testing.T) {
	s := NewService(newFakeRepository(), testSecretKey)

	_, err := s.Login(context.Background(), &LoginInput{Email: "nobody@example.com", Password: "password"})
	if !errors.Is(err, app.ErrInvalidCredentials) {
		t.Fatalf("err = %v, want %v", err, app.ErrInvalidCredentials)
	}
}
