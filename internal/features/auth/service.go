package auth

import (
	"context"
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"errors"
	"fmt"
	"log/slog"
	"math/big"
	"regexp"
	"strings"
	"sync"
	"time"

	"github.com/caslumes/solado_certo_backend/internal/app"
	"github.com/caslumes/solado_certo_backend/internal/helpers"
	"github.com/caslumes/solado_certo_backend/pkg/mailer"
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
	LastPasswordResetAt(context context.Context, userID string) (*time.Time, error)
	CreatePasswordReset(context context.Context, reset *PasswordResetEntity) error
	ClaimPasswordResetAttempt(context context.Context, userID string, maxAttempts int) (*PasswordResetEntity, error)
	CompletePasswordReset(context context.Context, resetID string, userID string, passwordHash string) error
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

type ForgotPasswordUseCase interface {
	ForgotPassword(context context.Context, email string) error
}

type ResetPasswordUseCase interface {
	ResetPassword(context context.Context, input *ResetPasswordInput) error
}

type WaitForMailUseCase interface {
	WaitForMail()
}

const (
	accessTokenType  = "access"
	refreshTokenType = "refresh"
)

const (
	ResetCodeLifetime   = 15 * time.Minute
	MaxResetAttempts    = 5
	ResetResendInterval = time.Minute
	resetCodeDigits     = 6
	resetNonceBytes     = 16
)

var resetCodePattern = regexp.MustCompile(`^[0-9]{6}$`)

var dummyPasswordHash, _ = hashPassword("dummy-password-for-timing")

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
	mail      mailer.Mailer
	sending   sync.WaitGroup
	now       func() time.Time
}

func NewService(r repository, secretKey string) *service {
	byteSecretKey := []byte(secretKey)
	return &service{
		r:         r,
		secretKey: byteSecretKey,
		mail:      mailer.Disabled{},
		now:       time.Now,
	}
}

func (s *service) SetMailer(m mailer.Mailer) {
	s.mail = m
}

func (s *service) Register(context context.Context, input *RegisterInput) error {
	if input == nil {
		return app.ErrInvalidInput
	}

	name, err := helpers.NormalizeName(input.Name)
	if err != nil {
		return err
	}

	email, err := helpers.NormalizeEmail(input.Email)
	if err != nil {
		return err
	}

	phone, err := helpers.NormalizePhone(input.Phone)
	if err != nil {
		return err
	}

	if err := helpers.ValidatePassword(input.Password); err != nil {
		return err
	}

	hashedPassword, err := hashPassword(input.Password)
	if err != nil {
		return err
	}

	ue := &UserEntity{
		Name:         name,
		Email:        email,
		Phone:        phone,
		PasswordHash: hashedPassword,
	}

	return s.r.CreateUser(context, ue)
}

