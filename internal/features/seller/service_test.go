package seller

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"testing"

	"github.com/caslumes/solado_certo_backend/internal/app"
)

const (
	testSellerID  = "seller-1"
	otherSellerID = "seller-2"
	insolesID     = "00000000-0000-0000-0000-000000000301"
	socksID       = "00000000-0000-0000-0000-000000000304"
	conditionID   = "00000000-0000-0000-0000-000000000501"
	unknownID     = "00000000-0000-0000-0000-000000000000"
)

type fakeRepository struct {
	ads          map[string]*AdvertisementEntity
	categories   map[string][]string
	purposes     []string
	conditions   []string
	nextID       int
	nextOptionID int
	// changedStatus simulates another request changing the status between the read and the write.
	changedStatus AdvertisementStatus
}

func newFakeRepository() *fakeRepository {
	return &fakeRepository{
		ads: map[string]*AdvertisementEntity{},
		categories: map[string][]string{
			insolesID: {"side", "size", "material", "rigidity", "usage_indication"},
			socksID:   {"size", "material"},
		},
		purposes:   []string{"prevention", "pain_relief"},
		conditions: []string{conditionID},
	}
}

func clone(ad *AdvertisementEntity) *AdvertisementEntity {
	copied := *ad
	copied.PurposeCodes = slices.Clone(ad.PurposeCodes)
	copied.ConditionIDs = slices.Clone(ad.ConditionIDs)
	copied.ShippingOptions = slices.Clone(ad.ShippingOptions)
	copied.Images = slices.Clone(ad.Images)
	return &copied
}

func (f *fakeRepository) GetAdvertisementsBySeller(_ context.Context, sellerID string, status *AdvertisementStatus) ([]*AdvertisementEntity, error) {
	ads := []*AdvertisementEntity{}
	for _, ad := range f.ads {
		if ad.SellerID == sellerID && ad.Status != AdvertisementStatusRemoved && (status == nil || ad.Status == *status) {
			ads = append(ads, clone(ad))
		}
	}
	return ads, nil
}

func (f *fakeRepository) GetAdvertisementByID(_ context.Context, id string) (*AdvertisementEntity, error) {
	ad, ok := f.ads[id]
	if !ok || ad.Status == AdvertisementStatusRemoved {
		return nil, app.ErrAdvertisementNotFound
	}
	return clone(ad), nil
}

func (f *fakeRepository) GetCategoryRequiredAttributes(_ context.Context, categoryID string) ([]string, error) {
	required, ok := f.categories[categoryID]
	if !ok {
		return nil, app.ErrCategoryNotFound
	}
	return required, nil
}

func (f *fakeRepository) CountPurposes(_ context.Context, codes []string) (int, error) {
	count := 0
	for _, code := range codes {
		if slices.Contains(f.purposes, code) {
			count++
		}
	}
	return count, nil
}

func (f *fakeRepository) CountActiveConditions(_ context.Context, ids []string) (int, error) {
	count := 0
	for _, id := range ids {
		if slices.Contains(f.conditions, id) {
			count++
		}
	}
	return count, nil
}

func (f *fakeRepository) saveOptions(ad *AdvertisementEntity) {
	for i := range ad.ShippingOptions {
		if ad.ShippingOptions[i].ID == "" {
			f.nextOptionID++
			ad.ShippingOptions[i].ID = fmt.Sprintf("option-%d", f.nextOptionID)
		}
	}
}

func (f *fakeRepository) CreateAdvertisement(_ context.Context, ad *AdvertisementEntity) (string, error) {
	f.nextID++
	stored := clone(ad)
	stored.ID = fmt.Sprintf("00000000-0000-0000-0000-%012d", f.nextID)
	f.saveOptions(stored)
	f.ads[stored.ID] = stored
	return stored.ID, nil
}

func (f *fakeRepository) UpdateAdvertisement(_ context.Context, ad *AdvertisementEntity, expectedStatus AdvertisementStatus) error {
	if f.changedStatus != "" {
		f.ads[ad.ID].Status = f.changedStatus
	}
	if f.ads[ad.ID].Status != expectedStatus {
		return app.ErrInvalidStatusTransition
	}
	stored := clone(ad)
	f.saveOptions(stored)
	f.ads[ad.ID] = stored
	return nil
}

