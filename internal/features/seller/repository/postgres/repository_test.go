package postgres

import (
	"context"
	"errors"
	"slices"
	"testing"
	"time"

	"github.com/caslumes/solado_certo_backend/internal/app"
	"github.com/caslumes/solado_certo_backend/internal/features/seller"
	"github.com/caslumes/solado_certo_backend/internal/testutil"
	"github.com/jackc/pgx/v5/pgxpool"
)

const unknownID = "00000000-0000-0000-0000-000000000000"

type fixture struct {
	r         *postgresRepository
	pool      *pgxpool.Pool
	seller    testutil.User
	category  testutil.Category
	condition string
}

func newFixture(t *testing.T) fixture {
	t.Helper()
	pool := testutil.DB(t)
	f := fixture{
		r:        NewPostgresRepository(pool),
		pool:     pool,
		seller:   testutil.InsertUser(t, pool),
		category: testutil.InsertCategory(t, pool, nil),
	}
	f.exec(t, "UPDATE categories SET required_attributes = '{size,material}' WHERE id = $1", f.category.ID)
	f.exec(t, "INSERT INTO podological_purposes (code, name) VALUES ('prevention', 'Prevenção'), ('pain_relief', 'Alívio de dor')")
	if err := pool.QueryRow(context.Background(), "INSERT INTO podological_conditions (name) VALUES ('Fascite plantar') RETURNING id").Scan(&f.condition); err != nil {
		t.Fatalf("insert condition: %v", err)
	}
	return f
}

func (f fixture) exec(t *testing.T, sql string, args ...any) {
	t.Helper()
	if _, err := f.pool.Exec(context.Background(), sql, args...); err != nil {
		t.Fatalf("exec %q: %v", sql, err)
	}
}

func ptr(s string) *string {
	return &s
}

func (f fixture) newAd() *seller.AdvertisementEntity {
	return &seller.AdvertisementEntity{
		SellerID:        f.seller.ID,
		CategoryID:      f.category.ID,
		Title:           "Palmilha de silicone",
		Description:     "Absorve impacto",
		Price:           "89.90",
		Stock:           10,
		Status:          seller.AdvertisementStatusDraft,
		Size:            ptr("35-44"),
		Material:        ptr("Silicone"),
		PurposeCodes:    []string{"pain_relief", "prevention"},
		ConditionIDs:    []string{f.condition},
		ShippingOptions: []seller.ShippingOptionEntity{{Name: "PAC", Carrier: ptr("Correios"), Price: "19.90", DeliveryDays: 7}},
	}
}

func (f fixture) create(t *testing.T, ad *seller.AdvertisementEntity) *seller.AdvertisementEntity {
	t.Helper()
	id, err := f.r.CreateAdvertisement(context.Background(), ad)
	if err != nil {
		t.Fatalf("CreateAdvertisement: %v", err)
	}
	return f.get(t, id)
}

func (f fixture) get(t *testing.T, id string) *seller.AdvertisementEntity {
	t.Helper()
	ad, err := f.r.GetAdvertisementByID(context.Background(), id)
	if err != nil {
		t.Fatalf("GetAdvertisementByID: %v", err)
	}
	return ad
}

func TestCreateAdvertisementCanBeReadBack(t *testing.T) {
	f := newFixture(t)

	ad := f.create(t, f.newAd())

	if ad.ID == "" || ad.SellerID != f.seller.ID || ad.Title != "Palmilha de silicone" || ad.Price != "89.90" ||
		ad.Stock != 10 || ad.Status != seller.AdvertisementStatusDraft {
		t.Fatalf("ad = %+v", ad)
	}
	if ad.Material == nil || *ad.Material != "Silicone" || ad.Side != nil {
		t.Fatalf("material = %v, side = %v", ad.Material, ad.Side)
	}
	if !slices.Equal(ad.PurposeCodes, []string{"pain_relief", "prevention"}) || !slices.Equal(ad.ConditionIDs, []string{f.condition}) {
		t.Fatalf("purposes = %v, conditions = %v", ad.PurposeCodes, ad.ConditionIDs)
	}
	if len(ad.ShippingOptions) != 1 || ad.ShippingOptions[0].ID == "" || ad.ShippingOptions[0].Price != "19.90" ||
		*ad.ShippingOptions[0].Carrier != "Correios" || ad.ShippingOptions[0].DeliveryDays != 7 {
		t.Fatalf("shipping options = %+v", ad.ShippingOptions)
	}
	if !slices.Equal(ad.RequiredAttributes, []string{"size", "material"}) {
		t.Fatalf("required attributes = %v", ad.RequiredAttributes)
	}
	if len(ad.Images) != 0 || ad.CreatedAt.IsZero() {
		t.Fatalf("images = %v, created at = %v", ad.Images, ad.CreatedAt)
	}
}

