# Backend

Go REST API для MVP инвестиционной аналитико-агрегационной платформы.

## Что реализовано во второй итерации

- JWT-аутентификация:
  - `POST /auth/register`
  - `POST /auth/login`
  - `GET /auth/me`
  - `POST /auth/refresh`
  - `POST /auth/logout`
- Портфели:
  - `GET /portfolios`
  - `POST /portfolios`
  - `GET /portfolios/{id}`
- Сделки и позиции:
  - `POST /portfolios/{id}/transactions`
  - `GET /portfolios/{id}/transactions`
  - `GET /portfolios/{id}/positions`
  - `GET /portfolios/{id}/summary`
- Активы:
  - `GET /assets`
  - `GET /assets/search?q=btc`
  - `GET /assets/{id}`
  - `GET /assets/{id}/price-history?timeframe=1d`
- Mock новости:
  - `GET /news`

## Локальный запуск

```bash
cp ../.env.example ../.env
docker compose up --build
```

Проверка:

```bash
curl http://localhost:8080/health
```

## Быстрый сценарий через curl

```bash
curl -X POST http://localhost:8080/auth/register \
  -H 'Content-Type: application/json' \
  -d '{"email":"user@example.com","password":"StrongPass123","display_name":"Алексей"}'

TOKEN=$(curl -s -X POST http://localhost:8080/auth/login \
  -H 'Content-Type: application/json' \
  -d '{"email":"user@example.com","password":"StrongPass123"}' | jq -r .access_token)

curl http://localhost:8080/auth/me -H "Authorization: Bearer $TOKEN"

PORTFOLIO_ID=$(curl -s -X POST http://localhost:8080/portfolios \
  -H "Authorization: Bearer $TOKEN" \
  -H 'Content-Type: application/json' \
  -d '{"name":"Основной портфель","base_currency":"USD"}' | jq -r .id)

curl -X POST http://localhost:8080/portfolios/$PORTFOLIO_ID/transactions \
  -H "Authorization: Bearer $TOKEN" \
  -H 'Content-Type: application/json' \
  -d '{"asset_id":"10000000-0000-0000-0000-000000000001","transaction_type":"buy","quantity":0.1,"unit_price":98000,"fee_amount":5}'

curl http://localhost:8080/portfolios/$PORTFOLIO_ID/summary -H "Authorization: Bearer $TOKEN"
```

## Важное замечание

В текущей итерации refresh/logout реализованы упрощённо: refresh token подписывается отдельным секретом, но полноценное хранение и отзыв сессий через `user_sessions` будет добавлено позже.

## V5: Market Data + Analytics

V5 adds backend market-data and analytics endpoints for the asset analytics screen.

### External data strategy

The project uses reliable fallback logic:

1. Redis cache for fresh quotes.
2. CoinMarketCap for crypto assets when `COINMARKETCAP_API_KEY` is configured.
3. MOEX ISS for MOEX stocks.
4. PostgreSQL `price_history` fallback when an external API is unavailable, the API key is missing, or a rate limit/network error occurs.

### Required `.env` values

```env
COINMARKETCAP_API_KEY=your_key_here
COINMARKETCAP_BASE_URL=https://pro-api.coinmarketcap.com
MOEX_ISS_BASE_URL=https://iss.moex.com/iss
MARKET_DATA_CACHE_TTL_SECONDS=60
```

### New endpoints

```http
GET /assets/{id}/price
GET /analytics/assets/{id}/chart?timeframe=1d
GET /analytics/assets/{id}/indicators?timeframe=1d
GET /analytics/assets/{id}/overview?timeframe=1d
```

### Indicator calculations

The backend calculates indicators from `price_history.close_price`:

- RSI(14)
- MACD(12, 26, 9)
- Bollinger Bands(20, 2)
- SMA(20)
- EMA(50)

If there are too few historical candles, the MVP enriches the chart with generated demo candles so that the analytics screen remains usable during demonstrations.

## V6 news aggregation

The backend uses a resilient news flow:

- Finam RSS for stock-market news.
- RBC Crypto public page extraction for crypto news.
- Redis cache with `NEWS_CACHE_TTL_SECONDS`.
- Local fallback items when external sources are unavailable.

Examples:

```bash
curl.exe "http://localhost:8080/news?market=stock"
curl.exe "http://localhost:8080/news?market=crypto"
```