func (f *fakeRepository) ChangeStatus(_ context.Context, id string, from AdvertisementStatus, to AdvertisementStatus) error {
	if f.ads[id].Status != from {
		return app.ErrInvalidStatusTransition
	}
	f.ads[id].Status = to
	return nil
}

func (f *fakeRepository) RemoveAdvertisement(_ context.Context, id string) error {
	f.ads[id].Status = AdvertisementStatusRemoved
	return nil
}

func (f *fakeRepository) add(ad *AdvertisementEntity) *AdvertisementEntity {
	f.nextID++
	ad.ID = fmt.Sprintf("00000000-0000-0000-0000-%012d", f.nextID)
	if ad.SellerID == "" {
		ad.SellerID = testSellerID
	}
	f.ads[ad.ID] = clone(ad)
	return ad
}

func intPtr(i int) *int {
	return &i
}

func validInput() AdvertisementInput {
	return AdvertisementInput{
		CategoryID:      strPtr(insolesID),
		Title:           strPtr("  Palmilha de silicone "),
		Description:     strPtr("Absorve impacto"),
		Price:           strPtr("89.9"),
		Stock:           intPtr(10),
		Side:            strPtr("both"),
		Size:            strPtr("35-44"),
		Material:        strPtr("Silicone"),
		Rigidity:        strPtr("flexible"),
		UsageIndication: strPtr("Uso diário"),
		PurposeCodes:    &[]string{"pain_relief", "pain_relief", " prevention "},
		ShippingOptions: &[]ShippingOptionInput{{Name: "PAC", Carrier: strPtr("Correios"), Price: "19.9", DeliveryDays: 7}},
	}
}

func TestCreateAdvertisementSavesDraft(t *testing.T) {
	repository := newFakeRepository()

	ad, err := NewService(repository).CreateAdvertisement(context.Background(), testSellerID, validInput())
	if err != nil {
		t.Fatalf("CreateAdvertisement: %v", err)
	}

	if ad.Status != AdvertisementStatusDraft || ad.SellerID != testSellerID {
		t.Fatalf("status = %q, seller = %q", ad.Status, ad.SellerID)
	}
	if ad.Title != "Palmilha de silicone" || ad.Price != "89.90" || ad.Stock != 10 {
		t.Fatalf("ad = %+v", ad)
	}
	if !slices.Equal(ad.PurposeCodes, []string{"pain_relief", "prevention"}) {
		t.Fatalf("purposes = %v", ad.PurposeCodes)
	}
	if len(ad.ShippingOptions) != 1 || ad.ShippingOptions[0].Price != "19.90" || ad.ShippingOptions[0].ID == "" {
		t.Fatalf("shipping options = %+v", ad.ShippingOptions)
	}
	if !slices.Equal(ad.RequiredAttributes, repository.categories[insolesID]) {
		t.Fatalf("required attributes = %v", ad.RequiredAttributes)
	}
}

func TestCreateAdvertisementDraftNeedsTitleAndCategory(t *testing.T) {
	_, err := NewService(newFakeRepository()).CreateAdvertisement(context.Background(), testSellerID, AdvertisementInput{Title: strPtr("  ")})

	var missing *app.MissingFieldsError
	if !errors.As(err, &missing) || !slices.Equal(missing.Fields, []string{"title", "category_id"}) {
		t.Fatalf("err = %v, want missing title and category_id", err)
	}
}

