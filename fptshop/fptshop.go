// Package fptshop is the library behind the fptshop command line:
// the HTTP client, HTML scraping, and typed data models for FPT Shop
// (fptshop.com.vn), Vietnam's major electronics retail chain.
//
// Product detail pages embed JSON-LD Product schema for structured data.
// Category listings are fetched from an internal JSON API at /api/product/list.
// Product URLs follow the pattern: https://fptshop.com.vn/{category}/{slug}.
package fptshop

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"regexp"
	"strconv"
	"strings"
	"time"
)

// Host is the canonical site hostname.
const Host = "fptshop.com.vn"

// baseURL is the site root.
const baseURL = "https://fptshop.com.vn"

// DefaultUserAgent mimics a real browser.
const DefaultUserAgent = "Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/125.0.0.0 Safari/537.36"

// Config holds the tunable knobs for the HTTP client.
type Config struct {
	BaseURL   string
	Rate      time.Duration
	Retries   int
	Timeout   time.Duration
	UserAgent string
}

// DefaultConfig returns sensible production defaults.
func DefaultConfig() Config {
	return Config{
		BaseURL:   baseURL,
		Rate:      2 * time.Second,
		Retries:   3,
		Timeout:   30 * time.Second,
		UserAgent: DefaultUserAgent,
	}
}

// Client talks to the FPT Shop website over HTTP.
type Client struct {
	cfg  Config
	http *http.Client
	last time.Time
}

// NewClient returns a Client from DefaultConfig.
func NewClient() *Client { return NewClientWithConfig(DefaultConfig()) }

// NewClientWithConfig returns a Client built from cfg.
func NewClientWithConfig(cfg Config) *Client {
	return &Client{cfg: cfg, http: &http.Client{Timeout: cfg.Timeout}}
}

// Get fetches rawURL and returns the body bytes, pacing and retrying on transient errors.
func (c *Client) Get(ctx context.Context, rawURL string) ([]byte, error) {
	var lastErr error
	for attempt := 0; attempt <= c.cfg.Retries; attempt++ {
		if attempt > 0 {
			select {
			case <-ctx.Done():
				return nil, ctx.Err()
			case <-time.After(backoff(attempt)):
			}
		}
		body, retry, err := c.do(ctx, rawURL)
		if err == nil {
			return body, nil
		}
		lastErr = err
		if !retry {
			return nil, err
		}
	}
	return nil, fmt.Errorf("get %s: %w", rawURL, lastErr)
}

func (c *Client) do(ctx context.Context, rawURL string) ([]byte, bool, error) {
	c.pace()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, rawURL, nil)
	if err != nil {
		return nil, false, err
	}
	req.Header.Set("User-Agent", c.cfg.UserAgent)
	req.Header.Set("Accept", "text/html,application/json,*/*")
	req.Header.Set("Referer", baseURL+"/")

	resp, err := c.http.Do(req)
	if err != nil {
		return nil, true, err
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode == http.StatusTooManyRequests || resp.StatusCode >= 500 {
		return nil, true, fmt.Errorf("http %d", resp.StatusCode)
	}
	if resp.StatusCode != http.StatusOK {
		return nil, false, fmt.Errorf("http %d", resp.StatusCode)
	}

	b, err := io.ReadAll(resp.Body)
	return b, err != nil, err
}

func (c *Client) pace() {
	if c.cfg.Rate <= 0 {
		return
	}
	if wait := c.cfg.Rate - time.Since(c.last); wait > 0 {
		time.Sleep(wait)
	}
	c.last = time.Now()
}

func backoff(attempt int) time.Duration {
	d := time.Duration(attempt) * 500 * time.Millisecond
	if d > 5*time.Second {
		d = 5 * time.Second
	}
	return d
}

// --- wire JSON-LD types ---

type wireJSONLD struct {
	Type    string           `json:"@type"`
	Name    string           `json:"name"`
	Desc    string           `json:"description"`
	Brand   wireJSONLDBrand  `json:"brand"`
	Offers  wireJSONLDOffer  `json:"offers"`
	Rating  wireJSONLDRating `json:"aggregateRating"`
	SKU     string           `json:"sku"`
}

type wireJSONLDBrand struct {
	Name string `json:"name"`
}

