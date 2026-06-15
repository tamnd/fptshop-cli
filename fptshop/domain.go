package fptshop

import (
	"context"
	"strings"

	"github.com/tamnd/any-cli/kit"
	"github.com/tamnd/any-cli/kit/errs"
)

func init() { kit.Register(Domain{}) }

// Domain is the FPT Shop kit driver.
type Domain struct{}

func (Domain) Info() kit.DomainInfo {
	return kit.DomainInfo{
		Scheme: "fptshop",
		Hosts:  []string{Host},
		Identity: kit.Identity{
			Binary: "fptshop",
			Short:  "A command line for FPT Shop.",
			Long: `A command line for FPT Shop (fptshop.com.vn).

Fetches product details, category listings, and customer reviews
from Vietnam's major electronics retail chain.
No API key required.`,
			Site: "https://" + Host,
			Repo: "https://github.com/tamnd/fptshop-cli",
		},
	}
}

func (Domain) Register(app *kit.App) {
	app.SetClient(newClient)

	kit.Handle(app, kit.OpMeta{Name: "product", Group: "product", Single: true,
		URIType: "product", Resolver: true, Summary: "Fetch a product by path or URL",
		Args: []kit.Arg{{Name: "ref", Help: "product path (category/slug) or URL"}}}, getProduct)

	kit.Handle(app, kit.OpMeta{Name: "products", Group: "product", List: true,
		URIType: "product", Summary: "List products from a category",
		Args: []kit.Arg{{Name: "category", Help: "category slug (e.g. dien-thoai)"}}}, listProducts)

	kit.Handle(app, kit.OpMeta{Name: "reviews", Group: "product", List: true,
		URIType: "review", Summary: "List customer reviews for a product",
		Args: []kit.Arg{{Name: "ref", Help: "product path (category/slug) or URL"}}}, listReviews)
}

func newClient(_ context.Context, cfg kit.Config) (any, error) {
	c := NewClientWithConfig(DefaultConfig())
	if cfg.UserAgent != "" {
		c.cfg.UserAgent = cfg.UserAgent
	}
	if cfg.Rate > 0 {
		c.cfg.Rate = cfg.Rate
	}
	if cfg.Retries > 0 {
		c.cfg.Retries = cfg.Retries
	}
	if cfg.Timeout > 0 {
		c.cfg.Timeout = cfg.Timeout
		c.http.Timeout = cfg.Timeout
	}
	return c, nil
}

// --- inputs ---

type productRef struct {
	Ref    string  `kit:"arg" help:"product path or URL"`
	Client *Client `kit:"inject"`
}

type productsIn struct {
	Category string  `kit:"arg" help:"category slug (e.g. dien-thoai)"`
	Limit    int     `kit:"flag,inherit" help:"max results"`
	Client   *Client `kit:"inject"`
}

type reviewsIn struct {
	Ref    string  `kit:"arg" help:"product path or URL"`
	Limit  int     `kit:"flag,inherit" help:"max results"`
	Client *Client `kit:"inject"`
}

// --- handlers ---

func getProduct(ctx context.Context, in productRef, emit func(*Product) error) error {
	path := productPath(in.Ref)
	if path == "" {
		return errs.Usage("unrecognized FPT Shop product reference: %q", in.Ref)
	}
	p, err := in.Client.GetProduct(ctx, path)
	if err != nil {
		return err
	}
	return emit(p)
}

func listProducts(ctx context.Context, in productsIn, emit func(*Product) error) error {
	products, err := in.Client.ListProducts(ctx, in.Category, 1, in.Limit)
	if err != nil {
		return err
	}
	for _, p := range products {
		if err := emit(p); err != nil {
			return err
		}
	}
	return nil
}

func listReviews(ctx context.Context, in reviewsIn, emit func(*Review) error) error {
	path := productPath(in.Ref)
	if path == "" {
		return errs.Usage("unrecognized FPT Shop product reference: %q", in.Ref)
	}
	reviews, err := in.Client.ListReviews(ctx, path, in.Limit)
	if err != nil {
		return err
	}
	for _, r := range reviews {
		if err := emit(r); err != nil {
			return err
		}
	}
	return nil
}

// --- Resolver ---

func (Domain) Classify(input string) (uriType, id string, err error) {
	path := productPath(input)
	if path != "" {
		return "product", path, nil
	}
	return "", "", errs.Usage("unrecognized FPT Shop reference: %q", input)
}

func (Domain) Locate(uriType, id string) (string, error) {
	switch uriType {
	case "product":
		return baseURL + "/" + strings.Trim(id, "/"), nil
	default:
		return "", errs.Usage("fptshop has no resource type %q", uriType)
	}
}

// productPath normalises any user input into a canonical path.
func productPath(input string) string {
	input = strings.TrimSpace(input)
	if input == "" {
		return ""
	}
	if strings.Contains(input, "fptshop.com.vn") || strings.HasPrefix(input, "http") {
		return extractPath(input)
	}
	path := strings.Trim(input, "/")
	if path != "" && !strings.Contains(path, " ") {
		return path
	}
	return ""
}
