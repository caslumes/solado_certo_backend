package postgres

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/caslumes/solado_certo_backend/internal/app"
	"github.com/caslumes/solado_certo_backend/internal/app/enum"
	"github.com/caslumes/solado_certo_backend/internal/features/me"
	"github.com/caslumes/solado_certo_backend/internal/helpers"
	"github.com/caslumes/solado_certo_backend/internal/testutil"
	"github.com/jackc/pgx/v5/pgxpool"
)

const unknownID = "00000000-0000-0000-0000-000000000000"

func newRepository(t *testing.T) (*postgresRepository, *pgxpool.Pool) {
	t.Helper()
	pool := testutil.DB(t)
	return NewPostgresRepository(pool), pool
}

func TestGetProfileReturnsUser(t *testing.T) {
	r, pool := newRepository(t)
	user := testutil.InsertUser(t, pool)

	profile, err := r.GetProfileByUserID(context.Background(), user.ID)
	if err != nil {
		t.Fatalf("GetProfileByUserID: %v", err)
	}

	want := me.ProfileEntity{ID: user.ID, Name: user.Name, Email: user.Email, Phone: user.Phone, Status: enum.ProfileStatusActive}
	if *profile != want {
		t.Fatalf("profile = %+v, want %+v", *profile, want)
	}
}

func TestGetProfileReturnsNotFoundForUnknownUser(t *testing.T) {
	r, _ := newRepository(t)

	_, err := r.GetProfileByUserID(context.Background(), unknownID)

	if !errors.Is(err, app.ErrUserNotFound) {
		t.Fatalf("err = %v, want %v", err, app.ErrUserNotFound)
	}
}

func TestUpdateProfileChangesUser(t *testing.T) {
	r, pool := newRepository(t)
	ctx := context.Background()
	user := testutil.InsertUser(t, pool)
	want := me.ProfileEntity{
		ID: user.ID, Name: "Maria Lima", Email: "maria.lima@example.com", Phone: "19988887777",
		AvatarURL: "https://example.com/a.png", Status: enum.ProfileStatusPending,
	}

	if err := r.UpdateProfile(ctx, &want); err != nil {
		t.Fatalf("UpdateProfile: %v", err)
	}

	profile, err := r.GetProfileByUserID(ctx, user.ID)
	if err != nil {
		t.Fatalf("GetProfileByUserID: %v", err)
	}
	if *profile != want {
		t.Fatalf("profile = %+v, want %+v", *profile, want)
	}
}

func TestUpdateProfileRejectsEmailOrPhoneInUse(t *testing.T) {
	r, pool := newRepository(t)
	existing := testutil.InsertUser(t, pool)
	user := testutil.InsertUser(t, pool)

	tests := map[string]me.ProfileEntity{
		"e-mail": {ID: user.ID, Name: user.Name, Email: existing.Email, Phone: user.Phone, Status: enum.ProfileStatusActive},
		"phone":  {ID: user.ID, Name: user.Name, Email: user.Email, Phone: existing.Phone, Status: enum.ProfileStatusActive},
	}

	for name, profile := range tests {
		t.Run(name, func(t *testing.T) {
			err := r.UpdateProfile(context.Background(), &profile)
			if !errors.Is(err, app.ErrUserAlreadyExists) {
				t.Fatalf("err = %v, want %v", err, app.ErrUserAlreadyExists)
			}
		})
	}
}

func newAddress() *me.AddressEntity {
	return &me.AddressEntity{
		Receiver: "Maria Silva",
		ZIPCode:  "01001000",
		Street:   "Praça da Sé",
		Number:   "100",
		District: "Sé",
		City:     "São Paulo",
		State:    "SP",
		Country:  "BR",
	}
}

func createAddress(t *testing.T, r *postgresRepository, userID string, address *me.AddressEntity) *me.AddressEntity {
	t.Helper()
	created, err := r.CreateAddress(context.Background(), userID, address)
	if err != nil {
		t.Fatalf("CreateAddress: %v", err)
	}
	return created
}

func TestCreateAddressCanBeReadByID(t *testing.T) {
	r, pool := newRepository(t)
	user := testutil.InsertUser(t, pool)

	created := createAddress(t, r, user.ID, newAddress())

	address, err := r.GetAddressByID(context.Background(), created.ID)
	if err != nil {
		t.Fatalf("GetAddressByID: %v", err)
	}
	want := *newAddress()
	want.ID = created.ID
	want.UserID = user.ID
	if address.Label != nil || address.Complement != nil {
		t.Fatalf("label = %v, complement = %v, want null", address.Label, address.Complement)
	}
	address.Label, address.Complement = nil, nil
	if *address != want {
		t.Fatalf("address = %+v, want %+v", *address, want)
	}
}

