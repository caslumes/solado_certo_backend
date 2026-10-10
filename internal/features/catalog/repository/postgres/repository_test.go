package repository

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/caslumes/solado_certo_backend/internal/app"
	"github.com/caslumes/solado_certo_backend/internal/features/catalog"
	"github.com/caslumes/solado_certo_backend/internal/helpers"
	"github.com/caslumes/solado_certo_backend/internal/testutil"
	"github.com/jackc/pgx/v5/pgxpool"
)

type fixture struct {
	r        *postgresRepository
	pool     *pgxpool.Pool
	seller   testutil.User
	category testutil.Category
}

func newFixture(t *testing.T) fixture {
	t.Helper()
	pool := testutil.DB(t)
	return fixture{
		r:        NewPostgresRepository(pool),
		pool:     pool,
		seller:   testutil.InsertUser(t, pool),
		category: testutil.InsertCategory(t, pool, nil),
	}
}

func (f fixture) insertAd(t *testing.T, ad testutil.Ad) testutil.Ad {
	t.Helper()
	ad.SellerID = f.seller.ID
	if ad.CategoryID == "" {
		ad.CategoryID = f.category.ID
	}
	return testutil.InsertAd(t, f.pool, ad)
}

func (f fixture) insertImage(t *testing.T, adID, url string, position int, isCover bool) {
	t.Helper()
	_, err := f.pool.Exec(
		context.Background(),
		"INSERT INTO advertisement_images (advertisement_id, url, alt_text, position, is_cover) VALUES ($1, $2, $3, $4, $5)",
		adID, url, "Foto "+url, position, isCover,
	)
	if err != nil {
		t.Fatalf("insert image: %v", err)
	}
}

func (f fixture) list(t *testing.T, limit, offset uint, filters catalog.AdvertisementFilters) []*catalog.AdvertisementEntity {
	t.Helper()
	return f.listSorted(t, limit, offset, filters, catalog.AdvertisementSortNewest)
}

func (f fixture) listSorted(t *testing.T, limit, offset uint, filters catalog.AdvertisementFilters, sort catalog.AdvertisementSort) []*catalog.AdvertisementEntity {
	t.Helper()
	ads, err := f.r.GetAdvertisements(context.Background(), limit, offset, filters, sort)
	if err != nil {
		t.Fatalf("GetAdvertisements: %v", err)
	}
	return ads
}

func titles(ads []*catalog.AdvertisementEntity) []string {
	result := []string{}
	for _, ad := range ads {
		result = append(result, ad.Title)
	}
	return result
}

