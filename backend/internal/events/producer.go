package events

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"time"

	"github.com/segmentio/kafka-go"
)

type Producer struct {
	brokers  []string
	clientID string
}

func NewProducer(brokers []string, clientID string) *Producer {
	return &Producer{brokers: brokers, clientID: clientID}
}

func (p *Producer) PublishPriceUpdated(ctx context.Context, event PriceUpdatedEvent) error {
	return p.publish(ctx, TopicMarketPriceUpdated, event.AssetID, event)
}

func (p *Producer) PublishAlertTriggered(ctx context.Context, event AlertTriggeredEvent) error {
	return p.publish(ctx, TopicAlertTriggered, event.AlertEventID, event)
}

func (p *Producer) PublishNotificationDeliveryRequested(ctx context.Context, event NotificationDeliveryRequestedEvent) error {
	return p.publish(ctx, TopicNotificationDeliveryRequest, event.AlertEventID+":"+event.Channel, event)
}

func (p *Producer) publish(ctx context.Context, topic string, key string, payload any) error {
	if p == nil || len(p.brokers) == 0 {
		return fmt.Errorf("kafka producer is not configured")
	}
	body, err := json.Marshal(payload)
	if err != nil {
		return err
	}
	writer := &kafka.Writer{
		Addr:                   kafka.TCP(p.brokers...),
		Topic:                  topic,
		Balancer:               &kafka.Hash{},
		AllowAutoTopicCreation: true,
		RequiredAcks:           kafka.RequireOne,
		Async:                  false,
		Completion: func(messages []kafka.Message, err error) {
			if err != nil {
				log.Printf("kafka completion error topic=%s: %v", topic, err)
			}
		},
	}
	defer writer.Close()
	writeCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	return writer.WriteMessages(writeCtx, kafka.Message{
		Key:   []byte(key),
		Value: body,
		Time:  time.Now(),
		Headers: []kafka.Header{{
			Key:   "client_id",
			Value: []byte(p.clientID),
		}},
	})
}