func TestCreateAdvertisementRejectsInvalidInput(t *testing.T) {
	tests := map[string]func(*AdvertisementInput){
		"category not a UUID":       func(i *AdvertisementInput) { i.CategoryID = strPtr("palmilhas") },
		"unknown category":          func(i *AdvertisementInput) { i.CategoryID = strPtr(unknownID) },
		"long title":                func(i *AdvertisementInput) { i.Title = strPtr(string(make([]rune, 151))) },
		"price with comma":          func(i *AdvertisementInput) { i.Price = strPtr("89,90") },
		"price with three decimals": func(i *AdvertisementInput) { i.Price = strPtr("89.901") },
		"negative price":            func(i *AdvertisementInput) { i.Price = strPtr("-1") },
		"price too large":           func(i *AdvertisementInput) { i.Price = strPtr("100000000") },
		"negative stock":            func(i *AdvertisementInput) { i.Stock = intPtr(-1) },
		"unknown side":              func(i *AdvertisementInput) { i.Side = strPtr("top") },
		"unknown rigidity":          func(i *AdvertisementInput) { i.Rigidity = strPtr("hard") },
		"long size":                 func(i *AdvertisementInput) { i.Size = strPtr("123456789012345678901") },
		"unknown purpose":           func(i *AdvertisementInput) { i.PurposeCodes = &[]string{"marketing"} },
		"condition not a UUID":      func(i *AdvertisementInput) { i.ConditionIDs = &[]string{"fascite"} },
		"unknown condition":         func(i *AdvertisementInput) { i.ConditionIDs = &[]string{unknownID} },
		"shipping without name": func(i *AdvertisementInput) {
			i.ShippingOptions = &[]ShippingOptionInput{{Name: " ", Price: "10"}}
		},
		"shipping with bad price": func(i *AdvertisementInput) {
			i.ShippingOptions = &[]ShippingOptionInput{{Name: "PAC", Price: "dez"}}
		},
		"shipping with negative days": func(i *AdvertisementInput) {
			i.ShippingOptions = &[]ShippingOptionInput{{Name: "PAC", Price: "10", DeliveryDays: -1}}
		},
		"shipping option of another ad": func(i *AdvertisementInput) {
			i.ShippingOptions = &[]ShippingOptionInput{{ID: strPtr("option-99"), Name: "PAC", Price: "10"}}
		},
		"too many shipping options": func(i *AdvertisementInput) {
			options := make([]ShippingOptionInput, maxShippingOptions+1)
			for j := range options {
				options[j] = ShippingOptionInput{Name: "PAC", Price: "10"}
			}
			i.ShippingOptions = &options
		},
	}

	for name, mutate := range tests {
		t.Run(name, func(t *testing.T) {
			repository := newFakeRepository()
			input := validInput()
			mutate(&input)

			_, err := NewService(repository).CreateAdvertisement(context.Background(), testSellerID, input)

			if !errors.Is(err, app.ErrInvalidInput) {
				t.Fatalf("err = %v, want %v", err, app.ErrInvalidInput)
			}
			if len(repository.ads) != 0 {
				t.Fatal("ad was saved")
			}
		})
	}
}

func TestNormalizePrice(t *testing.T) {
	tests := map[string]string{"89.9": "89.90", "89": "89.00", "0089.90": "89.90", "0": "0.00", " 1.5 ": "1.50", "99999999.99": "99999999.99"}

	for input, want := range tests {
		got, err := normalizePrice(input)
		if err != nil || got != want {
			t.Errorf("normalizePrice(%q) = %q, %v; want %q", input, got, err, want)
		}
	}
}

func TestListAdvertisementsFiltersByStatus(t *testing.T) {
	repository := newFakeRepository()
	repository.add(&AdvertisementEntity{Title: "draft", Status: AdvertisementStatusDraft})
	repository.add(&AdvertisementEntity{Title: "published", Status: AdvertisementStatusPublished})
	repository.add(&AdvertisementEntity{Title: "removed", Status: AdvertisementStatusRemoved})
	repository.add(&AdvertisementEntity{Title: "other seller", Status: AdvertisementStatusDraft, SellerID: otherSellerID})
	s := NewService(repository)

	all, err := s.ListAdvertisements(context.Background(), testSellerID, "")
	if err != nil || len(all) != 2 {
		t.Fatalf("all: %d ads, err = %v; want 2", len(all), err)
	}
	drafts, err := s.ListAdvertisements(context.Background(), testSellerID, "draft")
	if err != nil || len(drafts) != 1 || drafts[0].Title != "draft" {
		t.Fatalf("drafts = %v, err = %v", drafts, err)
	}
}

