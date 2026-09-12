package repository

import "github.com/caslumes/solado_certo_backend/internal/features/me"

type ProfileModel struct {
	ID 	  	  string
	Name      string
	Email     string
	Phone     string
	AvatarURL *string
}

func (p *ProfileModel) ToEntity() *me.ProfileEntity {
	avatarURL := ""
	if p.AvatarURL != nil {
		avatarURL = *p.AvatarURL
	}

	return &me.ProfileEntity{
		ID:        p.ID,
		Name:      p.Name,
		Email:     p.Email,
		Phone:     p.Phone,
		AvatarURL: avatarURL,
	}
}