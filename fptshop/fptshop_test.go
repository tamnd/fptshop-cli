package fptshop

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func newTestClient(srv *httptest.Server) *Client {
	cfg := DefaultConfig()
	cfg.BaseURL = srv.URL
	cfg.Rate = 0
	cfg.Retries = 0
	return NewClientWithConfig(cfg)
}

func sampleProductPageHTML(name, brand string, price float64) string {
	ld, _ := json.Marshal(map[string]any{
		"@type":       "Product",
		"name":        name,
		"description": "A great product",
		"brand":       map[string]string{"name": brand},
		"offers": map[string]any{
			"price":     "32990000",
			"highPrice": "35000000",
		},
		"aggregateRating": map[string]string{
			"ratingValue": "4.7",
			"reviewCount": "567",
		},
	})
	return `<!DOCTYPE html><html><head>
<script type="application/ld+json">` + string(ld) + `</script>
</head><body><h1>` + name + `</h1></body></html>`
}

func sampleProductListJSON(n int, category string) string {
	type item struct {
		Name         string  `json:"name"`
		Slug         string  `json:"slug"`
		CategorySlug string  `json:"categorySlug"`
		Price        float64 `json:"price"`
		OldPrice     float64 `json:"oldPrice"`
		RatingPoint  float64 `json:"ratingPoint"`
		ReviewCount  int     `json:"reviewCount"`
		Brand        string  `json:"brand"`
	}
	var items []item
	for i := 0; i < n; i++ {
		items = append(items, item{
			Name:         "Product " + string(rune('A'+i)),
			Slug:         "product-" + string(rune('a'+i)),
			CategorySlug: category,
			Price:        float64(5000000 * (i + 1)),
			OldPrice:     float64(6000000 * (i + 1)),
			RatingPoint:  4.5,
			ReviewCount:  100,
			Brand:        "TestBrand",
		})
	}
	b, _ := json.Marshal(map[string]any{"data": items})
	return string(b)
}

func sampleReviewListJSON(n int) string {
	type r struct {
		ReviewID     int64  `json:"reviewId"`
		FullName     string `json:"fullName"`
		RatingPoint  int    `json:"ratingPoint"`
		Content      string `json:"content"`
		HelpfulCount int    `json:"helpfulCount"`
	}
	var list []r
	for i := 0; i < n; i++ {
		list = append(list, r{
			ReviewID:    int64(2000 + i),
			FullName:    "Customer " + string(rune('A'+i)),
			RatingPoint: 5,
			Content:     "Very good",
		})
	}
	b, _ := json.Marshal(map[string]any{"data": list})
	return string(b)
}

func TestGet(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("User-Agent") == "" {
			t.Error("request carried no User-Agent")
		}
		_, _ = w.Write([]byte("ok"))
	}))
	defer srv.Close()

	c := newTestClient(srv)
	body, err := c.Get(context.Background(), srv.URL)
	if err != nil {
		t.Fatal(err)
	}
	if string(body) != "ok" {
		t.Errorf("body = %q, want ok", body)
	}
}

func TestGetRetriesOn503(t *testing.T) {
	var hits int
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hits++
		if hits < 3 {
			w.WriteHeader(http.StatusServiceUnavailable)
			return
		}
		_, _ = w.Write([]byte("recovered"))
	}))
	defer srv.Close()

	cfg := DefaultConfig()
	cfg.BaseURL = srv.URL
	cfg.Rate = 0
	cfg.Retries = 5
	c := NewClientWithConfig(cfg)

	start := time.Now()
	body, err := c.Get(context.Background(), srv.URL)
	if err != nil {
		t.Fatal(err)
	}
	if string(body) != "recovered" {
		t.Errorf("body = %q after retries", body)
	}
	if hits != 3 {
		t.Errorf("server saw %d hits, want 3", hits)
	}
	if time.Since(start) < 500*time.Millisecond {
		t.Error("retries did not back off")
	}
}

func TestGetProduct(t *testing.T) {
	html := sampleProductPageHTML("iPhone 16 Pro Max 256GB", "Apple", 32990000)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(html))
	}))
	defer srv.Close()

	c := newTestClient(srv)
	p, err := c.GetProduct(context.Background(), "dien-thoai/iphone-16-pro-max-256gb")
	if err != nil {
		t.Fatal(err)
	}
	if p.Path != "dien-thoai/iphone-16-pro-max-256gb" {
		t.Errorf("Path = %q", p.Path)
	}
	if p.Name != "iPhone 16 Pro Max 256GB" {
		t.Errorf("Name = %q", p.Name)
	}
	if p.Brand != "Apple" {
		t.Errorf("Brand = %q", p.Brand)
	}
	if p.Rating != 4.7 {
		t.Errorf("Rating = %v, want 4.7", p.Rating)
	}
	if p.ReviewCount != 567 {
		t.Errorf("ReviewCount = %d, want 567", p.ReviewCount)
	}
}

func TestListProducts(t *testing.T) {
	listJSON := sampleProductListJSON(4, "dien-thoai")
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(listJSON))
	}))
	defer srv.Close()

	c := newTestClient(srv)
	products, err := c.ListProducts(context.Background(), "dien-thoai", 1, 10)
	if err != nil {
		t.Fatal(err)
	}
	if len(products) != 4 {
		t.Fatalf("got %d products, want 4", len(products))
	}
	if products[0].Path != "dien-thoai/product-a" {
		t.Errorf("products[0].Path = %q, want dien-thoai/product-a", products[0].Path)
	}
}

func TestListProductsLimit(t *testing.T) {
	listJSON := sampleProductListJSON(8, "laptop")
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(listJSON))
	}))
	defer srv.Close()

	c := newTestClient(srv)
	products, err := c.ListProducts(context.Background(), "laptop", 1, 3)
	if err != nil {
		t.Fatal(err)
	}
	if len(products) != 3 {
		t.Errorf("got %d products, want 3 (limit respected)", len(products))
	}
}

func TestListReviews(t *testing.T) {
	reviews := sampleReviewListJSON(3)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(reviews))
	}))
	defer srv.Close()

	c := newTestClient(srv)
	got, err := c.ListReviews(context.Background(), "dien-thoai/iphone-16-pro-max-256gb", 10)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 3 {
		t.Fatalf("got %d reviews, want 3", len(got))
	}
	if got[0].ID != "2000" {
		t.Errorf("got[0].ID = %q, want 2000", got[0].ID)
	}
	if got[0].ProductPath != "dien-thoai/iphone-16-pro-max-256gb" {
		t.Errorf("got[0].ProductPath = %q", got[0].ProductPath)
	}
}

func TestExtractPath(t *testing.T) {
	cases := []struct{ in, want string }{
		{"https://fptshop.com.vn/dien-thoai/iphone-16-pro-max-256gb", "dien-thoai/iphone-16-pro-max-256gb"},
		{"https://fptshop.com.vn/laptop/macbook-pro-m4", "laptop/macbook-pro-m4"},
		{"dien-thoai/iphone-16-pro-max-256gb", "dien-thoai/iphone-16-pro-max-256gb"},
		{"", ""},
	}
	for _, tc := range cases {
		got := extractPath(tc.in)
		if got != tc.want {
			t.Errorf("extractPath(%q) = %q, want %q", tc.in, got, tc.want)
		}
	}
}

func TestParseProductPageNoJSONLD(t *testing.T) {
	html := `<html><body><h1>No JSON-LD here</h1></body></html>`
	p := parseProductPage([]byte(html), "dien-thoai/some-phone", "https://fptshop.com.vn")
	if p != nil {
		t.Errorf("expected nil for page without JSON-LD, got %+v", p)
	}
}
