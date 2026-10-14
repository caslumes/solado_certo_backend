package orders

import "testing"

func item(adID string, sellerID string, quantity int, price string, stock int) CartItemEntity {
	return CartItemEntity{
		AdvertisementID: adID,
		SellerID:        sellerID,
		SellerName:      "Loja " + sellerID,
		Title:           "Anúncio " + adID,
		Quantity:        quantity,
		AddedPrice:      price,
		Price:           price,
		Stock:           stock,
		AdStatus:        AdvertisementStatusPublished,
		ShippingOptions: []ShippingOptionEntity{},
	}
}

func TestCartItemProblem(t *testing.T) {
	available := item("a", "s", 2, "10.00", 5)
	paused := item("a", "s", 1, "10.00", 5)
	paused.AdStatus = "paused"
	removed := item("a", "s", 1, "10.00", 5)
	removed.AdStatus = "removed"

	tests := []struct {
		name string
		item CartItemEntity
		want ItemProblem
	}{
		{"available", available, ""},
		{"exact stock", item("a", "s", 5, "10.00", 5), ""},
		{"paused", paused, ItemProblemUnavailable},
		{"removed", removed, ItemProblemUnavailable},
		{"no stock", item("a", "s", 1, "10.00", 0), ItemProblemOutOfStock},
		{"less stock than quantity", item("a", "s", 3, "10.00", 2), ItemProblemInsufficientStock},
	}
	for _, tt := range tests {
		if got := tt.item.Problem(); got != tt.want {
			t.Errorf("%s: Problem() = %q, want %q", tt.name, got, tt.want)
		}
	}
}

func TestEmptyCartOutput(t *testing.T) {
	output := (&CartEntity{}).ToOutput()
	if output.ID != nil || len(output.Groups) != 0 || output.Subtotal != "0.00" || output.ItemCount != 0 {
		t.Errorf("empty cart output = %+v", output)
	}
	if output.Groups == nil {
		t.Error("groups must be an empty list, not null")
	}
}

func TestCartOutputGroupsBySellerAndSkipsProblemsInTotals(t *testing.T) {
	changed := item("a2", "s1", 1, "15.00", 9)
	changed.AddedPrice = "12.00"
	paused := item("a4", "s2", 1, "99.00", 9)
	paused.AdStatus = "paused"
	cart := &CartEntity{ID: "cart-1", Items: []CartItemEntity{
		item("a1", "s1", 2, "10.50", 9),
		changed,
		item("a3", "s2", 3, "5.00", 9),
		paused,
	}}

	output := cart.ToOutput()

	if output.ID == nil || *output.ID != "cart-1" {
		t.Fatalf("ID = %v, want cart-1", output.ID)
	}
	if len(output.Groups) != 2 || output.Groups[0].Seller.ID != "s1" || output.Groups[1].Seller.ID != "s2" {
		t.Fatalf("groups = %+v, want s1 then s2", output.Groups)
	}
	if output.Groups[0].Subtotal != "36.00" {
		t.Errorf("s1 subtotal = %s, want 36.00 (2 × 10.50 + 15.00 at the current price)", output.Groups[0].Subtotal)
	}
	if output.Groups[1].Subtotal != "15.00" {
		t.Errorf("s2 subtotal = %s, want 15.00 (the paused item is not counted)", output.Groups[1].Subtotal)
	}
	if output.Subtotal != "51.00" || output.ItemCount != 6 {
		t.Errorf("subtotal = %s, item count = %d, want 51.00 and 6", output.Subtotal, output.ItemCount)
	}
	if !output.HasProblems || !output.HasPriceChanges {
		t.Errorf("has problems = %v, has price changes = %v, want both true", output.HasProblems, output.HasPriceChanges)
	}

	changedOutput := output.Groups[0].Items[1]
	if !changedOutput.PriceChanged || changedOutput.UnitPrice != "15.00" || changedOutput.AddedUnitPrice != "12.00" {
		t.Errorf("changed item = %+v", changedOutput)
	}
	pausedOutput := output.Groups[1].Items[1]
	if pausedOutput.Available || pausedOutput.Problem == nil || *pausedOutput.Problem != ItemProblemUnavailable {
		t.Errorf("paused item = %+v", pausedOutput)
	}
}
