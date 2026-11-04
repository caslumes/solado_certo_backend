package postgres

import (
	"context"
	"encoding/json"
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

func TestGetProfileTellsWhetherTheUserHasATaxID(t *testing.T) {
	r, pool := newRepository(t)
	ctx := context.Background()
	user := testutil.InsertUser(t, pool)
	if _, err := pool.Exec(ctx, "UPDATE users SET tax_id = '52998224725' WHERE id = $1", user.ID); err != nil {
		t.Fatalf("set tax_id: %v", err)
	}

	profile, err := r.GetProfileByUserID(ctx, user.ID)

	if err != nil || !profile.HasTaxID {
		t.Fatalf("GetProfileByUserID = %+v, %v, want HasTaxID", profile, err)
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
		Status: enum.ProfileStatusPending,
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

func TestGetRolesByUserIDReturnsCodesSorted(t *testing.T) {
	r, pool := newRepository(t)
	ctx := context.Background()
	user := testutil.InsertUser(t, pool)

	roles, err := r.GetRolesByUserID(ctx, user.ID)
	if err != nil {
		t.Fatalf("GetRolesByUserID: %v", err)
	}
	if len(roles) != 0 {
		t.Fatalf("roles = %v, want none", roles)
	}

	for _, role := range []enum.Role{enum.RoleSeller, enum.RoleBuyer} {
		if _, err := r.AddRole(ctx, user.ID, role); err != nil {
			t.Fatalf("AddRole(%q): %v", role, err)
		}
	}

	roles, err = r.GetRolesByUserID(ctx, user.ID)
	if err != nil {
		t.Fatalf("GetRolesByUserID: %v", err)
	}
	if len(roles) != 2 || roles[0] != enum.RoleBuyer || roles[1] != enum.RoleSeller {
		t.Fatalf("roles = %v, want [buyer seller]", roles)
	}
}

func TestAddRoleIsIdempotent(t *testing.T) {
	r, pool := newRepository(t)
	user := testutil.InsertUser(t, pool)

	created, err := r.AddRole(context.Background(), user.ID, enum.RoleSeller)
	if err != nil || !created {
		t.Fatalf("first AddRole: created = %v, err = %v", created, err)
	}

	created, err = r.AddRole(context.Background(), user.ID, enum.RoleSeller)
	if err != nil || created {
		t.Fatalf("second AddRole: created = %v, err = %v", created, err)
	}
}

func TestAddRoleReturnsNotFoundForUnknownUser(t *testing.T) {
	r, _ := newRepository(t)

	_, err := r.AddRole(context.Background(), unknownID, enum.RoleSeller)

	if !errors.Is(err, app.ErrUserNotFound) {
		t.Fatalf("err = %v, want %v", err, app.ErrUserNotFound)
	}
}

func TestUpdateProfileKeepsAvatar(t *testing.T) {
	r, pool := newRepository(t)
	ctx := context.Background()
	user := testutil.InsertUser(t, pool)
	if err := r.UpdateAvatar(ctx, user.ID, helpers.StringPtr("/files/public/avatars/a.png")); err != nil {
		t.Fatalf("UpdateAvatar: %v", err)
	}

	profile := &me.ProfileEntity{ID: user.ID, Name: "Maria", Email: user.Email, Phone: user.Phone, Status: enum.ProfileStatusActive}
	if err := r.UpdateProfile(ctx, profile); err != nil {
		t.Fatalf("UpdateProfile: %v", err)
	}

	got, err := r.GetProfileByUserID(ctx, user.ID)
	if err != nil || got.AvatarURL != "/files/public/avatars/a.png" {
		t.Fatalf("avatar = %q, err = %v", got.AvatarURL, err)
	}
}

func TestUpdateAvatarSetsAndClears(t *testing.T) {
	r, pool := newRepository(t)
	ctx := context.Background()
	user := testutil.InsertUser(t, pool)

	if err := r.UpdateAvatar(ctx, user.ID, helpers.StringPtr("/files/public/avatars/a.png")); err != nil {
		t.Fatalf("UpdateAvatar: %v", err)
	}
	if got, _ := r.GetProfileByUserID(ctx, user.ID); got.AvatarURL != "/files/public/avatars/a.png" {
		t.Fatalf("avatar = %q", got.AvatarURL)
	}

	if err := r.UpdateAvatar(ctx, user.ID, nil); err != nil {
		t.Fatalf("UpdateAvatar(nil): %v", err)
	}
	var stored *string
	if err := pool.QueryRow(ctx, "SELECT avatar_url FROM users WHERE id = $1", user.ID).Scan(&stored); err != nil || stored != nil {
		t.Fatalf("avatar_url = %v, err = %v; want NULL", stored, err)
	}
}

func TestUpdateAvatarReturnsNotFoundForUnknownUser(t *testing.T) {
	r, _ := newRepository(t)

	if err := r.UpdateAvatar(context.Background(), unknownID, nil); !errors.Is(err, app.ErrUserNotFound) {
		t.Fatalf("err = %v, want %v", err, app.ErrUserNotFound)
	}
}

func exec(t *testing.T, pool *pgxpool.Pool, sql string, args ...any) {
	t.Helper()
	if _, err := pool.Exec(context.Background(), sql, args...); err != nil {
		t.Fatalf("%s: %v", sql, err)
	}
}

func insertOrder(t *testing.T, pool *pgxpool.Pool, buyerID string, ad testutil.Ad, status string) string {
	t.Helper()
	ctx := context.Background()
	var checkoutID, orderID string
	err := pool.QueryRow(ctx, "INSERT INTO checkouts (buyer_id, idempotency_key, created_at) VALUES ($1, gen_random_uuid()::text, NOW()) RETURNING id", buyerID).Scan(&checkoutID)
	if err != nil {
		t.Fatalf("insert checkout: %v", err)
	}
	err = pool.QueryRow(
		ctx,
		"INSERT INTO orders (buyer_id, checkout_id, number, status, subtotal, total, shipping_address, created_at, updated_at) "+
			"VALUES ($1, $2, 'SC-' || substr(md5(random()::text), 1, 10), $3, 100, 100, json_build_object('receiver', 'Maria', 'city', 'Recife'), NOW(), NOW()) RETURNING id",
		buyerID, checkoutID, status,
	).Scan(&orderID)
	if err != nil {
		t.Fatalf("insert order: %v", err)
	}
	exec(t, pool,
		"INSERT INTO order_items (order_id, advertisement_id, seller_id, quantity, unit_price, subtotal, product_snapshot, created_at) "+
			"VALUES ($1, $2, $3, 1, 100, 100, json_build_object('title', $4::text, 'seller_name', 'Loja'), NOW())",
		orderID, ad.ID, ad.SellerID, ad.Title,
	)
	exec(t, pool,
		"INSERT INTO payments (order_id, gateway, idempotency_key, status, method, amount, created_at, updated_at) "+
			"VALUES ($1, 'fake', gen_random_uuid()::text, 'captured', 'pix', 100, NOW(), NOW())",
		orderID,
	)
	return orderID
}

type accountFixture struct {
	buyer   testutil.User
	seller  testutil.User
	ad      testutil.Ad
	orderID string
}

func newAccountFixture(t *testing.T, pool *pgxpool.Pool, orderStatus string) accountFixture {
	t.Helper()
	f := accountFixture{buyer: testutil.InsertUser(t, pool), seller: testutil.InsertUser(t, pool)}
	category := testutil.InsertCategory(t, pool, nil)
	f.ad = testutil.InsertAd(t, pool, testutil.Ad{SellerID: f.seller.ID, CategoryID: category.ID, Stock: 3})
	f.orderID = insertOrder(t, pool, f.buyer.ID, f.ad, orderStatus)

	exec(t, pool, "UPDATE users SET tax_id = '52998224725', avatar_url = '/files/public/avatars/a.png' WHERE id = $1", f.buyer.ID)
	exec(t, pool,
		"INSERT INTO addresses (user_id, receiver, zip_code, street, number, district, city, state, created_at, updated_at) "+
			"VALUES ($1, 'Maria', '50000000', 'Rua A', '10', 'Centro', 'Recife', 'PE', NOW(), NOW())",
		f.buyer.ID,
	)
	exec(t, pool, "INSERT INTO podological_profiles (user_id, footstrike_type, notes, updated_at) VALUES ($1, 'pronated', 'Dor ao correr', NOW())", f.buyer.ID)
	exec(t, pool, "INSERT INTO consents (user_id, purpose, terms_version, granted_at) VALUES ($1, 'podological_profile', 'podological-profile-v1', NOW())", f.buyer.ID)
	exec(t, pool, "INSERT INTO refresh_tokens (user_id, token_hash, expires_at, created_at) VALUES ($1, gen_random_uuid()::text, NOW() + interval '1 day', NOW())", f.buyer.ID)
	exec(t, pool, "INSERT INTO notifications (user_id, type, title, body, created_at) VALUES ($1, 'order_paid', 'Pagamento aprovado', 'Seu pedido foi pago', NOW())", f.buyer.ID)
	return f
}

func TestExportPersonalDataCollectsTheUsersDataAndAudits(t *testing.T) {
	r, pool := newRepository(t)
	ctx := context.Background()
	f := newAccountFixture(t, pool, "delivered")

	data, err := r.ExportPersonalData(ctx, f.buyer.ID)
	if err != nil {
		t.Fatalf("ExportPersonalData: %v", err)
	}

	var export struct {
		Profile            map[string]any   `json:"profile"`
		Roles              []map[string]any `json:"roles"`
		Addresses          []map[string]any `json:"addresses"`
		PodologicalProfile []map[string]any `json:"podological_profile"`
		Consents           []map[string]any `json:"consents"`
		Orders             []map[string]any `json:"orders"`
		OrderItems         []map[string]any `json:"order_items"`
		Payments           []map[string]any `json:"payments"`
		Notifications      []map[string]any `json:"notifications"`
		SupportTickets     []map[string]any `json:"support_tickets"`
	}
	if err := json.Unmarshal(data, &export); err != nil {
		t.Fatalf("unmarshal %s: %v", data, err)
	}
	if export.Profile["email"] != f.buyer.Email || export.Profile["tax_id"] != "52998224725" || export.Profile["password_hash"] != nil {
		t.Fatalf("profile = %v", export.Profile)
	}
	if len(export.Addresses) != 1 || len(export.PodologicalProfile) != 1 || export.PodologicalProfile[0]["notes"] != "Dor ao correr" || len(export.Consents) != 1 || len(export.Notifications) != 1 {
		t.Fatalf("export = %s", data)
	}
	if len(export.Orders) != 1 || len(export.OrderItems) != 1 || export.OrderItems[0]["title"] != f.ad.Title || len(export.Payments) != 1 ||
		export.OrderItems[0]["order_number"] != export.Orders[0]["number"] {
		t.Fatalf("export = %s", data)
	}
	if export.Roles == nil || export.SupportTickets == nil {
		t.Fatalf("empty sections should be [] not null: %s", data)
	}

	var audits int
	if err := pool.QueryRow(ctx, "SELECT count(*) FROM audit_logs WHERE user_id = $1 AND action = 'user.data_exported'", f.buyer.ID).Scan(&audits); err != nil || audits != 1 {
		t.Fatalf("audit rows = %d, %v", audits, err)
	}
}

func TestDeleteAccountAnonymizesTheUserAndKeepsTheOrders(t *testing.T) {
	r, pool := newRepository(t)
	ctx := context.Background()
	f := newAccountFixture(t, pool, "delivered")

	avatarURL, err := r.DeleteAccount(ctx, f.buyer.ID)
	if err != nil {
		t.Fatalf("DeleteAccount: %v", err)
	}
	if avatarURL != "/files/public/avatars/a.png" {
		t.Fatalf("avatarURL = %q", avatarURL)
	}

	var name, email, phone, passwordHash, status string
	var taxID *string
	var deleted bool
	err = pool.QueryRow(ctx, "SELECT name, email, phone, password_hash, tax_id, status, deleted_at IS NOT NULL FROM users WHERE id = $1", f.buyer.ID).
		Scan(&name, &email, &phone, &passwordHash, &taxID, &status, &deleted)
	if err != nil {
		t.Fatalf("read user: %v", err)
	}
	if name != "Usuário removido" || email == f.buyer.Email || phone == f.buyer.Phone || passwordHash != "" || taxID != nil || status != "inactive" || !deleted {
		t.Fatalf("user = %q %q %q %q %v %q %v", name, email, phone, passwordHash, taxID, status, deleted)
	}

	var remaining int
	err = pool.QueryRow(ctx,
		"SELECT (SELECT count(*) FROM addresses WHERE user_id = $1) + (SELECT count(*) FROM podological_profiles WHERE user_id = $1) + "+
			"(SELECT count(*) FROM consents WHERE user_id = $1 AND revoked_at IS NULL) + (SELECT count(*) FROM refresh_tokens WHERE user_id = $1 AND revoked_at IS NULL) + "+
			"(SELECT count(*) FROM notifications WHERE user_id = $1)",
		f.buyer.ID,
	).Scan(&remaining)
	if err != nil || remaining != 0 {
		t.Fatalf("personal rows left = %d, %v", remaining, err)
	}

	var orderBuyer, receiver string
	err = pool.QueryRow(ctx, "SELECT buyer_id, shipping_address->>'receiver' FROM orders WHERE id = $1", f.orderID).Scan(&orderBuyer, &receiver)
	if err != nil || orderBuyer != f.buyer.ID || receiver != "Maria" {
		t.Fatalf("order = %q %q, %v", orderBuyer, receiver, err)
	}

	var audits int
	if err := pool.QueryRow(ctx, "SELECT count(*) FROM audit_logs WHERE user_id = $1 AND action = 'user.account_deleted'", f.buyer.ID).Scan(&audits); err != nil || audits != 1 {
		t.Fatalf("audit rows = %d, %v", audits, err)
	}

	if _, err := r.DeleteAccount(ctx, f.buyer.ID); !errors.Is(err, app.ErrUserNotFound) {
		t.Fatalf("second DeleteAccount error = %v, want ErrUserNotFound", err)
	}
	if _, err := r.GetPasswordHash(ctx, f.buyer.ID); !errors.Is(err, app.ErrUserNotFound) {
		t.Fatalf("GetPasswordHash error = %v, want ErrUserNotFound", err)
	}
	if _, err := r.ExportPersonalData(ctx, f.buyer.ID); !errors.Is(err, app.ErrUserNotFound) {
		t.Fatalf("ExportPersonalData error = %v, want ErrUserNotFound", err)
	}
}

func TestDeleteAccountArchivesTheSellersAds(t *testing.T) {
	r, pool := newRepository(t)
	ctx := context.Background()
	f := newAccountFixture(t, pool, "delivered")

	if _, err := r.DeleteAccount(ctx, f.seller.ID); err != nil {
		t.Fatalf("DeleteAccount: %v", err)
	}

	var status string
	if err := pool.QueryRow(ctx, "SELECT status FROM advertisements WHERE id = $1", f.ad.ID).Scan(&status); err != nil || status != "archived" {
		t.Fatalf("ad status = %q, %v", status, err)
	}
}

func TestDeleteAccountRefusesWhileOrdersAreOpen(t *testing.T) {
	r, pool := newRepository(t)
	ctx := context.Background()
	f := newAccountFixture(t, pool, "paid")

	for name, userID := range map[string]string{"buyer": f.buyer.ID, "seller": f.seller.ID} {
		if _, err := r.DeleteAccount(ctx, userID); !errors.Is(err, app.ErrAccountHasOpenOrders) {
			t.Fatalf("%s: DeleteAccount error = %v, want ErrAccountHasOpenOrders", name, err)
		}
	}

	var deleted int
	if err := pool.QueryRow(ctx, "SELECT count(*) FROM users WHERE deleted_at IS NOT NULL").Scan(&deleted); err != nil || deleted != 0 {
		t.Fatalf("deleted users = %d, %v", deleted, err)
	}
}

func TestDeleteAccountRefusesWhileACustomOrderIsOpen(t *testing.T) {
	r, pool := newRepository(t)
	ctx := context.Background()
	f := newAccountFixture(t, pool, "delivered")
	exec(t, pool,
		"INSERT INTO custom_order_requests (buyer_id, advertisement_id, status, measurements, created_at, updated_at) VALUES ($1, $2, 'under_review', '{}', NOW(), NOW())",
		f.buyer.ID, f.ad.ID,
	)

	if _, err := r.DeleteAccount(ctx, f.buyer.ID); !errors.Is(err, app.ErrAccountHasOpenOrders) {
		t.Fatalf("DeleteAccount error = %v, want ErrAccountHasOpenOrders", err)
	}
}
