package worker

import (
	"context"
	"crypto/tls"
	"encoding/base64"
	"fmt"
	"log"
	"net"
	"net/smtp"
	"strings"
	"time"

	"github.com/example/invest-portfolio-platform/backend/internal/config"
	"github.com/example/invest-portfolio-platform/backend/internal/events"
	"github.com/example/invest-portfolio-platform/backend/internal/marketdata"
	"github.com/example/invest-portfolio-platform/backend/internal/repository"
	"github.com/jackc/pgx/v5/pgxpool"
)

type MarketWorker struct {
	db       *pgxpool.Pool
	cfg      config.Config
	market   *marketdata.Service
	assets   *repository.AssetRepository
	producer *events.Producer
}

func NewMarketWorker(db *pgxpool.Pool, cfg config.Config, market *marketdata.Service, producer *events.Producer) *MarketWorker {
	return &MarketWorker{db: db, cfg: cfg, market: market, assets: repository.NewAssetRepository(db), producer: producer}
}

func (w *MarketWorker) Start(ctx context.Context) {
	refreshSeconds := w.cfg.MarketRefreshSeconds
	if refreshSeconds <= 0 {
		refreshSeconds = 60
	}
	ticker := time.NewTicker(time.Duration(refreshSeconds) * time.Second)
	defer ticker.Stop()

	log.Printf("market worker started: refresh=%ds alert_check=%ds", refreshSeconds, w.cfg.AlertCheckSeconds)
	w.runOnce(ctx)
	for {
		select {
		case <-ctx.Done():
			log.Println("market worker stopped")
			return
		case <-ticker.C:
			w.runOnce(ctx)
		}
	}
}

func (w *MarketWorker) runOnce(ctx context.Context) {
	if err := w.RefreshPrices(ctx); err != nil {
		log.Printf("market worker price refresh error: %v", err)
	}
	if !w.cfg.KafkaEnabled {
		if err := w.CheckAlerts(ctx); err != nil {
			log.Printf("market worker alert check error: %v", err)
		}
	}
}

func (w *MarketWorker) RefreshPrices(ctx context.Context) error {
	assets, err := w.assets.List(ctx, "")
	if err != nil {
		return err
	}
	for _, asset := range assets {
		price, source, err := w.market.LatestPrice(ctx, asset.ID)
		if err != nil {
			log.Printf("price refresh skipped for %s: %v", asset.Ticker, err)
			continue
		}
		log.Printf("price refreshed: %s %.2f %s (%s)", price.Ticker, price.Price, price.Currency, source)
		if w.cfg.KafkaEnabled && w.producer != nil {
			event := events.PriceUpdatedEvent{
				AssetID:      asset.ID,
				Ticker:       price.Ticker,
				Price:        price.Price,
				CurrencyCode: price.Currency,
				Source:       source,
				UpdatedAt:    price.UpdatedAt,
			}
			if err := w.producer.PublishPriceUpdated(ctx, event); err != nil {
				log.Printf("failed to publish market.price.updated for %s: %v", asset.Ticker, err)
			} else {
				log.Printf("market.price.updated published: %s %.2f %s", price.Ticker, price.Price, price.Currency)
			}
		}
	}
	return nil
}

func (w *MarketWorker) CheckAlerts(ctx context.Context) error {
	rows, err := w.db.Query(ctx, `
		SELECT ar.id::text, ar.asset_id::text, ar.target_value, ar.comparison_operator, a.ticker, a.currency_code, u.email
		FROM alert_rules ar
		JOIN assets a ON a.id = ar.asset_id
		JOIN users u ON u.id = ar.user_id
		WHERE ar.is_enabled = true
	`)
	if err != nil {
		return err
	}
	defer rows.Close()

	cooldown := w.cfg.AlertEventCooldownSeconds
	if cooldown <= 0 {
		cooldown = 300
	}

	for rows.Next() {
		var ruleID, assetID, op, ticker, currency, email string
		var target float64
		if err := rows.Scan(&ruleID, &assetID, &target, &op, &ticker, &currency, &email); err != nil {
			return err
		}
		price, _, err := w.market.LatestPrice(ctx, assetID)
		if err != nil || price == nil {
			continue
		}
		triggered := (op == "above" && price.Price >= target) || (op == "below" && price.Price <= target)
		if !triggered {
			continue
		}
		var recentExists bool
		if err := w.db.QueryRow(ctx, `
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
		message := fmt.Sprintf("%s: текущая цена %.2f %s, условие %s %.2f %s", ticker, price.Price, currency, translateOperator(op), target, currency)
		var eventID string
		if err := w.db.QueryRow(ctx, `
			INSERT INTO alert_events (alert_rule_id, trigger_value, event_status, message, sent_at)
			VALUES ($1, $2, 'triggered', $3, NOW()) RETURNING id
		`, ruleID, price.Price, message).Scan(&eventID); err != nil {
			return err
		}
		_, _ = w.db.Exec(ctx, `INSERT INTO notification_deliveries (alert_event_id, channel, recipient, delivery_status, provider_response, delivered_at) VALUES ($1, 'in_app', $2, 'sent', 'created by background worker', NOW())`, eventID, email)
		emailStatus, providerResponse := "mocked", "SMTP is disabled; email delivery was simulated"
		if w.cfg.SMTPEnabled {
			if err := sendAlertEmail(w.cfg, email, ticker, message); err != nil {
				emailStatus, providerResponse = "failed", err.Error()
			} else {
				emailStatus, providerResponse = "sent", "email sent via SMTP"
			}
		}
		_, _ = w.db.Exec(ctx, `INSERT INTO notification_deliveries (alert_event_id, channel, recipient, delivery_status, provider_response, delivered_at) VALUES ($1, 'email', $2, $3, $4, NOW())`, eventID, email, emailStatus, providerResponse)
		log.Printf("alert triggered: %s email=%s", message, emailStatus)
	}
	return rows.Err()
}

func sendAlertEmail(cfg config.Config, to string, ticker string, message string) error {
	if cfg.SMTPHost == "" || cfg.SMTPUsername == "" || cfg.SMTPPassword == "" || cfg.SMTPFrom == "" {
		return fmt.Errorf("SMTP is not fully configured")
	}
	subject := fmt.Sprintf("Invest Portfolio: сработало уведомление %s", ticker)
	fromName := cfg.SMTPFromName
	if fromName == "" {
		fromName = "Invest Portfolio"
	}
	body := "Здравствуйте!\n\nСработало правило уведомления:\n\n" + message + "\n\nОткройте приложение, чтобы посмотреть подробности.\n\nInvest Portfolio"
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
