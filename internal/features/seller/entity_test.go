package seller

import (
	"slices"
	"testing"
)

func strPtr(s string) *string {
	return &s
}

func completeAd() *AdvertisementEntity {
	return &AdvertisementEntity{
		CategoryID:         "category-1",
		Title:              "Palmilha",
		Description:        "Palmilha de silicone",
		Price:              "89.90",
		Status:             AdvertisementStatusDraft,
		Side:               strPtr("both"),
		Size:               strPtr("35-44"),
		Material:           strPtr("Silicone"),
		Rigidity:           strPtr("flexible"),
		UsageIndication:    strPtr("Uso diário"),
		RequiredAttributes: []string{"side", "size", "material", "rigidity", "usage_indication"},
		Images:             []ImageEntity{{ID: "image-1", URL: "/files/public/ads/a.jpg", Position: 1, IsCover: true}},
		ShippingOptions:    []ShippingOptionEntity{{ID: "option-1", Name: "PAC", Price: "19.90", DeliveryDays: 7}},
	}
}

func TestMissingForPublishingCompleteAd(t *testing.T) {
	if missing := completeAd().MissingForPublishing(); len(missing) != 0 {
		t.Fatalf("missing = %v, want none", missing)
	}
}

func TestMissingForPublishingListsEveryBlank(t *testing.T) {
	ad := &AdvertisementEntity{
		Price:              "0.00",
		Material:           strPtr("  "),
		RequiredAttributes: []string{"side", "size", "material", "rigidity", "usage_indication"},
	}

	want := []string{"title", "description", "category_id", "price", "side", "size", "material", "rigidity",
		"usage_indication", "images", "shipping_options"}
	if missing := ad.MissingForPublishing(); !slices.Equal(missing, want) {
		t.Fatalf("missing = %v, want %v", missing, want)
	}
}

func TestMissingForPublishingOnlyChecksTheCategoryAttributes(t *testing.T) {
	ad := completeAd()
	ad.RequiredAttributes = []string{"size", "material"}
	ad.Side, ad.Rigidity, ad.UsageIndication = nil, nil, nil

	if missing := ad.MissingForPublishing(); len(missing) != 0 {
		t.Fatalf("missing = %v, want none", missing)
	}
}

func TestMissingForPublishingSkipsSizeForCustomMade(t *testing.T) {
	ad := completeAd()
	ad.Size = nil
	ad.CustomMadeAllowed = true

	if missing := ad.MissingForPublishing(); len(missing) != 0 {
		t.Fatalf("missing = %v, want none", missing)
	}
}

func TestMissingForPublishingRequiresSafetyInfoForClinicalItems(t *testing.T) {
	tests := map[string]func(*AdvertisementEntity){
		"prescription required": func(ad *AdvertisementEntity) { ad.PrescriptionRequired = true },
		"linked to a condition": func(ad *AdvertisementEntity) { ad.ConditionIDs = []string{"condition-1"} },
	}

	for name, mutate := range tests {
		t.Run(name, func(t *testing.T) {
			ad := completeAd()
			mutate(ad)

			want := []string{"safety_alerts", "contraindications"}
			if missing := ad.MissingForPublishing(); !slices.Equal(missing, want) {
				t.Fatalf("missing = %v, want %v", missing, want)
			}
		})
	}
}

func TestSoldOut(t *testing.T) {
	tests := []struct {
		status AdvertisementStatus
		stock  int
		want   bool
	}{
		{AdvertisementStatusPublished, 0, true},
		{AdvertisementStatusPublished, 1, false},
		{AdvertisementStatusPaused, 0, false},
		{AdvertisementStatusDraft, 0, false},
	}

	for _, test := range tests {
		ad := &AdvertisementEntity{Status: test.status, Stock: test.stock}
		if got := ad.SoldOut(); got != test.want {
			t.Errorf("SoldOut(%s, stock %d) = %v, want %v", test.status, test.stock, got, test.want)
		}
	}
}

func TestToOutputPicksCoverAndUsesEmptyLists(t *testing.T) {
	ad := &AdvertisementEntity{
		Status: AdvertisementStatusDraft,
		Images: []ImageEntity{
			{ID: "1", URL: "/files/public/ads/first.jpg", Position: 1},
			{ID: "2", URL: "/files/public/ads/cover.jpg", Position: 2, IsCover: true},
		},
	}

	output := ad.ToOutput()

	if output.CoverImageURL == nil || *output.CoverImageURL != "/files/public/ads/cover.jpg" {
		t.Fatalf("cover = %v", output.CoverImageURL)
	}
	if output.Purposes == nil || output.ConditionIDs == nil || output.ShippingOptions == nil {
		t.Fatalf("lists must be empty, not null: %+v", output)
	}
}

func TestToOutputFallsBackToFirstImage(t *testing.T) {
	ad := &AdvertisementEntity{Images: []ImageEntity{{URL: "a"}, {URL: "b"}}}

	if cover := ad.ToOutput().CoverImageURL; cover == nil || *cover != "a" {
		t.Fatalf("cover = %v, want a", cover)
	}
}