func TestListAdvertisementsRejectsUnknownStatus(t *testing.T) {
	for _, status := range []string{"removed", "archived", "sold_out"} {
		_, err := NewService(newFakeRepository()).ListAdvertisements(context.Background(), testSellerID, status)
		if !errors.Is(err, app.ErrInvalidInput) {
			t.Errorf("status %q: err = %v, want %v", status, err, app.ErrInvalidInput)
		}
	}
}

func TestOwnershipIsChecked(t *testing.T) {
	repository := newFakeRepository()
	ad := repository.add(&AdvertisementEntity{Title: "x", CategoryID: insolesID, Status: AdvertisementStatusPublished, SellerID: otherSellerID})
	s := NewService(repository)
	ctx := context.Background()

	calls := map[string]func() error{
		"get": func() error { _, err := s.GetAdvertisement(ctx, testSellerID, ad.ID); return err },
		"edit": func() error {
			_, err := s.EditAdvertisement(ctx, testSellerID, ad.ID, AdvertisementInput{})
			return err
		},
		"pause": func() error {
			_, err := s.ChangeAdvertisementStatus(ctx, testSellerID, ad.ID, StatusActionPause)
			return err
		},
		"remove": func() error { return s.RemoveAdvertisement(ctx, testSellerID, ad.ID) },
	}
	for name, call := range calls {
		t.Run(name, func(t *testing.T) {
			if err := call(); !errors.Is(err, app.ErrForbidden) {
				t.Fatalf("err = %v, want %v", err, app.ErrForbidden)
			}
		})
	}
	if repository.ads[ad.ID].Status != AdvertisementStatusPublished {
		t.Fatal("another seller changed the ad")
	}
}

func TestGetAdvertisementReturnsNotFound(t *testing.T) {
	repository := newFakeRepository()
	removed := repository.add(&AdvertisementEntity{Status: AdvertisementStatusRemoved})
	s := NewService(repository)

	for _, id := range []string{"not-a-uuid", unknownID, removed.ID} {
		if _, err := s.GetAdvertisement(context.Background(), testSellerID, id); !errors.Is(err, app.ErrAdvertisementNotFound) {
			t.Errorf("id %q: err = %v, want %v", id, err, app.ErrAdvertisementNotFound)
		}
	}
}

func adWithStatus(repository *fakeRepository, status AdvertisementStatus) *AdvertisementEntity {
	ad := completeAd()
	ad.SellerID = testSellerID
	ad.CategoryID = insolesID
	ad.Status = status
	return repository.add(ad)
}

func TestStatusTransitions(t *testing.T) {
	tests := []struct {
		from   AdvertisementStatus
		action StatusAction
		want   AdvertisementStatus
	}{
		{AdvertisementStatusDraft, StatusActionPublish, AdvertisementStatusPublished},
		{AdvertisementStatusPublished, StatusActionPause, AdvertisementStatusPaused},
		{AdvertisementStatusPaused, StatusActionReactivate, AdvertisementStatusPublished},
	}

	for _, test := range tests {
		t.Run(string(test.action), func(t *testing.T) {
			repository := newFakeRepository()
			ad := adWithStatus(repository, test.from)

			changed, err := NewService(repository).ChangeAdvertisementStatus(context.Background(), testSellerID, ad.ID, test.action)
			if err != nil {
				t.Fatalf("ChangeAdvertisementStatus: %v", err)
			}
			if changed.Status != test.want {
				t.Fatalf("status = %q, want %q", changed.Status, test.want)
			}
		})
	}
}

func TestInvalidStatusTransitions(t *testing.T) {
	tests := []struct {
		from   AdvertisementStatus
		action StatusAction
	}{
		{AdvertisementStatusPublished, StatusActionPublish},
		{AdvertisementStatusPaused, StatusActionPublish},
		{AdvertisementStatusDraft, StatusActionPause},
		{AdvertisementStatusPaused, StatusActionPause},
		{AdvertisementStatusDraft, StatusActionReactivate},
		{AdvertisementStatusPublished, StatusActionReactivate},
	}

	for _, test := range tests {
		t.Run(fmt.Sprintf("%s %s", test.action, test.from), func(t *testing.T) {
			repository := newFakeRepository()
			ad := adWithStatus(repository, test.from)

			_, err := NewService(repository).ChangeAdvertisementStatus(context.Background(), testSellerID, ad.ID, test.action)

			if !errors.Is(err, app.ErrInvalidStatusTransition) {
				t.Fatalf("err = %v, want %v", err, app.ErrInvalidStatusTransition)
			}
			if repository.ads[ad.ID].Status != test.from {
				t.Fatalf("status changed to %q", repository.ads[ad.ID].Status)
			}
		})
	}
}

