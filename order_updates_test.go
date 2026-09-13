package main

import "testing"

func TestOrderStageTransitions(t *testing.T) {
	tests := []struct {
		name    string
		current OrderStage
		next    OrderStage
		wantErr bool
	}{
		{"checkout starts an order", "", StageCheckout, false},
		{"fulfillment follows checkout", StageCheckout, StageFulfillment, false},
		{"receipt follows fulfillment", StageFulfillment, StageReceipt, false},
		{"receipt cannot skip fulfillment", StageCheckout, StageReceipt, true},
		{"completed order cannot reopen", StageReceipt, StageCheckout, true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			update := OrderUpdate{OrderID: "ord_42", Stage: tt.next, Message: "status changed", AccountID: "acct_7"}
			err := validateTransition(tt.current, update)
			if (err != nil) != tt.wantErr {
				t.Fatalf("validateTransition() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}
