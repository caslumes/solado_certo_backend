package catalog

import (
	"context"
	"errors"
	"testing"

	"github.com/caslumes/solado_certo_backend/internal/app"
	"github.com/caslumes/solado_certo_backend/internal/helpers"
)

var errDatabase = errors.New("database is down")

type fakeRepository struct {
	ads        map[string]*AdvertisementEntity
	total      uint
	categories []*CategoryEntity
	err        error

	gotLimit   uint
	gotOffset  uint
	gotFilters AdvertisementFilters
}

func (f *fakeRepository) GetAdvertisements(_ context.Context, limit uint, offset uint, filters AdvertisementFilters) ([]*AdvertisementEntity, error) {
	f.gotLimit, f.gotOffset, f.gotFilters = limit, offset, filters
	if f.err != nil {
		return nil, f.err
	}
	ads := []*AdvertisementEntity{}
	for _, ad := range f.ads {
		ads = append(ads, ad)
	}
	return ads, nil
}

func (f *fakeRepository) GetAdvertisementByID(_ context.Context, id string) (*AdvertisementEntity, error) {
	if f.err != nil {
		return nil, f.err
	}
	ad, ok := f.ads[id]
	if !ok {
		return nil, app.ErrAdvertisementNotFound
	}
	return ad, nil
}

func (f *fakeRepository) CountAdvertisements(context.Context) (uint, error) {
	if f.err != nil {
		return 0, f.err
	}
	return f.total, nil
}

func (f *fakeRepository) GetCategories(context.Context) ([]*CategoryEntity, error) {
	if f.err != nil {
		return nil, f.err
	}
	return f.categories, nil
}

func getAdvertisements(t *testing.T, repository *fakeRepository, input GetAdvertisementsInput) AdvertisementPage {
	t.Helper()
	if input.Filters == nil {
		input.Filters = &AdvertisementFilters{}
	}
	page, err := NewService(repository).GetAdvertisements(context.Background(), input)
	if err != nil {
		t.Fatalf("GetAdvertisements: %v", err)
	}
	return page
}

func TestGetAdvertisementsAppliesDefaultPageAndLimit(t *testing.T) {
	repository := &fakeRepository{total: 25}

	page := getAdvertisements(t, repository, GetAdvertisementsInput{})

	if repository.gotLimit != 10 || repository.gotOffset != 0 {
		t.Fatalf("limit = %d, offset = %d, want 10 and 0", repository.gotLimit, repository.gotOffset)
	}
	if page.CurrentPage != 1 {
		t.Fatalf("current page = %d, want 1", page.CurrentPage)
	}
}

func TestGetAdvertisementsPassesOffsetAndFilters(t *testing.T) {
	repository := &fakeRepository{total: 50}
	filters := &AdvertisementFilters{Query: helpers.StringPtr("palmilha"), Side: helpers.StringPtr("left")}

	getAdvertisements(t, repository, GetAdvertisementsInput{Page: 3, Limit: 20, Filters: filters})

	if repository.gotLimit != 20 || repository.gotOffset != 40 {
		t.Fatalf("limit = %d, offset = %d, want 20 and 40", repository.gotLimit, repository.gotOffset)
	}
	if repository.gotFilters.Query != filters.Query || repository.gotFilters.Side != filters.Side {
		t.Fatalf("filters = %+v, want %+v", repository.gotFilters, *filters)
	}
}

func TestGetAdvertisementsComputesPagination(t *testing.T) {
	tests := map[string]struct {
		total, page, limit     uint
		wantPages              uint
		wantNext, wantPrevious bool
	}{
		"no items":               {total: 0, page: 1, limit: 10, wantPages: 0},
		"single partial page":    {total: 3, page: 1, limit: 10, wantPages: 1},
		"exact multiple":         {total: 20, page: 1, limit: 10, wantPages: 2, wantNext: true},
		"first of three":         {total: 25, page: 1, limit: 10, wantPages: 3, wantNext: true},
		"middle page":            {total: 25, page: 2, limit: 10, wantPages: 3, wantNext: true, wantPrevious: true},
		"last partial page":      {total: 25, page: 3, limit: 10, wantPages: 3, wantPrevious: true},
		"page beyond last":       {total: 25, page: 5, limit: 10, wantPages: 3, wantPrevious: true},
		"limit of one":           {total: 2, page: 1, limit: 1, wantPages: 2, wantNext: true},
		"limit larger than data": {total: 5, page: 1, limit: 100, wantPages: 1},
	}

	for name, tt := range tests {
		t.Run(name, func(t *testing.T) {
			page := getAdvertisements(t, &fakeRepository{total: tt.total}, GetAdvertisementsInput{Page: tt.page, Limit: tt.limit})

			if page.TotalItems != tt.total || page.TotalPages != tt.wantPages || page.CurrentPage != tt.page {
				t.Fatalf("page = %+v, want %d items in %d pages", page, tt.total, tt.wantPages)
			}
			if page.HasNext != tt.wantNext || page.HasPrevious != tt.wantPrevious {
				t.Fatalf("has next = %v, has previous = %v, want %v and %v", page.HasNext, page.HasPrevious, tt.wantNext, tt.wantPrevious)
			}
		})
	}
}

