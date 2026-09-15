package me

type ProfileInput struct {
	Name      string
	Email     string
	Phone     string
	AvatarURL string
}

type AddressInput struct {
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
