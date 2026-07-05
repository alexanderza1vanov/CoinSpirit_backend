package marketdata

import (
	"context"
	"time"

	"github.com/example/invest-portfolio-platform/backend/internal/models"
)

type Client interface {
	LatestPrice(ctx context.Context, asset models.Asset) (*models.AssetPrice, error)
}

type Config struct {
	CoinMarketCapAPIKey  string
	CoinMarketCapBaseURL string
	MOEXISSBaseURL       string
	CacheTTL             time.Duration
}
