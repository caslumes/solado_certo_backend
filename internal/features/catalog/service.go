package catalog

import (
	"context"
	"encoding/base64"
	"encoding/json"
)

type GetAdvertisementsUseCase interface {
	GetAdvertisements(ctx context.Context, input GetAdvertisementsInput) (AdvertisementPage, error)
}

type GetCategoriesUseCase interface {
	GetCategories(ctx context.Context) (*CategoriesEntity, error)
}

type repository interface {
	GetAdvertisements(ctx context.Context, limit uint, cursor *AdvertisementCursor) ([]*AdvertisementEntity, error)
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
	limit := uint(10)
	if input.Limit != nil {
		limit = *input.Limit
	}

	var cursor *AdvertisementCursor
	if input.Cursor != nil {
		decodedCursor, err := decodeCursor(*input.Cursor)
		if err != nil {
			return AdvertisementPage{}, err
		}
		cursor = &decodedCursor
	}

	ads, err := s.repo.GetAdvertisements(ctx, limit+1, cursor)
	if err != nil {
		return AdvertisementPage{}, err
	}

	hasNext := len(ads) > int(limit)
	if hasNext {
		ads = ads[:len(ads)-1]
	}

	var nextCursor string
	if hasNext {
		lastAd := ads[len(ads)-1]
		nextCursor = encodeCursor(AdvertisementCursor{
			CreatedAt: lastAd.CreatedAt,
			ID:        lastAd.SellerID,
		})
	}

	return AdvertisementPage{
		Items:      ads,
		NextCursor: &nextCursor,
		HasNext:    hasNext,
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

func encodeCursor(ac AdvertisementCursor) string {
	payload, _ := json.Marshal(AdvertisementCursor{
		CreatedAt: ac.CreatedAt,
		ID:        ac.ID,
	})

	return base64.RawURLEncoding.EncodeToString(payload)
}

func decodeCursor(encoded string) (AdvertisementCursor, error) {
	var ac AdvertisementCursor
	payload, err := base64.RawURLEncoding.DecodeString(encoded)
	if err != nil {
		return ac, err
	}

	err = json.Unmarshal(payload, &ac)
	if err != nil {
		return ac, err
	}

	return ac, nil
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
