package model

import (
	"time"

	"github.com/caslumes/solado_certo_backend/internal/features/auth"
)

type UserModel struct {
	ID        string
	Nome      string
	Email     string
	Telefone  string
	SenhaHash string
	Status    string
	CreatedAt *time.Time
	UpdatedAt *time.Time
	DeletedAt *time.Time
}

func (u *UserModel) ToEntity() auth.UserEntity {
	var ue auth.UserEntity
	ue.ID = u.ID
	ue.Nome = u.Nome
	ue.Email = u.Email
	ue.Telefone = u.Telefone
	ue.SenhaHash = u.SenhaHash
	ue.Status = u.Status
	ue.CreatedAt = u.CreatedAt
	ue.UpdatedAt = u.UpdatedAt
	ue.DeletedAt = u.DeletedAt

	return ue
}

type RefreshTokenModel struct {
	ID         string
	UsuarioID  string
	TokenHash  string
	DeviceName string
	ExpiresAt  *time.Time
	RevokedAt  *time.Time
	CreatedAt  *time.Time
}
