package me

import (
	"context"
	"errors"
	"fmt"

	"github.com/caslumes/solado_certo_backend/internal/app"
)

type GetProfileUseCase interface {
	GetProfileByUserID(context context.Context, userID string) (*ProfileEntity, error)
}

type EditProfileUseCase interface {
	EditProfile(context context.Context, userID string, input ProfileInput) (*ProfileEntity, error)
}

type GetAddressesUseCase interface {
	GetAddressesByUserID(context context.Context, userID string) ([]*AddressEntity, error)
}

type AddAddressUseCase interface {
	AddAddress(context context.Context, userID string, input AddressInput) (*AddressEntity, error)
}

type EditAddressUseCase interface {
	EditAddress(context context.Context, userID string, addressID string, input AddressInput) (*AddressEntity, error)
}

type DeleteAddressUseCase interface {
	DeleteAddress(context context.Context, userID string, addressID string) error
}

type GetPodologicalProfileUseCase interface {
	GetPodologicalProfileByUserID(context context.Context, userID string) (*PodologicalProfileEntity, error)
}

type EditPodologicalProfileUseCase interface {
	EditPodologicalProfile(context context.Context, userID string, input PodologicalProfileInput) (*PodologicalProfileEntity, error)
}

type repository interface {
	GetProfileByUserID(context context.Context, userID string) (*ProfileEntity, error)
	UpdateProfile(context context.Context, profile *ProfileEntity) error
	GetAddressesByUserID(context context.Context, userID string) ([]*AddressEntity, error)
	GetAddressByID(context context.Context, addressID string) (*AddressEntity, error)
	CreateAddress(context context.Context, userID string, address *AddressEntity) (*AddressEntity, error)
	UpdateAddress(context context.Context, address *AddressEntity) error
	DeleteAddress(context context.Context, addressID string) error
	GetPodologicalProfileByUserID(context context.Context, userID string) (*PodologicalProfileEntity, error)
	UpsertPodologicalProfile(context context.Context, profile *PodologicalProfileEntity) error
}

type service struct {
	r repository
}

func NewService(r repository) *service {
	return &service{r: r}
}

func (s *service) GetProfileByUserID(context context.Context, userID string) (*ProfileEntity, error) {
	profile, err := s.r.GetProfileByUserID(context, userID)
	if err != nil {
		fmt.Printf("err %v\n", err)
		return nil, err
	}

	return profile, nil
}

func (s *service) EditProfile(context context.Context, userID string, input ProfileInput) (*ProfileEntity, error) {
	profile, err := s.r.GetProfileByUserID(context, userID)
	if err != nil {
		fmt.Printf("err %v\n", err)
		return nil, err
	}

	profile.ID = userID
	profile.Name = input.Name
	profile.Email = input.Email
	profile.Phone = input.Phone
	profile.AvatarURL = input.AvatarURL

	err = s.r.UpdateProfile(context, profile)
	if err != nil {
		fmt.Printf("err %v\n", err)
		return nil, err
	}

	return profile, nil
}

func (s *service) GetAddressesByUserID(context context.Context, userID string) ([]*AddressEntity, error) {
	addresses, err := s.r.GetAddressesByUserID(context, userID)
	if err != nil {
		fmt.Printf("err %v\n", err)
		return nil, err
	}

	return addresses, nil
}

func (s *service) AddAddress(context context.Context, userID string, input AddressInput) (*AddressEntity, error) {
	address := &AddressEntity{
		UserID:     userID,
		Label:      input.Label,
		Receiver:   input.Receiver,
		ZIPCode:    input.ZIPCode,
		Street:     input.Street,
		Number:     input.Number,
		Complement: input.Complement,
		District:   input.District,
		City:       input.City,
		State:      input.State,
		Country:    input.Country,
		IsDefault:  input.IsDefault,
	}

	newAddress, err := s.r.CreateAddress(context, userID, address)
	if err != nil {
		fmt.Printf("err %v\n", err)
		return nil, err
	}

	return newAddress, nil
}

func (s *service) EditAddress(context context.Context, userID string, addressID string, input AddressInput) (*AddressEntity, error) {
	address, err := s.r.GetAddressByID(context, addressID)
	if err != nil {
		fmt.Printf("err %v\n", err)
		return nil, err
	}

	if address.UserID != userID {
		return nil, app.ErrAddressNotFound
	}

	address.Label = input.Label
	address.Receiver = input.Receiver
	address.ZIPCode = input.ZIPCode
	address.Street = input.Street
	address.Number = input.Number
	address.Complement = input.Complement
	address.District = input.District
	address.City = input.City
	address.State = input.State
	address.Country = input.Country
	address.IsDefault = input.IsDefault

	err = s.r.UpdateAddress(context, address)
	if err != nil {
		fmt.Printf("err %v\n", err)
		return nil, err
	}

	return address, nil
}

func (s *service) DeleteAddress(context context.Context, userID string, addressID string) error {
	address, err := s.r.GetAddressByID(context, addressID)
	if err != nil {
		fmt.Printf("err %v\n", err)
		return err
	}

	if address.UserID != userID {
		return app.ErrAddressNotFound
	}

	err = s.r.DeleteAddress(context, addressID)
	if err != nil {
		fmt.Printf("err %v\n", err)
		return err
	}

	return nil
}

func (s *service) GetPodologicalProfileByUserID(context context.Context, userID string) (*PodologicalProfileEntity, error) {
	profile, err := s.r.GetPodologicalProfileByUserID(context, userID)
	if err != nil {
		if errors.Is(err, app.ErrPodologicalProfileNotFound) {
			return &PodologicalProfileEntity{}, nil
		}

		fmt.Printf("err %v\n", err)
		return nil, err
	}

	return profile, nil
}

func (s *service) EditPodologicalProfile(context context.Context, userID string, input PodologicalProfileInput) (*PodologicalProfileEntity, error) {
	profile, err := s.r.GetPodologicalProfileByUserID(context, userID)
	if err != nil {
		if !errors.Is(err, app.ErrPodologicalProfileNotFound) {
			fmt.Printf("err %v\n", err)
			return nil, err
		}

		profile = &PodologicalProfileEntity{
			UserID: userID,
		}
	}

	profile.FootstrikeType = input.FootstrikeType
	profile.ClinicalCondition = input.ClinicalCondition
	profile.Obs = input.Obs

	err = s.r.UpsertPodologicalProfile(context, profile)
	if err != nil {
		fmt.Printf("err %v\n", err)
		return nil, err
	}

	return profile, nil
}
