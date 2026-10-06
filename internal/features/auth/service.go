package auth

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"errors"
	"fmt"
	"time"

	"github.com/caslumes/solado_certo_backend/internal/app"
	"github.com/golang-jwt/jwt/v5"
	"golang.org/x/crypto/bcrypt"
)

type repository interface {
	CreateUser(context context.Context, ue *UserEntity) error
	GetUserByEmail(context context.Context, email string) (*UserEntity, error)
	SaveRefreshToken(context context.Context, rt *RefreshTokenEntity) error
	GetTokenByHash(context context.Context, tokenHash string) (*RefreshTokenEntity, error)
	RevokeToken(context context.Context, tokenHash string) error
	RotateRefreshToken(context context.Context, oldTokenHash string, newToken *RefreshTokenEntity) error
	RevokeUserTokens(context context.Context, userID string) error
}

type RegisterUseCase interface {
	Register(context context.Context, input *RegisterInput) error
}

type LoginUseCase interface {
	Login(context context.Context, input *LoginInput) (TokenPair, error)
}

type LogoutUseCase interface {
	Logout(context context.Context, refreshToken string) error
}

type RefreshTokenUseCase interface {
	RefreshToken(context context.Context, refreshToken string) (TokenPair, error)
}

const (
	accessTokenType  = "access"
	refreshTokenType = "refresh"
)

type Claims struct {
	UserID    string `json:"user_id"`
	TokenType string `json:"typ"`
	jwt.RegisteredClaims
}

type TokenPair struct {
	AccessToken  string `json:"access_token"`
	RefreshToken string `json:"refresh_token"`
}

type service struct {
	r         repository
	secretKey []byte
}

func NewService(r repository, secretKey string) *service {
	byteSecretKey := []byte(secretKey)
	return &service{
		r:         r,
		secretKey: byteSecretKey,
	}
}

func (s *service) Register(context context.Context, input *RegisterInput) error {
	if input == nil || input.Name == "" || input.Email == "" || input.Phone == "" || len(input.Password) < 8 {
		return app.ErrInvalidInput
	}

	hashedPassword, err := hashPassword(input.Password)
	if err != nil {
		return err
	}

	ue := &UserEntity{
		Nome:      input.Name,
		Email:     input.Email,
		Telefone:  input.Phone,
		SenhaHash: hashedPassword,
	}

	return s.r.CreateUser(context, ue)
}

func (s *service) Login(context context.Context, input *LoginInput) (TokenPair, error) {
	if input == nil || input.Email == "" || input.Password == "" {
		return TokenPair{}, app.ErrInvalidInput
	}

	ue, err := s.r.GetUserByEmail(context, input.Email)
	if err != nil {
		if errors.Is(err, app.ErrUserNotFound) {
			return TokenPair{}, app.ErrInvalidCredentials
		}

		return TokenPair{}, err
	}

	if !passwordMatches(ue.SenhaHash, input.Password) {
		return TokenPair{}, app.ErrInvalidCredentials
	}

	tokenPair, err := s.createTokens(ue.ID)
	if err != nil {
		fmt.Printf("%+v", err)
		return TokenPair{}, err
	}

	return tokenPair, nil
}

func (s *service) Logout(context context.Context, refreshToken string) error {
	claims, err := s.parseToken(refreshToken, refreshTokenType)
	if err != nil {
		return err
	}

	tokenID := claims.ID
	if tokenID == "" {
		return app.ErrInvalidToken
	}

	err = s.r.RevokeToken(context, hashTokenID(tokenID))
	if err != nil {
		return err
	}

	return nil
}

func (s *service) RefreshToken(context context.Context, refreshToken string) (TokenPair, error) {
	claims, err := s.parseToken(refreshToken, refreshTokenType)
	if err != nil {
		return TokenPair{}, err
	}

	userID := claims.UserID
	tokenID := claims.ID
	if tokenID == "" {
		return TokenPair{}, app.ErrInvalidToken
	}

	tokenHash := hashTokenID(tokenID)
	savedRefreshToken, err := s.r.GetTokenByHash(context, tokenHash)
	if err != nil {
		return TokenPair{}, err
	}

	if savedRefreshToken.UsuarioID != userID {
		return TokenPair{}, app.ErrInvalidToken
	}

	if savedRefreshToken.IsRevoked() {
		if err := s.r.RevokeUserTokens(context, userID); err != nil {
			return TokenPair{}, err
		}

		return TokenPair{}, app.ErrTokenRevoked
	}

	if savedRefreshToken.IsExpired() {
		return TokenPair{}, app.ErrTokenExpired
	}

	accessTokenString, err := s.createAccessToken(context, userID)
	if err != nil {
		return TokenPair{}, err
	}

	newRefreshToken, newRefreshTokenString, err := s.createRefreshToken(context, userID)
	if err != nil {
		return TokenPair{}, err
	}

	err = s.r.RotateRefreshToken(context, tokenHash, &newRefreshToken)
	if err != nil {
		return TokenPair{}, err
	}

	return TokenPair{
		AccessToken:  accessTokenString,
		RefreshToken: newRefreshTokenString,
	}, nil
}