func TestChangeStatusRejectsUnknownAction(t *testing.T) {
	repository := newFakeRepository()
	ad := adWithStatus(repository, AdvertisementStatusDraft)

	_, err := NewService(repository).ChangeAdvertisementStatus(context.Background(), testSellerID, ad.ID, "archive")

	if !errors.Is(err, app.ErrInvalidInput) {
		t.Fatalf("err = %v, want %v", err, app.ErrInvalidInput)
	}
}

func TestPublishingRequiresCompleteAd(t *testing.T) {
	for _, action := range []StatusAction{StatusActionPublish, StatusActionReactivate} {
		t.Run(string(action), func(t *testing.T) {
			repository := newFakeRepository()
			from := transitions[action].From
			ad := adWithStatus(repository, from)
			repository.ads[ad.ID].Images = nil
			repository.ads[ad.ID].Material = nil

			_, err := NewService(repository).ChangeAdvertisementStatus(context.Background(), testSellerID, ad.ID, action)

			var missing *app.MissingFieldsError
			if !errors.As(err, &missing) || !slices.Equal(missing.Fields, []string{"material", "images"}) {
				t.Fatalf("err = %v, want missing material and images", err)
			}
			if repository.ads[ad.ID].Status != from {
				t.Fatal("status changed")
			}
		})
	}
}

func TestEditAdvertisementChangesOnlyGivenFields(t *testing.T) {
	repository := newFakeRepository()
	ad := adWithStatus(repository, AdvertisementStatusDraft)

	edited, err := NewService(repository).EditAdvertisement(context.Background(), testSellerID, ad.ID, AdvertisementInput{
		Price:         strPtr("120"),
		ShippingNotes: strPtr("Enviado em 2 dias"),
		Material:      strPtr(""),
	})
	if err != nil {
		t.Fatalf("EditAdvertisement: %v", err)
	}

	if edited.Price != "120.00" || edited.Title != ad.Title || edited.Material != nil {
		t.Fatalf("edited = %+v", edited)
	}
	if edited.ShippingNotes == nil || *edited.ShippingNotes != "Enviado em 2 dias" {
		t.Fatalf("shipping notes = %v", edited.ShippingNotes)
	}
}

func TestEditAdvertisementKeepsPublishedAdComplete(t *testing.T) {
	repository := newFakeRepository()
	ad := adWithStatus(repository, AdvertisementStatusPublished)

	_, err := NewService(repository).EditAdvertisement(context.Background(), testSellerID, ad.ID, AdvertisementInput{
		ShippingOptions: &[]ShippingOptionInput{},
	})

	var missing *app.MissingFieldsError
	if !errors.As(err, &missing) || !slices.Equal(missing.Fields, []string{"shipping_options"}) {
		t.Fatalf("err = %v, want missing shipping_options", err)
	}
	if len(repository.ads[ad.ID].ShippingOptions) != 1 {
		t.Fatal("shipping options were saved")
	}
}

func TestEditAdvertisementChangingCategoryUsesItsRequirements(t *testing.T) {
	repository := newFakeRepository()
	ad := adWithStatus(repository, AdvertisementStatusPublished)

	edited, err := NewService(repository).EditAdvertisement(context.Background(), testSellerID, ad.ID, AdvertisementInput{
		CategoryID: strPtr(socksID),
		Side:       strPtr(""),
		Rigidity:   strPtr(""),
	})
	if err != nil {
		t.Fatalf("EditAdvertisement: %v", err)
	}
	if !slices.Equal(edited.RequiredAttributes, []string{"size", "material"}) {
		t.Fatalf("required attributes = %v", edited.RequiredAttributes)
	}
}

