package seller

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"regexp"
	"slices"
	"strings"
	"unicode/utf8"

	"github.com/caslumes/solado_certo_backend/internal/app"
	"github.com/caslumes/solado_certo_backend/internal/helpers"
	"github.com/caslumes/solado_certo_backend/pkg/storage"
)

type ListAdvertisementsUseCase interface {
	ListAdvertisements(ctx context.Context, sellerID string, status string) ([]*AdvertisementEntity, error)
}

type GetAdvertisementUseCase interface {
	GetAdvertisement(ctx context.Context, sellerID string, id string) (*AdvertisementEntity, error)
}

type CreateAdvertisementUseCase interface {
	CreateAdvertisement(ctx context.Context, sellerID string, input AdvertisementInput) (*AdvertisementEntity, error)
}

type EditAdvertisementUseCase interface {
	EditAdvertisement(ctx context.Context, sellerID string, id string, input AdvertisementInput) (*AdvertisementEntity, error)
}

type ChangeAdvertisementStatusUseCase interface {
	ChangeAdvertisementStatus(ctx context.Context, sellerID string, id string, action StatusAction) (*AdvertisementEntity, error)
}

type RemoveAdvertisementUseCase interface {
	RemoveAdvertisement(ctx context.Context, sellerID string, id string) error
}

type repository interface {
	GetAdvertisementsBySeller(ctx context.Context, sellerID string, status *AdvertisementStatus) ([]*AdvertisementEntity, error)
	GetAdvertisementByID(ctx context.Context, id string) (*AdvertisementEntity, error)
	GetCategoryRequiredAttributes(ctx context.Context, categoryID string) ([]string, error)
	CountPurposes(ctx context.Context, codes []string) (int, error)
	CountActiveConditions(ctx context.Context, ids []string) (int, error)
	CreateAdvertisement(ctx context.Context, ad *AdvertisementEntity) (string, error)
	UpdateAdvertisement(ctx context.Context, ad *AdvertisementEntity, expectedStatus AdvertisementStatus) error
	ChangeStatus(ctx context.Context, id string, from AdvertisementStatus, to AdvertisementStatus) error
	RemoveAdvertisement(ctx context.Context, id string) error
	AddImage(ctx context.Context, advertisementID string, url string, altText *string, maxImages int) error
	RemoveImage(ctx context.Context, advertisementID string, imageID string) error
	ReorderImages(ctx context.Context, advertisementID string, imageIDs []string, coverImageID string) error
}

const (
	maxTitleLength           = 150
	maxSizeLength            = 20
	maxMaterialLength        = 80
	maxUsageIndicationLength = 120
	maxLongTextLength        = 2000
	maxStock                 = 999999
	maxShippingOptions       = 5
	maxShippingNameLength    = 80
	maxDeliveryDays          = 90
)

var pricePattern = regexp.MustCompile(`^\d{1,8}(\.\d{1,2})?$`)

type service struct {
	r     repository
	files storage.Storage
}

func NewService(r repository, files storage.Storage) *service {
	return &service{r: r, files: files}
}

func isExpected(err error, expected ...error) bool {
	for _, target := range expected {
		if errors.Is(err, target) {
			return true
		}
	}
	return false
}

func (s *service) ListAdvertisements(ctx context.Context, sellerID string, status string) ([]*AdvertisementEntity, error) {
	var filter *AdvertisementStatus
	if status != "" {
		st := AdvertisementStatus(status)
		if !st.IsListable() {
			return nil, fmt.Errorf("%w: status must be draft, published or paused", app.ErrInvalidInput)
		}
		filter = &st
	}

	ads, err := s.r.GetAdvertisementsBySeller(ctx, sellerID, filter)
	if err != nil {
		slog.ErrorContext(ctx, "ListAdvertisements failed", "seller_id", sellerID, "error", err)
		return nil, err
	}
	return ads, nil
}

