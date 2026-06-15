package fptshop

import (
	"testing"

	"github.com/tamnd/any-cli/kit"
)

func TestDomainInfo(t *testing.T) {
	info := Domain{}.Info()
	if info.Scheme != "fptshop" {
		t.Errorf("Scheme = %q, want fptshop", info.Scheme)
	}
	if len(info.Hosts) == 0 || info.Hosts[0] != Host {
		t.Errorf("Hosts = %v, want [%s]", info.Hosts, Host)
	}
	if info.Identity.Binary != "fptshop" {
		t.Errorf("Identity.Binary = %q, want fptshop", info.Identity.Binary)
	}
}

func TestClassify(t *testing.T) {
	cases := []struct{ in, typ, id string }{
		{"dien-thoai/iphone-16-pro-max-256gb", "product", "dien-thoai/iphone-16-pro-max-256gb"},
		{"laptop/macbook-pro-m4", "product", "laptop/macbook-pro-m4"},
		{"https://" + Host + "/dien-thoai/iphone-16-pro-max-256gb", "product", "dien-thoai/iphone-16-pro-max-256gb"},
	}
	for _, tc := range cases {
		typ, id, err := Domain{}.Classify(tc.in)
		if err != nil || typ != tc.typ || id != tc.id {
			t.Errorf("Classify(%q) = (%q, %q, %v), want (%q, %q, nil)",
				tc.in, typ, id, err, tc.typ, tc.id)
		}
	}
}

func TestLocate(t *testing.T) {
	got, err := Domain{}.Locate("product", "dien-thoai/iphone-16-pro-max-256gb")
	want := baseURL + "/dien-thoai/iphone-16-pro-max-256gb"
	if err != nil || got != want {
		t.Errorf("Locate = (%q, %v), want (%q, nil)", got, err, want)
	}
}

func TestLocateUnknownType(t *testing.T) {
	_, err := Domain{}.Locate("unknown", "x")
	if err == nil {
		t.Error("expected error for unknown type, got nil")
	}
}

func TestHostWiring(t *testing.T) {
	h, err := kit.Open()
	if err != nil {
		t.Fatal(err)
	}

	p := &Product{
		Path:  "dien-thoai/iphone-16-pro-max-256gb",
		URL:   baseURL + "/dien-thoai/iphone-16-pro-max-256gb",
		Name:  "iPhone 16 Pro Max 256GB",
		Price: 32990000,
	}
	u, err := h.Mint(p)
	if err != nil {
		t.Fatalf("Mint: %v", err)
	}
	want := "fptshop://product/dien-thoai/iphone-16-pro-max-256gb"
	if u.String() != want {
		t.Errorf("Mint = %q, want %q", u.String(), want)
	}

	got, err := h.ResolveOn("fptshop", "laptop/macbook-pro-m4")
	if err != nil || got.String() != "fptshop://product/laptop/macbook-pro-m4" {
		t.Errorf("ResolveOn = (%q, %v), want fptshop://product/laptop/macbook-pro-m4", got.String(), err)
	}
}