func TestGetAdvertisementByIDReadsImages(t *testing.T) {
	f := newFixture(t)
	ad := f.create(t, f.newAd())
	f.exec(t, "INSERT INTO advertisement_images (advertisement_id, url, alt_text, position, is_cover) VALUES "+
		"($1, 'b.jpg', NULL, 2, true), ($1, 'a.jpg', 'Foto', 1, false)", ad.ID)

	images := f.get(t, ad.ID).Images

	if len(images) != 2 || images[0].URL != "a.jpg" || images[0].AltText != "Foto" || images[1].AltText != "" || !images[1].IsCover {
		t.Fatalf("images = %+v", images)
	}
}

func TestGetAdvertisementByIDReturnsNotFound(t *testing.T) {
	f := newFixture(t)
	removed := f.create(t, f.newAd())
	if err := f.r.RemoveAdvertisement(context.Background(), removed.ID); err != nil {
		t.Fatalf("RemoveAdvertisement: %v", err)
	}

	for _, id := range []string{unknownID, removed.ID} {
		if _, err := f.r.GetAdvertisementByID(context.Background(), id); !errors.Is(err, app.ErrAdvertisementNotFound) {
			t.Errorf("id %s: err = %v, want %v", id, err, app.ErrAdvertisementNotFound)
		}
	}
}

func TestGetAdvertisementsBySellerFiltersAndSorts(t *testing.T) {
	f := newFixture(t)
	other := testutil.InsertUser(t, f.pool)
	ctx := context.Background()

	older := f.create(t, f.newAd())
	newer := f.newAd()
	newer.Status = seller.AdvertisementStatusPublished
	newerAd := f.create(t, newer)
	f.exec(t, "UPDATE advertisements SET updated_at = $2 WHERE id = $1", older.ID, time.Now().Add(-time.Hour))
	removed := f.create(t, f.newAd())
	if err := f.r.RemoveAdvertisement(ctx, removed.ID); err != nil {
		t.Fatalf("RemoveAdvertisement: %v", err)
	}
	othersAd := f.newAd()
	othersAd.SellerID = other.ID
	f.create(t, othersAd)

	all, err := f.r.GetAdvertisementsBySeller(ctx, f.seller.ID, nil)
	if err != nil {
		t.Fatalf("GetAdvertisementsBySeller: %v", err)
	}
	if len(all) != 2 || all[0].ID != newerAd.ID || all[1].ID != older.ID {
		t.Fatalf("ads = %v, want newer then older", ids(all))
	}
	if len(all[0].ShippingOptions) != 1 || len(all[1].PurposeCodes) != 2 {
		t.Fatal("relations were not loaded for every ad")
	}

	draft := seller.AdvertisementStatusDraft
	drafts, err := f.r.GetAdvertisementsBySeller(ctx, f.seller.ID, &draft)
	if err != nil {
		t.Fatalf("GetAdvertisementsBySeller(draft): %v", err)
	}
	if len(drafts) != 1 || drafts[0].ID != older.ID {
		t.Fatalf("drafts = %v", ids(drafts))
	}
}

func TestGetAdvertisementsBySellerReturnsEmptyList(t *testing.T) {
	f := newFixture(t)

	ads, err := f.r.GetAdvertisementsBySeller(context.Background(), f.seller.ID, nil)

	if err != nil || ads == nil || len(ads) != 0 {
		t.Fatalf("ads = %v, err = %v", ads, err)
	}
}