func TestGetAddressByIDReturnsNotFound(t *testing.T) {
	r, _ := newRepository(t)

	_, err := r.GetAddressByID(context.Background(), unknownID)

	if !errors.Is(err, app.ErrAddressNotFound) {
		t.Fatalf("err = %v, want %v", err, app.ErrAddressNotFound)
	}
}

func TestGetAddressesReturnsOnlyTheUsersAddresses(t *testing.T) {
	r, pool := newRepository(t)
	user := testutil.InsertUser(t, pool)
	other := testutil.InsertUser(t, pool)
	first := createAddress(t, r, user.ID, newAddress())
	second := createAddress(t, r, user.ID, newAddress())
	createAddress(t, r, other.ID, newAddress())

	addresses, err := r.GetAddressesByUserID(context.Background(), user.ID)
	if err != nil {
		t.Fatalf("GetAddressesByUserID: %v", err)
	}

	ids := map[string]bool{}
	for _, address := range addresses {
		ids[address.ID] = true
	}
	if len(addresses) != 2 || !ids[first.ID] || !ids[second.ID] {
		t.Fatalf("addresses = %+v, want %s and %s", addresses, first.ID, second.ID)
	}
}

func TestUpdateAddressChangesFields(t *testing.T) {
	r, pool := newRepository(t)
	ctx := context.Background()
	user := testutil.InsertUser(t, pool)
	address := createAddress(t, r, user.ID, newAddress())
	address.Label = helpers.StringPtr("Trabalho")
	address.Complement = helpers.StringPtr("Sala 3")
	address.Number = "200"
	address.IsDefault = true

	if err := r.UpdateAddress(ctx, address); err != nil {
		t.Fatalf("UpdateAddress: %v", err)
	}

	updated, err := r.GetAddressByID(ctx, address.ID)
	if err != nil {
		t.Fatalf("GetAddressByID: %v", err)
	}
	if updated.Label == nil || *updated.Label != "Trabalho" || updated.Complement == nil || *updated.Complement != "Sala 3" ||
		updated.Number != "200" || !updated.IsDefault {
		t.Fatalf("address = %+v", updated)
	}
}

func TestDeleteAddressRemovesIt(t *testing.T) {
	r, pool := newRepository(t)
	ctx := context.Background()
	user := testutil.InsertUser(t, pool)
	address := createAddress(t, r, user.ID, newAddress())

	if err := r.DeleteAddress(ctx, address.ID); err != nil {
		t.Fatalf("DeleteAddress: %v", err)
	}

	if _, err := r.GetAddressByID(ctx, address.ID); !errors.Is(err, app.ErrAddressNotFound) {
		t.Fatalf("err = %v, want %v", err, app.ErrAddressNotFound)
	}
}

func insertPainPoint(t *testing.T, pool *pgxpool.Pool, name string) string {
	t.Helper()
	var id string
	err := pool.QueryRow(context.Background(), "INSERT INTO pain_points (name) VALUES ($1) RETURNING id", name).Scan(&id)
	if err != nil {
		t.Fatalf("insert pain point: %v", err)
	}
	return id
}

func painPointNames(painPoints []me.PainPointEntity) []string {
	names := []string{}
	for _, painPoint := range painPoints {
		names = append(names, painPoint.Name)
	}
	return names
}

func TestGetPodologicalProfileReturnsNotFound(t *testing.T) {
	r, pool := newRepository(t)
	user := testutil.InsertUser(t, pool)

	_, err := r.GetPodologicalProfileByUserID(context.Background(), user.ID)

	if !errors.Is(err, app.ErrPodologicalProfileNotFound) {
		t.Fatalf("err = %v, want %v", err, app.ErrPodologicalProfileNotFound)
	}
}

