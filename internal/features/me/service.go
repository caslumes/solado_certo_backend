package me

import (
	"context"
	"fmt"
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

type repository interface {
	GetProfileByUserID(context context.Context, userID string) (*ProfileEntity, error)
	UpdateProfile(context context.Context, profile *ProfileEntity) error
	GetAddressesByUserID(context context.Context, userID string) ([]*AddressEntity, error)
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
