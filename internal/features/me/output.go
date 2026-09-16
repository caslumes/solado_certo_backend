package me

import "time"

type ProfileOutput struct {
	Name      string `json:"name"`
	Email     string `json:"email"`
	Phone     string `json:"phone"`
	AvatarURL string `json:"avatar_url"`
}

type AddressOutput struct {
	ID         string  `json:"id"`
	UserID     string  `json:"user_id"`
	Label      *string `json:"label"`
	Receiver   string  `json:"receiver"`
	ZIPCode    string  `json:"zip_code"`
	Street     string  `json:"street"`
	Number     string  `json:"number"`
	Complement *string `json:"complement"`
	District   string  `json:"district"`
	City       string  `json:"city"`
	State      string  `json:"state"`
	Country    string  `json:"country"`
	IsDefault  bool    `json:"is_default"`
}

type AddressesOutput []AddressOutput

func (a *AddressesOutput) FromEntities(entities []*AddressEntity) []AddressOutput {
	outputs := make([]AddressOutput, 0, len(entities))
	for _, e := range entities {
		outputs = append(outputs, *e.ToOutput())
	}
	return outputs
}

type PodologicalProfileOutput struct {
	FootstrikeType    *string    `json:"footstrike_type"`
	ClinicalCondition *string    `json:"clinical_condition"`
	Obs               *string    `json:"obs"`
	UpdatedAt         *time.Time `json:"updated_at"`
}