func TestSavePodologicalProfileCreatesAndReplaces(t *testing.T) {
	r, pool := newRepository(t)
	ctx := context.Background()
	user := testutil.InsertUser(t, pool)
	heel := insertPainPoint(t, pool, "Calcanhar")
	arch := insertPainPoint(t, pool, "Arco plantar")
	toes := insertPainPoint(t, pool, "Dedos")

	created := &me.PodologicalProfileEntity{UserID: user.ID, FootstrikeType: "pronated", ClinicalCondition: helpers.StringPtr("Fascite plantar")}
	if err := r.SavePodologicalProfile(ctx, created, []string{heel, arch}); err != nil {
		t.Fatalf("SavePodologicalProfile: %v", err)
	}

	profile, err := r.GetPodologicalProfileByUserID(ctx, user.ID)
	if err != nil {
		t.Fatalf("GetPodologicalProfileByUserID: %v", err)
	}
	if profile.FootstrikeType != "pronated" || profile.ClinicalCondition == nil || *profile.ClinicalCondition != "Fascite plantar" ||
		profile.Obs != nil || profile.UpdatedAt.IsZero() {
		t.Fatalf("profile = %+v", profile)
	}
	if names := painPointNames(profile.PainPoints); len(names) != 2 || names[0] != "Arco plantar" || names[1] != "Calcanhar" {
		t.Fatalf("pain points = %v, want [Arco plantar Calcanhar]", names)
	}

	replaced := &me.PodologicalProfileEntity{UserID: user.ID, FootstrikeType: "neutral", Obs: helpers.StringPtr("Usa palmilha")}
	if err := r.SavePodologicalProfile(ctx, replaced, []string{toes}); err != nil {
		t.Fatalf("SavePodologicalProfile again: %v", err)
	}

	profile, err = r.GetPodologicalProfileByUserID(ctx, user.ID)
	if err != nil {
		t.Fatalf("GetPodologicalProfileByUserID: %v", err)
	}
	if profile.FootstrikeType != "neutral" || profile.ClinicalCondition != nil || profile.Obs == nil || *profile.Obs != "Usa palmilha" {
		t.Fatalf("profile = %+v", profile)
	}
	if names := painPointNames(profile.PainPoints); len(names) != 1 || names[0] != "Dedos" {
		t.Fatalf("pain points = %v, want [Dedos]", names)
	}
}

func TestSavePodologicalProfileClearsPainPoints(t *testing.T) {
	r, pool := newRepository(t)
	ctx := context.Background()
	user := testutil.InsertUser(t, pool)
	heel := insertPainPoint(t, pool, "Calcanhar")
	profile := &me.PodologicalProfileEntity{UserID: user.ID, FootstrikeType: "neutral"}
	if err := r.SavePodologicalProfile(ctx, profile, []string{heel}); err != nil {
		t.Fatalf("SavePodologicalProfile: %v", err)
	}

	if err := r.SavePodologicalProfile(ctx, profile, nil); err != nil {
		t.Fatalf("SavePodologicalProfile without pain points: %v", err)
	}

	saved, err := r.GetPodologicalProfileByUserID(ctx, user.ID)
	if err != nil {
		t.Fatalf("GetPodologicalProfileByUserID: %v", err)
	}
	if len(saved.PainPoints) != 0 {
		t.Fatalf("pain points = %+v, want none", saved.PainPoints)
	}
}

func TestGetPainPointsIsSortedByName(t *testing.T) {
	r, pool := newRepository(t)
	insertPainPoint(t, pool, "Calcanhar")
	insertPainPoint(t, pool, "Arco plantar")

	painPoints, err := r.GetPainPoints(context.Background())
	if err != nil {
		t.Fatalf("GetPainPoints: %v", err)
	}

	if len(painPoints) != 2 || painPoints[0].Name != "Arco plantar" || painPoints[1].Name != "Calcanhar" {
		t.Fatalf("pain points = %+v", painPoints)
	}
}

func TestCountPainPointsCountsOnlyExisting(t *testing.T) {
	r, pool := newRepository(t)
	heel := insertPainPoint(t, pool, "Calcanhar")
	arch := insertPainPoint(t, pool, "Arco plantar")

	count, err := r.CountPainPoints(context.Background(), []string{heel, arch, unknownID})
	if err != nil {
		t.Fatalf("CountPainPoints: %v", err)
	}
	if count != 2 {
		t.Fatalf("count = %d, want 2", count)
	}
}

func grantConsent(t *testing.T, r *postgresRepository, userID string, purpose me.ConsentPurpose, version string) (*me.ConsentEntity, bool) {
	t.Helper()
	consent := &me.ConsentEntity{UserID: userID, Purpose: purpose, TermsVersion: version}
	created, err := r.GrantConsent(context.Background(), consent)
	if err != nil {
		t.Fatalf("GrantConsent: %v", err)
	}
	return consent, created
}

func TestGrantConsentIsIdempotentForSameTerms(t *testing.T) {
	r, pool := newRepository(t)
	user := testutil.InsertUser(t, pool)

	first, created := grantConsent(t, r, user.ID, me.ConsentPurposePodologicalProfile, "v1")
	if !created || first.ID == "" || first.GrantedAt.IsZero() {
		t.Fatalf("first grant: created = %v, consent = %+v", created, first)
	}

	second, created := grantConsent(t, r, user.ID, me.ConsentPurposePodologicalProfile, "v1")
	if created || second.ID != first.ID || !second.GrantedAt.Equal(first.GrantedAt) {
		t.Fatalf("second grant: created = %v, consent = %+v, want %+v", created, second, first)
	}
}

