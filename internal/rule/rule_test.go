package rule_test

import (
	"context"
	"fmt"
	"net/netip"
	"os"
	"testing"

	"github.com/CaptainFallaway/Allower/internal/config"
	"github.com/CaptainFallaway/Allower/internal/rule"
	"github.com/CaptainFallaway/Allower/pkg/ipinfo"
)

// helpers

func mustAddr(s string) netip.Addr  { return netip.MustParseAddr(s) }
func mustPfx(s string) netip.Prefix { return netip.MustParsePrefix(s) }

func fromTo(from, to string) config.Range {
	return config.Range{Type: config.RangeTypeFromTo, From: mustAddr(from), To: mustAddr(to)}
}

func cidr(pfx string) config.Range {
	return config.Range{Type: config.RangeTypePrefix, Prefix: mustPfx(pfx)}
}

var db *ipinfo.DB

type Fataler interface {
	Fatal(args ...any)
	Fatalf(format string, args ...any)
}

func LoadAndGetDB(t Fataler) *ipinfo.DB {
	if db != nil {
		return db
	}

	db := ipinfo.New(os.Getenv("IPINFO_TOKEN"), "../../test-data")

	updated, err := db.Sync(context.Background())
	if err != nil {
		t.Fatalf("failed to sync dataset: %v", err)
	}
	if updated {
		fmt.Println("dataset was update")
	}

	err = db.Load()
	if err != nil {
		t.Fatalf("failed to load dataset: %v", err)
	}

	return db
}