func equal(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

func TestGetAdvertisementsReturnsOnlyPublishedNewestFirst(t *testing.T) {
	f := newFixture(t)
	now := time.Now().UTC()
	f.insertAd(t, testutil.Ad{Title: "Antigo", CreatedAt: now.Add(-2 * time.Hour)})
	f.insertAd(t, testutil.Ad{Title: "Novo", CreatedAt: now})
	f.insertAd(t, testutil.Ad{Title: "Meio", CreatedAt: now.Add(-time.Hour)})
	for _, status := range []string{"draft", "paused", "archived", "removed"} {
		f.insertAd(t, testutil.Ad{Title: status, Status: status, CreatedAt: now.Add(time.Hour)})
	}

	got := titles(f.list(t, 10, 0, catalog.AdvertisementFilters{}))

	if want := []string{"Novo", "Meio", "Antigo"}; !equal(got, want) {
		t.Fatalf("titles = %v, want %v", got, want)
	}
}

func TestGetAdvertisementsSorts(t *testing.T) {
	f := newFixture(t)
	now := time.Now().UTC()
	f.insertAd(t, testutil.Ad{Title: "Barata antiga", Price: "50.00", CreatedAt: now.Add(-3 * time.Hour)})
	f.insertAd(t, testutil.Ad{Title: "Cara", Price: "300.00", CreatedAt: now.Add(-2 * time.Hour)})
	f.insertAd(t, testutil.Ad{Title: "Barata nova", Price: "50.00", CreatedAt: now.Add(-time.Hour)})
	f.insertAd(t, testutil.Ad{Title: "Média", Price: "120.00", CreatedAt: now})

	tests := map[catalog.AdvertisementSort][]string{
		catalog.AdvertisementSortNewest:    {"Média", "Barata nova", "Cara", "Barata antiga"},
		catalog.AdvertisementSortRelevance: {"Média", "Barata nova", "Cara", "Barata antiga"},
		catalog.AdvertisementSortPriceAsc:  {"Barata nova", "Barata antiga", "Média", "Cara"},
		catalog.AdvertisementSortPriceDesc: {"Cara", "Média", "Barata nova", "Barata antiga"},
	}

	for sort, want := range tests {
		t.Run(string(sort), func(t *testing.T) {
			if got := titles(f.listSorted(t, 10, 0, catalog.AdvertisementFilters{}, sort)); !equal(got, want) {
				t.Fatalf("titles = %v, want %v", got, want)
			}
		})
	}
}

func TestGetAdvertisementsSortsAcrossPages(t *testing.T) {
	f := newFixture(t)
	for _, price := range []string{"300.00", "9.00", "50.00", "20.00", "100.00"} {
		f.insertAd(t, testutil.Ad{Title: price, Price: price})
	}

	first := titles(f.listSorted(t, 2, 0, catalog.AdvertisementFilters{}, catalog.AdvertisementSortPriceAsc))
	second := titles(f.listSorted(t, 2, 2, catalog.AdvertisementFilters{}, catalog.AdvertisementSortPriceAsc))

	if !equal(first, []string{"9.00", "20.00"}) || !equal(second, []string{"50.00", "100.00"}) {
		t.Fatalf("pages = %v, %v", first, second)
	}
}

func TestGetAdvertisementsRejectsUnknownSort(t *testing.T) {
	f := newFixture(t)

	_, err := f.r.GetAdvertisements(context.Background(), 10, 0, catalog.AdvertisementFilters{}, "price; DROP TABLE advertisements")

	if !errors.Is(err, app.ErrInvalidInput) {
		t.Fatalf("err = %v, want %v", err, app.ErrInvalidInput)
	}
}

func TestGetAdvertisementsPaginates(t *testing.T) {
	f := newFixture(t)
	now := time.Now().UTC()
	for i, title := range []string{"A", "B", "C", "D", "E"} {
		f.insertAd(t, testutil.Ad{Title: title, CreatedAt: now.Add(-time.Duration(i) * time.Minute)})
	}

	tests := map[string]struct {
		limit, offset uint
		want          []string
	}{
		"first page":   {2, 0, []string{"A", "B"}},
		"second page":  {2, 2, []string{"C", "D"}},
		"partial page": {2, 4, []string{"E"}},
		"past the end": {2, 6, []string{}},
	}

	for name, tt := range tests {
		t.Run(name, func(t *testing.T) {
			if got := titles(f.list(t, tt.limit, tt.offset, catalog.AdvertisementFilters{})); !equal(got, tt.want) {
				t.Fatalf("titles = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestGetAdvertisementsAppliesEachFilter(t *testing.T) {
	f := newFixture(t)
	otherCategory := testutil.InsertCategory(t, f.pool, nil)
	f.insertAd(t, testutil.Ad{
		Title: "Palmilha ortopédica", Description: "Para fascite", Price: "150.00", CategoryID: otherCategory.ID,
		PrescriptionRequired: true, CustomMadeAllowed: true,
		Side: helpers.StringPtr("left"), Size: helpers.StringPtr("38"), Material: helpers.StringPtr("EVA"),
		Rigidity: helpers.StringPtr("rigid"), UsageIndication: helpers.StringPtr("running"),
	})
	f.insertAd(t, testutil.Ad{
		Title: "Calcanheira", Description: "Silicone macio", Price: "50.00",
		Side: helpers.StringPtr("right"), Size: helpers.StringPtr("40"), Material: helpers.StringPtr("silicone"),
		Rigidity: helpers.StringPtr("soft"), UsageIndication: helpers.StringPtr("daily"),
	})

	tests := map[string]struct {
		filters catalog.AdvertisementFilters
		want    []string
	}{
		"query in title, any case":  {catalog.AdvertisementFilters{Query: helpers.StringPtr("PALMILHA")}, []string{"Palmilha ortopédica"}},
		"query in description":      {catalog.AdvertisementFilters{Query: helpers.StringPtr("silicone")}, []string{"Calcanheira"}},
		"category":                  {catalog.AdvertisementFilters{Category: &otherCategory.ID}, []string{"Palmilha ortopédica"}},
		"minimum price":             {catalog.AdvertisementFilters{PriceMin: ptr(100.0)}, []string{"Palmilha ortopédica"}},
		"maximum price":             {catalog.AdvertisementFilters{PriceMax: ptr(100.0)}, []string{"Calcanheira"}},
		"price range is inclusive":  {catalog.AdvertisementFilters{PriceMin: ptr(50.0), PriceMax: ptr(150.0)}, []string{"Palmilha ortopédica", "Calcanheira"}},
		"prescription required":     {catalog.AdvertisementFilters{PrescriptionRequired: ptr(true)}, []string{"Palmilha ortopédica"}},
		"prescription not required": {catalog.AdvertisementFilters{PrescriptionRequired: ptr(false)}, []string{"Calcanheira"}},
		"custom made allowed":       {catalog.AdvertisementFilters{CustomMadeAllowed: ptr(true)}, []string{"Palmilha ortopédica"}},
		"side":                      {catalog.AdvertisementFilters{Side: helpers.StringPtr("right")}, []string{"Calcanheira"}},
		"size":                      {catalog.AdvertisementFilters{Size: helpers.StringPtr("38")}, []string{"Palmilha ortopédica"}},
		"material":                  {catalog.AdvertisementFilters{Material: helpers.StringPtr("silicone")}, []string{"Calcanheira"}},
		"rigidity":                  {catalog.AdvertisementFilters{Rigidity: helpers.StringPtr("rigid")}, []string{"Palmilha ortopédica"}},
		"usage indication":          {catalog.AdvertisementFilters{UsageIndication: helpers.StringPtr("daily")}, []string{"Calcanheira"}},
		"combined filters":          {catalog.AdvertisementFilters{Query: helpers.StringPtr("palmilha"), Side: helpers.StringPtr("right")}, []string{}},
	}

	for name, tt := range tests {
		t.Run(name, func(t *testing.T) {
			got := titles(f.list(t, 10, 0, tt.filters))
			if !sameItems(got, tt.want) {
				t.Fatalf("titles = %v, want %v", got, tt.want)
			}
		})
	}
}

func (f fixture) insertPurpose(t *testing.T, code, name string) string {
	t.Helper()
	var id string
	err := f.pool.QueryRow(context.Background(), "INSERT INTO podological_purposes (code, name) VALUES ($1, $2) RETURNING id", code, name).Scan(&id)
	if err != nil {
		t.Fatalf("insert purpose: %v", err)
	}
	return id
}

func (f fixture) insertCondition(t *testing.T, name string, active bool) string {
	t.Helper()
	var id string
	err := f.pool.QueryRow(context.Background(), "INSERT INTO podological_conditions (name, active) VALUES ($1, $2) RETURNING id", name, active).Scan(&id)
	if err != nil {
		t.Fatalf("insert condition: %v", err)
	}
	return id
}

func (f fixture) link(t *testing.T, table, column, adID, id string) {
	t.Helper()
	_, err := f.pool.Exec(context.Background(), "INSERT INTO "+table+" (advertisement_id, "+column+") VALUES ($1, $2)", adID, id)
	if err != nil {
		t.Fatalf("link %s: %v", table, err)
	}
}

func TestGetAdvertisementsFiltersByPurposeAndCondition(t *testing.T) {
	f := newFixture(t)
	prevention := f.insertPurpose(t, "prevention", "Prevenção")
	correction := f.insertPurpose(t, "correction", "Correção")
	f.insertPurpose(t, "pain_relief", "Alívio de dor")
	fasciitis := f.insertCondition(t, "Fascite plantar", true)
	flatFoot := f.insertCondition(t, "Pé plano", true)

	insole := f.insertAd(t, testutil.Ad{Title: "Palmilha", Material: helpers.StringPtr("EVA")})
	f.link(t, "advertisement_purposes", "purpose_id", insole.ID, prevention)
	f.link(t, "advertisement_purposes", "purpose_id", insole.ID, correction)
	f.link(t, "advertisement_conditions", "condition_id", insole.ID, fasciitis)
	heelCup := f.insertAd(t, testutil.Ad{Title: "Calcanheira", Material: helpers.StringPtr("silicone")})
	f.link(t, "advertisement_purposes", "purpose_id", heelCup.ID, correction)
	f.link(t, "advertisement_conditions", "condition_id", heelCup.ID, flatFoot)
	f.insertAd(t, testutil.Ad{Title: "Sem classificação"})

	tests := map[string]struct {
		filters catalog.AdvertisementFilters
		want    []string
	}{
		"one purpose":           {catalog.AdvertisementFilters{Purposes: []string{"prevention"}}, []string{"Palmilha"}},
		"any of the purposes":   {catalog.AdvertisementFilters{Purposes: []string{"prevention", "correction"}}, []string{"Palmilha", "Calcanheira"}},
		"purpose with no ads":   {catalog.AdvertisementFilters{Purposes: []string{"pain_relief"}}, []string{}},
		"unknown purpose":       {catalog.AdvertisementFilters{Purposes: []string{"magic"}}, []string{}},
		"one condition":         {catalog.AdvertisementFilters{Conditions: []string{flatFoot}}, []string{"Calcanheira"}},
		"any of the conditions": {catalog.AdvertisementFilters{Conditions: []string{flatFoot, fasciitis}}, []string{"Palmilha", "Calcanheira"}},
		"purpose and condition": {catalog.AdvertisementFilters{Purposes: []string{"correction"}, Conditions: []string{fasciitis}}, []string{"Palmilha"}},
		"purpose and attribute": {catalog.AdvertisementFilters{Purposes: []string{"correction"}, Material: helpers.StringPtr("silicone")}, []string{"Calcanheira"}},
		"no filters":            {catalog.AdvertisementFilters{}, []string{"Palmilha", "Calcanheira", "Sem classificação"}},
	}

	for name, tt := range tests {
		t.Run(name, func(t *testing.T) {
			got := titles(f.list(t, 10, 0, tt.filters))
			if !sameItems(got, tt.want) {
				t.Fatalf("titles = %v, want %v", got, tt.want)
			}
			count, err := f.r.CountAdvertisements(context.Background(), tt.filters)
			if err != nil || count != uint(len(tt.want)) {
				t.Fatalf("count = %d, err = %v, want %d", count, err, len(tt.want))
			}
		})
	}
}

func TestGetPurposesIsSortedByName(t *testing.T) {
	f := newFixture(t)
	f.insertPurpose(t, "prevention", "Prevenção")
	f.insertPurpose(t, "correction", "Correção")

	purposes, err := f.r.GetPurposes(context.Background())
	if err != nil {
		t.Fatalf("GetPurposes: %v", err)
	}

	if len(purposes) != 2 || purposes[0].Code != "correction" || purposes[1].Code != "prevention" || purposes[0].ID == "" {
		t.Fatalf("purposes = %+v", purposes)
	}
}

func TestGetConditionsReturnsOnlyActiveSortedByName(t *testing.T) {
	f := newFixture(t)
	f.insertCondition(t, "Pé plano", true)
	f.insertCondition(t, "Fascite plantar", true)
	f.insertCondition(t, "Condição desativada", false)

	conditions, err := f.r.GetConditions(context.Background())
	if err != nil {
		t.Fatalf("GetConditions: %v", err)
	}

	if len(conditions) != 2 || conditions[0].Name != "Fascite plantar" || conditions[1].Name != "Pé plano" {
		t.Fatalf("conditions = %+v", conditions)
	}
}

func ptr[T any](v T) *T {
	return &v
}

func sameItems(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	seen := map[string]int{}
	for _, s := range a {
		seen[s]++
	}
	for _, s := range b {
		seen[s]--
	}
	for _, n := range seen {
		if n != 0 {
			return false
		}
	}
	return true
}

func TestGetAdvertisementsIncludesImagesInOrder(t *testing.T) {
	f := newFixture(t)
	ad := f.insertAd(t, testutil.Ad{Title: "Palmilha"})
	f.insertImage(t, ad.ID, "second.jpg", 2, false)
	f.insertImage(t, ad.ID, "first.jpg", 1, true)

	ads := f.list(t, 10, 0, catalog.AdvertisementFilters{})

	if len(ads) != 1 || len(ads[0].Images) != 2 {
		t.Fatalf("ads = %+v", ads)
	}
	if first := ads[0].Images[0]; first.URL != "first.jpg" || !first.IsCover || first.Order != 1 || first.AltText != "Foto first.jpg" {
		t.Fatalf("first image = %+v", first)
	}
	if ads[0].Images[1].URL != "second.jpg" {
		t.Fatalf("second image = %+v", ads[0].Images[1])
	}
}

func TestGetAdvertisementByIDReturnsDetail(t *testing.T) {
	f := newFixture(t)
	ad := f.insertAd(t, testutil.Ad{
		Title: "Palmilha", Description: "Para fascite", Price: "89.90", Stock: 4,
		PrescriptionRequired: true, Material: helpers.StringPtr("EVA"),
	})
	f.insertImage(t, ad.ID, "b.jpg", 2, false)
	f.insertImage(t, ad.ID, "a.jpg", 1, true)

	got, err := f.r.GetAdvertisementByID(context.Background(), ad.ID)
	if err != nil {
		t.Fatalf("GetAdvertisementByID: %v", err)
	}

	if got.Title != "Palmilha" || got.Description != "Para fascite" || got.Price != "89.90" || got.Stock != 4 ||
		got.Status != "published" || !got.PrescriptionRequired || got.SellerID != f.seller.ID {
		t.Fatalf("ad = %+v", got)
	}
	if got.Material == nil || *got.Material != "EVA" || got.Side != nil {
		t.Fatalf("material = %v, side = %v", got.Material, got.Side)
	}
	if len(got.Images) != 2 || got.Images[0].URL != "a.jpg" || got.Images[1].URL != "b.jpg" {
		t.Fatalf("images = %+v", got.Images)
	}
}

func TestPriceIsReadAsDecimalWithTwoPlaces(t *testing.T) {
	f := newFixture(t)
	tests := map[string]string{"420": "420.00", "89.9": "89.90", "0.05": "0.05", "12345678.99": "12345678.99"}

	for stored, want := range tests {
		t.Run(stored, func(t *testing.T) {
			ad := f.insertAd(t, testutil.Ad{Price: stored})

			got, err := f.r.GetAdvertisementByID(context.Background(), ad.ID)
			if err != nil {
				t.Fatalf("GetAdvertisementByID: %v", err)
			}
			if got.Price != want {
				t.Fatalf("price = %q, want %q", got.Price, want)
			}
		})
	}
}

func TestGetAdvertisementByIDReturnsNotFound(t *testing.T) {
	f := newFixture(t)

	_, err := f.r.GetAdvertisementByID(context.Background(), "00000000-0000-0000-0000-000000000000")

	if !errors.Is(err, app.ErrAdvertisementNotFound) {
		t.Fatalf("err = %v, want %v", err, app.ErrAdvertisementNotFound)
	}
}

func TestCountAdvertisementsMatchesFilteredList(t *testing.T) {
	f := newFixture(t)
	for range 3 {
		f.insertAd(t, testutil.Ad{Material: helpers.StringPtr("EVA")})
	}
	f.insertAd(t, testutil.Ad{Material: helpers.StringPtr("silicone")})
	f.insertAd(t, testutil.Ad{Material: helpers.StringPtr("EVA"), Status: "draft"})

	tests := map[string]struct {
		filters catalog.AdvertisementFilters
		want    uint
	}{
		"no filters": {catalog.AdvertisementFilters{}, 4},
		"material":   {catalog.AdvertisementFilters{Material: helpers.StringPtr("EVA")}, 3},
		"no match":   {catalog.AdvertisementFilters{Material: helpers.StringPtr("couro")}, 0},
	}

	for name, tt := range tests {
		t.Run(name, func(t *testing.T) {
			count, err := f.r.CountAdvertisements(context.Background(), tt.filters)
			if err != nil {
				t.Fatalf("CountAdvertisements: %v", err)
			}
			listed := uint(len(f.list(t, 100, 0, tt.filters)))
			if count != tt.want || listed != tt.want {
				t.Fatalf("count = %d, listed = %d, want %d", count, listed, tt.want)
			}
		})
	}
}

func TestAdvertisementsIncludeIDAndCategory(t *testing.T) {
	f := newFixture(t)
	ad := f.insertAd(t, testutil.Ad{})

	listed := f.list(t, 10, 0, catalog.AdvertisementFilters{})
	detail, err := f.r.GetAdvertisementByID(context.Background(), ad.ID)
	if err != nil {
		t.Fatalf("GetAdvertisementByID: %v", err)
	}

	for name, got := range map[string]*catalog.AdvertisementEntity{"list": listed[0], "detail": detail} {
		if got.ID != ad.ID || got.CategoryID != f.category.ID {
			t.Fatalf("%s: id = %q, category = %q, want %q and %q", name, got.ID, got.CategoryID, ad.ID, f.category.ID)
		}
	}
}

func TestImageWithoutAltTextIsReadAsEmpty(t *testing.T) {
	f := newFixture(t)
	ad := f.insertAd(t, testutil.Ad{})
	_, err := f.pool.Exec(context.Background(), "INSERT INTO advertisement_images (advertisement_id, url) VALUES ($1, 'photo.jpg')", ad.ID)
	if err != nil {
		t.Fatalf("insert image: %v", err)
	}

	got, err := f.r.GetAdvertisementByID(context.Background(), ad.ID)
	if err != nil {
		t.Fatalf("GetAdvertisementByID: %v", err)
	}
	if len(got.Images) != 1 || got.Images[0].URL != "photo.jpg" || got.Images[0].AltText != "" {
		t.Fatalf("images = %+v", got.Images)
	}
	if listed := f.list(t, 10, 0, catalog.AdvertisementFilters{}); len(listed) != 1 || len(listed[0].Images) != 1 {
		t.Fatalf("listed = %+v", listed)
	}
}

func TestGetCategoriesReturnsAllWithParents(t *testing.T) {
	f := newFixture(t)
	child := testutil.InsertCategory(t, f.pool, &f.category.ID)

	categories, err := f.r.GetCategories(context.Background())
	if err != nil {
		t.Fatalf("GetCategories: %v", err)
	}

	byID := map[string]*catalog.CategoryEntity{}
	for _, category := range categories {
		byID[category.ID] = category
	}
	root, ok := byID[f.category.ID]
	if len(categories) != 2 || !ok || root.ParentCategoryID != nil || root.Slug != f.category.Slug || !root.Active {
		t.Fatalf("categories = %+v", categories)
	}
	if got := byID[child.ID]; got == nil || got.ParentCategoryID == nil || *got.ParentCategoryID != f.category.ID {
		t.Fatalf("child = %+v", got)
	}
}