type wireJSONLDOffer struct {
	Price    string `json:"price"`
	OldPrice string `json:"highPrice"`
}

type wireJSONLDRating struct {
	Value       string `json:"ratingValue"`
	ReviewCount string `json:"reviewCount"`
}

// wireProductList is the internal listing API response.
type wireProductList struct {
	Data []wireProduct `json:"data"`
}

type wireProduct struct {
	Name         string  `json:"name"`
	Slug         string  `json:"slug"`
	CategorySlug string  `json:"categorySlug"`
	Price        float64 `json:"price"`
	OldPrice     float64 `json:"oldPrice"`
	RatingPoint  float64 `json:"ratingPoint"`
	ReviewCount  int     `json:"reviewCount"`
	Brand        string  `json:"brand"`
	Thumbnail    string  `json:"thumbnail"`
}

// --- public types ---

// Product is one FPT Shop product.
type Product struct {
	// Path is "{category-slug}/{product-slug}" — the canonical ID.
	Path        string  `json:"path"                    kit:"id" table:"path"`
	Name        string  `json:"name"                             table:"name"`
	URL         string  `json:"url,omitempty"                    table:"url,url"`
	Price       float64 `json:"price"                            table:"price"`
	OldPrice    float64 `json:"old_price,omitempty"              table:"old_price"`
	Brand       string  `json:"brand,omitempty"                  table:"brand"`
	Description string  `json:"description,omitempty"            table:"-"`
	Rating      float64 `json:"rating,omitempty"                 table:"rating"`
	ReviewCount int     `json:"review_count,omitempty"           table:"reviews"`
	FetchedAt   string  `json:"fetched_at,omitempty"             table:"fetched_at"`
}

// Review is one customer review from the FPT Shop API.
type Review struct {
	ID           string `json:"id"                    kit:"id" table:"id"`
	ProductPath  string `json:"product_path"                    table:"product_path"`
	CustomerName string `json:"customer_name,omitempty"         table:"customer_name"`
	Rating       int    `json:"rating"                          table:"rating"`
	Content      string `json:"content,omitempty"               table:"-"`
	HelpfulCount int    `json:"helpful_count,omitempty"         table:"helpful"`
	CreatedAt    string `json:"created_at,omitempty"            table:"created_at"`
	FetchedAt    string `json:"fetched_at,omitempty"            table:"fetched_at"`
}

// --- wire review API ---

type wireReviewList struct {
	Data []wireReview `json:"data"`
}

type wireReview struct {
	ID           int64  `json:"reviewId"`
	CustomerName string `json:"fullName"`
	Rating       int    `json:"ratingPoint"`
	Content      string `json:"content"`
	HelpfulCount int    `json:"helpfulCount"`
	CreatedAt    string `json:"createdDate"`
}

// --- regexps ---

// jsonLdRE finds JSON-LD script blocks in HTML.
var jsonLdRE = regexp.MustCompile(`(?is)<script[^>]+type="application/ld\+json"[^>]*>([\s\S]*?)</script>`)

// --- client methods ---

// GetProduct fetches a product detail page by its two-segment path.
func (c *Client) GetProduct(ctx context.Context, path string) (*Product, error) {
	base := c.cfg.BaseURL
	if base == "" {
		base = baseURL
	}
	path = strings.Trim(path, "/")
	pageURL := base + "/" + path
	body, err := c.Get(ctx, pageURL)
	if err != nil {
		return nil, fmt.Errorf("product %s: %w", path, err)
	}
	p := parseProductPage(body, path, base)
	if p == nil {
		return &Product{Path: path, URL: pageURL, FetchedAt: time.Now().UTC().Format(time.RFC3339)}, nil
	}
	return p, nil
}

// ListProducts fetches products from the internal category listing API.
func (c *Client) ListProducts(ctx context.Context, categorySlug string, page, limit int) ([]*Product, error) {
	if page <= 0 {
		page = 1
	}
	if limit <= 0 {
		limit = 20
	}
	base := c.cfg.BaseURL
	if base == "" {
		base = baseURL
	}
	apiURL := base + "/api/product/list?categorySlug=" + categorySlug + "&page=" + strconv.Itoa(page)
	body, err := c.Get(ctx, apiURL)
	if err != nil {
		return nil, fmt.Errorf("list %s: %w", categorySlug, err)
	}
	return parseProductList(body, limit, base), nil
}

