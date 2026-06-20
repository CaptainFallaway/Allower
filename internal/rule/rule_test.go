package rule_test

import (
	"net/netip"
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

func rec(countryCode, continentCode, asn, asName, asDomain string) *ipinfo.Record {
	return &ipinfo.Record{
		CountryCode:   countryCode,
		ContinentCode: continentCode,
		AsNumber:      asn,
		ASName:        asName,
		ASDomain:      asDomain,
	}
}

var emptyRecord = &ipinfo.Record{}

// ── test table ───────────────────────────────────────────────────────────────

func TestIsAllowed(t *testing.T) {
	tests := []struct {
		name   string
		rule   config.Rule
		ip     netip.Addr
		record *ipinfo.Record
		want   bool
	}{
		// ── allow list ───────────────────────────────────────────────────────
		{
			name:   "allow list: matching IP is permitted",
			rule:   config.Rule{Allow: []netip.Addr{mustAddr("1.2.3.4")}},
			ip:     mustAddr("1.2.3.4"),
			record: emptyRecord,
			want:   true,
		},
		{
			name:   "allow list: non-matching IP is denied when nothing else matches",
			rule:   config.Rule{Allow: []netip.Addr{mustAddr("1.2.3.4")}},
			ip:     mustAddr("5.6.7.8"),
			record: emptyRecord,
			want:   false,
		},

		// ── block list ───────────────────────────────────────────────────────
		{
			name:   "block list: matching IP is denied",
			rule:   config.Rule{Block: []netip.Addr{mustAddr("1.2.3.4")}},
			ip:     mustAddr("1.2.3.4"),
			record: emptyRecord,
			want:   false,
		},

		// ── allow vs block priority ──────────────────────────────────────────
		{
			// allow is checked before block, so it wins
			name: "allow takes priority over block for the same IP",
			rule: config.Rule{
				Allow: []netip.Addr{mustAddr("1.2.3.4")},
				Block: []netip.Addr{mustAddr("1.2.3.4")},
			},
			ip:     mustAddr("1.2.3.4"),
			record: emptyRecord,
			want:   true,
		},
		{
			// block is checked before country/continent/ranges/AS, so it wins
			name: "block takes priority over a matching country",
			rule: config.Rule{
				Block:     []netip.Addr{mustAddr("85.24.194.40")},
				Countries: []string{"SE"},
			},
			ip:     mustAddr("85.24.194.40"),
			record: rec("SE", "EU", "", "", ""),
			want:   false,
		},
		{
			name: "block takes priority over a matching range",
			rule: config.Rule{
				Block:  []netip.Addr{mustAddr("10.0.0.5")},
				Ranges: []config.Range{fromTo("10.0.0.1", "10.0.0.10")},
			},
			ip:     mustAddr("10.0.0.5"),
			record: emptyRecord,
			want:   false,
		},

		// ── countries ────────────────────────────────────────────────────────
		{
			name:   "country: matching code is allowed",
			rule:   config.Rule{Countries: []string{"SE"}},
			ip:     mustAddr("85.24.194.40"),
			record: rec("SE", "", "", "", ""),
			want:   true,
		},
		{
			name:   "country: config stored in lowercase still matches uppercase record",
			rule:   config.Rule{Countries: []string{"se"}},
			ip:     mustAddr("85.24.194.40"),
			record: rec("SE", "", "", "", ""),
			want:   true,
		},
		{
			name:   "country: non-matching code is denied",
			rule:   config.Rule{Countries: []string{"SE"}},
			ip:     mustAddr("1.2.3.4"),
			record: rec("US", "", "", "", ""),
			want:   false,
		},

		// ── continents ───────────────────────────────────────────────────────
		{
			name:   "continent: matching code is allowed",
			rule:   config.Rule{Continents: []string{"EU"}},
			ip:     mustAddr("85.24.194.40"),
			record: rec("", "EU", "", "", ""),
			want:   true,
		},
		{
			name:   "continent: non-matching code is denied",
			rule:   config.Rule{Continents: []string{"EU"}},
			ip:     mustAddr("1.2.3.4"),
			record: rec("", "AS", "", "", ""),
			want:   false,
		},

		// ── from-to ranges ───────────────────────────────────────────────────
		{
			name:   "from-to range: IP inside range is allowed",
			rule:   config.Rule{Ranges: []config.Range{fromTo("85.24.194.40", "85.24.194.42")}},
			ip:     mustAddr("85.24.194.41"),
			record: emptyRecord,
			want:   true,
		},
		{
			name:   "from-to range: IP at the From boundary is allowed",
			rule:   config.Rule{Ranges: []config.Range{fromTo("85.24.194.40", "85.24.194.42")}},
			ip:     mustAddr("85.24.194.40"),
			record: emptyRecord,
			want:   true,
		},
		{
			name:   "from-to range: IP at the To boundary is allowed",
			rule:   config.Rule{Ranges: []config.Range{fromTo("85.24.194.40", "85.24.194.42")}},
			ip:     mustAddr("85.24.194.42"),
			record: emptyRecord,
			want:   true,
		},
		{
			name:   "from-to range: IP one below From is denied",
			rule:   config.Rule{Ranges: []config.Range{fromTo("85.24.194.40", "85.24.194.42")}},
			ip:     mustAddr("85.24.194.39"),
			record: emptyRecord,
			want:   false,
		},
		{
			name:   "from-to range: IP one above To is denied",
			rule:   config.Rule{Ranges: []config.Range{fromTo("85.24.194.40", "85.24.194.42")}},
			ip:     mustAddr("85.24.194.43"),
			record: emptyRecord,
			want:   false,
		},

		// ── prefix ranges ────────────────────────────────────────────────────
		{
			name:   "prefix range: IP inside CIDR is allowed",
			rule:   config.Rule{Ranges: []config.Range{cidr("85.24.194.0/25")}},
			ip:     mustAddr("85.24.194.40"),
			record: emptyRecord,
			want:   true,
		},
		{
			name:   "prefix range: IP outside CIDR is denied",
			rule:   config.Rule{Ranges: []config.Range{cidr("85.24.194.0/25")}},
			ip:     mustAddr("85.24.194.200"),
			record: emptyRecord,
			want:   false,
		},
		{
			// /25 covers .0–.127; .128 falls in the second half
			name:   "prefix range: IP in adjacent subnet is denied",
			rule:   config.Rule{Ranges: []config.Range{cidr("85.24.194.0/25")}},
			ip:     mustAddr("85.24.194.128"),
			record: emptyRecord,
			want:   false,
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
			ip:     mustAddr("192.168.1.50"),
			record: emptyRecord,
			want:   true,
		},

		// ── autonomous systems ───────────────────────────────────────────────
		{
			name:   "AS number: exact match is allowed",
			rule:   config.Rule{ASs: []config.AS{{Number: "AS24429"}}},
			ip:     mustAddr("47.88.0.1"),
			record: rec("CN", "AS", "AS24429", "Alibaba Cloud", "alibabacloud.com"),
			want:   true,
		},
		{
			name:   "AS domain: exact match is allowed",
			rule:   config.Rule{ASs: []config.AS{{Domain: "alibabacloud.com"}}},
			ip:     mustAddr("47.88.0.1"),
			record: rec("CN", "AS", "AS24429", "Alibaba Cloud", "alibabacloud.com"),
			want:   true,
		},
		{
			name:   "AS name: case-insensitive substring match is allowed",
			rule:   config.Rule{ASs: []config.AS{{Name: "Alibaba"}}},
			ip:     mustAddr("47.88.0.1"),
			record: rec("CN", "AS", "AS24429", "Alibaba Cloud Computing", "alibabacloud.com"),
			want:   true,
		},
		{
			name:   "AS name: partial lowercase match is allowed",
			rule:   config.Rule{ASs: []config.AS{{Name: "taobao"}}},
			ip:     mustAddr("47.88.0.1"),
			record: rec("CN", "AS", "AS24429", "Taobao (China) Software Co.", "taobao.com"),
			want:   true,
		},
		{
			name:   "AS: no field matches denies IP",
			rule:   config.Rule{ASs: []config.AS{{Number: "AS24429", Domain: "alibabacloud.com", Name: "Alibaba"}}},
			ip:     mustAddr("1.2.3.4"),
			record: rec("US", "NA", "AS15169", "Google LLC", "google.com"),
			want:   false,
		},

		// ── empty / catch-all ────────────────────────────────────────────────
		{
			name:   "empty rule denies every IP",
			rule:   config.Rule{},
			ip:     mustAddr("1.2.3.4"),
			record: emptyRecord,
			want:   false,
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
			ip:     mustAddr("1.2.3.4"),
			record: rec("US", "NA", "AS15169", "Google LLC", "google.com"),
			want:   false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			r := rule.New(&tt.rule)
			got := r.IsAllowed(tt.ip, tt.record)
			if got != tt.want {
				t.Errorf("IsAllowed(%v) = %v, want %v", tt.ip, got, tt.want)
			}
		})
	}
}