func TestIsAllowed(t *testing.T) {
	tests := []struct {
		name string
		rule config.Rule
		ip   netip.Addr
		want bool
	}{
		// ── allow list ───────────────────────────────────────────────────────
		{
			name: "allow list: matching IP is permitted",
			rule: config.Rule{Allow: []netip.Addr{mustAddr("1.2.3.4")}},
			ip:   mustAddr("1.2.3.4"),
			want: true,
		},
		{
			name: "allow list: non-matching IP is denied when nothing else matches",
			rule: config.Rule{Allow: []netip.Addr{mustAddr("1.2.3.4")}},
			ip:   mustAddr("5.6.7.8"),
			want: false,
		},

		// ── block list ───────────────────────────────────────────────────────
		{
			name: "block list: matching IP is denied",
			rule: config.Rule{Block: []netip.Addr{mustAddr("1.2.3.4")}},
			ip:   mustAddr("1.2.3.4"),
			want: false,
		},

		// ── allow vs block priority ──────────────────────────────────────────
		{
			// allow is checked before block, so it wins
			name: "allow takes priority over block for the same IP",
			rule: config.Rule{
				Allow: []netip.Addr{mustAddr("1.2.3.4")},
				Block: []netip.Addr{mustAddr("1.2.3.4")},
			},
			ip:   mustAddr("1.2.3.4"),
			want: true,
		},
		{
			// block is checked before country/continent/ranges/AS, so it wins
			name: "block takes priority over a matching country",
			rule: config.Rule{
				Block:     []netip.Addr{mustAddr("85.24.194.40")},
				Countries: []string{"SE"},
			},
			ip:   mustAddr("85.24.194.40"),
			want: false,
		},
		{
			name: "block takes priority over a matching range",
			rule: config.Rule{
				Block:  []netip.Addr{mustAddr("10.0.0.5")},
				Ranges: []config.Range{fromTo("10.0.0.1", "10.0.0.10")},
			},
			ip:   mustAddr("10.0.0.5"),
			want: false,
		},

		// ── countries ────────────────────────────────────────────────────────
		{
			name: "country: matching code is allowed",
			rule: config.Rule{Countries: []string{"SE"}},
			ip:   mustAddr("85.24.194.40"),
			want: true,
		},
		{
			name: "country: config stored in lowercase still matches uppercase record",
			rule: config.Rule{Countries: []string{"se"}},
			ip:   mustAddr("85.24.194.40"),
			want: true,
		},
		{
			name: "country: non-matching code is denied",
			rule: config.Rule{Countries: []string{"SE"}},
			ip:   mustAddr("1.2.3.4"),
			want: false,
		},

		// ── continents ───────────────────────────────────────────────────────
		{
			name: "continent: matching code is allowed",
			rule: config.Rule{Continents: []string{"EU"}},
			ip:   mustAddr("85.24.194.40"),
			want: true,
		},
		{
			name: "continent: non-matching code is denied",
			rule: config.Rule{Continents: []string{"EU"}},
			ip:   mustAddr("1.2.3.4"),
			want: false,
		},

		// ── from-to ranges ───────────────────────────────────────────────────
		{
			name: "from-to range: IP inside range is allowed",
			rule: config.Rule{Ranges: []config.Range{fromTo("85.24.194.40", "85.24.194.42")}},
			ip:   mustAddr("85.24.194.41"),
			want: true,
		},
		{
			name: "from-to range: IP at the From boundary is allowed",
			rule: config.Rule{Ranges: []config.Range{fromTo("85.24.194.40", "85.24.194.42")}},
			ip:   mustAddr("85.24.194.40"),
			want: true,
		},
		{
			name: "from-to range: IP at the To boundary is allowed",
			rule: config.Rule{Ranges: []config.Range{fromTo("85.24.194.40", "85.24.194.42")}},
			ip:   mustAddr("85.24.194.42"),
			want: true,
		},
		{
			name: "from-to range: IP one below From is denied",
			rule: config.Rule{Ranges: []config.Range{fromTo("85.24.194.40", "85.24.194.42")}},
			ip:   mustAddr("85.24.194.39"),
			want: false,
		},
		{
			name: "from-to range: IP one above To is denied",
			rule: config.Rule{Ranges: []config.Range{fromTo("85.24.194.40", "85.24.194.42")}},
			ip:   mustAddr("85.24.194.43"),
			want: false,
		},

		// ── prefix ranges ────────────────────────────────────────────────────
		{
			name: "prefix range: IP inside CIDR is allowed",
			rule: config.Rule{Ranges: []config.Range{cidr("85.24.194.0/25")}},
			ip:   mustAddr("85.24.194.40"),
			want: true,
		},
		{
			name: "prefix range: IP outside CIDR is denied",
			rule: config.Rule{Ranges: []config.Range{cidr("85.24.194.0/25")}},
			ip:   mustAddr("85.24.194.200"),
			want: false,
		},
		{
			// /25 covers .0–.127; .128 falls in the second half
			name: "prefix range: IP in adjacent subnet is denied",
			rule: config.Rule{Ranges: []config.Range{cidr("85.24.194.0/25")}},
			ip:   mustAddr("85.24.194.128"),
			want: false,
		},

		// ── multiple ranges: any match is sufficient ──────────────────────────
		{
			name: "multiple ranges: IP matching the second range is allowed",
			rule: config.Rule{
				Ranges: []config.Range{
					fromTo("10.0.0.1", "10.0.0.5"),
					cidr("192.168.1.0/24"),
				},
			},
			ip:   mustAddr("192.168.1.50"),
			want: true,
		},

		// ── autonomous systems ───────────────────────────────────────────────
		{
			name: "AS number: exact match is allowed",
			rule: config.Rule{ASs: []config.AS{{Number: "AS24429"}}},
			ip:   mustAddr("47.88.0.1"),
			want: true,
		},
		{
			name: "AS domain: exact match is allowed",
			rule: config.Rule{ASs: []config.AS{{Domain: "alibabacloud.com"}}},
			ip:   mustAddr("47.88.0.1"),
			want: true,
		},
		{
			name: "AS name: case-insensitive substring match is allowed",
			rule: config.Rule{ASs: []config.AS{{Name: "Alibaba"}}},
			ip:   mustAddr("47.88.0.1"),
			want: true,
		},
		{
			name: "AS name: partial lowercase match is allowed",
			rule: config.Rule{ASs: []config.AS{{Name: "taobao"}}},
			ip:   mustAddr("47.88.0.1"),
			want: true,
		},
		{
			name: "AS: no field matches denies IP",
			rule: config.Rule{ASs: []config.AS{{Number: "AS24429", Domain: "alibabacloud.com", Name: "Alibaba"}}},
			ip:   mustAddr("1.2.3.4"),
			want: false,
		},

		// ── empty / catch-all ────────────────────────────────────────────────
		{
			name: "empty rule denies every IP",
			rule: config.Rule{},
			ip:   mustAddr("1.2.3.4"),
			want: false,
		},
		{
			name: "rich rule: no condition matches denies IP",
			rule: config.Rule{
				Allow:      []netip.Addr{mustAddr("9.9.9.9")},
				Block:      []netip.Addr{mustAddr("8.8.8.8")},
				Countries:  []string{"SE"},
				Continents: []string{"EU"},
				Ranges:     []config.Range{cidr("10.0.0.0/8")},
				ASs:        []config.AS{{Number: "AS24429"}},
			},
			ip:   mustAddr("1.2.3.4"),
			want: false,
		},
	}

	db := LoadAndGetDB(t)

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			r := rule.New(tt.rule, db)
			got := r.IsAllowed(tt.ip)
			if got != tt.want {
				t.Errorf("IsAllowed(%v) = %v, want %v", tt.ip, got, tt.want)
			}
		})
	}
}
