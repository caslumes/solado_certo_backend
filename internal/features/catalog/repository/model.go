package repository

import (
	"time"

	"github.com/caslumes/solado_certo_backend/internal/features/catalog"
)

type AdvertisementImageModel struct {
	URL     string
	AltText string
	Order   int
	IsCover bool
}

func (m *AdvertisementImageModel) ToEntity() *catalog.AdvertisementImageEntity {
	return &catalog.AdvertisementImageEntity{
		URL:     m.URL,
		AltText: m.AltText,
		Order:   m.Order,
		IsCover: m.IsCover,
	}
}

type AdvertisementModel struct {
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
	Images               []*AdvertisementImageModel
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

func (m *AdvertisementModel) ToEntity() *catalog.AdvertisementEntity {
	images := make([]*catalog.AdvertisementImageEntity, len(m.Images))
	for i, image := range m.Images {
		images[i] = image.ToEntity()
	}

	return &catalog.AdvertisementEntity{
		ID:                   m.ID,
		SellerID:             m.SellerID,
		CategoryID:           m.CategoryID,
		Title:                m.Title,
		Description:          m.Description,
		Price:                m.Price,
		Stock:                m.Stock,
		Status:               m.Status,
		PrescriptionRequired: m.PrescriptionRequired,
		CustomMadeAllowed:    m.CustomMadeAllowed,
		Images:               images,
		Side:                 m.Side,
		Size:                 m.Size,
		Material:             m.Material,
		Rigidity:             m.Rigidity,
		UsageIndication:      m.UsageIndication,
		SafetyAlerts:         m.SafetyAlerts,
		Contraindications:    m.Contraindications,
		ShippingNotes:        m.ShippingNotes,
		CreatedAt:            m.CreatedAt,
		UpdatedAt:            m.UpdatedAt,
		RemovedAt:            m.RemovedAt,
	}
}

type CategoryModel struct {
	ID               string
	ParentCategoryID *string
	Name             string
	Slug             string
	Active           bool
}

func (m *CategoryModel) ToEntity() *catalog.CategoryEntity {
	return &catalog.CategoryEntity{
		ID:               m.ID,
		ParentCategoryID: m.ParentCategoryID,
		Name:             m.Name,
		Slug:             m.Slug,
		Active:           m.Active,
	}
}
