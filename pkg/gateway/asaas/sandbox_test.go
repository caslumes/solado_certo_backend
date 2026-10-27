package asaas

import (
	"context"
	"errors"
	"fmt"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/caslumes/solado_certo_backend/pkg/gateway"
)

func sandboxClient(t *testing.T) *Client {
	t.Helper()
	key := os.Getenv("ASAAS_API_KEY")
	if key == "" {
		t.Skip("ASAAS_API_KEY not set")
	}
	baseURL := os.Getenv("ASAAS_BASE_URL")
	if baseURL == "" {
		baseURL = SandboxURL
	}
	if !strings.Contains(baseURL, "sandbox") {
		t.Skipf("ASAAS_BASE_URL %q is not a sandbox", baseURL)
	}
	return New(baseURL, key)
}

func sandboxCharge(t *testing.T, client *Client, method gateway.Method) gateway.ChargeRequest {
	t.Helper()
	customer, err := client.CreateCustomer(context.Background(), gateway.Customer{
		Name: "Cliente Teste Solado Certo", Email: "teste@example.com", Phone: "11999990000", TaxID: "52998224725",
	})
	if err != nil {
		t.Fatalf("CreateCustomer: %v", err)
	}
	return gateway.ChargeRequest{
		CustomerID:  customer,
		Method:      method,
		Amount:      "10.00",
		DueDate:     time.Now().In(brazilTime).AddDate(0, 0, 3),
		Description: "Teste de integração",
		Reference:   fmt.Sprintf("test-%d", time.Now().UnixNano()),
		RemoteIP:    "127.0.0.1",
	}
}

func TestSandboxPixCharge(t *testing.T) {
	client := sandboxClient(t)
	ctx := context.Background()

	charge, err := client.CreateCharge(ctx, sandboxCharge(t, client, gateway.MethodPix))
	if err != nil {
		t.Fatalf("CreateCharge: %v", err)
	}
	if charge.Status != gateway.StatusPending || charge.InvoiceURL == "" {
		t.Errorf("charge = %+v, want pending with an invoice", charge)
	}

	qrCode, err := client.GetPixQRCode(ctx, charge.ID)
	if err != nil || qrCode.Payload == "" || qrCode.ExpiresAt == nil {
		t.Errorf("GetPixQRCode = %+v, %v", qrCode, err)
	}
	got, err := client.GetCharge(ctx, charge.ID)
	if err != nil || got.ID != charge.ID || got.Status != gateway.StatusPending {
		t.Errorf("GetCharge = %+v, %v", got, err)
	}

	if err := client.CancelCharge(ctx, charge.ID); err != nil {
		t.Fatalf("CancelCharge: %v", err)
	}
	if got, err := client.GetCharge(ctx, charge.ID); err != nil || got.Status != gateway.StatusCancelled {
		t.Errorf("after cancelling: GetCharge = %+v, %v, want cancelled", got, err)
	}
}

func TestSandboxBoletoCharge(t *testing.T) {
	client := sandboxClient(t)

	charge, err := client.CreateCharge(context.Background(), sandboxCharge(t, client, gateway.MethodBoleto))

	if err != nil || charge.Status != gateway.StatusPending || charge.BoletoURL == "" {
		t.Errorf("CreateCharge = %+v, %v, want pending with a boleto link", charge, err)
	}
	if err == nil {
		_ = client.CancelCharge(context.Background(), charge.ID)
	}
}

func sandboxCard(t *testing.T, client *Client, number string) gateway.ChargeRequest {
	t.Helper()
	request := sandboxCharge(t, client, gateway.MethodCreditCard)
	request.Card = &gateway.Card{HolderName: "CLIENTE TESTE", Number: number, ExpiryMonth: "12", ExpiryYear: fmt.Sprint(time.Now().Year() + 3), CVV: "123"}
	request.Holder = &gateway.CardHolder{
		Name: "Cliente Teste Solado Certo", Email: "teste@example.com", Phone: "11999990000",
		TaxID: "52998224725", PostalCode: "01310100", AddressNumber: "100",
	}
	return request
}

func TestSandboxCardApproved(t *testing.T) {
	client := sandboxClient(t)

	charge, err := client.CreateCharge(context.Background(), sandboxCard(t, client, "4444444444444444"))

	if err != nil {
		t.Fatalf("CreateCharge: %v", err)
	}
	if charge.Status != gateway.StatusCaptured || charge.CardLast4 != "4444" || charge.CardBrand == "" {
		t.Errorf("charge = %+v, want captured with the card's last digits", charge)
	}

	var refunded *gateway.Charge
	for start := time.Now(); ; {
		refunded, err = client.Refund(context.Background(), charge.ID, gateway.MethodCreditCard)
		if err == nil || time.Since(start) > 90*time.Second {
			break
		}
		t.Logf("refund refused, asking again: %v", err)
		time.Sleep(15 * time.Second)
	}
	if err != nil {
		t.Fatalf("Refund: %v", err)
	}
	if refunded.Status != gateway.StatusRefunded && !refunded.RefundRequested {
		t.Errorf("refund = %+v, want refunded or on its way", refunded)
	}
	t.Logf("refund status: %s", refunded.GatewayStatus)
}

func TestSandboxCardRefused(t *testing.T) {
	client := sandboxClient(t)

	_, err := client.CreateCharge(context.Background(), sandboxCard(t, client, "5184019740373151"))

	refused, ok := errors.AsType[*gateway.CardRefusedError](err)
	if !ok || refused.Message == "" {
		t.Fatalf("error = %v, want CardRefusedError with Asaas's message", err)
	}
	t.Logf("refusal message: %s", refused.Message)
}
