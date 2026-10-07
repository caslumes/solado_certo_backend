package me

import (
	"context"
	"errors"
	"testing"

	"github.com/caslumes/solado_certo_backend/internal/app"
	"github.com/caslumes/solado_certo_backend/internal/app/enum"
)

const testUserID = "user-1"

var testOnboardingSteps = []OnboardingStepEntity{
	{ID: 1, Tag: "welcome", Order: 1, IsRequired: true},
	{ID: 2, Tag: "profile_setup", Order: 2, IsRequired: true},
	{ID: 3, Tag: "address_setup", Order: 3, IsRequired: false},
	{ID: 4, Tag: "podological_profile", Order: 4, IsRequired: false},
	{ID: 5, Tag: "completion", Order: 5, IsRequired: true},
}

type fakeRepository struct {
	repository
	onboardings map[string]*OnboardingEntity
	profiles    map[string]*ProfileEntity
}

func newFakeRepository() *fakeRepository {
	return &fakeRepository{
		onboardings: map[string]*OnboardingEntity{},
		profiles:    map[string]*ProfileEntity{testUserID: {ID: testUserID, Status: enum.ProfileStatusPending}},
	}
}

func (f *fakeRepository) GetProfileByUserID(_ context.Context, userID string) (*ProfileEntity, error) {
	profile, ok := f.profiles[userID]
	if !ok {
		return nil, app.ErrUserNotFound
	}
	copied := *profile
	return &copied, nil
}

func (f *fakeRepository) UpdateProfile(_ context.Context, profile *ProfileEntity) error {
	copied := *profile
	f.profiles[profile.ID] = &copied
	return nil
}

func (f *fakeRepository) CreateOnboarding(_ context.Context, onboarding *OnboardingEntity) error {
	copied := *onboarding
	copied.CurrentStep = testOnboardingSteps[0]
	f.onboardings[onboarding.UserID] = &copied
	return nil
}

func (f *fakeRepository) GetOnboardingByUserID(_ context.Context, userID string) (*OnboardingEntity, error) {
	onboarding, ok := f.onboardings[userID]
	if !ok {
		return nil, app.ErrOnboardingNotFound
	}
	copied := *onboarding
	return &copied, nil
}

func (f *fakeRepository) GetOnboardingStepByTag(_ context.Context, tag string) (*OnboardingStepEntity, error) {
	for _, step := range testOnboardingSteps {
		if step.Tag == tag {
			return &step, nil
		}
	}
	return nil, app.ErrOnboardingStepNotFound
}

func (f *fakeRepository) GetOnboardingStepByOrder(_ context.Context, order int) (*OnboardingStepEntity, error) {
	for _, step := range testOnboardingSteps {
		if step.Order == order {
			return &step, nil
		}
	}
	return nil, app.ErrOnboardingStepNotFound
}

func (f *fakeRepository) UpdateOnboarding(_ context.Context, userID string, onboarding *OnboardingEntity) error {
	copied := *onboarding
	f.onboardings[userID] = &copied
	return nil
}

func startOnboarding(t *testing.T) (*service, *fakeRepository) {
	t.Helper()
	repository := newFakeRepository()
	s := NewService(repository)

	onboarding, err := s.GetOnboardingByUserID(context.Background(), testUserID)
	if err != nil {
		t.Fatalf("GetOnboardingByUserID: %v", err)
	}
	if onboarding.Status != OnboardingStatusNotStarted {
		t.Fatalf("status = %s, want %s", onboarding.Status, OnboardingStatusNotStarted)
	}

	return s, repository
}

func changeStep(t *testing.T, s *service, step string, action OnboardingAction) {
	t.Helper()
	err := s.ChangeOnboardingStep(context.Background(), testUserID, OnboardingInput{Step: step, Action: action.String()})
	if err != nil {
		t.Fatalf("ChangeOnboardingStep(%s, %s): %v", step, action, err)
	}
}

func TestOnboardingStatusGoesFromNotStartedToInProgressToCompleted(t *testing.T) {
	s, repository := startOnboarding(t)

	changeStep(t, s, "welcome", OnboardingActionAdvance)
	onboarding := repository.onboardings[testUserID]
	if onboarding.Status != OnboardingStatusInProgress {
		t.Fatalf("status after advance = %s, want %s", onboarding.Status, OnboardingStatusInProgress)
	}
	if onboarding.CurrentStep.Tag != "profile_setup" {
		t.Fatalf("current step = %s, want profile_setup", onboarding.CurrentStep.Tag)
	}

	changeStep(t, s, "profile_setup", OnboardingActionAdvance)
	changeStep(t, s, "address_setup", OnboardingActionSkip)
	changeStep(t, s, "podological_profile", OnboardingActionSkip)
	if got := repository.onboardings[testUserID].Status; got != OnboardingStatusInProgress {
		t.Fatalf("status after skips = %s, want %s", got, OnboardingStatusInProgress)
	}

	changeStep(t, s, "completion", OnboardingActionComplete)
	onboarding = repository.onboardings[testUserID]
	if onboarding.Status != OnboardingStatusCompleted {
		t.Fatalf("status after complete = %s, want %s", onboarding.Status, OnboardingStatusCompleted)
	}
	if onboarding.CompletedAt == nil {
		t.Fatal("completed_at was not set")
	}
	if got := repository.profiles[testUserID].Status; got != enum.ProfileStatusActive {
		t.Fatalf("profile status = %s, want %s", got, enum.ProfileStatusActive)
	}
}

func TestOnboardingDismissSetsDismissedStatus(t *testing.T) {
	s, repository := startOnboarding(t)

	changeStep(t, s, "welcome", OnboardingActionDismiss)

	onboarding := repository.onboardings[testUserID]
	if onboarding.Status != OnboardingStatusDismissed {
		t.Fatalf("status = %s, want %s", onboarding.Status, OnboardingStatusDismissed)
	}
	if onboarding.DismissedAt == nil {
		t.Fatal("dismissed_at was not set")
	}
}

func TestOnboardingResumesAfterDismiss(t *testing.T) {
	s, repository := startOnboarding(t)

	changeStep(t, s, "welcome", OnboardingActionDismiss)
	changeStep(t, s, "welcome", OnboardingActionAdvance)

	if got := repository.onboardings[testUserID].Status; got != OnboardingStatusInProgress {
		t.Fatalf("status = %s, want %s", got, OnboardingStatusInProgress)
	}
}

func TestOnboardingRejectsChangesAfterCompletion(t *testing.T) {
	s, _ := startOnboarding(t)
	changeStep(t, s, "welcome", OnboardingActionComplete)

	err := s.ChangeOnboardingStep(context.Background(), testUserID, OnboardingInput{Step: "welcome", Action: "advance"})
	if !errors.Is(err, app.ErrOnboardingAlreadyCompleted) {
		t.Fatalf("err = %v, want %v", err, app.ErrOnboardingAlreadyCompleted)
	}
}

func TestOnboardingRejectsSkippingRequiredStep(t *testing.T) {
	s, repository := startOnboarding(t)

	err := s.ChangeOnboardingStep(context.Background(), testUserID, OnboardingInput{Step: "profile_setup", Action: "skip"})
	if err == nil {
		t.Fatal("skipping a required step succeeded")
	}
	if got := repository.onboardings[testUserID].Status; got != OnboardingStatusNotStarted {
		t.Fatalf("status = %s, want %s", got, OnboardingStatusNotStarted)
	}
}
