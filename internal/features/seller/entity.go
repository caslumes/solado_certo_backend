package seller

import (
	"slices"
	"strings"
	"time"
)

type ImageEntity struct {
	ID       string
	URL      string
	AltText  string
	Position int
	IsCover  bool
}

func (i *ImageEntity) ToOutput() ImageOutput {
	return ImageOutput{ID: i.ID, URL: i.URL, AltText: i.AltText, Position: i.Position, IsCover: i.IsCover}
}

type ShippingOptionEntity struct {
	ID           string
	Name         string
	Carrier      *string
	Price        string
	DeliveryDays int
}

func (o *ShippingOptionEntity) ToOutput() ShippingOptionOutput {
	return ShippingOptionOutput{ID: o.ID, Name: o.Name, Carrier: o.Carrier, Price: o.Price, DeliveryDays: o.DeliveryDays}
}

type AdvertisementEntity struct {
	ID                   string
	SellerID             string
	CategoryID           string
	Title                string
	Description          string
	Price                string
	Stock                int
	Status               AdvertisementStatus
	PrescriptionRequired bool
	CustomMadeAllowed    bool
	Side                 *string
	Size                 *string
	Material             *string
	Rigidity             *string
	UsageIndication      *string
	SafetyAlerts         *string
	Contraindications    *string
	ShippingNotes        *string
	PurposeCodes         []string
	ConditionIDs         []string
	ShippingOptions      []ShippingOptionEntity
	Images               []ImageEntity
	// RequiredAttributes comes from the category (categories.required_attributes).
	RequiredAttributes []string
	CreatedAt          time.Time
	UpdatedAt          time.Time
}

// SoldOut is not stored: a published ad with no stock shows as "Esgotado" (RF-007).
func (a *AdvertisementEntity) SoldOut() bool {
	return a.Status == AdvertisementStatusPublished && a.Stock == 0
}

func (a *AdvertisementEntity) attribute(name string) *string {
	switch name {
	case AttributeSide:
		return a.Side
	case AttributeSize:
		return a.Size
	case AttributeMaterial:
		return a.Material
	case AttributeRigidity:
		return a.Rigidity
	case AttributeUsageIndication:
		return a.UsageIndication
	}
	return nil
}

// MissingForPublishing returns the JSON names of the fields that block publishing (UC-003),
// in the order the app's form shows them.
func (a *AdvertisementEntity) MissingForPublishing() []string {
	missing := []string{}
	if strings.TrimSpace(a.Title) == "" {
		missing = append(missing, "title")
	}
	if strings.TrimSpace(a.Description) == "" {
		missing = append(missing, "description")
	}
	if a.CategoryID == "" {
		missing = append(missing, "category_id")
	}
	if isZeroPrice(a.Price) {
		missing = append(missing, "price")
	}
	for _, name := range []string{AttributeSide, AttributeSize, AttributeMaterial, AttributeRigidity, AttributeUsageIndication} {
		if !slices.Contains(a.RequiredAttributes, name) || (name == AttributeSize && a.CustomMadeAllowed) {
			continue
		}
		if !isFilled(a.attribute(name)) {
			missing = append(missing, name)
		}
	}
	// Same rule as the catalog's safety_info_complete (RF-034).
	if a.PrescriptionRequired || len(a.ConditionIDs) > 0 {
		if !isFilled(a.SafetyAlerts) {
			missing = append(missing, "safety_alerts")
		}
		if !isFilled(a.Contraindications) {
			missing = append(missing, "contraindications")
		}
	}
	if len(a.Images) == 0 {
		missing = append(missing, "images")
	}
	if len(a.ShippingOptions) == 0 {
		missing = append(missing, "shipping_options")
	}
	return missing
}

func isFilled(s *string) bool {
	return s != nil && strings.TrimSpace(*s) != ""
}

func isZeroPrice(price string) bool {
	return strings.Trim(strings.ReplaceAll(price, ".", ""), "0") == ""
}

func (a *AdvertisementEntity) ToOutput() *AdvertisementOutput {
	purposes := a.PurposeCodes
	if purposes == nil {
		purposes = []string{}
	}
	conditions := a.ConditionIDs
	if conditions == nil {
		conditions = []string{}
	}
	shippingOptions := make([]ShippingOptionOutput, len(a.ShippingOptions))
	for i, option := range a.ShippingOptions {
		shippingOptions[i] = option.ToOutput()
	}
	images := make([]ImageOutput, len(a.Images))
	var coverImageURL *string
	for i, image := range a.Images {
		images[i] = image.ToOutput()
		if image.IsCover || coverImageURL == nil {
			url := image.URL
			coverImageURL = &url
		}
	}

	return &AdvertisementOutput{
		ID:                   a.ID,
		CategoryID:           a.CategoryID,
		Title:                a.Title,
		Description:          a.Description,
		Price:                a.Price,
		Stock:                a.Stock,
		Status:               string(a.Status),
		SoldOut:              a.SoldOut(),
		PrescriptionRequired: a.PrescriptionRequired,
		CustomMadeAllowed:    a.CustomMadeAllowed,
		Side:                 a.Side,
		Size:                 a.Size,
		Material:             a.Material,
		Rigidity:             a.Rigidity,
		UsageIndication:      a.UsageIndication,
		SafetyAlerts:         a.SafetyAlerts,
		Contraindications:    a.Contraindications,
		ShippingNotes:        a.ShippingNotes,
		Purposes:             purposes,
		ConditionIDs:         conditions,
		ShippingOptions:      shippingOptions,
		Images:               images,
		CoverImageURL:        coverImageURL,
		MissingFields:        a.MissingForPublishing(),
		CreatedAt:            a.CreatedAt,
		UpdatedAt:            a.UpdatedAt,
	}
}