func (s *service) Login(context context.Context, input *LoginInput) (TokenPair, error) {
	if input == nil || input.Email == "" || input.Password == "" {
		return TokenPair{}, app.ErrInvalidInput
	}

	email, err := helpers.NormalizeEmail(input.Email)
	if err != nil {
		return TokenPair{}, app.ErrInvalidCredentials
	}

	ue, err := s.r.GetUserByEmail(context, email)
	if err != nil {
		if errors.Is(err, app.ErrUserNotFound) {
			passwordMatches(dummyPasswordHash, input.Password)
			return TokenPair{}, app.ErrInvalidCredentials
		}

		return TokenPair{}, err
	}

	if !passwordMatches(ue.PasswordHash, input.Password) {
		return TokenPair{}, app.ErrInvalidCredentials
	}

	tokenPair, err := s.createTokens(ue.ID)
	if err != nil {
		slog.ErrorContext(context, "could not create tokens", "user_id", ue.ID, "error", err)
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

	if savedRefreshToken.UserID != userID {
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

func (s *service) ForgotPassword(ctx context.Context, emailInput string) error {
	email, err := helpers.NormalizeEmail(emailInput)
	if err != nil {
		return err
	}

	ue, err := s.r.GetUserByEmail(ctx, email)
	if errors.Is(err, app.ErrUserNotFound) {
		return nil
	}
	if err != nil {
		return err
	}

	now := s.now()
	last, err := s.r.LastPasswordResetAt(ctx, ue.ID)
	if err != nil {
		return err
	}
	if last != nil && now.Sub(*last) < ResetResendInterval {
		return nil
	}

	code, err := newResetCode()
	if err != nil {
		return err
	}
	codeHash, err := s.hashResetCode(ue.ID, code)
	if err != nil {
		return err
	}

	reset := &PasswordResetEntity{UserID: ue.ID, CodeHash: codeHash, ExpiresAt: now.Add(ResetCodeLifetime)}
	if err := s.r.CreatePasswordReset(ctx, reset); err != nil {
		return err
	}

	message := resetCodeMessage(ue, code)
	sendCtx := context.WithoutCancel(ctx)
	s.sending.Go(func() {
		if err := s.mail.Send(sendCtx, message); err != nil {
			slog.ErrorContext(sendCtx, "could not send the password reset code", "user_id", ue.ID, "error", err)
		}
	})

	return nil
}

func (s *service) ResetPassword(ctx context.Context, input *ResetPasswordInput) error {
	if input == nil {
		return app.ErrInvalidInput
	}

	email, err := helpers.NormalizeEmail(input.Email)
	if err != nil {
		return err
	}
	if err := helpers.ValidatePassword(input.NewPassword); err != nil {
		return err
	}
	if !resetCodePattern.MatchString(input.Code) {
		return app.ErrInvalidResetCode
	}

	ue, err := s.r.GetUserByEmail(ctx, email)
	if errors.Is(err, app.ErrUserNotFound) {
		return app.ErrInvalidResetCode
	}
	if err != nil {
		return err
	}

	reset, err := s.r.ClaimPasswordResetAttempt(ctx, ue.ID, MaxResetAttempts)
	if err != nil {
		return err
	}
	if !s.resetCodeMatches(reset.CodeHash, ue.ID, input.Code) {
		return app.ErrInvalidResetCode
	}

	passwordHash, err := hashPassword(input.NewPassword)
	if err != nil {
		return err
	}

	return s.r.CompletePasswordReset(ctx, reset.ID, ue.ID, passwordHash)
}

func (s *service) WaitForMail() {
	s.sending.Wait()
}

func resetCodeMessage(ue *UserEntity, code string) mailer.Message {
	return mailer.Message{
		To:      ue.Email,
		Subject: "Seu código para redefinir a senha",
		Body: fmt.Sprintf(
			"Olá, %s.\n\n"+
				"Use o código %s para criar uma nova senha no Solado Certo. "+
				"Ele vale por %d minutos e só pode ser usado uma vez.\n\n"+
				"Se você não pediu a troca de senha, ignore este e-mail: sua senha continua a mesma.\n",
			ue.Name, code, int(ResetCodeLifetime.Minutes()),
		),
	}
}

func newResetCode() (string, error) {
	n, err := rand.Int(rand.Reader, big.NewInt(1_000_000))
	if err != nil {
		return "", err
	}

	return fmt.Sprintf("%0*d", resetCodeDigits, n.Int64()), nil
}

func (s *service) hashResetCode(userID, code string) (string, error) {
	nonce := make([]byte, resetNonceBytes)
	if _, err := rand.Read(nonce); err != nil {
		return "", err
	}

	return hex.EncodeToString(nonce) + "." + hex.EncodeToString(s.resetCodeMAC(nonce, userID, code)), nil
}

func (s *service) resetCodeMatches(codeHash, userID, code string) bool {
	nonceHex, macHex, ok := strings.Cut(codeHash, ".")
	if !ok {
		return false
	}
	nonce, err := hex.DecodeString(nonceHex)
	if err != nil {
		return false
	}
	mac, err := hex.DecodeString(macHex)
	if err != nil {
		return false
	}

	return hmac.Equal(mac, s.resetCodeMAC(nonce, userID, code))
}

func (s *service) resetCodeMAC(nonce []byte, userID, code string) []byte {
	h := hmac.New(sha256.New, s.secretKey)
	h.Write(nonce)
	h.Write([]byte(userID + ":" + code))
	return h.Sum(nil)
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
		UserID:     userID,
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
