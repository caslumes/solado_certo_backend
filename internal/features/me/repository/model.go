package repository

import (
	"time"

	"github.com/caslumes/solado_certo_backend/internal/features/me"
)

type ProfileModel struct {
	ID        string
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

type AddressModel struct {
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

func (a *AddressModel) ToEntity() *me.AddressEntity {
	return &me.AddressEntity{
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

type PodologicalProfileModel struct {
	UserID            string
	FootstrikeType    string
	ClinicalCondition string
	Obs               *string
	UpdatedAt         time.Time
}

func (p *PodologicalProfileModel) ToEntity() *me.PodologicalProfileEntity {
	return &me.PodologicalProfileEntity{
		UserID:            p.UserID,
		FootstrikeType:    p.FootstrikeType,
		ClinicalCondition: p.ClinicalCondition,
		Obs:               p.Obs,
		UpdatedAt:         p.UpdatedAt,
	}
}

type OnboardingStepModel struct {
	ID         uint64
	Tag        string
	Name       string
	Order      int
	IsRequired bool
}

func (s *OnboardingStepModel) ToEntity() *me.OnboardingStepEntity {
	return &me.OnboardingStepEntity{
		ID:         s.ID,
		Tag:        s.Tag,
		Name:       s.Name,
		Order:      s.Order,
		IsRequired: s.IsRequired,
	}
}

type OnboardingModel struct {
	UserID      string
	Status      string
	CurrentStep OnboardingStepModel
	CompletedAt *time.Time
	CreatedAt   time.Time
	UpdatedAt   time.Time
}

func (o *OnboardingModel) ToEntity() *me.OnboardingEntity {
	return &me.OnboardingEntity{
		UserID:      o.UserID,
		Status:      me.OnboardingStatus(o.Status),
		CurrentStep: *o.CurrentStep.ToEntity(),
		CompletedAt: o.CompletedAt,
		CreatedAt:   o.CreatedAt,
		UpdatedAt:   o.UpdatedAt,
	}
}
