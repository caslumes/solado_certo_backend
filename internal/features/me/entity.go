package me

type ProfileEntity struct {
	ID      string
	Name  string
	Email string
	Phone string
	AvatarURL string
}

func (p *ProfileEntity) ToOutput() *ProfileOutput {
	return &ProfileOutput{
		Name: p.Name,
		Email: p.Email,
		Phone: p.Phone,
		AvatarURL: p.AvatarURL,
	}
}