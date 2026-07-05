package marketdata

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/example/invest-portfolio-platform/backend/internal/models"
)

type MOEXClient struct {
	baseURL string
	http    *http.Client
}

func NewMOEXClient(baseURL string) *MOEXClient {
	if baseURL == "" {
		baseURL = "https://iss.moex.com/iss"
	}
	return &MOEXClient{baseURL: strings.TrimRight(baseURL, "/"), http: &http.Client{Timeout: 8 * time.Second}}
}

func (c *MOEXClient) LatestPrice(ctx context.Context, asset models.Asset) (*models.AssetPrice, error) {
	secID := strings.ToUpper(asset.Ticker)
	u, err := url.Parse(fmt.Sprintf("%s/engines/stock/markets/shares/securities/%s.json", c.baseURL, secID))
	if err != nil {
		return nil, err
	}
	q := u.Query()
	q.Set("iss.meta", "off")
	q.Set("iss.only", "marketdata")
	q.Set("marketdata.columns", "SECID,LAST,LCURRENTPRICE,MARKETPRICE,LASTTOPREVPRICE,UPDATETIME")
	u.RawQuery = q.Encode()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u.String(), nil)
	if err != nil {
		return nil, err
	}
	resp, err := c.http.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, fmt.Errorf("moex status %d", resp.StatusCode)
	}
	var payload struct {
		MarketData struct {
			Columns []string        `json:"columns"`
			Data    [][]interface{} `json:"data"`
		} `json:"marketdata"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&payload); err != nil {
		return nil, err
	}
	if len(payload.MarketData.Data) == 0 {
		return nil, errors.New("moex empty marketdata")
	}
	row := payload.MarketData.Data[0]
	idx := map[string]int{}
	for i, c := range payload.MarketData.Columns {
		idx[c] = i
	}
	price := firstNumber(row, idx, "LAST", "LCURRENTPRICE", "MARKETPRICE")
	if price <= 0 {
		return nil, errors.New("moex price not found")
	}
	currency := asset.CurrencyCode
	if currency == "" {
		currency = "RUB"
	}
	change := firstNumber(row, idx, "LASTTOPREVPRICE")
	return &models.AssetPrice{AssetID: asset.ID, Ticker: asset.Ticker, Price: price, Currency: currency, Source: "moex", UpdatedAt: time.Now().UTC(), Change24hPercent: &change}, nil
}

func firstNumber(row []interface{}, idx map[string]int, names ...string) float64 {
	for _, name := range names {
		i, ok := idx[name]
		if !ok || i >= len(row) || row[i] == nil {
			continue
		}
		switch v := row[i].(type) {
		case float64:
			return v
		case int:
			return float64(v)
		}
	}
	return 0
}

func (c *MOEXClient) Candles(ctx context.Context, asset models.Asset, timeframe string) ([]models.PriceCandle, error) {
	secID := strings.ToUpper(asset.Ticker)
	u, err := url.Parse(fmt.Sprintf("%s/engines/stock/markets/shares/securities/%s/candles.json", c.baseURL, secID))
	if err != nil {
		return nil, err
	}
	from, till, interval := moexCandleRequest(timeframe)
	q := u.Query()
	q.Set("iss.meta", "off")
	q.Set("from", from.Format("2006-01-02"))
	q.Set("till", till.Format("2006-01-02"))
	q.Set("interval", interval)
	q.Set("candles.columns", "begin,open,high,low,close,value,volume")
	u.RawQuery = q.Encode()

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u.String(), nil)
	if err != nil {
		return nil, err
	}
	resp, err := c.http.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, fmt.Errorf("moex candles status %d", resp.StatusCode)
	}

	var payload struct {
		Candles struct {
			Columns []string        `json:"columns"`
			Data    [][]interface{} `json:"data"`
		} `json:"candles"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&payload); err != nil {
		return nil, err
	}
	if len(payload.Candles.Data) == 0 {
		return nil, errors.New("moex empty candles")
	}
	idx := map[string]int{}
	for i, name := range payload.Candles.Columns {
		idx[strings.ToLower(name)] = i
	}

	out := make([]models.PriceCandle, 0, len(payload.Candles.Data))
	for _, row := range payload.Candles.Data {
		recordedAt := moexTime(rowString(row, idx, "begin"))
		openPrice := rowNumber(row, idx, "open")
		highPrice := rowNumber(row, idx, "high")
		lowPrice := rowNumber(row, idx, "low")
		closePrice := rowNumber(row, idx, "close")
		volume := rowNumber(row, idx, "volume")
		if closePrice <= 0 || recordedAt.IsZero() {
			continue
		}
		if openPrice <= 0 {
			openPrice = closePrice
		}
		if highPrice <= 0 {
			highPrice = closePrice
		}
		if lowPrice <= 0 {
			lowPrice = closePrice
		}
		out = append(out, models.PriceCandle{
			AssetID:    asset.ID,
			Timeframe:  normalizeMarketTimeframe(timeframe),
			OpenPrice:  openPrice,
			HighPrice:  highPrice,
			LowPrice:   lowPrice,
			ClosePrice: closePrice,
			Volume:     volume,
			RecordedAt: recordedAt,
		})
	}
	if len(out) == 0 {
		return nil, errors.New("moex no valid candles")
	}
	return out, nil
}

func moexCandleRequest(tf string) (time.Time, time.Time, string) {
	now := time.Now().UTC()
	switch normalizeMarketTimeframe(tf) {
	case "1d":
		return now.AddDate(0, 0, -2), now, "10"
	case "1w":
		return now.AddDate(0, 0, -10), now, "60"
	case "1m":
		return now.AddDate(0, -1, -3), now, "24"
	case "3m":
		return now.AddDate(0, -3, -3), now, "24"
	case "1y":
		return now.AddDate(-1, 0, -7), now, "24"
	default:
		return now.AddDate(0, 0, -2), now, "10"
	}
}

func moexTime(v string) time.Time {
	v = strings.TrimSpace(v)
	if v == "" {
		return time.Time{}
	}
	layouts := []string{
		"2006-01-02 15:04:05",
		"2006-01-02T15:04:05",
		"2006-01-02",
		time.RFC3339,
	}
	for _, layout := range layouts {
		if t, err := time.ParseInLocation(layout, v, time.Local); err == nil {
			return t.UTC()
		}
	}
	return time.Time{}
}

func rowString(row []interface{}, idx map[string]int, name string) string {
	i, ok := idx[strings.ToLower(name)]
	if !ok || i >= len(row) || row[i] == nil {
		return ""
	}
	if v, ok := row[i].(string); ok {
		return v
	}
	return fmt.Sprint(row[i])
}

func rowNumber(row []interface{}, idx map[string]int, name string) float64 {
	i, ok := idx[strings.ToLower(name)]
	if !ok || i >= len(row) || row[i] == nil {
		return 0
	}
	switch v := row[i].(type) {
	case float64:
		return v
	case int:
		return float64(v)
	case string:
		var f float64
		_, _ = fmt.Sscanf(v, "%f", &f)
		return f
	default:
		return 0
	}
}
