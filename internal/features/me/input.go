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

type PodologicalProfileInput struct {
	FootstrikeType    string
	ClinicalCondition string
	Obs               *string
}

type OnboardingInput struct {
	Step   string
	Action string
}
