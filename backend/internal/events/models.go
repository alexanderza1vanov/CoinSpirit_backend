package events

import "time"

type PriceUpdatedEvent struct {
	AssetID      string    `json:"asset_id"`
	Ticker       string    `json:"ticker"`
	Price        float64   `json:"price"`
	CurrencyCode string    `json:"currency_code"`
	Source       string    `json:"source"`
	UpdatedAt    time.Time `json:"updated_at"`
}

type AlertTriggeredEvent struct {
	AlertEventID string    `json:"alert_event_id"`
	AlertRuleID  string    `json:"alert_rule_id"`
	UserID       string    `json:"user_id"`
	AssetID      string    `json:"asset_id"`
	Ticker       string    `json:"ticker"`
	CurrencyCode string    `json:"currency_code"`
	Email        string    `json:"email"`
	Message      string    `json:"message"`
	TriggerValue float64   `json:"trigger_value"`
	TriggeredAt  time.Time `json:"triggered_at"`
}

type NotificationDeliveryRequestedEvent struct {
	AlertEventID string    `json:"alert_event_id"`
	UserID       string    `json:"user_id"`
	Channel      string    `json:"channel"`
	Recipient    string    `json:"recipient"`
	Subject      string    `json:"subject"`
	Body         string    `json:"body"`
	RequestedAt  time.Time `json:"requested_at"`
}
