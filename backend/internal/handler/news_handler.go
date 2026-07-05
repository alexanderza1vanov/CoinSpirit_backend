package handler

import (
	"context"
	"encoding/json"
	"encoding/xml"
	"html"
	"io"
	"net/http"
	"regexp"
	"sort"
	"strings"
	"time"

	"github.com/example/invest-portfolio-platform/backend/internal/cache"
	"github.com/example/invest-portfolio-platform/backend/internal/config"
)

type NewsItem struct {
	ID          string    `json:"id"`
	Title       string    `json:"title"`
	Source      string    `json:"source"`
	SourceType  string    `json:"source_type"`
	Market      string    `json:"market"`
	Category    string    `json:"category"`
	Date        string    `json:"date"`
	Description string    `json:"description"`
	FullText    string    `json:"full_text"`
	URL         string    `json:"url"`
	PublishedAt time.Time `json:"published_at"`
}

type rssFeed struct {
	Channel struct {
		Items []rssItem `xml:"item"`
	} `xml:"channel"`
}

type rssItem struct {
	Title       string `xml:"title"`
	Link        string `xml:"link"`
	Description string `xml:"description"`
	PubDate     string `xml:"pubDate"`
}

func RegisterNewsRoutes(router *http.ServeMux, cfg config.Config, redisCache *cache.RedisCache) {
	client := &http.Client{Timeout: 8 * time.Second}
	router.HandleFunc("GET /news", func(w http.ResponseWriter, r *http.Request) {
		ctx := r.Context()
		market := strings.ToLower(r.URL.Query().Get("market"))
		query := strings.ToLower(strings.TrimSpace(r.URL.Query().Get("query")))
		cacheKey := "news:multi:" + market + ":" + query
		if cached, ok := redisCache.Get(ctx, cacheKey); ok {
			var items []NewsItem
			if json.Unmarshal([]byte(cached), &items) == nil {
				writeJSON(w, http.StatusOK, items)
				return
			}
		}

		items := make([]NewsItem, 0, 24)
		if market == "" || market == "stock" {
			items = append(items, loadFinam(ctx, client, cfg)...)
		}
		if market == "" || market == "crypto" {
			items = append(items, loadRBCCrypto(ctx, client, cfg.RBCCryptoURL)...)
		}
		if query != "" {
			filtered := make([]NewsItem, 0, len(items))
			for _, item := range items {
				text := strings.ToLower(item.Title + " " + item.Description + " " + item.Source)
				if strings.Contains(text, query) {
					filtered = append(filtered, item)
				}
			}
			items = filtered
		}
		if len(items) == 0 {
			items = fallbackNews(market)
		}
		sort.SliceStable(items, func(i, j int) bool { return items[i].PublishedAt.After(items[j].PublishedAt) })
		enrichNewsItems(ctx, client, items)
		if len(items) > 30 {
			items = items[:30]
		}
		if b, err := json.Marshal(items); err == nil {
			redisCache.Set(ctx, cacheKey, string(b), time.Duration(cfg.NewsCacheTTLSeconds)*time.Second)
		}
		writeJSON(w, http.StatusOK, items)
	})
}

func loadFinam(ctx context.Context, client *http.Client, cfg config.Config) []NewsItem {
	sources := []struct{ url, category string }{
		{cfg.FinamMarketRSSURL, "Новости и комментарии"},
		{cfg.FinamCompaniesRSSURL, "Новости компаний"},
		{cfg.FinamGlobalRSSURL, "Мировые рынки"},
	}
	out := make([]NewsItem, 0, 18)
	for _, src := range sources {
		items, err := fetchRSS(ctx, client, src.url, "Финам", "stock", src.category)
		if err == nil {
			out = append(out, items...)
		}
	}
	return out
}

