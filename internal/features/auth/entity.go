package auth

type UserEntity struct {
	ID        string
	Nome      string
	Email     string
	Telefone  string
	SenhaHash string
	AvatarURL string
	Status    string
	CreatedAt string
	UpdatedAt string
	DeletedAt string
}

type RefreshTokenEntity struct {
	ID         string
	UsuarioID  string
	TokenHash  string
	DeviceName string
	ExpiresAt  string
	RevokedAt  string
	CreatedAt  string
}
