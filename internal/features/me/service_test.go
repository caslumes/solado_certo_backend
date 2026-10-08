package me

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/caslumes/solado_certo_backend/internal/app"
	"github.com/caslumes/solado_certo_backend/internal/app/enum"
	"github.com/caslumes/solado_certo_backend/internal/helpers"
)

const testUserID = "user-1"

var testOnboardingSteps = []OnboardingStepEntity{
	{ID: 1, Tag: "welcome", Order: 1, IsRequired: true},
	{ID: 2, Tag: "profile_setup", Order: 2, IsRequired: true},
	{ID: 3, Tag: "address_setup", Order: 3, IsRequired: false},
	{ID: 4, Tag: "podological_profile", Order: 4, IsRequired: false},
	{ID: 5, Tag: "completion", Order: 5, IsRequired: true},
}

const (
	painPointHeel = "00000000-0000-0000-0000-000000000701"
	painPointArch = "00000000-0000-0000-0000-000000000702"
)

type fakeRepository struct {
	repository
	onboardings         map[string]*OnboardingEntity
	profiles            map[string]*ProfileEntity
	podologicalProfiles map[string]*PodologicalProfileEntity
	painPoints          map[string]PainPointEntity
	consents            []*ConsentEntity
}

func newFakeRepository() *fakeRepository {
	return &fakeRepository{
		onboardings:         map[string]*OnboardingEntity{},
		profiles:            map[string]*ProfileEntity{testUserID: {ID: testUserID, Status: enum.ProfileStatusPending}},
		podologicalProfiles: map[string]*PodologicalProfileEntity{},
		painPoints: map[string]PainPointEntity{
			painPointHeel: {ID: painPointHeel, Name: "Calcanhar"},
			painPointArch: {ID: painPointArch, Name: "Arco plantar"},
		},
	}
}

func (f *fakeRepository) GetPodologicalProfileByUserID(_ context.Context, userID string) (*PodologicalProfileEntity, error) {
	profile, ok := f.podologicalProfiles[userID]
	if !ok {
		return nil, app.ErrPodologicalProfileNotFound
	}
	copied := *profile
	return &copied, nil
}

func (f *fakeRepository) SavePodologicalProfile(_ context.Context, profile *PodologicalProfileEntity, painPointIDs []string) error {
	copied := *profile
	copied.PainPoints = []PainPointEntity{}
	for _, id := range painPointIDs {
		copied.PainPoints = append(copied.PainPoints, f.painPoints[id])
	}
	f.podologicalProfiles[profile.UserID] = &copied
	return nil
}

func (f *fakeRepository) CountPainPoints(_ context.Context, ids []string) (int, error) {
	count := 0
	for _, id := range ids {
		if _, ok := f.painPoints[id]; ok {
			count++
		}
	}
	return count, nil
}

func (f *fakeRepository) activeConsent(userID string, purpose ConsentPurpose) *ConsentEntity {
	for _, consent := range f.consents {
		if consent.UserID == userID && consent.Purpose == purpose && consent.RevokedAt == nil {
			return consent
		}
	}
	return nil
}

func (f *fakeRepository) GetActiveConsent(_ context.Context, userID string, purpose ConsentPurpose) (*ConsentEntity, error) {
	consent := f.activeConsent(userID, purpose)
	if consent == nil {
		return nil, app.ErrConsentNotFound
	}
	copied := *consent
	return &copied, nil
}

func (f *fakeRepository) GetActiveConsents(_ context.Context, userID string) ([]*ConsentEntity, error) {
	consents := []*ConsentEntity{}
	for _, consent := range f.consents {
		if consent.UserID == userID && consent.RevokedAt == nil {
			consents = append(consents, consent)
		}
	}
	return consents, nil
}

func (f *fakeRepository) GrantConsent(_ context.Context, consent *ConsentEntity) (bool, error) {
	active := f.activeConsent(consent.UserID, consent.Purpose)
	if active != nil && active.TermsVersion == consent.TermsVersion {
		*consent = *active
		return false, nil
	}
	if active != nil {
		active.RevokedAt = &time.Time{}
	}

	copied := *consent
	copied.GrantedAt = time.Now()
	f.consents = append(f.consents, &copied)
	*consent = copied
	return true, nil
}

func (f *fakeRepository) RevokeConsent(_ context.Context, userID string, purpose ConsentPurpose) error {
	if consent := f.activeConsent(userID, purpose); consent != nil {
		now := time.Now()
		consent.RevokedAt = &now
	}
	return nil
}

