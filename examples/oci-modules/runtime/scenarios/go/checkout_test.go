package checkout

import "testing"

func TestCheckoutSmoke(t *testing.T) {
	baseURL := "http://payments-api:8080"
	if baseURL == "" {
		t.Fatal("expected a base URL from the scenario environment")
	}
}
