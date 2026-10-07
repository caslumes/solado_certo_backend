package auth

import "time"

type UserEntity struct {
	ID           string
	Name         string
	Email        string
	Phone        string
	PasswordHash string
	AvatarURL    string
	Status       string
	CreatedAt    *time.Time
	UpdatedAt    *time.Time
	DeletedAt    *time.Time
}

type RefreshTokenEntity struct {
	ID         string
	UserID     string
	TokenHash  string
	DeviceName string
	ExpiresAt  *time.Time
	RevokedAt  *time.Time
	CreatedAt  *time.Time
}

func (r *RefreshTokenEntity) IsRevoked() bool {
	return r.RevokedAt != nil
}

func (r *RefreshTokenEntity) IsExpired() bool {
	if r.ExpiresAt == nil {
		return false
	}

	return r.ExpiresAt.Before(time.Now())
}