func ids(ads []*seller.AdvertisementEntity) []string {
	out := make([]string, len(ads))
	for i, ad := range ads {
		out[i] = ad.ID
	}
	return out
}

func TestUpdateAdvertisementReplacesFieldsAndLinks(t *testing.T) {
	f := newFixture(t)
	ad := f.create(t, f.newAd())
	kept := ad.ShippingOptions[0]

	ad.Title = "Palmilha nova"
	ad.Price = "99.00"
	ad.Material = nil
	ad.PurposeCodes = []string{"prevention"}
	ad.ConditionIDs = []string{}
	kept.Price = "21.50"
	ad.ShippingOptions = []seller.ShippingOptionEntity{kept, {Name: "SEDEX", Price: "34.90", DeliveryDays: 2}}

	if err := f.r.UpdateAdvertisement(context.Background(), ad, seller.AdvertisementStatusDraft); err != nil {
		t.Fatalf("UpdateAdvertisement: %v", err)
	}

	got := f.get(t, ad.ID)
	if got.Title != "Palmilha nova" || got.Price != "99.00" || got.Material != nil {
		t.Fatalf("ad = %+v", got)
	}
	if !slices.Equal(got.PurposeCodes, []string{"prevention"}) || len(got.ConditionIDs) != 0 {
		t.Fatalf("purposes = %v, conditions = %v", got.PurposeCodes, got.ConditionIDs)
	}
	if len(got.ShippingOptions) != 2 || got.ShippingOptions[0].ID != kept.ID || got.ShippingOptions[0].Price != "21.50" ||
		got.ShippingOptions[1].Name != "SEDEX" {
		t.Fatalf("shipping options = %+v", got.ShippingOptions)
	}
}

func TestUpdateAdvertisementDeactivatesDroppedShippingOptions(t *testing.T) {
	f := newFixture(t)
	ad := f.create(t, f.newAd())
	droppedID := ad.ShippingOptions[0].ID
	ad.ShippingOptions = []seller.ShippingOptionEntity{}

	if err := f.r.UpdateAdvertisement(context.Background(), ad, seller.AdvertisementStatusDraft); err != nil {
		t.Fatalf("UpdateAdvertisement: %v", err)
	}

	if options := f.get(t, ad.ID).ShippingOptions; len(options) != 0 {
		t.Fatalf("shipping options = %+v, want none active", options)
	}
	var active bool
	if err := f.pool.QueryRow(context.Background(), "SELECT active FROM shipping_options WHERE id = $1", droppedID).Scan(&active); err != nil {
		t.Fatalf("dropped option was deleted: %v", err)
	}
	if active {
		t.Fatal("dropped option is still active")
	}
}

func TestUpdateAdvertisementChecksExpectedStatus(t *testing.T) {
	f := newFixture(t)
	ad := f.create(t, f.newAd())
	ad.Title = "Outro título"

	err := f.r.UpdateAdvertisement(context.Background(), ad, seller.AdvertisementStatusPublished)

	if !errors.Is(err, app.ErrInvalidStatusTransition) {
		t.Fatalf("err = %v, want %v", err, app.ErrInvalidStatusTransition)
	}
	if f.get(t, ad.ID).Title != "Palmilha de silicone" {
		t.Fatal("ad was updated")
	}
}

func TestChangeStatusOnlyFromExpectedStatus(t *testing.T) {
	f := newFixture(t)
	ad := f.create(t, f.newAd())
	ctx := context.Background()

	if err := f.r.ChangeStatus(ctx, ad.ID, seller.AdvertisementStatusDraft, seller.AdvertisementStatusPublished); err != nil {
		t.Fatalf("ChangeStatus: %v", err)
	}
	if status := f.get(t, ad.ID).Status; status != seller.AdvertisementStatusPublished {
		t.Fatalf("status = %q", status)
	}

	err := f.r.ChangeStatus(ctx, ad.ID, seller.AdvertisementStatusDraft, seller.AdvertisementStatusPublished)
	if !errors.Is(err, app.ErrInvalidStatusTransition) {
		t.Fatalf("second change: err = %v, want %v", err, app.ErrInvalidStatusTransition)
	}
}

