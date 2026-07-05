package marketdata

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/example/invest-portfolio-platform/backend/internal/models"
)

type CoinGeckoClient struct {
	baseURL string
	http    *http.Client
}

func NewCoinGeckoClient(baseURL string) *CoinGeckoClient {
	if baseURL == "" {
		baseURL = "https://api.coingecko.com/api/v3"
	}
	return &CoinGeckoClient{baseURL: strings.TrimRight(baseURL, "/"), http: &http.Client{Timeout: 10 * time.Second}}
}

func (c *CoinGeckoClient) MarketChart(ctx context.Context, asset models.Asset, timeframe string) ([]models.PriceCandle, error) {
	coinID := coinGeckoID(asset.Ticker)
	if coinID == "" {
		return nil, fmt.Errorf("coingecko id not mapped for %s", asset.Ticker)
	}

	// Use CoinGecko OHLC for analytics charts. market_chart returns only a line
	// series and forces us to invent open/high/low values, which makes the chart
	// look like an area graph instead of real candles.
	days := coinGeckoDays(timeframe)
	u, err := url.Parse(fmt.Sprintf("%s/coins/%s/ohlc", c.baseURL, coinID))
	if err != nil {
		return nil, err
	}
	q := u.Query()
	q.Set("vs_currency", strings.ToLower(defaultString(asset.CurrencyCode, "USD")))
	q.Set("days", days)
	u.RawQuery = q.Encode()

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u.String(), nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Accept", "application/json")
	resp, err := c.http.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, fmt.Errorf("coingecko ohlc status %d", resp.StatusCode)
	}

	var rows [][]float64
	if err := json.NewDecoder(resp.Body).Decode(&rows); err != nil {
		return nil, err
	}
	if len(rows) == 0 {
		return nil, fmt.Errorf("coingecko empty ohlc")
	}

	out := make([]models.PriceCandle, 0, len(rows))
	for _, row := range rows {
		if len(row) < 5 || row[4] <= 0 {
			continue
		}
		millis := int64(row[0])
		openPrice := row[1]
		highPrice := row[2]
		lowPrice := row[3]
		closePrice := row[4]
		if openPrice <= 0 || highPrice <= 0 || lowPrice <= 0 {
			continue
		}
		if highPrice < mathMax(openPrice, closePrice) {
			highPrice = mathMax(openPrice, closePrice)
		}
		if lowPrice > mathMin(openPrice, closePrice) {
			lowPrice = mathMin(openPrice, closePrice)
		}
		out = append(out, models.PriceCandle{
			AssetID:    asset.ID,
			Timeframe:  normalizeMarketTimeframe(timeframe),
			OpenPrice:  openPrice,
			HighPrice:  highPrice,
			LowPrice:   lowPrice,
			ClosePrice: closePrice,
			Volume:     0,
			RecordedAt: time.UnixMilli(millis).UTC(),
		})
	}
	if len(out) == 0 {
		return nil, fmt.Errorf("coingecko no valid ohlc")
	}
	return out, nil
}

func mathMax(a, b float64) float64 {
	if a > b {
		return a
	}
	return b
}

func mathMin(a, b float64) float64 {
	if a < b {
		return a
	}
	return b
}

func coinGeckoID(ticker string) string {
	switch strings.ToUpper(strings.TrimSpace(ticker)) {
	case "BTC", "XBT":
		return "bitcoin"
	case "ETH":
		return "ethereum"
	case "BNB":
		return "binancecoin"
	case "SOL":
		return "solana"
	case "XRP":
		return "ripple"
	case "ADA":
		return "cardano"
	case "DOGE":
		return "dogecoin"
	case "TON":
		return "the-open-network"
	case "DOT":
		return "polkadot"
	case "AVAX":
		return "avalanche-2"
	case "LINK":
		return "chainlink"
	case "LTC":
		return "litecoin"
	default:
		return ""
	}
}

func coinGeckoDays(tf string) string {
	switch normalizeMarketTimeframe(tf) {
	case "1d":
		return "1"
	case "1w":
		return "7"
	case "1m":
		return "30"
	case "3m":
		return "90"
	case "1y":
		return "365"
	default:
		return "1"
	}
}

func defaultString(v string, fallback string) string {
	if strings.TrimSpace(v) == "" {
		return fallback
	}
	return v
}
