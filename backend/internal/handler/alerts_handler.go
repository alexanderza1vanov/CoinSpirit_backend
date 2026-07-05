package handler

import (
	"crypto/tls"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"net/smtp"
	"strings"
	"time"

	"github.com/example/invest-portfolio-platform/backend/internal/cache"
	"github.com/example/invest-portfolio-platform/backend/internal/config"
	"github.com/example/invest-portfolio-platform/backend/internal/marketdata"
	"github.com/example/invest-portfolio-platform/backend/internal/middleware"
	"github.com/example/invest-portfolio-platform/backend/internal/repository"
	"github.com/jackc/pgx/v5/pgxpool"
)

type createAlertRequest struct {
	PortfolioID        string   `json:"portfolio_id"`
	AssetID            string   `json:"asset_id"`
	RuleType           string   `json:"rule_type"`
	TargetValue        float64  `json:"target_value"`
	ComparisonOperator string   `json:"comparison_operator"`
	Channels           []string `json:"channels"`
	IsEnabled          *bool    `json:"is_enabled"`
}

type updateAlertRequest struct {
	TargetValue        *float64 `json:"target_value"`
	ComparisonOperator string   `json:"comparison_operator"`
	IsEnabled          *bool    `json:"is_enabled"`
}

func RegisterAlertRoutes(router *http.ServeMux, pool *pgxpool.Pool, cfg config.Config, redisCache *cache.RedisCache) {
	assets := repository.NewAssetRepository(pool)
	prices := repository.NewPriceRepository(pool)
	market := marketdata.NewService(assets, prices, redisCache, marketdata.Config{
		CoinMarketCapAPIKey:  cfg.CoinMarketCapAPIKey,
		CoinMarketCapBaseURL: cfg.CoinMarketCapBaseURL,
		MOEXISSBaseURL:       cfg.MOEXISSBaseURL,
		CacheTTL:             time.Duration(cfg.MarketDataCacheTTLSeconds) * time.Second,
	})

	router.Handle("GET /alerts", middleware.JWTAuth(cfg.JWTAccessSecret, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		claims, ok := middleware.ClaimsFromContext(r.Context())
		if !ok {
			writeError(w, http.StatusUnauthorized, "missing claims")
			return
		}
		rows, err := pool.Query(r.Context(), `
			SELECT ar.id::text, COALESCE(ar.portfolio_id::text, ''), ar.asset_id::text, a.ticker, a.name, a.currency_code,
			       ar.rule_type, ar.target_value, ar.comparison_operator, ar.is_enabled, ar.created_at, ar.updated_at
			FROM alert_rules ar
			JOIN assets a ON a.id = ar.asset_id
			WHERE ar.user_id=$1
			ORDER BY ar.created_at DESC
		`, claims.UserID)
		if err != nil {
			writeError(w, http.StatusInternalServerError, "failed to load alerts")
			return
		}
		defer rows.Close()
		items := make([]map[string]any, 0)
		for rows.Next() {
			var id, portfolioID, assetID, ticker, name, currency, ruleType, op string
			var target float64
			var enabled bool
			var created, updated time.Time
			if err := rows.Scan(&id, &portfolioID, &assetID, &ticker, &name, &currency, &ruleType, &target, &op, &enabled, &created, &updated); err != nil {
				writeError(w, http.StatusInternalServerError, "failed to scan alerts")
				return
			}
			items = append(items, map[string]any{"id": id, "portfolio_id": portfolioID, "asset_id": assetID, "ticker": ticker, "asset_name": name, "currency_code": currency, "rule_type": ruleType, "target_value": target, "comparison_operator": op, "is_enabled": enabled, "status": enabledStatus(enabled), "created_at": created, "updated_at": updated})
		}
		writeJSON(w, http.StatusOK, items)
	})))

	router.Handle("POST /alerts", middleware.JWTAuth(cfg.JWTAccessSecret, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		claims, ok := middleware.ClaimsFromContext(r.Context())
		if !ok {
			writeError(w, http.StatusUnauthorized, "missing claims")
			return
		}
		var req createAlertRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			writeError(w, http.StatusBadRequest, "invalid json")
			return
		}
		if req.RuleType == "" {
			req.RuleType = "price"
		}
		if req.ComparisonOperator == "" {
			req.ComparisonOperator = "above"
		}
		if req.AssetID == "" || req.TargetValue <= 0 {
			writeError(w, http.StatusBadRequest, "asset_id and positive target_value are required")
			return
		}
		enabled := true
		if req.IsEnabled != nil {
			enabled = *req.IsEnabled
		}
		var id string
		err := pool.QueryRow(r.Context(), `
			INSERT INTO alert_rules (user_id, portfolio_id, asset_id, rule_type, target_value, comparison_operator, is_enabled)
			VALUES ($1, NULLIF($2,'')::uuid, $3, $4, $5, $6, $7)
			RETURNING id
		`, claims.UserID, req.PortfolioID, req.AssetID, req.RuleType, req.TargetValue, req.ComparisonOperator, enabled).Scan(&id)
		if err != nil {
			writeError(w, http.StatusBadRequest, err.Error())
			return
		}
		writeJSON(w, http.StatusCreated, map[string]any{"id": id})
	})))

	router.Handle("PUT /alerts/{id}", middleware.JWTAuth(cfg.JWTAccessSecret, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		claims, ok := middleware.ClaimsFromContext(r.Context())
		if !ok {
			writeError(w, http.StatusUnauthorized, "missing claims")
			return
		}
		var req updateAlertRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			writeError(w, http.StatusBadRequest, "invalid json")
			return
		}
		_, err := pool.Exec(r.Context(), `
			UPDATE alert_rules
			SET target_value = COALESCE($1, target_value),
			    comparison_operator = COALESCE(NULLIF($2,''), comparison_operator),
			    is_enabled = COALESCE($3, is_enabled),
			    updated_at = NOW()
			WHERE id=$4 AND user_id=$5
		`, req.TargetValue, req.ComparisonOperator, req.IsEnabled, r.PathValue("id"), claims.UserID)
		if err != nil {
			writeError(w, http.StatusBadRequest, err.Error())
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{"status": "updated"})
	})))

	router.Handle("POST /alerts/{id}/toggle", middleware.JWTAuth(cfg.JWTAccessSecret, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		claims, ok := middleware.ClaimsFromContext(r.Context())
		if !ok {
			writeError(w, http.StatusUnauthorized, "missing claims")
			return
		}
		_, err := pool.Exec(r.Context(), `UPDATE alert_rules SET is_enabled = NOT is_enabled, updated_at=NOW() WHERE id=$1 AND user_id=$2`, r.PathValue("id"), claims.UserID)
		if err != nil {
			writeError(w, http.StatusBadRequest, err.Error())
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{"status": "updated"})
	})))

	router.Handle("DELETE /alerts/{id}", middleware.JWTAuth(cfg.JWTAccessSecret, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		claims, ok := middleware.ClaimsFromContext(r.Context())
		if !ok {
			writeError(w, http.StatusUnauthorized, "missing claims")
			return
		}
		_, err := pool.Exec(r.Context(), `DELETE FROM alert_rules WHERE id=$1 AND user_id=$2`, r.PathValue("id"), claims.UserID)
		if err != nil {
			writeError(w, http.StatusBadRequest, err.Error())
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{"status": "deleted"})
	})))

	router.Handle("GET /alerts/events", middleware.JWTAuth(cfg.JWTAccessSecret, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		claims, ok := middleware.ClaimsFromContext(r.Context())
		if !ok {
			writeError(w, http.StatusUnauthorized, "missing claims")
			return
		}
		rows, err := pool.Query(r.Context(), `
			SELECT ae.id::text, ae.alert_rule_id::text, ae.trigger_value, ae.event_status, ae.message, ae.triggered_at, ae.sent_at,
			       a.ticker, a.currency_code
			FROM alert_events ae
			JOIN alert_rules ar ON ar.id = ae.alert_rule_id
			JOIN assets a ON a.id = ar.asset_id
			WHERE ar.user_id=$1
			ORDER BY ae.triggered_at DESC LIMIT 100
		`, claims.UserID)
		if err != nil {
			writeError(w, http.StatusInternalServerError, "failed to load alert events")
			return
		}
		defer rows.Close()
		items := make([]map[string]any, 0)
		for rows.Next() {
			var id, ruleID, status, message, ticker, currency string
			var trigger float64
			var triggered time.Time
			var sent *time.Time
			if err := rows.Scan(&id, &ruleID, &trigger, &status, &message, &triggered, &sent, &ticker, &currency); err != nil {
				writeError(w, http.StatusInternalServerError, "failed to scan events")
				return
			}
			items = append(items, map[string]any{"id": id, "alert_rule_id": ruleID, "trigger_value": trigger, "event_status": status, "message": message, "triggered_at": triggered, "sent_at": sent, "ticker": ticker, "currency_code": currency})
		}
		writeJSON(w, http.StatusOK, items)
	})))

	router.Handle("POST /alerts/check", middleware.JWTAuth(cfg.JWTAccessSecret, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		claims, ok := middleware.ClaimsFromContext(r.Context())
		if !ok {
			writeError(w, http.StatusUnauthorized, "missing claims")
			return
		}
		events, err := checkAlerts(r, pool, cfg, market, claims.UserID)
		if err != nil {
			writeError(w, http.StatusInternalServerError, err.Error())
			return
		}
		writeJSON(w, http.StatusOK, events)
	})))
}

