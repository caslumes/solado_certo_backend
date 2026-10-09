package me

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"strings"
	"time"
	"unicode/utf8"

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

type GetPainPointsUseCase interface {
	GetPainPoints(context context.Context) ([]*PainPointEntity, error)
}

type GetConsentsUseCase interface {
	GetConsents(context context.Context, userID string) ([]*ConsentEntity, error)
}

type GrantConsentUseCase interface {
	GrantConsent(context context.Context, userID string, input ConsentInput) (consent *ConsentEntity, created bool, err error)
}

type RevokeConsentUseCase interface {
	RevokeConsent(context context.Context, userID string, purpose string) error
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
	SavePodologicalProfile(context context.Context, profile *PodologicalProfileEntity, painPointIDs []string) error
	GetPainPoints(context context.Context) ([]*PainPointEntity, error)
	CountPainPoints(context context.Context, ids []string) (int, error)
	GetActiveConsents(context context.Context, userID string) ([]*ConsentEntity, error)
	GetActiveConsent(context context.Context, userID string, purpose ConsentPurpose) (*ConsentEntity, error)
	GrantConsent(context context.Context, consent *ConsentEntity) (created bool, err error)
	RevokeConsent(context context.Context, userID string, purpose ConsentPurpose) error
	RevokeConsentAndDeletePodologicalProfile(context context.Context, userID string) error
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

const (
	maxClinicalConditionLength = 120
	maxPodologicalNotesLength  = 1000
)

func (s *service) EditPodologicalProfile(context context.Context, userID string, input PodologicalProfileInput) (*PodologicalProfileEntity, error) {
	if err := s.requireConsent(context, userID, ConsentPurposePodologicalProfile); err != nil {
		return nil, err
	}

	profile, painPointIDs, err := s.validatePodologicalProfile(context, userID, input)
	if err != nil {
		return nil, err
	}

	err = s.r.SavePodologicalProfile(context, profile, painPointIDs)
	if err != nil {
		slog.ErrorContext(context, "EditPodologicalProfile failed", "user_id", userID, "error", err)
		return nil, err
	}

	saved, err := s.r.GetPodologicalProfileByUserID(context, userID)
	if err != nil {
		slog.ErrorContext(context, "EditPodologicalProfile failed", "user_id", userID, "error", err)
		return nil, err
	}

	return saved, nil
}

func (s *service) requireConsent(context context.Context, userID string, purpose ConsentPurpose) error {
	consent, err := s.r.GetActiveConsent(context, userID, purpose)
	if err != nil {
		if errors.Is(err, app.ErrConsentNotFound) {
			return app.ErrConsentRequired
		}

		slog.ErrorContext(context, "requireConsent failed", "user_id", userID, "purpose", purpose, "error", err)
		return err
	}

	if consent.TermsVersion != currentTermsVersions[purpose] {
		return app.ErrConsentRequired
	}

	return nil
}

func (s *service) validatePodologicalProfile(context context.Context, userID string, input PodologicalProfileInput) (*PodologicalProfileEntity, []string, error) {
	footstrikeType := FootstrikeType(strings.TrimSpace(input.FootstrikeType))
	if !footstrikeType.IsValid() {
		return nil, nil, fmt.Errorf("%w: invalid footstrike type", app.ErrInvalidInput)
	}

	clinicalCondition := trimToNil(input.ClinicalCondition)
	if clinicalCondition != nil && utf8.RuneCountInString(*clinicalCondition) > maxClinicalConditionLength {
		return nil, nil, fmt.Errorf("%w: clinical condition is too long", app.ErrInvalidInput)
	}

	notes := trimToNil(input.Obs)
	if notes != nil && utf8.RuneCountInString(*notes) > maxPodologicalNotesLength {
		return nil, nil, fmt.Errorf("%w: notes are too long", app.ErrInvalidInput)
	}

	painPointIDs, err := s.validatePainPointIDs(context, input.PainPointIDs)
	if err != nil {
		return nil, nil, err
	}

	return &PodologicalProfileEntity{
		UserID:            userID,
		FootstrikeType:    string(footstrikeType),
		ClinicalCondition: clinicalCondition,
		Obs:               notes,
	}, painPointIDs, nil
}

func (s *service) validatePainPointIDs(context context.Context, ids []string) ([]string, error) {
	unique := make([]string, 0, len(ids))
	seen := make(map[string]bool, len(ids))
	for _, id := range ids {
		id = strings.ToLower(strings.TrimSpace(id))
		if !helpers.IsUUID(id) {
			return nil, fmt.Errorf("%w: invalid pain point id", app.ErrInvalidInput)
		}
		if !seen[id] {
			seen[id] = true
			unique = append(unique, id)
		}
	}

	if len(unique) == 0 {
		return unique, nil
	}

	count, err := s.r.CountPainPoints(context, unique)
	if err != nil {
		slog.ErrorContext(context, "validatePainPointIDs failed", "error", err)
		return nil, err
	}
	if count != len(unique) {
		return nil, fmt.Errorf("%w: unknown pain point id", app.ErrInvalidInput)
	}

	return unique, nil
}

func trimToNil(s *string) *string {
	if s == nil {
		return nil
	}
	trimmed := strings.TrimSpace(*s)
	if trimmed == "" {
		return nil
	}
	return &trimmed
}

func (s *service) GetPainPoints(context context.Context) ([]*PainPointEntity, error) {
	painPoints, err := s.r.GetPainPoints(context)
	if err != nil {
		slog.ErrorContext(context, "GetPainPoints failed", "error", err)
		return nil, err
	}

	return painPoints, nil
}

func (s *service) GetConsents(context context.Context, userID string) ([]*ConsentEntity, error) {
	consents, err := s.r.GetActiveConsents(context, userID)
	if err != nil {
		slog.ErrorContext(context, "GetConsents failed", "user_id", userID, "error", err)
		return nil, err
	}

	return consents, nil
}

func (s *service) GrantConsent(context context.Context, userID string, input ConsentInput) (*ConsentEntity, bool, error) {
	purpose := ConsentPurpose(input.Purpose)
	termsVersion, ok := currentTermsVersions[purpose]
	if !ok {
		return nil, false, fmt.Errorf("%w: unknown consent purpose", app.ErrInvalidInput)
	}
	if input.TermsVersion != termsVersion {
		return nil, false, fmt.Errorf("%w: terms version is not the current one", app.ErrInvalidInput)
	}

	consent := &ConsentEntity{
		UserID:       userID,
		Purpose:      purpose,
		TermsVersion: termsVersion,
	}
	created, err := s.r.GrantConsent(context, consent)
	if err != nil {
		slog.ErrorContext(context, "GrantConsent failed", "user_id", userID, "purpose", purpose, "error", err)
		return nil, false, err
	}

	return consent, created, nil
}

func (s *service) RevokeConsent(context context.Context, userID string, purpose string) error {
	consentPurpose := ConsentPurpose(purpose)

	var err error
	switch consentPurpose {
	case ConsentPurposePodologicalProfile:
		err = s.r.RevokeConsentAndDeletePodologicalProfile(context, userID)
	case ConsentPurposePrescription:
		err = s.r.RevokeConsent(context, userID, consentPurpose)
	default:
		return fmt.Errorf("%w: unknown consent purpose", app.ErrInvalidInput)
	}
	if err != nil {
		slog.ErrorContext(context, "RevokeConsent failed", "user_id", userID, "purpose", purpose, "error", err)
		return err
	}

	return nil
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

	action := OnboardingAction(input.Action)
	if onboarding.Status == OnboardingStatusCompleted {
		if action == OnboardingActionComplete {
			return nil
		}
		return app.ErrOnboardingAlreadyCompleted
	}

	step, err := s.r.GetOnboardingStepByTag(context, input.Step)
	if err != nil {
		slog.ErrorContext(context, "ChangeOnboardingStep failed", "user_id", userID, "error", err)
		return err
	}

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

	nextStep, err := s.resolveNextOnboardingStep(context, step, action)
	if err != nil {
		slog.ErrorContext(context, "ChangeOnboardingStep failed", "user_id", userID, "error", err)
		return err
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
			return fmt.Errorf("%w: step is required and cannot be skipped", app.ErrInvalidInput)
		}
	default:
		return fmt.Errorf("%w: invalid onboarding action: %s", app.ErrInvalidInput, action)
	}

	return nil
}

func (s *service) resolveNextOnboardingStep(context context.Context, currentStep *OnboardingStepEntity, action OnboardingAction) (*OnboardingStepEntity, error) {
	var order int
	switch action {
	case OnboardingActionAdvance, OnboardingActionSkip:
		order = currentStep.Order + 1
	case OnboardingActionRetreat:
		order = currentStep.Order - 1
	default:
		return nil, fmt.Errorf("%w: invalid onboarding action: %s", app.ErrInvalidInput, action)
	}

	nextStep, err := s.r.GetOnboardingStepByOrder(context, order)
	if err != nil {
		if errors.Is(err, app.ErrOnboardingStepNotFound) {
			return nil, fmt.Errorf("%w: no onboarding step to %s to from %s", app.ErrInvalidInput, action, currentStep.Tag)
		}
		return nil, err
	}

	return nextStep, nil
}
