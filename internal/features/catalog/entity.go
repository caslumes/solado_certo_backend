package catalog

import (
	"strings"
	"time"
)

type AdvertisementImageEntity struct {
	URL     string
	AltText string
	Order   int
	IsCover bool
}

func (ai *AdvertisementImageEntity) ToOutput() *AdvertisementImageOutput {
	return &AdvertisementImageOutput{
		URL:     ai.URL,
		AltText: ai.AltText,
		Order:   ai.Order,
		IsCover: ai.IsCover,
	}
}

type AdvertisementEntity struct {
	ID                   string
	SellerID             string
	CategoryID           string
	Title                string
	Description          string
	Price                string
	Stock                uint64
	Status               string
	PrescriptionRequired bool
	CustomMadeAllowed    bool
	Images               []*AdvertisementImageEntity
	Side                 *string
	Size                 *string
	Material             *string
	Rigidity             *string
	UsageIndication      *string
	SafetyAlerts         *string
	Contraindications    *string
	ShippingNotes        *string
	CoverImageURL        *string
	AverageRating        *float64
	ReviewCount          uint64
	SellerState          *string
	CreatedAt            time.Time
	UpdatedAt            time.Time
	RemovedAt            *time.Time
}

func (a *AdvertisementEntity) ToOutput() *AdvertisementOutput {
	images := make([]*AdvertisementImageOutput, len(a.Images))
	for i, image := range a.Images {
		images[i] = image.ToOutput()
	}

	return &AdvertisementOutput{
		ID:                   a.ID,
		SellerID:             a.SellerID,
		CategoryID:           a.CategoryID,
		Title:                a.Title,
		Description:          a.Description,
		Price:                a.Price,
		Stock:                a.Stock,
		Status:               a.Status,
		PrescriptionRequired: a.PrescriptionRequired,
		CustomMadeAllowed:    a.CustomMadeAllowed,
		Images:               images,
		Side:                 a.Side,
		Size:                 a.Size,
		Material:             a.Material,
		Rigidity:             a.Rigidity,
		UsageIndication:      a.UsageIndication,
		SafetyAlerts:         a.SafetyAlerts,
		Contraindications:    a.Contraindications,
		ShippingNotes:        a.ShippingNotes,
		CoverImageURL:        a.CoverImageURL,
		AverageRating:        a.AverageRating,
		ReviewCount:          a.ReviewCount,
		SellerState:          a.SellerState,
	}
}

type SellerEntity struct {
	ID            string
	Name          string
	State         *string
	AverageRating *float64
	MemberSince   time.Time
}

func (s *SellerEntity) ToOutput() *SellerOutput {
	return &SellerOutput{
		ID:            s.ID,
		Name:          s.Name,
		State:         s.State,
		AverageRating: s.AverageRating,
		MemberSince:   s.MemberSince,
	}
}

type ShippingOptionEntity struct {
	ID           string
	Name         string
	Carrier      *string
	Price        string
	DeliveryDays uint
}

func (so *ShippingOptionEntity) ToOutput() *ShippingOptionOutput {
	return &ShippingOptionOutput{
		ID:           so.ID,
		Name:         so.Name,
		Carrier:      so.Carrier,
		Price:        so.Price,
		DeliveryDays: so.DeliveryDays,
	}
}

type AdvertisementDetailEntity struct {
	Advertisement   *AdvertisementEntity
	Purposes        []*PurposeEntity
	Conditions      []*ConditionEntity
	Seller          *SellerEntity
	ShippingOptions []*ShippingOptionEntity
}

func (d *AdvertisementDetailEntity) SafetyInfoComplete() bool {
	ad := d.Advertisement
	if !ad.PrescriptionRequired && len(d.Conditions) == 0 {
		return true
	}
	return isFilled(ad.SafetyAlerts) && isFilled(ad.Contraindications)
}

func isFilled(s *string) bool {
	return s != nil && strings.TrimSpace(*s) != ""
}

func (d *AdvertisementDetailEntity) ToOutput() *AdvertisementDetailOutput {
	purposes := make([]*PurposeOutput, len(d.Purposes))
	for i, purpose := range d.Purposes {
		purposes[i] = purpose.ToOutput()
	}
	conditions := make([]*ConditionOutput, len(d.Conditions))
	for i, condition := range d.Conditions {
		conditions[i] = condition.ToOutput()
	}
	shippingOptions := make([]*ShippingOptionOutput, len(d.ShippingOptions))
	for i, option := range d.ShippingOptions {
		shippingOptions[i] = option.ToOutput()
	}
	var seller *SellerOutput
	if d.Seller != nil {
		seller = d.Seller.ToOutput()
	}

	return &AdvertisementDetailOutput{
		AdvertisementOutput: d.Advertisement.ToOutput(),
		Purposes:            purposes,
		Conditions:          conditions,
		Seller:              seller,
		ShippingOptions:     shippingOptions,
		SafetyInfoComplete:  d.SafetyInfoComplete(),
	}
}

type AdvertisementPage struct {
	Items       []*AdvertisementEntity
	CurrentPage uint
	TotalPages  uint
	TotalItems  uint
	HasNext     bool
	HasPrevious bool
}

func (ap *AdvertisementPage) ToOutput() *AdvertisementPageOutput {
	items := make([]*AdvertisementOutput, len(ap.Items))
	for i, item := range ap.Items {
		items[i] = item.ToOutput()
	}
	return &AdvertisementPageOutput{
		Items:       items,
		CurrentPage: ap.CurrentPage,
		TotalPages:  ap.TotalPages,
		TotalItems:  ap.TotalItems,
		HasNext:     ap.HasNext,
		HasPrevious: ap.HasPrevious,
	}
}

type CategoryEntity struct {
	ID               string
	ParentCategoryID *string
	Children         []*CategoryEntity
	Name             string
	Slug             string
	Active           bool
}

func (c *CategoryEntity) ToOutput() *CategoryOutput {
	children := CategoriesEntity{Categories: c.Children}
	return &CategoryOutput{
		ID:               c.ID,
		ParentCategoryID: c.ParentCategoryID,
		Children:         children.ToOutput().Categories,
		Name:             c.Name,
		Slug:             c.Slug,
		Active:           c.Active,
	}
}

type CategoriesEntity struct {
	Categories []*CategoryEntity
}

func (c *CategoriesEntity) ToOutput() *CategoriesOutput {
	categories := make([]*CategoryOutput, len(c.Categories))
	for i, category := range c.Categories {
		categories[i] = category.ToOutput()
	}
	return &CategoriesOutput{
		Categories: categories,
	}
}

type AdvertisementFilters struct {
	Query                *string
	Category             *string
	PriceMin             *float64
	PriceMax             *float64
	PrescriptionRequired *bool
	CustomMadeAllowed    *bool
	Side                 *string
	Size                 *string
	Material             *string
	Rigidity             *string
	UsageIndication      *string
	Purposes             []string
	Conditions           []string
	RatingMin            *float64
	State                *string
	Profile *ProfileMatch
}

type ProfileMatch struct {
	PainPointIDs      []string
	ClinicalCondition *string
}

type PurposeEntity struct {
	ID          string
	Code        string
	Name        string
	Description *string
}

func (p *PurposeEntity) ToOutput() *PurposeOutput {
	return &PurposeOutput{ID: p.ID, Code: p.Code, Name: p.Name, Description: p.Description}
}

type ConditionEntity struct {
	ID          string
	Name        string
	Description *string
}

func (c *ConditionEntity) ToOutput() *ConditionOutput {
	return &ConditionOutput{ID: c.ID, Name: c.Name, Description: c.Description}
}
