package me

import (
	"time"

	"github.com/caslumes/solado_certo_backend/internal/app/enum"
)

type ProfileEntity struct {
	ID        string
	Name      string
	Email     string
	Phone     string
	AvatarURL string
	Status    enum.ProfileStatus
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

type PodologicalProfileEntity struct {
	UserID            string
	FootstrikeType    string
	ClinicalCondition string
	Obs               *string
	UpdatedAt         time.Time
}

func (p *PodologicalProfileEntity) ToOutput() *PodologicalProfileOutput {
	return &PodologicalProfileOutput{
		FootstrikeType:    NullIfZeroValueString(p.FootstrikeType),
		ClinicalCondition: NullIfZeroValueString(p.ClinicalCondition),
		Obs:               p.Obs,
		UpdatedAt:         NullIfZeroValueTime(p.UpdatedAt),
	}
}

type OnboardingEntity struct {
	UserID      string
	Status      OnboardingStatus
	CurrentStep OnboardingStepEntity
	CompletedAt *time.Time
	DismissedAt *time.Time
	CreatedAt   time.Time
	UpdatedAt   time.Time
}

type OnboardingStepEntity struct {
	ID         uint64
	Tag        string
	Name       string
	Order      int
	IsRequired bool
}

func (o *OnboardingEntity) ToOutput() *OnboardingOutput {
	return &OnboardingOutput{
		Status:      o.Status.String(),
		CurrentStep: &o.CurrentStep.Tag,
	}
}

func NullIfZeroValueString(s string) *string {
	if s == "" {
		return nil
	}
	return &s
}

func NullIfZeroValueTime(t time.Time) *time.Time {
	if t.IsZero() {
		return nil
	}
	return &t
}
