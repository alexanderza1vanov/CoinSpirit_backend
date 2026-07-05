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
### New endpoints

```http
GET /assets/{id}/price
GET /analytics/assets/{id}/chart?timeframe=1d
GET /analytics/assets/{id}/indicators?timeframe=1d
GET /analytics/assets/{id}/overview?timeframe=1d
```

### Indicators

The backend calculates indicators from `price_history.close_price`:

- RSI(14)
- MACD(12, 26, 9)
- Bollinger Bands(20, 2)
- SMA(20)
- EMA(50)

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
