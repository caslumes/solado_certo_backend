package repository

import (
	"context"
	"errors"
	"fmt"
	"sync/atomic"
	"testing"
	"time"

	"github.com/caslumes/solado_certo_backend/internal/app"
	"github.com/caslumes/solado_certo_backend/internal/features/catalog"
	"github.com/caslumes/solado_certo_backend/internal/helpers"
	"github.com/caslumes/solado_certo_backend/internal/testutil"
	"github.com/jackc/pgx/v5/pgxpool"
)

var orderSequence atomic.Int64

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

func (f fixture) insertAddress(t *testing.T, userID, state string, isDefault bool) {
	t.Helper()
	now := time.Now().UTC()
	_, err := f.pool.Exec(
		context.Background(),
		"INSERT INTO addresses (user_id, receiver, zip_code, street, number, district, city, state, is_default, created_at, updated_at) "+
			"VALUES ($1, 'Recebedor', '01001000', 'Rua Teste', '1', 'Centro', 'Cidade', $2, $3, $4, $4)",
		userID, state, isDefault, now,
	)
	if err != nil {
		t.Fatalf("insert address: %v", err)
	}
}

func (f fixture) insertReview(t *testing.T, adID string, productRating int) {
	t.Helper()
	buyer := testutil.InsertUser(t, f.pool)
	f.insertAddress(t, buyer.ID, "RJ", true)

	now := time.Now().UTC()
	n := orderSequence.Add(1)
	var orderID string
	err := f.pool.QueryRow(
		context.Background(),
		"INSERT INTO orders (buyer_id, shipping_address_id, number, status, subtotal, total, created_at, updated_at) "+
			"SELECT $1, id, $2, 'paid', 100, 100, $3, $3 FROM addresses WHERE user_id = $1 RETURNING id",
		buyer.ID, fmt.Sprintf("ORD-%d", n), now,
	).Scan(&orderID)
	if err != nil {
		t.Fatalf("insert order: %v", err)
	}

	var orderItemID string
	err = f.pool.QueryRow(
		context.Background(),
		"INSERT INTO order_items (order_id, advertisement_id, seller_id, quantity, unit_price, subtotal, product_snapshot, created_at) "+
			"VALUES ($1, $2, $3, 1, 100, 100, '{}', $4) RETURNING id",
		orderID, adID, f.seller.ID, now,
	).Scan(&orderItemID)
	if err != nil {
		t.Fatalf("insert order item: %v", err)
	}

	_, err = f.pool.Exec(
		context.Background(),
		"INSERT INTO reviews (order_item_id, reviewer_id, product_rating, seller_rating, created_at, updated_at) VALUES ($1, $2, $3, 5, $4, $4)",
		orderItemID, buyer.ID, productRating, now,
	)
	if err != nil {
		t.Fatalf("insert review: %v", err)
	}
}