func TestEditAdvertisementUpdatesExistingShippingOption(t *testing.T) {
	repository := newFakeRepository()
	ad := adWithStatus(repository, AdvertisementStatusDraft)
	optionID := ad.ShippingOptions[0].ID

	edited, err := NewService(repository).EditAdvertisement(context.Background(), testSellerID, ad.ID, AdvertisementInput{
		ShippingOptions: &[]ShippingOptionInput{
			{ID: &optionID, Name: "PAC", Price: "21.50", DeliveryDays: 6},
			{Name: "SEDEX", Price: "34.90", DeliveryDays: 2},
		},
	})
	if err != nil {
		t.Fatalf("EditAdvertisement: %v", err)
	}

	if len(edited.ShippingOptions) != 2 || edited.ShippingOptions[0].ID != optionID || edited.ShippingOptions[0].Price != "21.50" {
		t.Fatalf("shipping options = %+v", edited.ShippingOptions)
	}
}

func TestEditAdvertisementFailsWhenStatusChangedMeanwhile(t *testing.T) {
	repository := newFakeRepository()
	ad := adWithStatus(repository, AdvertisementStatusDraft)
	repository.changedStatus = AdvertisementStatusPublished

	_, err := NewService(repository).EditAdvertisement(context.Background(), testSellerID, ad.ID, AdvertisementInput{Title: strPtr("Novo")})

	if !errors.Is(err, app.ErrInvalidStatusTransition) {
		t.Fatalf("err = %v, want %v", err, app.ErrInvalidStatusTransition)
	}
}

func TestRemoveAdvertisement(t *testing.T) {
	for _, status := range []AdvertisementStatus{AdvertisementStatusDraft, AdvertisementStatusPublished, AdvertisementStatusPaused} {
		t.Run(string(status), func(t *testing.T) {
			repository := newFakeRepository()
			ad := adWithStatus(repository, status)
			s := NewService(repository)

			if err := s.RemoveAdvertisement(context.Background(), testSellerID, ad.ID); err != nil {
				t.Fatalf("RemoveAdvertisement: %v", err)
			}
			if err := s.RemoveAdvertisement(context.Background(), testSellerID, ad.ID); !errors.Is(err, app.ErrAdvertisementNotFound) {
				t.Fatalf("second remove: err = %v, want %v", err, app.ErrAdvertisementNotFound)
			}
		})
	}
}

var errDatabase = errors.New("database is down")

type failingRepository struct {
	repository
}

func (failingRepository) GetAdvertisementsBySeller(context.Context, string, *AdvertisementStatus) ([]*AdvertisementEntity, error) {
	return nil, errDatabase
}

func (failingRepository) GetAdvertisementByID(context.Context, string) (*AdvertisementEntity, error) {
	return nil, errDatabase
}

func (failingRepository) GetCategoryRequiredAttributes(context.Context, string) ([]string, error) {
	return nil, errDatabase
}

func TestServicePropagatesRepositoryErrors(t *testing.T) {
	ctx := context.Background()
	s := NewService(failingRepository{})
	calls := map[string]func() error{
		"list":   func() error { _, err := s.ListAdvertisements(ctx, testSellerID, ""); return err },
		"get":    func() error { _, err := s.GetAdvertisement(ctx, testSellerID, unknownID); return err },
		"create": func() error { _, err := s.CreateAdvertisement(ctx, testSellerID, validInput()); return err },
		"edit": func() error {
			_, err := s.EditAdvertisement(ctx, testSellerID, unknownID, AdvertisementInput{})
			return err
		},
		"status": func() error {
			_, err := s.ChangeAdvertisementStatus(ctx, testSellerID, unknownID, StatusActionPublish)
			return err
		},
		"remove": func() error { return s.RemoveAdvertisement(ctx, testSellerID, unknownID) },
	}

	for name, call := range calls {
		t.Run(name, func(t *testing.T) {
			if err := call(); !errors.Is(err, errDatabase) {
				t.Fatalf("err = %v, want %v", err, errDatabase)
			}
		})
	}
}