func TestRemoveAdvertisementKeepsTheRow(t *testing.T) {
	f := newFixture(t)
	ad := f.create(t, f.newAd())
	ctx := context.Background()

	if err := f.r.RemoveAdvertisement(ctx, ad.ID); err != nil {
		t.Fatalf("RemoveAdvertisement: %v", err)
	}
	if err := f.r.RemoveAdvertisement(ctx, ad.ID); !errors.Is(err, app.ErrAdvertisementNotFound) {
		t.Fatalf("second remove: err = %v, want %v", err, app.ErrAdvertisementNotFound)
	}

	var status string
	var removedAt *time.Time
	if err := f.pool.QueryRow(ctx, "SELECT status, removed_at FROM advertisements WHERE id = $1", ad.ID).Scan(&status, &removedAt); err != nil {
		t.Fatalf("removed ad was deleted: %v", err)
	}
	if status != "removed" || removedAt == nil {
		t.Fatalf("status = %q, removed_at = %v", status, removedAt)
	}
}

func TestGetCategoryRequiredAttributes(t *testing.T) {
	f := newFixture(t)
	inactive := testutil.InsertCategory(t, f.pool, nil)
	f.exec(t, "UPDATE categories SET active = false WHERE id = $1", inactive.ID)
	ctx := context.Background()

	required, err := f.r.GetCategoryRequiredAttributes(ctx, f.category.ID)
	if err != nil || !slices.Equal(required, []string{"size", "material"}) {
		t.Fatalf("required = %v, err = %v", required, err)
	}
	for _, id := range []string{unknownID, inactive.ID} {
		if _, err := f.r.GetCategoryRequiredAttributes(ctx, id); !errors.Is(err, app.ErrCategoryNotFound) {
			t.Errorf("id %s: err = %v, want %v", id, err, app.ErrCategoryNotFound)
		}
	}
}

func TestCategoryRejectsUnknownRequiredAttribute(t *testing.T) {
	f := newFixture(t)

	_, err := f.pool.Exec(context.Background(), "UPDATE categories SET required_attributes = '{color}' WHERE id = $1", f.category.ID)

	if err == nil {
		t.Fatal("unknown attribute was accepted")
	}
}

func TestCountPurposesAndConditions(t *testing.T) {
	f := newFixture(t)
	inactive := ""
	if err := f.pool.QueryRow(context.Background(), "INSERT INTO podological_conditions (name, active) VALUES ('Antiga', false) RETURNING id").Scan(&inactive); err != nil {
		t.Fatalf("insert condition: %v", err)
	}
	ctx := context.Background()

	purposes, err := f.r.CountPurposes(ctx, []string{"prevention", "pain_relief", "marketing"})
	if err != nil || purposes != 2 {
		t.Fatalf("purposes = %d, err = %v; want 2", purposes, err)
	}
	conditions, err := f.r.CountActiveConditions(ctx, []string{f.condition, inactive, unknownID})
	if err != nil || conditions != 1 {
		t.Fatalf("conditions = %d, err = %v; want 1", conditions, err)
	}
}

func (f fixture) addImages(t *testing.T, adID string, n int) []seller.ImageEntity {
	t.Helper()
	for i := range n {
		if err := f.r.AddImage(context.Background(), adID, "/files/public/ads/"+string(rune('a'+i))+".jpg", nil, 6); err != nil {
			t.Fatalf("AddImage: %v", err)
		}
	}
	return f.get(t, adID).Images
}

func coverCount(images []seller.ImageEntity) int {
	count := 0
	for _, image := range images {
		if image.IsCover {
			count++
		}
	}
	return count
}

func TestAddImageNumbersPositionsAndSetsFirstCover(t *testing.T) {
	f := newFixture(t)
	ad := f.create(t, f.newAd())
	if err := f.r.AddImage(context.Background(), ad.ID, "/files/public/ads/a.jpg", ptr("Frente"), 6); err != nil {
		t.Fatalf("AddImage: %v", err)
	}

	images := f.addImages(t, ad.ID, 2)

	if len(images) != 3 || images[0].AltText != "Frente" || !images[0].IsCover || coverCount(images) != 1 {
		t.Fatalf("images = %+v", images)
	}
	for i, image := range images {
		if image.Position != i+1 {
			t.Fatalf("image %d has position %d", i, image.Position)
		}
	}
}