func TestGrantConsentForNewTermsReplacesActiveConsent(t *testing.T) {
	r, pool := newRepository(t)
	ctx := context.Background()
	user := testutil.InsertUser(t, pool)
	old, _ := grantConsent(t, r, user.ID, me.ConsentPurposePodologicalProfile, "v1")

	current, created := grantConsent(t, r, user.ID, me.ConsentPurposePodologicalProfile, "v2")

	if !created || current.ID == old.ID || current.TermsVersion != "v2" {
		t.Fatalf("created = %v, consent = %+v", created, current)
	}
	active, err := r.GetActiveConsents(ctx, user.ID)
	if err != nil {
		t.Fatalf("GetActiveConsents: %v", err)
	}
	if len(active) != 1 || active[0].ID != current.ID {
		t.Fatalf("active consents = %+v, want only %s", active, current.ID)
	}
}

func TestGetActiveConsentReturnsNotFound(t *testing.T) {
	r, pool := newRepository(t)
	user := testutil.InsertUser(t, pool)

	_, err := r.GetActiveConsent(context.Background(), user.ID, me.ConsentPurposePodologicalProfile)

	if !errors.Is(err, app.ErrConsentNotFound) {
		t.Fatalf("err = %v, want %v", err, app.ErrConsentNotFound)
	}
}

func TestGetActiveConsentsListsEachPurposeOfTheUser(t *testing.T) {
	r, pool := newRepository(t)
	user := testutil.InsertUser(t, pool)
	other := testutil.InsertUser(t, pool)
	grantConsent(t, r, user.ID, me.ConsentPurposePodologicalProfile, "v1")
	grantConsent(t, r, user.ID, me.ConsentPurposePrescription, "v1")
	grantConsent(t, r, other.ID, me.ConsentPurposePodologicalProfile, "v1")

	consents, err := r.GetActiveConsents(context.Background(), user.ID)
	if err != nil {
		t.Fatalf("GetActiveConsents: %v", err)
	}

	if len(consents) != 2 || consents[0].UserID != user.ID || consents[1].UserID != user.ID {
		t.Fatalf("consents = %+v", consents)
	}
}

func TestRevokeConsentKeepsRecord(t *testing.T) {
	r, pool := newRepository(t)
	ctx := context.Background()
	user := testutil.InsertUser(t, pool)
	consent, _ := grantConsent(t, r, user.ID, me.ConsentPurposePrescription, "v1")

	if err := r.RevokeConsent(ctx, user.ID, me.ConsentPurposePrescription); err != nil {
		t.Fatalf("RevokeConsent: %v", err)
	}

	if _, err := r.GetActiveConsent(ctx, user.ID, me.ConsentPurposePrescription); !errors.Is(err, app.ErrConsentNotFound) {
		t.Fatalf("err = %v, want %v", err, app.ErrConsentNotFound)
	}
	var revokedAt *time.Time
	if err := pool.QueryRow(ctx, "SELECT revoked_at FROM consents WHERE id = $1", consent.ID).Scan(&revokedAt); err != nil {
		t.Fatalf("read consent: %v", err)
	}
	if revokedAt == nil {
		t.Fatal("revoked_at was not set")
	}
}

func TestRevokeConsentAndDeletePodologicalProfile(t *testing.T) {
	r, pool := newRepository(t)
	ctx := context.Background()
	user := testutil.InsertUser(t, pool)
	heel := insertPainPoint(t, pool, "Calcanhar")
	grantConsent(t, r, user.ID, me.ConsentPurposePodologicalProfile, "v1")
	grantConsent(t, r, user.ID, me.ConsentPurposePrescription, "v1")
	profile := &me.PodologicalProfileEntity{UserID: user.ID, FootstrikeType: "neutral"}
	if err := r.SavePodologicalProfile(ctx, profile, []string{heel}); err != nil {
		t.Fatalf("SavePodologicalProfile: %v", err)
	}

	if err := r.RevokeConsentAndDeletePodologicalProfile(ctx, user.ID); err != nil {
		t.Fatalf("RevokeConsentAndDeletePodologicalProfile: %v", err)
	}

	if _, err := r.GetPodologicalProfileByUserID(ctx, user.ID); !errors.Is(err, app.ErrPodologicalProfileNotFound) {
		t.Fatalf("profile err = %v, want %v", err, app.ErrPodologicalProfileNotFound)
	}
	if _, err := r.GetActiveConsent(ctx, user.ID, me.ConsentPurposePodologicalProfile); !errors.Is(err, app.ErrConsentNotFound) {
		t.Fatalf("consent err = %v, want %v", err, app.ErrConsentNotFound)
	}
	if _, err := r.GetActiveConsent(ctx, user.ID, me.ConsentPurposePrescription); err != nil {
		t.Fatalf("prescription consent was revoked: %v", err)
	}
}

