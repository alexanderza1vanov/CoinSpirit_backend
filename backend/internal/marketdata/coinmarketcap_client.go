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

type CoinMarketCapClient struct {
	apiKey  string
	baseURL string
	http    *http.Client
}

func NewCoinMarketCapClient(apiKey string, baseURL string) *CoinMarketCapClient {
	if baseURL == "" {
		baseURL = "https://pro-api.coinmarketcap.com"
	}
	return &CoinMarketCapClient{apiKey: apiKey, baseURL: strings.TrimRight(baseURL, "/"), http: &http.Client{Timeout: 8 * time.Second}}
}

func (c *CoinMarketCapClient) LatestPrice(ctx context.Context, asset models.Asset) (*models.AssetPrice, error) {
	if c.apiKey == "" {
		return nil, errors.New("coinmarketcap api key is empty")
	}
	quoteCurrency := asset.CurrencyCode
	if quoteCurrency == "" {
		quoteCurrency = "USD"
	}
	u, err := url.Parse(c.baseURL + "/v1/cryptocurrency/quotes/latest")
	if err != nil {
		return nil, err
	}
	q := u.Query()
	q.Set("symbol", strings.ToUpper(asset.Ticker))
	q.Set("convert", quoteCurrency)
	u.RawQuery = q.Encode()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u.String(), nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Accept", "application/json")
	req.Header.Set("X-CMC_PRO_API_KEY", c.apiKey)
	resp, err := c.http.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, fmt.Errorf("coinmarketcap status %d", resp.StatusCode)
	}
	var payload struct {
		Data map[string]struct {
			Symbol string `json:"symbol"`
			Quote  map[string]struct {
				Price            float64   `json:"price"`
				LastUpdated      time.Time `json:"last_updated"`
				PercentChange24h float64   `json:"percent_change_24h"`
			} `json:"quote"`
		} `json:"data"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&payload); err != nil {
		return nil, err
	}
	item, ok := payload.Data[strings.ToUpper(asset.Ticker)]
	if !ok {
		return nil, fmt.Errorf("coinmarketcap symbol %s not found", asset.Ticker)
	}
	quote, ok := item.Quote[strings.ToUpper(quoteCurrency)]
	if !ok {
		return nil, fmt.Errorf("coinmarketcap quote %s not found", quoteCurrency)
	}
	updated := quote.LastUpdated
	if updated.IsZero() {
		updated = time.Now().UTC()
	}
	change := quote.PercentChange24h
	return &models.AssetPrice{AssetID: asset.ID, Ticker: asset.Ticker, Price: quote.Price, Currency: quoteCurrency, Source: "coinmarketcap", UpdatedAt: updated, Change24hPercent: &change}, nil
}