func TestAddImageStopsAtTheLimit(t *testing.T) {
	f := newFixture(t)
	ad := f.create(t, f.newAd())
	f.addImages(t, ad.ID, 2)

	err := f.r.AddImage(context.Background(), ad.ID, "/files/public/ads/z.jpg", nil, 2)

	if !errors.Is(err, app.ErrTooManyImages) {
		t.Fatalf("err = %v, want %v", err, app.ErrTooManyImages)
	}
}

func TestAddImageReturnsNotFoundForRemovedAd(t *testing.T) {
	f := newFixture(t)
	ad := f.create(t, f.newAd())
	if err := f.r.RemoveAdvertisement(context.Background(), ad.ID); err != nil {
		t.Fatalf("RemoveAdvertisement: %v", err)
	}

	err := f.r.AddImage(context.Background(), ad.ID, "/files/public/ads/a.jpg", nil, 6)

	if !errors.Is(err, app.ErrAdvertisementNotFound) {
		t.Fatalf("err = %v, want %v", err, app.ErrAdvertisementNotFound)
	}
}

func TestRemoveImageRenumbersAndMovesCover(t *testing.T) {
	f := newFixture(t)
	ad := f.create(t, f.newAd())
	images := f.addImages(t, ad.ID, 3)

	if err := f.r.RemoveImage(context.Background(), ad.ID, images[0].ID); err != nil {
		t.Fatalf("RemoveImage: %v", err)
	}

	left := f.get(t, ad.ID).Images
	if len(left) != 2 || left[0].ID != images[1].ID || left[0].Position != 1 || left[1].Position != 2 {
		t.Fatalf("images = %+v", left)
	}
	if !left[0].IsCover || coverCount(left) != 1 {
		t.Fatalf("cover was not moved: %+v", left)
	}
}

func TestRemoveImageKeepsExistingCover(t *testing.T) {
	f := newFixture(t)
	ad := f.create(t, f.newAd())
	images := f.addImages(t, ad.ID, 3)

	if err := f.r.RemoveImage(context.Background(), ad.ID, images[1].ID); err != nil {
		t.Fatalf("RemoveImage: %v", err)
	}

	left := f.get(t, ad.ID).Images
	if left[0].ID != images[0].ID || !left[0].IsCover || coverCount(left) != 1 || left[1].Position != 2 {
		t.Fatalf("images = %+v", left)
	}
}

func TestRemoveImageOnlyFromItsAd(t *testing.T) {
	f := newFixture(t)
	ad := f.create(t, f.newAd())
	other := f.create(t, f.newAd())
	images := f.addImages(t, ad.ID, 1)

	err := f.r.RemoveImage(context.Background(), other.ID, images[0].ID)

	if !errors.Is(err, app.ErrImageNotFound) {
		t.Fatalf("err = %v, want %v", err, app.ErrImageNotFound)
	}
	if len(f.get(t, ad.ID).Images) != 1 {
		t.Fatal("the image was removed")
	}
}

func TestReorderImagesSetsPositionsAndCover(t *testing.T) {
	f := newFixture(t)
	ad := f.create(t, f.newAd())
	images := f.addImages(t, ad.ID, 3)
	order := []string{images[2].ID, images[0].ID, images[1].ID}

	if err := f.r.ReorderImages(context.Background(), ad.ID, order, images[1].ID); err != nil {
		t.Fatalf("ReorderImages: %v", err)
	}

	got := f.get(t, ad.ID).Images
	for i, id := range order {
		if got[i].ID != id || got[i].Position != i+1 {
			t.Fatalf("images = %+v, want order %v", got, order)
		}
	}
	if !got[2].IsCover || coverCount(got) != 1 {
		t.Fatalf("cover = %+v", got)
	}
}
