package model

type UserModel struct {
	ID       string  `json:"id"`
	Nome     string `json:"nome"`
	Email    string `json:"email"`
	Telefone string `json:"telefone"`
	SenhaHash string `json:"senha_hash"`
	Status   string `json:"status"`
	CreatedAt string `json:"created_at"`
	UpdatedAt string `json:"updated_at"`
	DeletedAt string `json:"deleted_at"`
}

type RefreshTokenModel struct {
	ID        string  `json:"id"`
	UsuarioID string  `json:"usuario_id"`
	TokenHash string `json:"token_hash"`
	DeviceName string `json:"device_name"`
	ExpiresAt  string `json:"expires_at"`
	RevokedAt  string `json:"revoked_at"`
	CreatedAt  string `json:"created_at"`
}
