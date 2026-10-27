package orders

import (
	"github.com/caslumes/solado_certo_backend/internal/helpers"
)

type ShippingOptionEntity struct {
	ID           string
	Name         string
	Carrier      *string
	Price        string
	DeliveryDays int
}

func (o *ShippingOptionEntity) ToOutput() ShippingOptionOutput {
	return ShippingOptionOutput{ID: o.ID, Name: o.Name, Carrier: o.Carrier, Price: o.Price, DeliveryDays: o.DeliveryDays}
}

type AdvertisementEntity struct {
	ID       string
	SellerID string
	Price    string
	Stock    int
	Status   string
}

func (a *AdvertisementEntity) Published() bool {
	return a.Status == AdvertisementStatusPublished
}

type CartItemEntity struct {
	AdvertisementID string
	SellerID        string
	SellerName      string
	Title           string
	CoverImageURL   *string
	Quantity        int
	AddedPrice      string
	Price           string
	Stock           int
	AdStatus        string
	ShippingOptions []ShippingOptionEntity
}

func (i *CartItemEntity) Problem() ItemProblem {
	switch {
	case i.AdStatus != AdvertisementStatusPublished:
		return ItemProblemUnavailable
	case i.Stock == 0:
		return ItemProblemOutOfStock
	case i.Stock < i.Quantity:
		return ItemProblemInsufficientStock
	}
	return ""
}

func (i *CartItemEntity) PriceChanged() bool {
	return helpers.MustParseCents(i.AddedPrice) != helpers.MustParseCents(i.Price)
}

func (i *CartItemEntity) Subtotal() helpers.Cents {
	return helpers.MustParseCents(i.Price).Times(i.Quantity)
}

type CartEntity struct {
	ID    string
	Items []CartItemEntity
}

func (c *CartEntity) Item(advertisementID string) *CartItemEntity {
	for i := range c.Items {
		if c.Items[i].AdvertisementID == advertisementID {
			return &c.Items[i]
		}
	}
	return nil
}

func (c *CartEntity) ToOutput() *CartOutput {
	output := &CartOutput{Groups: []CartGroupOutput{}}
	if c.ID != "" {
		output.ID = &c.ID
	}

	var subtotal helpers.Cents
	groupIndex := map[string]int{}
	groupSubtotals := []helpers.Cents{}
	for _, item := range c.Items {
		index, ok := groupIndex[item.SellerID]
		if !ok {
			index = len(output.Groups)
			groupIndex[item.SellerID] = index
			output.Groups = append(output.Groups, CartGroupOutput{
				Seller: SellerOutput{ID: item.SellerID, Name: item.SellerName},
				Items:  []CartItemOutput{},
			})
			groupSubtotals = append(groupSubtotals, 0)
		}

		problem := item.Problem()
		itemOutput := CartItemOutput{
			AdvertisementID: item.AdvertisementID,
			Title:           item.Title,
			CoverImageURL:   item.CoverImageURL,
			Quantity:        item.Quantity,
			UnitPrice:       item.Price,
			AddedUnitPrice:  item.AddedPrice,
			PriceChanged:    item.PriceChanged(),
			Stock:           item.Stock,
			Available:       problem == "",
			Subtotal:        item.Subtotal().String(),
			ShippingOptions: make([]ShippingOptionOutput, len(item.ShippingOptions)),
		}
		if problem != "" {
			itemOutput.Problem = &problem
			output.HasProblems = true
		} else {
			groupSubtotals[index] += item.Subtotal()
			subtotal += item.Subtotal()
			output.ItemCount += item.Quantity
		}
		if itemOutput.PriceChanged {
			output.HasPriceChanges = true
		}
		for j, option := range item.ShippingOptions {
			itemOutput.ShippingOptions[j] = option.ToOutput()
		}
		output.Groups[index].Items = append(output.Groups[index].Items, itemOutput)
	}

	for i := range output.Groups {
		output.Groups[i].Subtotal = groupSubtotals[i].String()
	}
	output.Subtotal = subtotal.String()
	return output
}