func (s *service) GetAdvertisement(ctx context.Context, sellerID string, id string) (*AdvertisementEntity, error) {
	return s.ownedAdvertisement(ctx, sellerID, id)
}

func (s *service) CreateAdvertisement(ctx context.Context, sellerID string, input AdvertisementInput) (*AdvertisementEntity, error) {
	ad := &AdvertisementEntity{SellerID: sellerID, Price: "0.00", Status: AdvertisementStatusDraft}
	if err := s.apply(ctx, ad, input); err != nil {
		return nil, err
	}
	if err := requireDraftFields(ad); err != nil {
		return nil, err
	}

	id, err := s.r.CreateAdvertisement(ctx, ad)
	if err != nil {
		slog.ErrorContext(ctx, "CreateAdvertisement failed", "seller_id", sellerID, "error", err)
		return nil, err
	}
	return s.r.GetAdvertisementByID(ctx, id)
}

func (s *service) EditAdvertisement(ctx context.Context, sellerID string, id string, input AdvertisementInput) (*AdvertisementEntity, error) {
	ad, err := s.ownedAdvertisement(ctx, sellerID, id)
	if err != nil {
		return nil, err
	}

	if err := s.apply(ctx, ad, input); err != nil {
		return nil, err
	}
	if err := requireDraftFields(ad); err != nil {
		return nil, err
	}
	if ad.Status == AdvertisementStatusPublished {
		if missing := ad.MissingForPublishing(); len(missing) > 0 {
			return nil, &app.MissingFieldsError{Fields: missing}
		}
	}

	if err := s.r.UpdateAdvertisement(ctx, ad, ad.Status); err != nil {
		if !errors.Is(err, app.ErrInvalidStatusTransition) {
			slog.ErrorContext(ctx, "EditAdvertisement failed", "advertisement_id", id, "error", err)
		}
		return nil, err
	}
	return s.r.GetAdvertisementByID(ctx, id)
}

func (s *service) ChangeAdvertisementStatus(ctx context.Context, sellerID string, id string, action StatusAction) (*AdvertisementEntity, error) {
	t, ok := transitions[action]
	if !ok {
		return nil, fmt.Errorf("%w: unknown action %q", app.ErrInvalidInput, action)
	}

	ad, err := s.ownedAdvertisement(ctx, sellerID, id)
	if err != nil {
		return nil, err
	}
	if ad.Status != t.From {
		return nil, fmt.Errorf("%w: cannot %s an ad that is %s", app.ErrInvalidStatusTransition, action, ad.Status)
	}
	if t.To == AdvertisementStatusPublished {
		if missing := ad.MissingForPublishing(); len(missing) > 0 {
			return nil, &app.MissingFieldsError{Fields: missing}
		}
	}

	if err := s.r.ChangeStatus(ctx, id, t.From, t.To); err != nil {
		if !errors.Is(err, app.ErrInvalidStatusTransition) {
			slog.ErrorContext(ctx, "ChangeAdvertisementStatus failed", "advertisement_id", id, "action", action, "error", err)
		}
		return nil, err
	}
	return s.r.GetAdvertisementByID(ctx, id)
}

func (s *service) RemoveAdvertisement(ctx context.Context, sellerID string, id string) error {
	if _, err := s.ownedAdvertisement(ctx, sellerID, id); err != nil {
		return err
	}

	if err := s.r.RemoveAdvertisement(ctx, id); err != nil {
		if !errors.Is(err, app.ErrAdvertisementNotFound) {
			slog.ErrorContext(ctx, "RemoveAdvertisement failed", "advertisement_id", id, "error", err)
		}
		return err
	}
	return nil
}

func (s *service) ownedAdvertisement(ctx context.Context, sellerID string, id string) (*AdvertisementEntity, error) {
	if !helpers.IsUUID(id) {
		return nil, app.ErrAdvertisementNotFound
	}

	ad, err := s.r.GetAdvertisementByID(ctx, id)
	if err != nil {
		if !errors.Is(err, app.ErrAdvertisementNotFound) {
			slog.ErrorContext(ctx, "GetAdvertisementByID failed", "advertisement_id", id, "error", err)
		}
		return nil, err
	}
	if ad.SellerID != sellerID {
		return nil, app.ErrForbidden
	}
	return ad, nil
}

