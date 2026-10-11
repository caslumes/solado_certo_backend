package catalog

import (
	"context"
	"fmt"

	"github.com/caslumes/solado_certo_backend/internal/app"
	"github.com/caslumes/solado_certo_backend/internal/helpers"
)

type GetAdvertisementsUseCase interface {
	GetAdvertisements(ctx context.Context, input GetAdvertisementsInput) (AdvertisementPage, error)
}

type GetAdvertisementUseCase interface {
	GetAdvertisement(ctx context.Context, id string) (*AdvertisementDetailEntity, error)
}

type GetCategoriesUseCase interface {
	GetCategories(ctx context.Context) (*CategoriesEntity, error)
}

type GetPurposesUseCase interface {
	GetPurposes(ctx context.Context) ([]*PurposeEntity, error)
}

type GetConditionsUseCase interface {
	GetConditions(ctx context.Context) ([]*ConditionEntity, error)
}

type repository interface {
	GetAdvertisements(ctx context.Context, limit uint, offset uint, filters AdvertisementFilters, sort AdvertisementSort) ([]*AdvertisementEntity, error)
	GetAdvertisementByID(ctx context.Context, id string) (*AdvertisementDetailEntity, error)
	CountAdvertisements(ctx context.Context, filters AdvertisementFilters) (uint, error)
	GetCategories(ctx context.Context) ([]*CategoryEntity, error)
	GetPurposes(ctx context.Context) ([]*PurposeEntity, error)
	GetConditions(ctx context.Context) ([]*ConditionEntity, error)
}

type service struct {
	repo repository
}

func NewService(repo repository) *service {
	return &service{
		repo: repo,
	}
}

func (s *service) GetAdvertisements(ctx context.Context, input GetAdvertisementsInput) (AdvertisementPage, error) {
	page := input.Page
	if page == 0 {
		page = 1
	}

	limit := input.Limit
	if limit == 0 {
		limit = 10
	}

	filters := AdvertisementFilters{}
	if input.Filters != nil {
		filters = *input.Filters
	}
	sort := input.Sort
	if sort == "" {
		sort = AdvertisementSortRelevance
	}
	if !sort.IsValid() {
		return AdvertisementPage{}, fmt.Errorf("%w: invalid sort", app.ErrInvalidInput)
	}
	if filters.Category != nil && !helpers.IsUUID(*filters.Category) {
		return AdvertisementPage{}, fmt.Errorf("%w: invalid category id", app.ErrInvalidInput)
	}
	for _, condition := range filters.Conditions {
		if !helpers.IsUUID(condition) {
			return AdvertisementPage{}, fmt.Errorf("%w: invalid condition id", app.ErrInvalidInput)
		}
	}

	totalItems, err := s.repo.CountAdvertisements(ctx, filters)
	if err != nil {
		return AdvertisementPage{}, err
	}

	offset := (page - 1) * limit
	ads, err := s.repo.GetAdvertisements(ctx, limit, offset, filters, sort)
	if err != nil {
		return AdvertisementPage{}, err
	}

	totalPages := uint(0)
	if totalItems > 0 {
		totalPages = totalItems / limit
		if totalItems%limit != 0 {
			totalPages++
		}
	}

	return AdvertisementPage{
		Items:       ads,
		CurrentPage: page,
		TotalPages:  totalPages,
		TotalItems:  totalItems,
		HasNext:     page < totalPages,
		HasPrevious: page > 1,
	}, nil
}

func (s *service) GetAdvertisement(ctx context.Context, id string) (*AdvertisementDetailEntity, error) {
	if !helpers.IsUUID(id) {
		return nil, app.ErrAdvertisementNotFound
	}

	ad, err := s.repo.GetAdvertisementByID(ctx, id)
	if err != nil {
		return nil, err
	}

	return ad, nil
}

func (s *service) GetCategories(ctx context.Context) (*CategoriesEntity, error) {
	categories, err := s.repo.GetCategories(ctx)
	if err != nil {
		return nil, err
	}

	return &CategoriesEntity{
		Categories: groupCategories(categories),
	}, nil
}

func (s *service) GetPurposes(ctx context.Context) ([]*PurposeEntity, error) {
	return s.repo.GetPurposes(ctx)
}

func (s *service) GetConditions(ctx context.Context) ([]*ConditionEntity, error) {
	return s.repo.GetConditions(ctx)
}

func groupCategories(categories []*CategoryEntity) []*CategoryEntity {
	categoryMap := make(map[string]*CategoryEntity)
	var rootCategories []*CategoryEntity

	for _, category := range categories {
		category.Children = []*CategoryEntity{}
		categoryMap[category.ID] = category
	}

	for _, category := range categories {
		if category.ParentCategoryID != nil {
			parent, exists := categoryMap[*category.ParentCategoryID]
			if exists {
				parent.Children = append(parent.Children, category)
			}
		} else {
			rootCategories = append(rootCategories, category)
		}
	}

	return rootCategories
}