// ListReviews fetches customer reviews for a product by path.
func (c *Client) ListReviews(ctx context.Context, productPath string, limit int) ([]*Review, error) {
	if limit <= 0 {
		limit = 10
	}
	base := c.cfg.BaseURL
	if base == "" {
		base = baseURL
	}
	// Extract slug from path (last segment).
	slug := productPath
	if idx := strings.LastIndex(productPath, "/"); idx >= 0 {
		slug = productPath[idx+1:]
	}
	apiURL := base + "/api/product/reviews?slug=" + slug + "&page=1"
	body, err := c.Get(ctx, apiURL)
	if err != nil {
		return nil, fmt.Errorf("reviews for %s: %w", productPath, err)
	}
	return parseReviews(body, productPath, limit), nil
}

// --- parsers ---

func parseProductPage(body []byte, path, base string) *Product {
	html := string(body)
	now := time.Now().UTC().Format(time.RFC3339)

	for _, m := range jsonLdRE.FindAllStringSubmatch(html, -1) {
		if len(m) < 2 {
			continue
		}
		var ld wireJSONLD
		if err := json.Unmarshal([]byte(m[1]), &ld); err != nil {
			continue
		}
		if ld.Type != "Product" {
			continue
		}
		price, _ := strconv.ParseFloat(strings.ReplaceAll(ld.Offers.Price, ",", ""), 64)
		oldPrice, _ := strconv.ParseFloat(strings.ReplaceAll(ld.Offers.OldPrice, ",", ""), 64)
		rating, _ := strconv.ParseFloat(ld.Rating.Value, 64)
		reviewCount, _ := strconv.Atoi(ld.Rating.ReviewCount)

		return &Product{
			Path:        path,
			Name:        ld.Name,
			URL:         base + "/" + path,
			Price:       price,
			OldPrice:    oldPrice,
			Brand:       ld.Brand.Name,
			Description: ld.Desc,
			Rating:      rating,
			ReviewCount: reviewCount,
			FetchedAt:   now,
		}
	}
	return nil
}

func parseProductList(body []byte, limit int, base string) []*Product {
	var list wireProductList
	if err := json.Unmarshal(body, &list); err != nil {
		return nil
	}
	now := time.Now().UTC().Format(time.RFC3339)
	var out []*Product
	for _, w := range list.Data {
		if len(out) >= limit {
			break
		}
		path := w.CategorySlug + "/" + w.Slug
		if w.CategorySlug == "" {
			path = w.Slug
		}
		out = append(out, &Product{
			Path:        path,
			Name:        w.Name,
			URL:         base + "/" + path,
			Price:       w.Price,
			OldPrice:    w.OldPrice,
			Brand:       w.Brand,
			Rating:      w.RatingPoint,
			ReviewCount: w.ReviewCount,
			FetchedAt:   now,
		})
	}
	return out
}

func parseReviews(body []byte, productPath string, limit int) []*Review {
	var list wireReviewList
	if err := json.Unmarshal(body, &list); err != nil {
		return nil
	}
	now := time.Now().UTC().Format(time.RFC3339)
	var out []*Review
	for _, w := range list.Data {
		if len(out) >= limit {
			break
		}
		out = append(out, &Review{
			ID:           strconv.FormatInt(w.ID, 10),
			ProductPath:  productPath,
			CustomerName: w.CustomerName,
			Rating:       w.Rating,
			Content:      w.Content,
			HelpfulCount: w.HelpfulCount,
			CreatedAt:    w.CreatedAt,
			FetchedAt:    now,
		})
	}
	return out
}

// extractPath extracts the two-segment path from a FPT Shop product URL.
func extractPath(rawURL string) string {
	rawURL = strings.TrimSpace(rawURL)
	// Strip scheme + host.
	if idx := strings.Index(rawURL, "fptshop.com.vn/"); idx >= 0 {
		rawURL = rawURL[idx+len("fptshop.com.vn/"):]
	}
	// Strip query string.
	if idx := strings.Index(rawURL, "?"); idx >= 0 {
		rawURL = rawURL[:idx]
	}
	rawURL = strings.Trim(rawURL, "/")
	return rawURL
}