func (f *fakeRepository) RevokeConsentAndDeletePodologicalProfile(ctx context.Context, userID string) error {
	delete(f.podologicalProfiles, userID)
	return f.RevokeConsent(ctx, userID, ConsentPurposePodologicalProfile)
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

func grantPodologicalConsent(t *testing.T, s *service) {
	t.Helper()
	_, _, err := s.GrantConsent(context.Background(), testUserID, ConsentInput{
		Purpose:      ConsentPurposePodologicalProfile.String(),
		TermsVersion: "podological-profile-v1",
	})
	if err != nil {
		t.Fatalf("GrantConsent: %v", err)
	}
}

func validPodologicalProfileInput() PodologicalProfileInput {
	return PodologicalProfileInput{
		FootstrikeType:    " pronated ",
		ClinicalCondition: helpers.StringPtr("  Fascite plantar "),
		Obs:               helpers.StringPtr("   "),
		PainPointIDs:      []string{painPointHeel, strings.ToUpper(painPointArch), painPointHeel},
	}
}

func TestEditPodologicalProfileRequiresConsent(t *testing.T) {
	repository := newFakeRepository()
	s := NewService(repository)

	_, err := s.EditPodologicalProfile(context.Background(), testUserID, validPodologicalProfileInput())

	if !errors.Is(err, app.ErrConsentRequired) {
		t.Fatalf("err = %v, want %v", err, app.ErrConsentRequired)
	}
	if len(repository.podologicalProfiles) != 0 {
		t.Fatal("profile was saved without consent")
	}
}

func TestEditPodologicalProfileRequiresCurrentTermsVersion(t *testing.T) {
	repository := newFakeRepository()
	repository.consents = append(repository.consents, &ConsentEntity{
		UserID: testUserID, Purpose: ConsentPurposePodologicalProfile, TermsVersion: "podological-profile-v0",
	})
	s := NewService(repository)

	_, err := s.EditPodologicalProfile(context.Background(), testUserID, validPodologicalProfileInput())

	if !errors.Is(err, app.ErrConsentRequired) {
		t.Fatalf("err = %v, want %v", err, app.ErrConsentRequired)
	}
}

func TestEditPodologicalProfileSavesNormalizedDataWithConsent(t *testing.T) {
	repository := newFakeRepository()
	s := NewService(repository)
	grantPodologicalConsent(t, s)

	profile, err := s.EditPodologicalProfile(context.Background(), testUserID, validPodologicalProfileInput())
	if err != nil {
		t.Fatalf("EditPodologicalProfile: %v", err)
	}

	if profile.FootstrikeType != "pronated" {
		t.Fatalf("footstrike type = %q", profile.FootstrikeType)
	}
	if profile.ClinicalCondition == nil || *profile.ClinicalCondition != "Fascite plantar" {
		t.Fatalf("clinical condition = %v", profile.ClinicalCondition)
	}
	if profile.Obs != nil {
		t.Fatalf("blank notes were stored as %q", *profile.Obs)
	}
	if len(profile.PainPoints) != 2 {
		t.Fatalf("pain points = %+v, want heel and arch once each", profile.PainPoints)
	}
}

func TestEditPodologicalProfileAllowsEmptyClinicalCondition(t *testing.T) {
	s := NewService(newFakeRepository())
	grantPodologicalConsent(t, s)

	profile, err := s.EditPodologicalProfile(context.Background(), testUserID, PodologicalProfileInput{FootstrikeType: "neutral"})
	if err != nil {
		t.Fatalf("EditPodologicalProfile: %v", err)
	}
	if profile.ClinicalCondition != nil || len(profile.PainPoints) != 0 {
		t.Fatalf("profile = %+v", profile)
	}
}

func TestEditPodologicalProfileRejectsInvalidInput(t *testing.T) {
	tests := map[string]func(*PodologicalProfileInput){
		"missing footstrike type": func(in *PodologicalProfileInput) { in.FootstrikeType = "" },
		"unknown footstrike type": func(in *PodologicalProfileInput) { in.FootstrikeType = "flat" },
		"long clinical condition": func(in *PodologicalProfileInput) { in.ClinicalCondition = helpers.StringPtr(strings.Repeat("a", 121)) },
		"long notes":              func(in *PodologicalProfileInput) { in.Obs = helpers.StringPtr(strings.Repeat("a", 1001)) },
		"malformed pain point id": func(in *PodologicalProfileInput) { in.PainPointIDs = []string{"heel"} },
		"unknown pain point id":   func(in *PodologicalProfileInput) { in.PainPointIDs = []string{"00000000-0000-0000-0000-000000000799"} },
	}

	for name, mutate := range tests {
		t.Run(name, func(t *testing.T) {
			repository := newFakeRepository()
			s := NewService(repository)
			grantPodologicalConsent(t, s)
			input := validPodologicalProfileInput()
			mutate(&input)

			_, err := s.EditPodologicalProfile(context.Background(), testUserID, input)

			if !errors.Is(err, app.ErrInvalidInput) {
				t.Fatalf("err = %v, want %v", err, app.ErrInvalidInput)
			}
			if len(repository.podologicalProfiles) != 0 {
				t.Fatal("invalid profile was saved")
			}
		})
	}
}

func TestGrantConsentIsIdempotent(t *testing.T) {
	repository := newFakeRepository()
	s := NewService(repository)
	input := ConsentInput{Purpose: "podological_profile", TermsVersion: "podological-profile-v1"}

	first, created, err := s.GrantConsent(context.Background(), testUserID, input)
	if err != nil || !created {
		t.Fatalf("first GrantConsent: created = %v, err = %v", created, err)
	}
	second, created, err := s.GrantConsent(context.Background(), testUserID, input)
	if err != nil || created {
		t.Fatalf("second GrantConsent: created = %v, err = %v", created, err)
	}

	if len(repository.consents) != 1 {
		t.Fatalf("consents = %d, want 1", len(repository.consents))
	}
	if !first.GrantedAt.Equal(second.GrantedAt) || second.TermsVersion != "podological-profile-v1" {
		t.Fatalf("second consent = %+v, want the first one %+v", second, first)
	}
}

func TestGrantConsentRejectsUnknownPurposeOrOutdatedTerms(t *testing.T) {
	tests := map[string]ConsentInput{
		"unknown purpose":          {Purpose: "marketing", TermsVersion: "podological-profile-v1"},
		"no terms for purpose yet": {Purpose: "prescription", TermsVersion: "podological-profile-v1"},
		"outdated terms":           {Purpose: "podological_profile", TermsVersion: "podological-profile-v0"},
	}

	for name, input := range tests {
		t.Run(name, func(t *testing.T) {
			repository := newFakeRepository()

			_, _, err := NewService(repository).GrantConsent(context.Background(), testUserID, input)

			if !errors.Is(err, app.ErrInvalidInput) {
				t.Fatalf("err = %v, want %v", err, app.ErrInvalidInput)
			}
			if len(repository.consents) != 0 {
				t.Fatal("consent was stored")
			}
		})
	}
}

func TestRevokePodologicalConsentDeletesProfile(t *testing.T) {
	repository := newFakeRepository()
	s := NewService(repository)
	grantPodologicalConsent(t, s)
	if _, err := s.EditPodologicalProfile(context.Background(), testUserID, validPodologicalProfileInput()); err != nil {
		t.Fatalf("EditPodologicalProfile: %v", err)
	}

	if err := s.RevokeConsent(context.Background(), testUserID, "podological_profile"); err != nil {
		t.Fatalf("RevokeConsent: %v", err)
	}

	if _, ok := repository.podologicalProfiles[testUserID]; ok {
		t.Fatal("podological profile still exists after revoking consent")
	}
	consents, err := s.GetConsents(context.Background(), testUserID)
	if err != nil {
		t.Fatalf("GetConsents: %v", err)
	}
	if len(consents) != 0 {
		t.Fatalf("active consents = %+v, want none", consents)
	}
	if repository.consents[0].RevokedAt == nil {
		t.Fatal("consent record was not kept with revoked_at")
	}

	_, err = s.EditPodologicalProfile(context.Background(), testUserID, validPodologicalProfileInput())
	if !errors.Is(err, app.ErrConsentRequired) {
		t.Fatalf("err after revoke = %v, want %v", err, app.ErrConsentRequired)
	}
}

func TestRevokeConsentRejectsUnknownPurpose(t *testing.T) {
	err := NewService(newFakeRepository()).RevokeConsent(context.Background(), testUserID, "marketing")
	if !errors.Is(err, app.ErrInvalidInput) {
		t.Fatalf("err = %v, want %v", err, app.ErrInvalidInput)
	}
}