func requireDraftFields(ad *AdvertisementEntity) error {
	missing := []string{}
	if ad.Title == "" {
		missing = append(missing, "title")
	}
	if ad.CategoryID == "" {
		missing = append(missing, "category_id")
	}
	if len(missing) > 0 {
		return &app.MissingFieldsError{Fields: missing}
	}
	return nil
}

func invalid(format string, args ...any) error {
	return fmt.Errorf("%w: "+format, append([]any{app.ErrInvalidInput}, args...)...)
}

func (s *service) apply(ctx context.Context, ad *AdvertisementEntity, input AdvertisementInput) error {
	if input.CategoryID != nil {
		categoryID := strings.TrimSpace(*input.CategoryID)
		if !helpers.IsUUID(categoryID) {
			return invalid("invalid category_id")
		}
		required, err := s.r.GetCategoryRequiredAttributes(ctx, categoryID)
		if err != nil {
			if errors.Is(err, app.ErrCategoryNotFound) {
				return invalid("category not found")
			}
			return err
		}
		ad.CategoryID = categoryID
		ad.RequiredAttributes = required
	}
	if input.Title != nil {
		ad.Title = strings.TrimSpace(*input.Title)
		if utf8.RuneCountInString(ad.Title) > maxTitleLength {
			return invalid("title must have at most %d characters", maxTitleLength)
		}
	}
	if input.Description != nil {
		ad.Description = strings.TrimSpace(*input.Description)
		if utf8.RuneCountInString(ad.Description) > maxLongTextLength {
			return invalid("description must have at most %d characters", maxLongTextLength)
		}
	}
	if input.Price != nil {
		price, err := normalizePrice(*input.Price)
		if err != nil {
			return invalid("price must be a decimal like \"89.90\"")
		}
		ad.Price = price
	}
	if input.Stock != nil {
		if *input.Stock < 0 || *input.Stock > maxStock {
			return invalid("stock must be between 0 and %d", maxStock)
		}
		ad.Stock = *input.Stock
	}
	if input.PrescriptionRequired != nil {
		ad.PrescriptionRequired = *input.PrescriptionRequired
	}
	if input.CustomMadeAllowed != nil {
		ad.CustomMadeAllowed = *input.CustomMadeAllowed
	}

	texts := []struct {
		name      string
		input     *string
		target    **string
		maxLength int
		valid     map[string]bool
	}{
		{AttributeSide, input.Side, &ad.Side, 0, validSides},
		{AttributeSize, input.Size, &ad.Size, maxSizeLength, nil},
		{AttributeMaterial, input.Material, &ad.Material, maxMaterialLength, nil},
		{AttributeRigidity, input.Rigidity, &ad.Rigidity, 0, validRigidities},
		{AttributeUsageIndication, input.UsageIndication, &ad.UsageIndication, maxUsageIndicationLength, nil},
		{"safety_alerts", input.SafetyAlerts, &ad.SafetyAlerts, maxLongTextLength, nil},
		{"contraindications", input.Contraindications, &ad.Contraindications, maxLongTextLength, nil},
		{"shipping_notes", input.ShippingNotes, &ad.ShippingNotes, maxLongTextLength, nil},
	}
	for _, text := range texts {
		if text.input == nil {
			continue
		}
		value := trimToNil(*text.input)
		if value != nil {
			if text.valid != nil && !text.valid[*value] {
				return invalid("invalid %s %q", text.name, *value)
			}
			if text.maxLength > 0 && utf8.RuneCountInString(*value) > text.maxLength {
				return invalid("%s must have at most %d characters", text.name, text.maxLength)
			}
		}
		*text.target = value
	}

	if input.PurposeCodes != nil {
		codes, err := s.validatePurposes(ctx, *input.PurposeCodes)
		if err != nil {
			return err
		}
		ad.PurposeCodes = codes
	}
	if input.ConditionIDs != nil {
		ids, err := s.validateConditions(ctx, *input.ConditionIDs)
		if err != nil {
			return err
		}
		ad.ConditionIDs = ids
	}
	if input.ShippingOptions != nil {
		options, err := validateShippingOptions(*input.ShippingOptions, ad.ShippingOptions)
		if err != nil {
			return err
		}
		ad.ShippingOptions = options
	}

	return nil
}

