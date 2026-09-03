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
}

type RegisterUseCase interface {
	Register(context context.Context, input *RegisterInput) error
}

type LoginUseCase interface {
	Login(context context.Context, input *LoginInput) (TokenPair, error)
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
		return TokenPair{}, err
	}

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
		return TokenPair{}, err
	}

	err = s.r.SaveRefreshToken(context.Background(), &RefreshTokenEntity{
		UsuarioID:  userID,
		TokenHash:  tokenHashString,
		DeviceName: "default",
		ExpiresAt:  &refreshClaims.ExpiresAt.Time,
	})
	if err != nil {
		return TokenPair{}, err
	}

	return TokenPair{
		AccessToken:  accessTokenString,
		RefreshToken: refreshTokenString,
	}, nil
}

func (s *service) parseToken(tokenString string) (string, error) {
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