func (f fixture) detail(t *testing.T, id string) *catalog.AdvertisementDetailEntity {
	t.Helper()
	detail, err := f.r.GetAdvertisementByID(context.Background(), id)
	if err != nil {
		t.Fatalf("GetAdvertisementByID: %v", err)
	}
	return detail
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

func TestGetAdvertisementsFiltersByRatingMinAndState(t *testing.T) {
	f := newFixture(t)
	f.insertAddress(t, f.seller.ID, "SP", true)
	rated := f.insertAd(t, testutil.Ad{Title: "Bem avaliada"})
	f.insertReview(t, rated.ID, 5)
	f.insertReview(t, rated.ID, 4)
	f.insertAd(t, testutil.Ad{Title: "Sem avaliação"})

	tests := map[string]struct {
		filters catalog.AdvertisementFilters
		want    []string
	}{
		"rating_min excludes unrated ads":        {catalog.AdvertisementFilters{RatingMin: ptr(4.0)}, []string{"Bem avaliada"}},
		"rating_min above the average":           {catalog.AdvertisementFilters{RatingMin: ptr(4.6)}, []string{}},
		"state matches seller's default address": {catalog.AdvertisementFilters{State: helpers.StringPtr("SP")}, []string{"Bem avaliada", "Sem avaliação"}},
		"state with no matching seller address":  {catalog.AdvertisementFilters{State: helpers.StringPtr("MG")}, []string{}},
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

func TestGetAdvertisementsIncludesCoverImageRatingAndSellerState(t *testing.T) {
	f := newFixture(t)
	f.insertAddress(t, f.seller.ID, "PR", true)

	ad := f.insertAd(t, testutil.Ad{Title: "Com avaliações"})
	f.insertImage(t, ad.ID, "second.jpg", 2, false)
	f.insertImage(t, ad.ID, "cover.jpg", 1, true)
	f.insertReview(t, ad.ID, 5)
	f.insertReview(t, ad.ID, 3)

	f.insertAd(t, testutil.Ad{Title: "Sem avaliações"})

	ads := f.list(t, 10, 0, catalog.AdvertisementFilters{})
	byTitle := map[string]*catalog.AdvertisementEntity{}
	for _, a := range ads {
		byTitle[a.Title] = a
	}

	reviewed := byTitle["Com avaliações"]
	if reviewed.CoverImageURL == nil || *reviewed.CoverImageURL != "cover.jpg" {
		t.Fatalf("cover image = %v, want cover.jpg", reviewed.CoverImageURL)
	}
	if reviewed.AverageRating == nil || *reviewed.AverageRating != 4 {
		t.Fatalf("average rating = %v, want 4", reviewed.AverageRating)
	}
	if reviewed.ReviewCount != 2 {
		t.Fatalf("review count = %d, want 2", reviewed.ReviewCount)
	}
	if reviewed.SellerState == nil || *reviewed.SellerState != "PR" {
		t.Fatalf("seller state = %v, want PR", reviewed.SellerState)
	}

	unreviewed := byTitle["Sem avaliações"]
	if unreviewed.AverageRating != nil {
		t.Fatalf("average rating = %v, want nil", unreviewed.AverageRating)
	}
	if unreviewed.ReviewCount != 0 {
		t.Fatalf("review count = %d, want 0", unreviewed.ReviewCount)
	}
	if unreviewed.CoverImageURL != nil {
		t.Fatalf("cover image = %v, want nil", unreviewed.CoverImageURL)
	}
}

func TestGetAdvertisementSellerStateIsNilWithoutDefaultAddress(t *testing.T) {
	f := newFixture(t)
	ad := f.insertAd(t, testutil.Ad{})

	got := f.detail(t, ad.ID).Advertisement
	if got.SellerState != nil {
		t.Fatalf("seller state = %v, want nil", got.SellerState)
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

	got := f.detail(t, ad.ID).Advertisement

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

			got := f.detail(t, ad.ID).Advertisement
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
	detail := f.detail(t, ad.ID).Advertisement

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

	got := f.detail(t, ad.ID).Advertisement
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

func (f fixture) insertShippingOption(t *testing.T, adID, name, price string, deliveryDays int, active bool) {
	t.Helper()
	_, err := f.pool.Exec(
		context.Background(),
		"INSERT INTO shipping_options (advertisement_id, name, carrier, price, delivery_days, active) VALUES ($1, $2, 'Correios', $3, $4, $5)",
		adID, name, price, deliveryDays, active,
	)
	if err != nil {
		t.Fatalf("insert shipping option: %v", err)
	}
}

func TestGetAdvertisementByIDIncludesPurposesConditionsSellerAndShipping(t *testing.T) {
	f := newFixture(t)
	f.insertAddress(t, f.seller.ID, "SP", true)
	ad := f.insertAd(t, testutil.Ad{Title: "Palmilha"})
	other := f.insertAd(t, testutil.Ad{Title: "Outro anúncio do mesmo vendedor"})

	f.link(t, "advertisement_purposes", "purpose_id", ad.ID, f.insertPurpose(t, "prevention", "Prevenção"))
	f.link(t, "advertisement_purposes", "purpose_id", ad.ID, f.insertPurpose(t, "pain_relief", "Alívio de dor"))
	f.insertPurpose(t, "correction", "Correção")
	f.link(t, "advertisement_conditions", "condition_id", ad.ID, f.insertCondition(t, "Fascite plantar", true))
	f.link(t, "advertisement_conditions", "condition_id", ad.ID, f.insertCondition(t, "Condição desativada", false))

	f.insertShippingOption(t, ad.ID, "SEDEX", "34.9", 2, true)
	f.insertShippingOption(t, ad.ID, "PAC", "19.9", 7, true)
	f.insertShippingOption(t, ad.ID, "Inativa", "5", 10, false)
	f.insertShippingOption(t, other.ID, "De outro anúncio", "1", 1, true)

	f.insertReview(t, other.ID, 2)

	got := f.detail(t, ad.ID)

	purposes := []string{}
	for _, p := range got.Purposes {
		purposes = append(purposes, p.Name)
	}
	if !equal(purposes, []string{"Alívio de dor", "Prevenção"}) {
		t.Fatalf("purposes = %v", purposes)
	}
	if len(got.Conditions) != 1 || got.Conditions[0].Name != "Fascite plantar" {
		t.Fatalf("conditions = %+v, want only the active one", got.Conditions)
	}

	seller := got.Seller
	if seller.ID != f.seller.ID || seller.Name != f.seller.Name || seller.MemberSince.IsZero() {
		t.Fatalf("seller = %+v", seller)
	}
	if seller.State == nil || *seller.State != "SP" {
		t.Fatalf("seller state = %v, want SP", seller.State)
	}
	if seller.AverageRating == nil || *seller.AverageRating != 5 {
		t.Fatalf("seller rating = %v, want 5", seller.AverageRating)
	}

	if len(got.ShippingOptions) != 2 {
		t.Fatalf("shipping options = %+v, want the two active ones", got.ShippingOptions)
	}
	pac, sedex := got.ShippingOptions[0], got.ShippingOptions[1]
	if pac.Name != "PAC" || pac.Price != "19.90" || pac.DeliveryDays != 7 || pac.Carrier == nil || *pac.Carrier != "Correios" {
		t.Fatalf("first shipping option = %+v", pac)
	}
	if sedex.Name != "SEDEX" || sedex.Price != "34.90" {
		t.Fatalf("second shipping option = %+v", sedex)
	}
}

func TestGetAdvertisementByIDWithoutLinksReturnsEmptyLists(t *testing.T) {
	f := newFixture(t)
	ad := f.insertAd(t, testutil.Ad{})

	got := f.detail(t, ad.ID)

	if got.Purposes == nil || len(got.Purposes) != 0 || got.Conditions == nil || len(got.Conditions) != 0 ||
		got.ShippingOptions == nil || len(got.ShippingOptions) != 0 {
		t.Fatalf("purposes = %v, conditions = %v, shipping options = %v, want empty lists", got.Purposes, got.Conditions, got.ShippingOptions)
	}
	if got.Seller.AverageRating != nil || got.Seller.State != nil {
		t.Fatalf("seller = %+v, want no rating and no state", got.Seller)
	}
}

func TestGetAdvertisementsSearchesFullText(t *testing.T) {
	f := newFixture(t)
	f.insertAd(t, testutil.Ad{Title: "Palmilhas ortopédicas", Description: "Suporte para o calcanhar"})
	f.insertAd(t, testutil.Ad{Title: "Tênis para pé diabético", Description: "Interior sem costuras"})
	f.insertAd(t, testutil.Ad{Title: "Meia de compressão", Description: "Média compressão"})

	tests := map[string]struct {
		query string
		want  []string
	}{
		"prefix while typing":      {"palm", []string{"Palmilhas ortopédicas"}},
		"singular finds plural":    {"palmilha", []string{"Palmilhas ortopédicas"}},
		"without accents":          {"diabetico", []string{"Tênis para pé diabético"}},
		"in description":           {"calcanhar", []string{"Palmilhas ortopédicas"}},
		"every word must match":    {"palmilha diabetico", []string{}},
		"no words means no filter": {"!!!", []string{"Meia de compressão", "Tênis para pé diabético", "Palmilhas ortopédicas"}},
	}
	for name, tt := range tests {
		t.Run(name, func(t *testing.T) {
			got := titles(f.list(t, 10, 0, catalog.AdvertisementFilters{Query: helpers.StringPtr(tt.query)}))
			if !equal(got, tt.want) {
				t.Fatalf("titles = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestGetAdvertisementsRanksTitleMatchesFirstByRelevance(t *testing.T) {
	f := newFixture(t)
	f.insertAd(t, testutil.Ad{Title: "Palmilha de silicone", Description: "Gel"})
	f.insertAd(t, testutil.Ad{Title: "Calcanheira", Description: "Feita de silicone"})

	filters := catalog.AdvertisementFilters{Query: helpers.StringPtr("silicone")}
	if got := titles(f.listSorted(t, 10, 0, filters, catalog.AdvertisementSortRelevance)); !equal(got, []string{"Palmilha de silicone", "Calcanheira"}) {
		t.Fatalf("relevance = %v", got)
	}
	if got := titles(f.listSorted(t, 10, 0, filters, catalog.AdvertisementSortNewest)); !equal(got, []string{"Calcanheira", "Palmilha de silicone"}) {
		t.Fatalf("newest = %v", got)
	}
	filters.State = helpers.StringPtr("SP")
	f.insertAddress(t, f.seller.ID, "SP", true)
	if got := titles(f.listSorted(t, 10, 0, filters, catalog.AdvertisementSortRelevance)); !equal(got, []string{"Palmilha de silicone", "Calcanheira"}) {
		t.Fatalf("relevance with state = %v", got)
	}
}

func (f fixture) insertPainPoint(t *testing.T, name string, conditionIDs ...string) string {
	t.Helper()
	var id string
	if err := f.pool.QueryRow(context.Background(), "INSERT INTO pain_points (name) VALUES ($1) RETURNING id", name).Scan(&id); err != nil {
		t.Fatalf("insert pain point: %v", err)
	}
	for _, conditionID := range conditionIDs {
		_, err := f.pool.Exec(context.Background(), "INSERT INTO pain_point_conditions (pain_point_id, condition_id) VALUES ($1, $2)", id, conditionID)
		if err != nil {
			t.Fatalf("map pain point: %v", err)
		}
	}
	return id
}

func TestGetAdvertisementsFiltersByProfile(t *testing.T) {
	f := newFixture(t)
	fasciitis := f.insertCondition(t, "Fascite plantar", true)
	diabetic := f.insertCondition(t, "Pé diabético", true)
	bunion := f.insertCondition(t, "Joanete (hálux valgo)", true)
	heel := f.insertPainPoint(t, "Calcanhar", fasciitis)
	ankle := f.insertPainPoint(t, "Tornozelo")

	for title, conditionID := range map[string]string{"Palmilha": fasciitis, "Tênis": diabetic, "Órtese": bunion} {
		ad := f.insertAd(t, testutil.Ad{Title: title})
		f.link(t, "advertisement_conditions", "condition_id", ad.ID, conditionID)
	}
	f.insertAd(t, testutil.Ad{Title: "Sem condição"})

	tests := map[string]struct {
		profile catalog.ProfileMatch
		want    []string
	}{
		"pain point mapped to a condition":   {catalog.ProfileMatch{PainPointIDs: []string{heel}}, []string{"Palmilha"}},
		"pain point without conditions":      {catalog.ProfileMatch{PainPointIDs: []string{ankle}}, []string{}},
		"clinical condition without accents": {catalog.ProfileMatch{ClinicalCondition: helpers.StringPtr("sou diabetico")}, []string{"Tênis"}},
		"either source matches": {catalog.ProfileMatch{PainPointIDs: []string{heel}, ClinicalCondition: helpers.StringPtr("joanete")},
			[]string{"Palmilha", "Órtese"}},
		"short words are ignored": {catalog.ProfileMatch{ClinicalCondition: helpers.StringPtr("dor no pé")}, []string{}},
		"empty profile":           {catalog.ProfileMatch{}, []string{}},
	}
	for name, tt := range tests {
		t.Run(name, func(t *testing.T) {
			filters := catalog.AdvertisementFilters{Profile: &tt.profile}
			got := titles(f.list(t, 10, 0, filters))
			if !sameItems(got, tt.want) {
				t.Fatalf("titles = %v, want %v", got, tt.want)
			}
			count, err := f.r.CountAdvertisements(context.Background(), filters)
			if err != nil {
				t.Fatalf("CountAdvertisements: %v", err)
			}
			if count != uint(len(tt.want)) {
				t.Fatalf("count = %d, want %d", count, len(tt.want))
			}
		})
	}
}
