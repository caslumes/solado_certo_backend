package catalog

import "time"

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
	Price                float64
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
}
