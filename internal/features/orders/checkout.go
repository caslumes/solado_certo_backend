package orders

import (
	"crypto/rand"
	"fmt"
	"slices"
	"time"

	"github.com/caslumes/solado_certo_backend/internal/app"
	"github.com/caslumes/solado_certo_backend/pkg/money"
)

type AddressSnapshot struct {
	Label      *string `json:"label"`
	Receiver   string  `json:"receiver"`
	ZIPCode    string  `json:"zip_code"`
	Street     string  `json:"street"`
	Number     string  `json:"number"`
	Complement *string `json:"complement"`
	District   string  `json:"district"`
	City       string  `json:"city"`
	State      string  `json:"state"`
	Country    string  `json:"country"`
}

type ShippingSelection struct {
	AdvertisementID  string
	ShippingOptionID string
}

type CheckoutInput struct {
	AddressID string
	Shipping  []ShippingSelection
}

type PlannedItem struct {
	CartItemEntity
	ShippingOption ShippingOptionEntity
}

func (i *PlannedItem) Subtotal() money.Cents {
	return i.CartItemEntity.Subtotal()
}

// PlannedOrder is one order a checkout would create: the cart items of one seller.
type PlannedOrder struct {
	SellerID      string
	SellerName    string
	Items         []PlannedItem
	Subtotal      money.Cents
	ShippingTotal money.Cents
	DiscountTotal money.Cents
}

func (o *PlannedOrder) Total() money.Cents {
	return o.Subtotal + o.ShippingTotal - o.DiscountTotal
}

type CheckoutPlan struct {
	CartID    string
	AddressID string
	Address   AddressSnapshot
	Orders    []PlannedOrder
}

// PlanOrders splits the cart into one order per seller (RF-013, RF-014). Shipping is charged once per order:
// the most expensive option chosen among its items, since a seller sends everything in one package.
// Discounts come with payment (Slice 6), so they are zero here.
func PlanOrders(cart *CartEntity, shipping []ShippingSelection) ([]PlannedOrder, error) {
	if len(cart.Items) == 0 {
		return nil, app.ErrCartEmpty
	}

	shortages := []app.StockShortage{}
	for _, item := range cart.Items {
		if item.Problem() == "" {
			continue
		}
		available := item.Stock
		if item.AdStatus != AdvertisementStatusPublished {
			available = 0
		}
		shortages = append(shortages, app.StockShortage{AdvertisementID: item.AdvertisementID, Requested: item.Quantity, Available: available})
	}
	if len(shortages) > 0 {
		return nil, &app.OutOfStockError{Items: shortages}
	}

	chosen := make(map[string]string, len(shipping))
	for _, s := range shipping {
		if _, repeated := chosen[s.AdvertisementID]; repeated || cart.Item(s.AdvertisementID) == nil {
			return nil, fmt.Errorf("%w: shipping lists an item that is not in the cart or lists it twice", app.ErrInvalidInput)
		}
		chosen[s.AdvertisementID] = s.ShippingOptionID
	}

	orders := []PlannedOrder{}
	orderIndex := map[string]int{}
	missing := []string{}
	for _, item := range cart.Items {
		optionIndex := slices.IndexFunc(item.ShippingOptions, func(o ShippingOptionEntity) bool { return o.ID == chosen[item.AdvertisementID] })
		if optionIndex < 0 {
			missing = append(missing, item.AdvertisementID)
			continue
		}
		option := item.ShippingOptions[optionIndex]

		index, ok := orderIndex[item.SellerID]
		if !ok {
			index = len(orders)
			orderIndex[item.SellerID] = index
			orders = append(orders, PlannedOrder{SellerID: item.SellerID, SellerName: item.SellerName})
		}
		order := &orders[index]
		order.Items = append(order.Items, PlannedItem{CartItemEntity: item, ShippingOption: option})
		order.Subtotal += item.Subtotal()
		order.ShippingTotal = max(order.ShippingTotal, money.MustParse(option.Price))
	}
	if len(missing) > 0 {
		return nil, &app.ShippingRequiredError{AdvertisementIDs: missing}
	}
	return orders, nil
}

// Crockford's base32 alphabet: no I, L, O or U, so a number read aloud is not mistaken.
const orderNumberAlphabet = "0123456789ABCDEFGHJKMNPQRSTVWXYZ"

// Brazil has had no daylight saving time since 2019, so the order date can use a fixed offset.
var brasilia = time.FixedZone("BRT", -3*60*60)

// NewOrderNumber returns SC-YYYYMMDD-XXXXXX with the date in Brasília; orders.number is unique, so a repeat fails the insert.
func NewOrderNumber(now time.Time) string {
	random := make([]byte, 6)
	_, _ = rand.Read(random)
	suffix := make([]byte, len(random))
	for i, b := range random {
		suffix[i] = orderNumberAlphabet[int(b)%len(orderNumberAlphabet)]
	}
	return fmt.Sprintf("SC-%s-%s", now.In(brasilia).Format("20060102"), suffix)
}

func (p *CheckoutPlan) ToOutput() *CheckoutPreviewOutput {
	output := &CheckoutPreviewOutput{Address: p.Address, Orders: make([]PlannedOrderOutput, len(p.Orders))}
	var subtotal, shippingTotal, discountTotal money.Cents
	for i, order := range p.Orders {
		items := make([]OrderItemOutput, len(order.Items))
		for j, item := range order.Items {
			items[j] = OrderItemOutput{
				AdvertisementID: item.AdvertisementID,
				Title:           item.Title,
				CoverImageURL:   item.CoverImageURL,
				Quantity:        item.Quantity,
				UnitPrice:       item.Price,
				Subtotal:        item.Subtotal().String(),
				ShippingOption:  item.ShippingOption.ToOutput(),
			}
		}
		output.Orders[i] = PlannedOrderOutput{
			Seller:        SellerOutput{ID: order.SellerID, Name: order.SellerName},
			Items:         items,
			Subtotal:      order.Subtotal.String(),
			ShippingTotal: order.ShippingTotal.String(),
			DiscountTotal: order.DiscountTotal.String(),
			Total:         order.Total().String(),
		}
		subtotal += order.Subtotal
		shippingTotal += order.ShippingTotal
		discountTotal += order.DiscountTotal
	}
	output.Subtotal = subtotal.String()
	output.ShippingTotal = shippingTotal.String()
	output.DiscountTotal = discountTotal.String()
	output.Total = (subtotal + shippingTotal - discountTotal).String()
	return output
}
