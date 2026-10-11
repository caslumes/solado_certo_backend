package catalog

import (
	"testing"

	"github.com/caslumes/solado_certo_backend/internal/helpers"
)

func TestSafetyInfoComplete(t *testing.T) {
	alerts := helpers.StringPtr("Interrompa o uso se houver irritação.")
	contraindications := helpers.StringPtr("Feridas abertas.")
	blank := helpers.StringPtr("   ")
	linked := []*ConditionEntity{{ID: conditionID, Name: "Fascite plantar"}}

	tests := map[string]struct {
		ad         AdvertisementEntity
		conditions []*ConditionEntity
		want       bool
	}{
		"plain item without safety data":                  {ad: AdvertisementEntity{}, want: true},
		"prescription item with both fields":              {ad: AdvertisementEntity{PrescriptionRequired: true, SafetyAlerts: alerts, Contraindications: contraindications}, want: true},
		"prescription item without alerts":                {ad: AdvertisementEntity{PrescriptionRequired: true, Contraindications: contraindications}, want: false},
		"prescription item without contraindications":     {ad: AdvertisementEntity{PrescriptionRequired: true, SafetyAlerts: alerts}, want: false},
		"prescription item with blank alerts":             {ad: AdvertisementEntity{PrescriptionRequired: true, SafetyAlerts: blank, Contraindications: contraindications}, want: false},
		"condition-linked item with both fields":          {ad: AdvertisementEntity{SafetyAlerts: alerts, Contraindications: contraindications}, conditions: linked, want: true},
		"condition-linked item without contraindications": {ad: AdvertisementEntity{SafetyAlerts: alerts}, conditions: linked, want: false},
	}
	for name, tt := range tests {
		t.Run(name, func(t *testing.T) {
			detail := AdvertisementDetailEntity{Advertisement: &tt.ad, Conditions: tt.conditions}
			if got := detail.SafetyInfoComplete(); got != tt.want {
				t.Fatalf("SafetyInfoComplete() = %v, want %v", got, tt.want)
			}
			if got := detail.ToOutput().SafetyInfoComplete; got != tt.want {
				t.Fatalf("output safety_info_complete = %v, want %v", got, tt.want)
			}
		})
	}
}