func fetchRSS(ctx context.Context, client *http.Client, url string, source string, market string, category string) ([]NewsItem, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("User-Agent", "InvestPortfolioMVP/1.0")
	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(io.LimitReader(resp.Body, 2*1024*1024))
	if err != nil {
		return nil, err
	}
	var feed rssFeed
	if err := xml.Unmarshal(body, &feed); err != nil {
		return nil, err
	}
	out := make([]NewsItem, 0, len(feed.Channel.Items))
	for i, it := range feed.Channel.Items {
		published := parseNewsTime(it.PubDate)
		title := cleanText(it.Title)
		if title == "" {
			continue
		}
		out = append(out, NewsItem{
			ID: sourceID(source, market, title, i), Title: title, Source: source, SourceType: "rss", Market: market, Category: category,
			Date: published.Format("2006-01-02"), Description: cleanText(it.Description), FullText: cleanText(it.Description), URL: strings.TrimSpace(it.Link), PublishedAt: published,
		})
	}
	return out, nil
}

func loadRBCCrypto(ctx context.Context, client *http.Client, url string) []NewsItem {
	if strings.TrimSpace(url) == "" {
		url = "https://www.rbc.ru/crypto/"
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil
	}
	req.Header.Set("User-Agent", "InvestPortfolioMVP/1.0")
	resp, err := client.Do(req)
	if err != nil {
		return nil
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(io.LimitReader(resp.Body, 3*1024*1024))
	if err != nil {
		return nil
	}
	htmlText := string(body)
	// RBC Crypto does not always expose a stable RSS feed, so for MVP we extract article links from the public crypto page.
	re := regexp.MustCompile(`(?s)<a[^>]+href="(https://www\.rbc\.ru/crypto/news/[^"]+)"[^>]*>(.*?)</a>`)
	matches := re.FindAllStringSubmatch(htmlText, -1)
	seen := map[string]bool{}
	out := make([]NewsItem, 0, 12)
	for i, m := range matches {
		link := strings.TrimSpace(m[1])
		title := cleanText(stripTags(m[2]))
		if link == "" || title == "" || seen[link] || len([]rune(title)) < 20 {
			continue
		}
		seen[link] = true
		published := time.Now().Add(-time.Duration(i) * time.Minute)
		out = append(out, NewsItem{ID: sourceID("РБК Крипто", "crypto", title, i), Title: title, Source: "РБК Крипто", SourceType: "html", Market: "crypto", Category: "Криптовалюты", Date: published.Format("2006-01-02"), Description: title, FullText: title, URL: link, PublishedAt: published})
		if len(out) >= 12 {
			break
		}
	}
	return out
}

func fallbackNews(market string) []NewsItem {
	now := time.Now()
	stock := []NewsItem{
		{ID: "fallback-stock-1", Title: "ФИНАМ: Рынок оценивает перспективы снижения ключевой ставки", Source: "Fallback", SourceType: "mock", Market: "stock", Category: "Фондовый рынок", Date: now.Format("2006-01-02"), Description: "Демонстрационная новость используется, если RSS-источник временно недоступен.", FullText: "Демонстрационная новость используется, если RSS-источник временно недоступен. В рабочем режиме приложение получает материалы из русскоязычных финансовых источников и сохраняет ссылку на первоисточник.", URL: "https://www.finam.ru/", PublishedAt: now.Add(-10 * time.Minute)},
		{ID: "fallback-stock-2", Title: "Российские акции торгуются разнонаправленно на фоне внешних сигналов", Source: "Fallback", SourceType: "mock", Market: "stock", Category: "Фондовый рынок", Date: now.Format("2006-01-02"), Description: "Локальная fallback-запись для устойчивой демонстрации.", FullText: "Локальная fallback-запись для устойчивой демонстрации. При доступности внешних источников эта запись заменяется актуальными новостями фондового рынка.", URL: "https://www.finam.ru/", PublishedAt: now.Add(-25 * time.Minute)},
	}
	crypto := []NewsItem{
		{ID: "fallback-crypto-1", Title: "РБК Крипто: Биткоин остаётся волатильным после движения рынка", Source: "Fallback", SourceType: "mock", Market: "crypto", Category: "Криптовалюты", Date: now.Format("2006-01-02"), Description: "Демонстрационная криптоновость используется, если внешний источник недоступен.", FullText: "Демонстрационная криптоновость используется, если внешний источник недоступен. При доступности РБК Крипто приложение показывает актуальные материалы и ссылку на оригинальную публикацию.", URL: "https://www.rbc.ru/crypto/", PublishedAt: now.Add(-15 * time.Minute)},
		{ID: "fallback-crypto-2", Title: "Ethereum и альткоины следуют за динамикой биткоина", Source: "Fallback", SourceType: "mock", Market: "crypto", Category: "Криптовалюты", Date: now.Format("2006-01-02"), Description: "Локальная fallback-запись для раздела криптовалют.", FullText: "Локальная fallback-запись для раздела криптовалют. Она нужна для устойчивой работы приложения при временной недоступности внешнего новостного источника.", URL: "https://www.rbc.ru/crypto/", PublishedAt: now.Add(-35 * time.Minute)},
	}
	if market == "stock" {
		return stock
	}
	if market == "crypto" {
		return crypto
	}
	return append(stock, crypto...)
}

func enrichNewsItems(ctx context.Context, client *http.Client, items []NewsItem) {
	limit := len(items)
	if limit > 12 {
		limit = 12
	}
	for i := 0; i < limit; i++ {
		if items[i].URL == "" {
			continue
		}
		text := fetchArticleText(ctx, client, items[i].URL)
		if len([]rune(text)) > len([]rune(items[i].Description)) {
			items[i].FullText = text
		}
		if items[i].FullText == "" {
			items[i].FullText = items[i].Description
		}
	}
}

func fetchArticleText(ctx context.Context, client *http.Client, link string) string {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, link, nil)
	if err != nil {
		return ""
	}
	req.Header.Set("User-Agent", "InvestPortfolioMVP/1.0")
	resp, err := client.Do(req)
	if err != nil {
		return ""
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(io.LimitReader(resp.Body, 4*1024*1024))
	if err != nil {
		return ""
	}
	h := string(body)
	h = regexp.MustCompile(`(?is)<script[^>]*>.*?</script>`).ReplaceAllString(h, " ")
	h = regexp.MustCompile(`(?is)<style[^>]*>.*?</style>`).ReplaceAllString(h, " ")
	candidates := []string{}
	for _, re := range []*regexp.Regexp{
		regexp.MustCompile(`(?is)<article[^>]*>(.*?)</article>`),
		regexp.MustCompile(`(?is)<div[^>]+class="[^"]*(article|article__text|article__content|news-detail|content)[^"]*"[^>]*>(.*?)</div>`),
	} {
		matches := re.FindAllStringSubmatch(h, -1)
		for _, m := range matches {
			part := m[len(m)-1]
			text := cleanText(part)
			if len([]rune(text)) > 180 {
				candidates = append(candidates, text)
			}
		}
	}
	if len(candidates) == 0 {
		if m := regexp.MustCompile(`(?is)<meta[^>]+property="og:description"[^>]+content="([^"]+)"`).FindStringSubmatch(h); len(m) > 1 {
			return cleanText(m[1])
		}
		return ""
	}
	sort.Slice(candidates, func(i, j int) bool { return len([]rune(candidates[i])) > len([]rune(candidates[j])) })
	text := candidates[0]
	runes := []rune(text)
	if len(runes) > 4000 {
		text = string(runes[:4000]) + "..."
	}
	return text
}

func sourceID(source, market, title string, index int) string {
	s := strings.ToLower(source + "-" + market + "-" + title)
	s = regexp.MustCompile(`[^a-zа-я0-9]+`).ReplaceAllString(s, "-")
	s = strings.Trim(s, "-")
	if len([]rune(s)) > 64 {
		s = string([]rune(s)[:64])
	}
	return s + "-" + time.Now().Format("20060102") + "-" + strings.TrimSpace(string(rune('a'+index%26)))
}

func parseNewsTime(value string) time.Time {
	value = strings.TrimSpace(value)
	formats := []string{time.RFC1123Z, time.RFC1123, time.RFC822Z, time.RFC822, "Mon, 02 Jan 2006 15:04:05 -0700", "02.01.2006 15:04:05"}
	for _, f := range formats {
		if t, err := time.Parse(f, value); err == nil {
			return t
		}
	}
	return time.Now()
}

func cleanText(value string) string {
	value = stripTags(value)
	value = html.UnescapeString(value)
	value = strings.ReplaceAll(value, "\u00a0", " ")
	return strings.Join(strings.Fields(value), " ")
}

func stripTags(value string) string {
	re := regexp.MustCompile(`<[^>]*>`)
	return re.ReplaceAllString(value, " ")
}