func (s *service) validatePurposes(ctx context.Context, codes []string) ([]string, error) {
	unique := uniqueTrimmed(codes)
	if len(unique) == 0 {
		return unique, nil
	}
	count, err := s.r.CountPurposes(ctx, unique)
	if err != nil {
		return nil, err
	}
	if count != len(unique) {
		return nil, invalid("unknown purpose")
	}
	return unique, nil
}

func (s *service) validateConditions(ctx context.Context, ids []string) ([]string, error) {
	unique := uniqueTrimmed(ids)
	if len(unique) == 0 {
		return unique, nil
	}
	for _, id := range unique {
		if !helpers.IsUUID(id) {
			return nil, invalid("invalid condition id")
		}
	}
	count, err := s.r.CountActiveConditions(ctx, unique)
	if err != nil {
		return nil, err
	}
	if count != len(unique) {
		return nil, invalid("unknown condition")
	}
	return unique, nil
}

func validateShippingOptions(inputs []ShippingOptionInput, current []ShippingOptionEntity) ([]ShippingOptionEntity, error) {
	if len(inputs) > maxShippingOptions {
		return nil, invalid("at most %d shipping options", maxShippingOptions)
	}

	options := make([]ShippingOptionEntity, 0, len(inputs))
	seen := map[string]bool{}
	for _, input := range inputs {
		var option ShippingOptionEntity
		if input.ID != nil {
			id := *input.ID
			if seen[id] || !slices.ContainsFunc(current, func(o ShippingOptionEntity) bool { return o.ID == id }) {
				return nil, invalid("unknown shipping option %q", id)
			}
			seen[id] = true
			option.ID = id
		}

		option.Name = strings.TrimSpace(input.Name)
		if option.Name == "" || utf8.RuneCountInString(option.Name) > maxShippingNameLength {
			return nil, invalid("shipping option name must have 1 to %d characters", maxShippingNameLength)
		}
		if input.Carrier != nil {
			option.Carrier = trimToNil(*input.Carrier)
		}
		if option.Carrier != nil && utf8.RuneCountInString(*option.Carrier) > maxShippingNameLength {
			return nil, invalid("shipping option carrier must have at most %d characters", maxShippingNameLength)
		}
		price, err := normalizePrice(input.Price)
		if err != nil {
			return nil, invalid("shipping option price must be a decimal like \"19.90\"")
		}
		option.Price = price
		if input.DeliveryDays < 0 || input.DeliveryDays > maxDeliveryDays {
			return nil, invalid("shipping option delivery_days must be between 0 and %d", maxDeliveryDays)
		}
		option.DeliveryDays = input.DeliveryDays

		options = append(options, option)
	}
	return options, nil
}

func normalizePrice(price string) (string, error) {
	price = strings.TrimSpace(price)
	if !pricePattern.MatchString(price) {
		return "", app.ErrInvalidInput
	}
	whole, cents, _ := strings.Cut(price, ".")
	whole = strings.TrimLeft(whole, "0")
	if whole == "" {
		whole = "0"
	}
	return whole + "." + (cents + "00")[:2], nil
}

func trimToNil(s string) *string {
	trimmed := strings.TrimSpace(s)
	if trimmed == "" {
		return nil
	}
	return &trimmed
}

func uniqueTrimmed(values []string) []string {
	unique := []string{}
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value != "" && !slices.Contains(unique, value) {
			unique = append(unique, value)
		}
	}
	return unique
}
