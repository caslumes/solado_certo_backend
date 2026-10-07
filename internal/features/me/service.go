package me

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"time"

	"github.com/caslumes/solado_certo_backend/internal/app"
	"github.com/caslumes/solado_certo_backend/internal/app/enum"
	"github.com/caslumes/solado_certo_backend/internal/helpers"
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

type GetOnboardingUseCase interface {
	GetOnboardingByUserID(context context.Context, userID string) (*OnboardingEntity, error)
}

type ChangeOnboardingStepUseCase interface {
	ChangeOnboardingStep(context context.Context, userID string, input OnboardingInput) error
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
	CreateOnboarding(context context.Context, onboarding *OnboardingEntity) error
	GetOnboardingByUserID(context context.Context, userID string) (*OnboardingEntity, error)
	GetOnboardingStepByTag(context context.Context, tag string) (*OnboardingStepEntity, error)
	GetOnboardingStepByOrder(context context.Context, order int) (*OnboardingStepEntity, error)
	UpdateOnboarding(context context.Context, userID string, onboarding *OnboardingEntity) error
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
		slog.ErrorContext(context, "GetProfileByUserID failed", "user_id", userID, "error", err)
		return nil, err
	}

	return profile, nil
}

func (s *service) EditProfile(context context.Context, userID string, input ProfileInput) (*ProfileEntity, error) {
	profile, err := s.r.GetProfileByUserID(context, userID)
	if err != nil {
		slog.ErrorContext(context, "EditProfile failed", "user_id", userID, "error", err)
		return nil, err
	}

	name, err := helpers.NormalizeName(input.Name)
	if err != nil {
		return nil, err
	}

	email, err := helpers.NormalizeEmail(input.Email)
	if err != nil {
		return nil, err
	}

	phone, err := helpers.NormalizePhone(input.Phone)
	if err != nil {
		return nil, err
	}

	profile.ID = userID
	profile.Name = name
	profile.Email = email
	profile.Phone = phone
	profile.AvatarURL = input.AvatarURL

	err = s.r.UpdateProfile(context, profile)
	if err != nil {
		slog.ErrorContext(context, "EditProfile failed", "user_id", userID, "error", err)
		return nil, err
	}

	return profile, nil
}

func (s *service) GetAddressesByUserID(context context.Context, userID string) ([]*AddressEntity, error) {
	addresses, err := s.r.GetAddressesByUserID(context, userID)
	if err != nil {
		slog.ErrorContext(context, "GetAddressesByUserID failed", "user_id", userID, "error", err)
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
		slog.ErrorContext(context, "AddAddress failed", "user_id", userID, "error", err)
		return nil, err
	}

	return newAddress, nil
}

func (s *service) EditAddress(context context.Context, userID string, addressID string, input AddressInput) (*AddressEntity, error) {
	address, err := s.r.GetAddressByID(context, addressID)
	if err != nil {
		slog.ErrorContext(context, "EditAddress failed", "user_id", userID, "error", err)
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
		slog.ErrorContext(context, "EditAddress failed", "user_id", userID, "error", err)
		return nil, err
	}

	return address, nil
}

func (s *service) DeleteAddress(context context.Context, userID string, addressID string) error {
	address, err := s.r.GetAddressByID(context, addressID)
	if err != nil {
		slog.ErrorContext(context, "DeleteAddress failed", "user_id", userID, "error", err)
		return err
	}

	if address.UserID != userID {
		return app.ErrAddressNotFound
	}

	err = s.r.DeleteAddress(context, addressID)
	if err != nil {
		slog.ErrorContext(context, "DeleteAddress failed", "user_id", userID, "error", err)
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

		slog.ErrorContext(context, "GetPodologicalProfileByUserID failed", "user_id", userID, "error", err)
		return nil, err
	}

	return profile, nil
}

