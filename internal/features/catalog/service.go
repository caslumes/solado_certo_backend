package catalog

import (
	"context"
)

type GetAdvertisementsUseCase interface {
	GetAdvertisements(ctx context.Context, input GetAdvertisementsInput) (AdvertisementPage, error)
}

type GetCategoriesUseCase interface {
	GetCategories(ctx context.Context) (*CategoriesEntity, error)
}

type repository interface {
	GetAdvertisements(ctx context.Context, limit uint, offset uint, filters AdvertisementFilters) ([]*AdvertisementEntity, error)
	CountAdvertisements(ctx context.Context) (uint, error)
	GetCategories(ctx context.Context) ([]*CategoryEntity, error)
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

	totalItems, err := s.repo.CountAdvertisements(ctx)
	if err != nil {
		return AdvertisementPage{}, err
	}

	offset := (page - 1) * limit
	ads, err := s.repo.GetAdvertisements(ctx, limit, offset, *input.Filters)
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

func (s *service) GetCategories(ctx context.Context) (*CategoriesEntity, error) {
	categories, err := s.repo.GetCategories(ctx)
	if err != nil {
		return nil, err
	}

	return &CategoriesEntity{
		Categories: groupCategories(categories),
	}, nil
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