func hashPassword(password string) (string, error) {
	bytes, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
	if err != nil {
		return "", err
	}

	return string(bytes), nil
}

func passwordMatches(hashedPassword, password string) bool {
	return bcrypt.CompareHashAndPassword([]byte(hashedPassword), []byte(password)) == nil
}

func (s *service) createTokens(userID string) (TokenPair, error) {
	accessTokenString, err := s.createAccessToken(context.Background(), userID)
	if err != nil {
		return TokenPair{}, err
	}

	refreshToken, refreshTokenString, err := s.createRefreshToken(context.Background(), userID)
	if err != nil {
		return TokenPair{}, err
	}

	err = s.r.SaveRefreshToken(context.Background(), &refreshToken)
	if err != nil {
		return TokenPair{}, err
	}

	return TokenPair{
		AccessToken:  accessTokenString,
		RefreshToken: refreshTokenString,
	}, nil
}

func (s *service) createAccessToken(context context.Context, userID string) (string, error) {
	accessClaims := &Claims{
		UserID:    userID,
		TokenType: accessTokenType,
		RegisteredClaims: jwt.RegisteredClaims{
			ExpiresAt: jwt.NewNumericDate(time.Now().Add(time.Minute * 15)),
			IssuedAt:  jwt.NewNumericDate(time.Now()),
		},
	}
	accessToken := jwt.NewWithClaims(jwt.SigningMethodHS256, accessClaims)
	accessTokenString, err := accessToken.SignedString([]byte(s.secretKey))
	if err != nil {
		return "", err
	}

	return accessTokenString, nil
}

func (s *service) createRefreshToken(context context.Context, userID string) (RefreshTokenEntity, string, error) {
	tokenID, err := newTokenID()
	if err != nil {
		return RefreshTokenEntity{}, "", err
	}

	refreshClaims := &Claims{
		UserID:    userID,
		TokenType: refreshTokenType,
		RegisteredClaims: jwt.RegisteredClaims{
			ID:        tokenID,
			ExpiresAt: jwt.NewNumericDate(time.Now().Add(7 * 24 * time.Hour)),
			IssuedAt:  jwt.NewNumericDate(time.Now()),
		},
	}
	refreshToken := jwt.NewWithClaims(jwt.SigningMethodHS256, refreshClaims)
	refreshTokenString, err := refreshToken.SignedString(s.secretKey)
	if err != nil {
		return RefreshTokenEntity{}, "", err
	}

	return RefreshTokenEntity{
		UsuarioID:  userID,
		TokenHash:  hashTokenID(tokenID),
		DeviceName: "default",
		ExpiresAt:  &refreshClaims.ExpiresAt.Time,
	}, refreshTokenString, nil
}

func newTokenID() (string, error) {
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}

	return base64.RawURLEncoding.EncodeToString(b), nil
}

func hashTokenID(tokenID string) string {
	hash := sha256.Sum256([]byte(tokenID))
	return hex.EncodeToString(hash[:])
}

func (s *service) ParseToken(tokenString string) (string, error) {
	claims, err := s.parseToken(tokenString, accessTokenType)
	if err != nil {
		return "", err
	}

	return claims.UserID, nil
}

func (s *service) parseToken(tokenString, expectedType string) (*Claims, error) {
	claims := &Claims{}
	_, err := jwt.ParseWithClaims(tokenString, claims,
		func(token *jwt.Token) (interface{}, error) {
			return s.secretKey, nil
		},
		jwt.WithValidMethods([]string{jwt.SigningMethodHS256.Alg()}),
		jwt.WithExpirationRequired(),
	)
	if errors.Is(err, jwt.ErrTokenExpired) {
		return nil, app.ErrTokenExpired
	}
	if err != nil {
		return nil, fmt.Errorf("%w: %v", app.ErrInvalidToken, err)
	}

	if claims.TokenType != expectedType {
		return nil, fmt.Errorf("%w: unexpected token type", app.ErrInvalidToken)
	}

	if claims.UserID == "" {
		return nil, fmt.Errorf("%w: missing user ID", app.ErrInvalidToken)
	}

	return claims, nil
}
