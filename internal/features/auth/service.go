package auth

import (
	"context"
	"crypto/sha256"
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

type Claims struct {
	UserID string `json:"user_id"`
	jwt.RegisteredClaims
}

type TokenPair struct {
	AccessToken  string `json:"access_token"`
	RefreshToken string `json:"refresh_token"`
}

type service struct {
	r repository
	secretKey []byte
}

func NewService(r repository, secretKey string) *service {
	byteSecretKey := []byte(secretKey)
	return &service{
		r: r,
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
	token, err := jwt.Parse(refreshToken, func(token *jwt.Token) (interface{}, error) {
		return s.secretKey, nil
	})
	if err != nil {
		return err
	}

	if !token.Valid {
		return errors.New("invalid refresh token")
	}

	claims, ok := token.Claims.(jwt.MapClaims)
	if !ok {
		return errors.New("failed to extract claims")
	}

	tokenID, ok := claims["jti"].(string)
	if !ok {
		return errors.New("failed to extract token ID")
	}

	err = s.r.RevokeToken(context, tokenID)
	if err != nil {
		return err
	}

	return nil
}

func (s *service) RefreshToken(context context.Context, refreshToken string) (TokenPair, error) {
	token, err := jwt.Parse(refreshToken, func(token *jwt.Token) (interface{}, error) {
		return s.secretKey, nil
	})
	if err != nil {
		return TokenPair{}, err
	}

	if !token.Valid {
		return TokenPair{}, errors.New("invalid refresh token")
	}

	claims, ok := token.Claims.(jwt.MapClaims)
	if !ok {
		return TokenPair{}, errors.New("failed to extract claims")
	}

	userID, ok := claims["user_id"].(string)
	if !ok {
		return TokenPair{}, errors.New("failed to extract user ID")
	}

	tokenID, ok := claims["jti"].(string)
	if !ok {
		return TokenPair{}, errors.New("failed to extract token ID")
	}

	savedRefreshToken, err := s.r.GetTokenByHash(context, tokenID)
	if err != nil {
		return TokenPair{}, err
	}

	if savedRefreshToken == nil || savedRefreshToken.UsuarioID != userID {
		return TokenPair{}, errors.New("refresh token not found or does not match user")
	}

	if savedRefreshToken.IsRevoked() {
		return TokenPair{}, app.ErrTokenRevoked
	}

	if savedRefreshToken.IsExpired() {
		newTokenPair, err := s.createTokens(userID)
		if err != nil {
			return TokenPair{}, err
		}

		s.r.RevokeToken(context, tokenID)

		return newTokenPair, nil
	}

	accessTokenString, err := s.createAccessToken(context, userID)
	if err != nil {
		return TokenPair{}, err
	}

	return TokenPair{
		AccessToken:  accessTokenString,
		RefreshToken: refreshToken,
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
		UserID: userID,
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
	tokenID := fmt.Sprintf("%d-%s", time.Now().UnixNano(), userID)
	tokenHashBytes := sha256.Sum256([]byte(tokenID))
	tokenHashString := hex.EncodeToString(tokenHashBytes[:])
	refreshClaims := &Claims{
		UserID: userID,
		RegisteredClaims: jwt.RegisteredClaims{
			ID:        tokenHashString,
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
		TokenHash:  tokenHashString,
		DeviceName: "default",
		ExpiresAt:  &refreshClaims.ExpiresAt.Time,
	}, refreshTokenString, nil
}

func (s *service) ParseToken(tokenString string) (string, error) {
	token, err := jwt.Parse(tokenString, func(token *jwt.Token) (interface{}, error) {
		return []byte(s.secretKey), nil
	})
	if err != nil {
		return "", err
	}

	if !token.Valid {
		return "", errors.New("invalid token")
	}

	claims, ok := token.Claims.(jwt.MapClaims)
	if !ok {
		return "", errors.New("failed to extract claims")
	}

	userID, ok := claims["user_id"].(string)
	if !ok {
		return "", errors.New("failed to extract user ID")
	}

	return userID, nil
}