func (s *service) EditPodologicalProfile(context context.Context, userID string, input PodologicalProfileInput) (*PodologicalProfileEntity, error) {
	profile, err := s.r.GetPodologicalProfileByUserID(context, userID)
	if err != nil {
		if !errors.Is(err, app.ErrPodologicalProfileNotFound) {
			slog.ErrorContext(context, "EditPodologicalProfile failed", "user_id", userID, "error", err)
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
		slog.ErrorContext(context, "EditPodologicalProfile failed", "user_id", userID, "error", err)
		return nil, err
	}

	return profile, nil
}

func (s *service) GetOnboardingByUserID(context context.Context, userID string) (*OnboardingEntity, error) {
	onboarding, err := s.r.GetOnboardingByUserID(context, userID)
	if err != nil {
		if errors.Is(err, app.ErrOnboardingNotFound) {
			err = s.r.CreateOnboarding(context, &OnboardingEntity{
				UserID: userID,
				Status: OnboardingStatusNotStarted,
			})
			if err != nil {
				slog.ErrorContext(context, "GetOnboardingByUserID failed", "user_id", userID, "error", err)
				return nil, err
			}

			onboarding, err = s.r.GetOnboardingByUserID(context, userID)
			if err != nil {
				slog.ErrorContext(context, "GetOnboardingByUserID failed", "user_id", userID, "error", err)
				return nil, err
			}

			return onboarding, nil
		}
		slog.ErrorContext(context, "GetOnboardingByUserID failed", "user_id", userID, "error", err)
		return nil, err
	}

	return onboarding, nil
}

func (s *service) ChangeOnboardingStep(context context.Context, userID string, input OnboardingInput) error {
	onboarding, err := s.r.GetOnboardingByUserID(context, userID)
	if err != nil {
		slog.ErrorContext(context, "ChangeOnboardingStep failed", "user_id", userID, "error", err)
		return err
	}

	if onboarding.Status == OnboardingStatusCompleted {
		return app.ErrOnboardingAlreadyCompleted
	}

	step, err := s.r.GetOnboardingStepByTag(context, input.Step)
	if err != nil {
		slog.ErrorContext(context, "ChangeOnboardingStep failed", "user_id", userID, "error", err)
		return err
	}

	action := OnboardingAction(input.Action)
	err = s.validateOnboardingAction(action, step)
	if err != nil {
		slog.ErrorContext(context, "ChangeOnboardingStep failed", "user_id", userID, "error", err)
		return err
	}

	if action == OnboardingActionDismiss {
		onboarding.Status = OnboardingStatusDismissed
		onboarding.DismissedAt = helpers.TimePtr(time.Now())
		err = s.r.UpdateOnboarding(context, userID, onboarding)
		if err != nil {
			slog.ErrorContext(context, "ChangeOnboardingStep failed", "user_id", userID, "error", err)
			return err
		}
		return nil
	}

	if action == OnboardingActionComplete {
		onboarding.Status = OnboardingStatusCompleted
		onboarding.CompletedAt = helpers.TimePtr(time.Now())
		err = s.r.UpdateOnboarding(context, userID, onboarding)
		if err != nil {
			slog.ErrorContext(context, "ChangeOnboardingStep failed", "user_id", userID, "error", err)
			return err
		}
		profile, err := s.r.GetProfileByUserID(context, userID)
		if err != nil {
			slog.ErrorContext(context, "ChangeOnboardingStep failed", "user_id", userID, "error", err)
			return err
		}
		profile.Status = enum.ProfileStatusActive
		err = s.r.UpdateProfile(context, profile)
		if err != nil {
			slog.ErrorContext(context, "ChangeOnboardingStep failed", "user_id", userID, "error", err)
			return err
		}
		return nil
	}

	nextStep, err := s.resolveNextOnboardingStep(step, action)
	if err != nil {
		slog.ErrorContext(context, "ChangeOnboardingStep failed", "user_id", userID, "error", err)
		return err
	}

	if nextStep == nil {
		return app.ErrOnboardingAlreadyCompleted
	}

	onboarding.Status = OnboardingStatusInProgress
	onboarding.CurrentStep = *nextStep
	err = s.r.UpdateOnboarding(context, userID, onboarding)
	if err != nil {
		slog.ErrorContext(context, "ChangeOnboardingStep failed", "user_id", userID, "error", err)
		return err
	}

	return nil
}

func (s *service) validateOnboardingAction(action OnboardingAction, step *OnboardingStepEntity) error {
	switch action {
	case OnboardingActionAdvance, OnboardingActionRetreat, OnboardingActionComplete, OnboardingActionDismiss:
		break
	case OnboardingActionSkip:
		if step.IsRequired {
			return fmt.Errorf("step is required and cannot be skipped")
		}
	default:
		return fmt.Errorf("invalid onboarding action: %s", action)
	}

	return nil
}

func (s *service) resolveNextOnboardingStep(currentStep *OnboardingStepEntity, action OnboardingAction) (*OnboardingStepEntity, error) {
	switch action {
	case OnboardingActionAdvance, OnboardingActionSkip:
		nextStep, err := s.r.GetOnboardingStepByOrder(context.Background(), currentStep.Order+1)
		if err != nil {
			return nil, err
		}
		return nextStep, nil
	case OnboardingActionRetreat:
		nextStep, err := s.r.GetOnboardingStepByOrder(context.Background(), currentStep.Order-1)
		if err != nil {
			return nil, err
		}
		return nextStep, nil
	default:
		return nil, fmt.Errorf("invalid onboarding action: %s", action)
	}
}
