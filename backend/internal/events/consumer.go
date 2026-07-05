package events

import (
	"context"
	"crypto/tls"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"log"
	"net"
	"net/smtp"
	"strings"
	"time"

	"github.com/example/invest-portfolio-platform/backend/internal/config"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/segmentio/kafka-go"
)

type AlertConsumer struct {
	db       *pgxpool.Pool
	cfg      config.Config
	producer *Producer
	reader   *kafka.Reader
}

type NotificationConsumer struct {
	db       *pgxpool.Pool
	cfg      config.Config
	producer *Producer
	reader   *kafka.Reader
}

func NewAlertConsumer(db *pgxpool.Pool, cfg config.Config, producer *Producer) *AlertConsumer {
	return &AlertConsumer{
		db:       db,
		cfg:      cfg,
		producer: producer,
		reader: kafka.NewReader(kafka.ReaderConfig{
			Brokers:        cfg.KafkaBrokers,
			Topic:          TopicMarketPriceUpdated,
			GroupID:        cfg.KafkaGroupID + "-alerts",
			MinBytes:       1,
			MaxBytes:       10e6,
			CommitInterval: time.Second,
		}),
	}
}

func NewNotificationConsumer(db *pgxpool.Pool, cfg config.Config, producer *Producer) *NotificationConsumer {
	return &NotificationConsumer{
		db:       db,
		cfg:      cfg,
		producer: producer,
		reader: kafka.NewReader(kafka.ReaderConfig{
			Brokers:        cfg.KafkaBrokers,
			Topic:          TopicAlertTriggered,
			GroupID:        cfg.KafkaGroupID + "-notifications",
			MinBytes:       1,
			MaxBytes:       10e6,
			CommitInterval: time.Second,
		}),
	}
}

func (c *AlertConsumer) Start(ctx context.Context) {
	log.Printf("alert consumer started: topic=%s", TopicMarketPriceUpdated)
	defer c.reader.Close()
	for {
		msg, err := c.reader.ReadMessage(ctx)
		if err != nil {
			if ctx.Err() != nil {
				log.Println("alert consumer stopped")
				return
			}
			log.Printf("alert consumer read error: %v", err)
			continue
		}
		var event PriceUpdatedEvent
		if err := json.Unmarshal(msg.Value, &event); err != nil {
			log.Printf("alert consumer invalid message: %v", err)
			continue
		}
		if event.AssetID == "" || event.Price <= 0 {
			continue
		}
		if err := c.handlePriceUpdated(ctx, event); err != nil {
			log.Printf("alert consumer handle error: %v", err)
		}
	}
}

func (c *AlertConsumer) handlePriceUpdated(ctx context.Context, event PriceUpdatedEvent) error {
	rows, err := c.db.Query(ctx, `
		SELECT ar.id::text, ar.user_id::text, ar.asset_id::text, ar.target_value, ar.comparison_operator,
		       a.ticker, a.currency_code, u.email
		FROM alert_rules ar
		JOIN assets a ON a.id = ar.asset_id
		JOIN users u ON u.id = ar.user_id
		WHERE ar.asset_id=$1 AND ar.is_enabled=true
	`, event.AssetID)
	if err != nil {
		return err
	}
	defer rows.Close()

	cooldown := c.cfg.AlertEventCooldownSeconds
	if cooldown <= 0 {
		cooldown = 300
	}

	for rows.Next() {
		var ruleID, userID, assetID, op, ticker, currency, email string
		var target float64
		if err := rows.Scan(&ruleID, &userID, &assetID, &target, &op, &ticker, &currency, &email); err != nil {
			return err
		}
		triggered := (op == "above" && event.Price >= target) || (op == "below" && event.Price <= target)
		if !triggered {
			continue
		}
		var recentExists bool
		if err := c.db.QueryRow(ctx, `
			SELECT EXISTS (
				SELECT 1 FROM alert_events
				WHERE alert_rule_id=$1 AND triggered_at > NOW() - ($2 || ' seconds')::interval
			)
		`, ruleID, cooldown).Scan(&recentExists); err != nil {
			return err
		}
		if recentExists {
			continue
		}
		message := fmt.Sprintf("%s: текущая цена %.2f %s, условие %s %.2f %s", ticker, event.Price, currency, translateOperator(op), target, currency)
		var eventID string
		if err := c.db.QueryRow(ctx, `
			INSERT INTO alert_events (alert_rule_id, trigger_value, event_status, message, sent_at)
			VALUES ($1, $2, 'triggered', $3, NOW()) RETURNING id::text
		`, ruleID, event.Price, message).Scan(&eventID); err != nil {
			return err
		}
		alertEvent := AlertTriggeredEvent{
			AlertEventID: eventID,
			AlertRuleID:  ruleID,
			UserID:       userID,
			AssetID:      assetID,
			Ticker:       ticker,
			CurrencyCode: currency,
			Email:        email,
			Message:      message,
			TriggerValue: event.Price,
			TriggeredAt:  time.Now(),
		}
		if err := c.producer.PublishAlertTriggered(ctx, alertEvent); err != nil {
			log.Printf("failed to publish %s: %v", TopicAlertTriggered, err)
			// Fallback: deliver directly when Kafka publish fails after the event was created.
			_ = deliverNotification(ctx, c.db, c.cfg, alertEvent)
		}
		log.Printf("alert triggered through kafka: %s", message)
	}
	return rows.Err()
}