func TestGetAdvertisementsReturnsItems(t *testing.T) {
	repository := &fakeRepository{
		total: 1,
		ads:   map[string]*AdvertisementEntity{"ad-1": {Title: "Palmilha ortopédica"}},
	}

	page := getAdvertisements(t, repository, GetAdvertisementsInput{})

	if len(page.Items) != 1 || page.Items[0].Title != "Palmilha ortopédica" {
		t.Fatalf("items = %+v", page.Items)
	}
}

func TestGetAdvertisementsPropagatesRepositoryErrors(t *testing.T) {
	_, err := NewService(&fakeRepository{err: errDatabase}).GetAdvertisements(context.Background(), GetAdvertisementsInput{Filters: &AdvertisementFilters{}})
	if !errors.Is(err, errDatabase) {
		t.Fatalf("err = %v, want %v", err, errDatabase)
	}
}

func TestGetAdvertisementReturnsAd(t *testing.T) {
	repository := &fakeRepository{ads: map[string]*AdvertisementEntity{"ad-1": {Title: "Palmilha ortopédica"}}}

	ad, err := NewService(repository).GetAdvertisement(context.Background(), "ad-1")
	if err != nil {
		t.Fatalf("GetAdvertisement: %v", err)
	}
	if ad.Title != "Palmilha ortopédica" {
		t.Fatalf("ad = %+v", ad)
	}
}

func TestGetAdvertisementReturnsNotFound(t *testing.T) {
	_, err := NewService(&fakeRepository{}).GetAdvertisement(context.Background(), "missing")
	if !errors.Is(err, app.ErrAdvertisementNotFound) {
		t.Fatalf("err = %v, want %v", err, app.ErrAdvertisementNotFound)
	}
}

func TestGetAdvertisementPropagatesRepositoryErrors(t *testing.T) {
	_, err := NewService(&fakeRepository{err: errDatabase}).GetAdvertisement(context.Background(), "ad-1")
	if !errors.Is(err, errDatabase) {
		t.Fatalf("err = %v, want %v", err, errDatabase)
	}
}

func TestGetCategoriesBuildsTree(t *testing.T) {
	repository := &fakeRepository{categories: []*CategoryEntity{
		{ID: "heel-cup", ParentCategoryID: helpers.StringPtr("insoles"), Name: "Calcanheiras"},
		{ID: "insoles", Name: "Palmilhas"},
		{ID: "silicone-heel-cup", ParentCategoryID: helpers.StringPtr("heel-cup"), Name: "Calcanheiras de silicone"},
		{ID: "orthoses", Name: "Órteses"},
		{ID: "orphan", ParentCategoryID: helpers.StringPtr("missing"), Name: "Sem pai"},
	}}

	categories, err := NewService(repository).GetCategories(context.Background())
	if err != nil {
		t.Fatalf("GetCategories: %v", err)
	}

	roots := categories.Categories
	if len(roots) != 2 || roots[0].ID != "insoles" || roots[1].ID != "orthoses" {
		t.Fatalf("roots = %+v, want insoles and orthoses", roots)
	}
	insoles := roots[0]
	if len(insoles.Children) != 1 || insoles.Children[0].ID != "heel-cup" {
		t.Fatalf("insoles children = %+v, want heel-cup", insoles.Children)
	}
	heelCup := insoles.Children[0]
	if len(heelCup.Children) != 1 || heelCup.Children[0].ID != "silicone-heel-cup" {
		t.Fatalf("heel-cup children = %+v, want silicone-heel-cup", heelCup.Children)
	}
	if roots[1].Children == nil || len(roots[1].Children) != 0 {
		t.Fatalf("leaf children = %#v, want an empty list", roots[1].Children)
	}
}

func TestGetCategoriesOutputKeepsNesting(t *testing.T) {
	repository := &fakeRepository{categories: []*CategoryEntity{
		{ID: "insoles", Name: "Palmilhas", Slug: "palmilhas", Active: true},
		{ID: "heel-cup", ParentCategoryID: helpers.StringPtr("insoles"), Name: "Calcanheiras", Slug: "calcanheiras", Active: true},
	}}

	categories, err := NewService(repository).GetCategories(context.Background())
	if err != nil {
		t.Fatalf("GetCategories: %v", err)
	}

	output := categories.ToOutput().Categories
	if len(output) != 1 || output[0].Slug != "palmilhas" || len(output[0].Children) != 1 {
		t.Fatalf("output = %+v", output)
	}
	child := output[0].Children[0]
	if child.Slug != "calcanheiras" || child.ParentCategoryID == nil || *child.ParentCategoryID != "insoles" {
		t.Fatalf("child = %+v", child)
	}
}

func TestGetCategoriesPropagatesRepositoryErrors(t *testing.T) {
	_, err := NewService(&fakeRepository{err: errDatabase}).GetCategories(context.Background())
	if !errors.Is(err, errDatabase) {
		t.Fatalf("err = %v, want %v", err, errDatabase)
	}
}
