package me

import (
	"context"
	"fmt"
)

type GetProfileUseCase interface {
	GetProfileByUserID(context context.Context, userID string) (*ProfileEntity, error)
}

type repository interface {
	GetProfileByUserID(context context.Context, userID string) (*ProfileEntity, error)
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