func (c *NotificationConsumer) Start(ctx context.Context) {
	log.Printf("notification consumer started: topic=%s", TopicAlertTriggered)
	defer c.reader.Close()
	for {
		msg, err := c.reader.ReadMessage(ctx)
		if err != nil {
			if ctx.Err() != nil {
				log.Println("notification consumer stopped")
				return
			}
			log.Printf("notification consumer read error: %v", err)
			continue
		}
		var event AlertTriggeredEvent
		if err := json.Unmarshal(msg.Value, &event); err != nil {
			log.Printf("notification consumer invalid message: %v", err)
			continue
		}
		if event.AlertEventID == "" || event.Email == "" {
			continue
		}
		if err := deliverNotification(ctx, c.db, c.cfg, event); err != nil {
			log.Printf("notification delivery error: %v", err)
		}
	}
}

func deliverNotification(ctx context.Context, db *pgxpool.Pool, cfg config.Config, event AlertTriggeredEvent) error {
	_, _ = db.Exec(ctx, `
		INSERT INTO notification_deliveries (alert_event_id, channel, recipient, delivery_status, provider_response, delivered_at)
		VALUES ($1, 'in_app', $2, 'sent', 'created by kafka notification consumer', NOW())
	`, event.AlertEventID, event.Email)

	emailStatus, providerResponse := "mocked", "SMTP is disabled; email delivery was simulated"
	if cfg.SMTPEnabled {
		if err := SendAlertEmail(cfg, event.Email, event.Ticker, event.Message); err != nil {
			emailStatus, providerResponse = "failed", err.Error()
		} else {
			emailStatus, providerResponse = "sent", "email sent via SMTP"
		}
	}
	_, err := db.Exec(ctx, `
		INSERT INTO notification_deliveries (alert_event_id, channel, recipient, delivery_status, provider_response, delivered_at)
		VALUES ($1, 'email', $2, $3, $4, NOW())
	`, event.AlertEventID, event.Email, emailStatus, providerResponse)
	if err == nil {
		log.Printf("notification delivered through kafka: %s email=%s", event.Message, emailStatus)
	}
	return err
}

func SendAlertEmail(cfg config.Config, to string, ticker string, message string) error {
	if cfg.SMTPHost == "" || cfg.SMTPUsername == "" || cfg.SMTPPassword == "" || cfg.SMTPFrom == "" {
		return fmt.Errorf("SMTP is not fully configured")
	}
	subject := fmt.Sprintf("CoinSpirit: сработало уведомление %s", ticker)
	fromName := cfg.SMTPFromName
	if fromName == "" {
		fromName = "CoinSpirit"
	}
	body := "Здравствуйте!\n\nСработало правило уведомления:\n\n" + message + "\n\nОткройте приложение, чтобы посмотреть подробности.\n\nCoinSpirit"
	msg := strings.Builder{}
	msg.WriteString(fmt.Sprintf("From: %s <%s>\r\n", encodeHeader(fromName), cfg.SMTPFrom))
	msg.WriteString(fmt.Sprintf("To: %s\r\n", to))
	msg.WriteString(fmt.Sprintf("Subject: %s\r\n", encodeHeader(subject)))
	msg.WriteString("MIME-Version: 1.0\r\n")
	msg.WriteString("Content-Type: text/plain; charset=UTF-8\r\n")
	msg.WriteString("\r\n")
	msg.WriteString(body)
	addr := fmt.Sprintf("%s:%d", cfg.SMTPHost, cfg.SMTPPort)
	if cfg.SMTPPort == 465 {
		conn, err := tls.Dial("tcp", addr, &tls.Config{ServerName: cfg.SMTPHost})
		if err != nil {
			return err
		}
		defer conn.Close()
		client, err := smtp.NewClient(conn, cfg.SMTPHost)
		if err != nil {
			return err
		}
		defer client.Quit()
		if err := client.Auth(smtp.PlainAuth("", cfg.SMTPUsername, cfg.SMTPPassword, cfg.SMTPHost)); err != nil {
			return err
		}
		if err := client.Mail(cfg.SMTPFrom); err != nil {
			return err
		}
		if err := client.Rcpt(to); err != nil {
			return err
		}
		wc, err := client.Data()
		if err != nil {
			return err
		}
		if _, err := wc.Write([]byte(msg.String())); err != nil {
			_ = wc.Close()
			return err
		}
		return wc.Close()
	}
	host, _, _ := net.SplitHostPort(addr)
	return smtp.SendMail(addr, smtp.PlainAuth("", cfg.SMTPUsername, cfg.SMTPPassword, host), cfg.SMTPFrom, []string{to}, []byte(msg.String()))
}

func encodeHeader(value string) string {
	return "=?UTF-8?B?" + base64.StdEncoding.EncodeToString([]byte(value)) + "?="
}

func translateOperator(op string) string {
	if op == "below" {
		return "ниже"
	}
	return "выше"
}
