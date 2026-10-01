package catalog

import "time"

type AdvertisementImage struct {
	URL string
	AltText string
	Order int
	IsCover bool
}

func (ai *AdvertisementImage) ToOutput() *AdvertisementImageOutput {
	return &AdvertisementImageOutput{
		URL: ai.URL,
		AltText: ai.AltText,
		Order: ai.Order,
		IsCover: ai.IsCover,
	}
}

type Advertisement struct {
	SellerID             string
	CategoryID           string
	Title                string
	Description          string
	Price                float64
	Stock                uint64
	Status               string
	PrescriptionRequired bool
	CustomMadeAllowed    bool
	Images			   []*AdvertisementImage
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

func (a *Advertisement) ToOutput() *AdvertisementOutput {
	images := make([]*AdvertisementImageOutput, len(a.Images))
	for i, image := range a.Images {
		images[i] = image.ToOutput()
	}

	return &AdvertisementOutput{
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
	Items []*Advertisement
	NextCursor     *string
	HasNext bool
}

func (ap *AdvertisementPage) ToOutput() *AdvertisementPageOutput {
	items := make([]*AdvertisementOutput, len(ap.Items))
	for i, item := range ap.Items {
		items[i] = item.ToOutput()
	}
	return &AdvertisementPageOutput{
		Items: items,
		NextCursor: ap.NextCursor,
		HasNext: ap.HasNext,
	}
}

type AdvertisementCursor struct {
	CreatedAt time.Time
	ID        string
}
