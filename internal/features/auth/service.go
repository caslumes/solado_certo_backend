package auth

import "context"

type repository interface {
	CreateUser(context context.Context, ue *UserEntity) error
}

type RegisterUseCase interface {
	Register(context context.Context, input *RegisterInput) error
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
		return ErrInvalidInput
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

func hashPassword(password string) (string, error) {
	// TODO: implement password hashing
	return password, nil
}