func TestGetOnboardingReturnsNotFound(t *testing.T) {
	r, pool := newRepository(t)
	user := testutil.InsertUser(t, pool)

	_, err := r.GetOnboardingByUserID(context.Background(), user.ID)

	if !errors.Is(err, app.ErrOnboardingNotFound) {
		t.Fatalf("err = %v, want %v", err, app.ErrOnboardingNotFound)
	}
}

func TestCreateOnboardingStartsAtFirstStep(t *testing.T) {
	r, pool := newRepository(t)
	ctx := context.Background()
	user := testutil.InsertUser(t, pool)

	if err := r.CreateOnboarding(ctx, &me.OnboardingEntity{UserID: user.ID, Status: me.OnboardingStatusNotStarted}); err != nil {
		t.Fatalf("CreateOnboarding: %v", err)
	}

	onboarding, err := r.GetOnboardingByUserID(ctx, user.ID)
	if err != nil {
		t.Fatalf("GetOnboardingByUserID: %v", err)
	}
	if onboarding.Status != me.OnboardingStatusNotStarted || onboarding.CurrentStep.Tag != "welcome" || onboarding.CurrentStep.Order != 1 {
		t.Fatalf("onboarding = %+v", onboarding)
	}
	if onboarding.CompletedAt != nil || onboarding.DismissedAt != nil {
		t.Fatalf("onboarding = %+v, want no completion or dismissal", onboarding)
	}
}

func TestUpdateOnboardingChangesStatusAndStep(t *testing.T) {
	r, pool := newRepository(t)
	ctx := context.Background()
	user := testutil.InsertUser(t, pool)
	if err := r.CreateOnboarding(ctx, &me.OnboardingEntity{UserID: user.ID, Status: me.OnboardingStatusNotStarted}); err != nil {
		t.Fatalf("CreateOnboarding: %v", err)
	}
	step, err := r.GetOnboardingStepByTag(ctx, "completion")
	if err != nil {
		t.Fatalf("GetOnboardingStepByTag: %v", err)
	}
	completedAt := time.Now().UTC().Truncate(time.Microsecond)

	err = r.UpdateOnboarding(ctx, user.ID, &me.OnboardingEntity{
		Status: me.OnboardingStatusCompleted, CurrentStep: *step, CompletedAt: &completedAt,
	})
	if err != nil {
		t.Fatalf("UpdateOnboarding: %v", err)
	}

	onboarding, err := r.GetOnboardingByUserID(ctx, user.ID)
	if err != nil {
		t.Fatalf("GetOnboardingByUserID: %v", err)
	}
	if onboarding.Status != me.OnboardingStatusCompleted || onboarding.CurrentStep.Tag != "completion" {
		t.Fatalf("onboarding = %+v", onboarding)
	}
	if onboarding.CompletedAt == nil || !onboarding.CompletedAt.Equal(completedAt) {
		t.Fatalf("completed at = %v, want %v", onboarding.CompletedAt, completedAt)
	}
}

func TestGetOnboardingStepByTagAndOrder(t *testing.T) {
	r, _ := newRepository(t)
	ctx := context.Background()

	byTag, err := r.GetOnboardingStepByTag(ctx, "address_setup")
	if err != nil {
		t.Fatalf("GetOnboardingStepByTag: %v", err)
	}
	if byTag.Order != 3 || byTag.IsRequired {
		t.Fatalf("step = %+v, want optional step 3", byTag)
	}

	byOrder, err := r.GetOnboardingStepByOrder(ctx, 3)
	if err != nil {
		t.Fatalf("GetOnboardingStepByOrder: %v", err)
	}
	if *byOrder != *byTag {
		t.Fatalf("step by order = %+v, want %+v", byOrder, byTag)
	}

	if _, err := r.GetOnboardingStepByTag(ctx, "missing"); !errors.Is(err, app.ErrOnboardingStepNotFound) {
		t.Fatalf("unknown tag err = %v, want %v", err, app.ErrOnboardingStepNotFound)
	}
	if _, err := r.GetOnboardingStepByOrder(ctx, 99); !errors.Is(err, app.ErrOnboardingStepNotFound) {
		t.Fatalf("unknown order err = %v, want %v", err, app.ErrOnboardingStepNotFound)
	}
}
