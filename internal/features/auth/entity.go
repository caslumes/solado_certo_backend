package auth

import "time"

type UserEntity struct {
	ID        string
	Nome      string
	Email     string
	Telefone  string
	SenhaHash string
	AvatarURL string
	Status    string
	CreatedAt *time.Time
	UpdatedAt *time.Time
	DeletedAt *time.Time
}

type RefreshTokenEntity struct {
	ID         string
	UsuarioID  string
	TokenHash  string
	DeviceName string
	ExpiresAt  *time.Time
	RevokedAt  *time.Time
	CreatedAt  *time.Time
}
