package config

import (
	"os"
	"strconv"
	"strings"
)

type Config struct {
	HTTPPort                  string
	DatabaseURL               string
	RedisAddr                 string
	KafkaEnabled              bool
	KafkaBrokers              []string
	KafkaClientID             string
	KafkaGroupID              string
	JWTAccessSecret           string
	JWTRefreshSecret          string
	CORSAllowedOrigins        string
	CoinMarketCapAPIKey       string
	CoinMarketCapBaseURL      string
	MOEXISSBaseURL            string
	MarketDataCacheTTLSeconds int
	MarketRefreshSeconds      int
	AlertCheckSeconds         int
	AlertEventCooldownSeconds int
	NewsCacheTTLSeconds       int
	FinamCompaniesRSSURL      string
	FinamMarketRSSURL         string
	FinamGlobalRSSURL         string
	RBCCryptoURL              string
	SMTPEnabled               bool
	SMTPHost                  string
	SMTPPort                  int
	SMTPUsername              string
	SMTPPassword              string
	SMTPFrom                  string
	SMTPFromName              string
}

func Load() Config {
	return Config{
		HTTPPort:                  getEnv("HTTP_PORT", "8080"),
		DatabaseURL:               getEnv("DATABASE_URL", "postgres://invest_user:invest_password@localhost:5432/invest_platform?sslmode=disable"),
		RedisAddr:                 getEnv("REDIS_ADDR", "localhost:6379"),
		KafkaEnabled:              getEnvBool("KAFKA_ENABLED", false),
		KafkaBrokers:              splitList(getEnv("KAFKA_BROKERS", "localhost:29092")),
		KafkaClientID:             getEnv("KAFKA_CLIENT_ID", "coinspirit-api"),
		KafkaGroupID:              getEnv("KAFKA_GROUP_ID", "coinspirit-workers"),
		JWTAccessSecret:           getEnv("JWT_ACCESS_SECRET", "dev_access_secret"),
		JWTRefreshSecret:          getEnv("JWT_REFRESH_SECRET", "dev_refresh_secret"),
		CORSAllowedOrigins:        getEnv("CORS_ALLOWED_ORIGINS", "*"),
		CoinMarketCapAPIKey:       getEnv("COINMARKETCAP_API_KEY", ""),
		CoinMarketCapBaseURL:      getEnv("COINMARKETCAP_BASE_URL", "https://pro-api.coinmarketcap.com"),
		MOEXISSBaseURL:            getEnv("MOEX_ISS_BASE_URL", "https://iss.moex.com/iss"),
		MarketDataCacheTTLSeconds: getEnvInt("MARKET_DATA_CACHE_TTL_SECONDS", 60),
		MarketRefreshSeconds:      getEnvInt("MARKET_REFRESH_SECONDS", 60),
		AlertCheckSeconds:         getEnvInt("ALERT_CHECK_SECONDS", 60),
		AlertEventCooldownSeconds: getEnvInt("ALERT_EVENT_COOLDOWN_SECONDS", 300),
		NewsCacheTTLSeconds:       getEnvInt("NEWS_CACHE_TTL_SECONDS", 300),
		FinamCompaniesRSSURL:      getEnv("FINAM_COMPANIES_RSS_URL", "https://www.finam.ru/analysis/conews/rsspoint/"),
		FinamMarketRSSURL:         getEnv("FINAM_MARKET_RSS_URL", "https://www.finam.ru/analysis/nslent/rsspoint/"),
		FinamGlobalRSSURL:         getEnv("FINAM_GLOBAL_RSS_URL", "https://www.finam.ru/international/advanced/rsspoint/"),
		RBCCryptoURL:              getEnv("RBC_CRYPTO_URL", "https://www.rbc.ru/crypto/"),
		SMTPEnabled:               getEnvBool("SMTP_ENABLED", false),
		SMTPHost:                  getEnv("SMTP_HOST", "smtp.gmail.com"),
		SMTPPort:                  getEnvInt("SMTP_PORT", 465),
		SMTPUsername:              getEnv("SMTP_USERNAME", ""),
		SMTPPassword:              getEnv("SMTP_PASSWORD", ""),
		SMTPFrom:                  getEnv("SMTP_FROM", ""),
		SMTPFromName:              getEnv("SMTP_FROM_NAME", "CoinSpirit"),
	}
}

func getEnv(key string, fallback string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return fallback
}

func getEnvInt(key string, fallback int) int {
	value := os.Getenv(key)
	if value == "" {
		return fallback
	}
	parsed, err := strconv.Atoi(value)
	if err != nil {
		return fallback
	}
	return parsed
}

func getEnvBool(key string, fallback bool) bool {
	value := strings.ToLower(strings.TrimSpace(os.Getenv(key)))
	if value == "" {
		return fallback
	}
	return value == "true" || value == "1" || value == "yes" || value == "on"
}

func splitList(value string) []string {
	parts := strings.Split(value, ",")
	out := make([]string, 0, len(parts))
	for _, part := range parts {
		part = strings.TrimSpace(part)
		if part != "" {
			out = append(out, part)
		}
	}
	return out
}
