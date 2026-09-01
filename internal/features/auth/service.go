package auth

import (
	"context"
	"errors"

	"github.com/caslumes/solado_certo_backend/internal/app"
	"golang.org/x/crypto/bcrypt"
)

type repository interface {
	CreateUser(context context.Context, ue *UserEntity) error
	GetUserByEmail(context context.Context, email string) (*UserEntity, error)
}

type RegisterUseCase interface {
	Register(context context.Context, input *RegisterInput) error
}

type LoginUseCase interface {
	Login(context context.Context, input *LoginInput) error
}

type service struct {
	r repository
}

func NewService(r repository) *service {
	return &service{
		r: r,
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

func (s *service) Login(context context.Context, input *LoginInput) error {
	if input == nil || input.Email == "" || input.Password == "" {
		return app.ErrInvalidInput
	}

	ue, err := s.r.GetUserByEmail(context, input.Email)
	if err != nil {
		if errors.Is(err, app.ErrUserNotFound) {
			return app.ErrInvalidCredentials
		}

		return err
	}

	if !passwordMatches(ue.SenhaHash, input.Password) {
		return app.ErrInvalidCredentials
	}

	return nil
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
