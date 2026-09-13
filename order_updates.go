package main

import (
	"errors"
	"fmt"
)

type OrderStage string

const (
	StageCheckout    OrderStage = "checkout"
	StageFulfillment OrderStage = "fulfillment"
	StageReceipt     OrderStage = "receipt"
)

type OrderUpdate struct {
	OrderID   string     `json:"order_id"`
	Stage     OrderStage `json:"stage"`
	Message   string     `json:"message"`
	AccountID string     `json:"account_id"`
}

var nextStage = map[OrderStage]OrderStage{
	"":               StageCheckout,
	StageCheckout:    StageFulfillment,
	StageFulfillment: StageReceipt,
}

func validateTransition(current OrderStage, update OrderUpdate) error {
	if update.OrderID == "" || update.AccountID == "" || update.Message == "" {
		return errors.New("order_id, account_id, and message are required")
	}
	want, ok := nextStage[current]
	if !ok || update.Stage != want {
		return fmt.Errorf("order stage %q must follow %q", update.Stage, current)
	}
	return nil
}

func orderChannel(orderID string) string {
	return "order:" + orderID
}