func checkAlerts(r *http.Request, pool *pgxpool.Pool, cfg config.Config, market *marketdata.Service, userID string) ([]map[string]any, error) {
	rows, err := pool.Query(r.Context(), `
		SELECT ar.id::text, ar.asset_id::text, ar.target_value, ar.comparison_operator, a.ticker, a.currency_code, u.email
		FROM alert_rules ar
		JOIN assets a ON a.id = ar.asset_id
		JOIN users u ON u.id = ar.user_id
		WHERE ar.user_id=$1 AND ar.is_enabled=true
	`, userID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := make([]map[string]any, 0)
	for rows.Next() {
		var ruleID, assetID, op, ticker, currency, email string
		var target float64
		if err := rows.Scan(&ruleID, &assetID, &target, &op, &ticker, &currency, &email); err != nil {
			return nil, err
		}
		price, _, err := market.LatestPrice(r.Context(), assetID)
		if err != nil || price == nil {
			continue
		}
		triggered := (op == "above" && price.Price >= target) || (op == "below" && price.Price <= target)
		if !triggered {
			continue
		}
		message := fmt.Sprintf("%s: текущая цена %.2f %s, условие %s %.2f %s", ticker, price.Price, currency, translateOperator(op), target, currency)
		var eventID string
		err = pool.QueryRow(r.Context(), `
			INSERT INTO alert_events (alert_rule_id, trigger_value, event_status, message, sent_at)
			VALUES ($1, $2, 'triggered', $3, NOW()) RETURNING id
		`, ruleID, price.Price, message).Scan(&eventID)
		if err != nil {
			return nil, err
		}
		_, _ = pool.Exec(r.Context(), `INSERT INTO notification_deliveries (alert_event_id, channel, recipient, delivery_status, provider_response, delivered_at) VALUES ($1, 'in_app', $2, 'sent', 'shown in application', NOW())`, eventID, email)
		emailStatus, providerResponse := "mocked", "SMTP is disabled; email delivery was simulated"
		if cfg.SMTPEnabled {
			if err := sendAlertEmail(cfg, email, ticker, message); err != nil {
				emailStatus, providerResponse = "failed", err.Error()
			} else {
				emailStatus, providerResponse = "sent", "email sent via SMTP"
			}
		}
		_, _ = pool.Exec(r.Context(), `INSERT INTO notification_deliveries (alert_event_id, channel, recipient, delivery_status, provider_response, delivered_at) VALUES ($1, 'email', $2, $3, $4, NOW())`, eventID, email, emailStatus, providerResponse)
		out = append(out, map[string]any{"id": eventID, "ticker": ticker, "message": message, "trigger_value": price.Price, "currency_code": currency, "email_status": emailStatus})
	}
	return out, rows.Err()
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

func enabledStatus(enabled bool) string {
	if enabled {
		return "Активно"
	}
	return "Выключено"
}
func translateOperator(op string) string {
	if op == "below" {
		return "ниже"
	}
	return "выше"
}
