package me

type ProfileEntity struct {
	ID        string
	Name      string
	Email     string
	Phone     string
	AvatarURL string
}

func (p *ProfileEntity) ToOutput() *ProfileOutput {
	return &ProfileOutput{
		Name:      p.Name,
		Email:     p.Email,
		Phone:     p.Phone,
		AvatarURL: p.AvatarURL,
	}
}

type AddressEntity struct {
	ID         string
	UserID     string
	Label      *string
	Receiver   string
	ZIPCode    string
	Street     string
	Number     string
	Complement *string
	District   string
	City       string
	State      string
	Country    string
	IsDefault  bool
}

func (a *AddressEntity) ToOutput() *AddressOutput {
	return &AddressOutput{
		ID:         a.ID,
		UserID:     a.UserID,
		Label:      a.Label,
		Receiver:   a.Receiver,
		ZIPCode:    a.ZIPCode,
		Street:     a.Street,
		Number:     a.Number,
		Complement: a.Complement,
		District:   a.District,
		City:       a.City,
		State:      a.State,
		Country:    a.Country,
		IsDefault:  a.IsDefault,
	}
}
