package rule

import (
	"errors"
	"net/netip"
	"strings"
	"unsafe"

	"github.com/CaptainFallaway/Allower/internal/config"
	"github.com/CaptainFallaway/Allower/pkg/hashset"
	"github.com/CaptainFallaway/Allower/pkg/ipinfo"
	"github.com/rs/zerolog"
	"github.com/rs/zerolog/log"
)

// Lookup interface abstracts the IP lookup dependency for testability.
type Lookuper interface {
	Lookup(addr netip.Addr) (*ipinfo.Record, error)
}

type Rule struct {
	allowSet     hashset.Set[netip.Addr]
	blockSet     hashset.Set[netip.Addr]
	countrySet   hashset.Set[string]
	continentSet hashset.Set[string]

	ass    []config.AS
	ranges []config.Range

	db  Lookuper
	log zerolog.Logger
}

func New(cr config.Rule, db Lookuper) *Rule {
	log := log.With().Str("rule", cr.Name).Logger()

	return &Rule{
		allowSet:     newIpSet(cr.Allow),
		blockSet:     newIpSet(cr.Block),
		countrySet:   newStringSet(cr.Countries),
		continentSet: newStringSet(cr.Continents),
		ass:          cr.ASs,
		ranges:       cr.Ranges,
		db:           db,
		log:          log,
	}
}

func newIpSet(slice []netip.Addr) hashset.Set[netip.Addr] {
	if len(slice) == 0 {
		return nil
	}
	return hashset.New(slice...)
}

func newStringSet(slice []string) hashset.Set[string] {
	if len(slice) == 0 {
		return nil
	}

	is := make([]string, len(slice)) // We could resuse the `slice` but might be extreme
	for i := range len(slice) {
		is[i] = toLower(slice[i])
	}

	return hashset.New(is...)
}

func contains[T comparable](set hashset.Set[T], item T) bool {
	return set != nil && set.Contains(item)
}

// The string is guaranteed to be ASCII as it's an AS name or country/continent code.
func toLower(str string) string {
	lenStr := len(str)
	if lenStr == 0 {
		return str
	}
	b := make([]byte, lenStr)

	for i := 0; i < lenStr; i++ {
		if str[i] >= 'A' && str[i] <= 'Z' {
			b[i] = str[i] + ('a' - 'A')
		} else {
			b[i] = str[i]
		}
	}

	return unsafe.String(&b[0], lenStr)
}

func (r *Rule) IsAllowed(ip netip.Addr) bool {
	if contains(r.allowSet, ip) {
		return true
	}

	if contains(r.blockSet, ip) {
		return false
	}

	for _, r := range r.ranges {
		switch r.Type {
		case config.RangeTypeFromTo:
			if ip.Compare(r.From) >= 0 && ip.Compare(r.To) <= 0 {
				return true
			}
		case config.RangeTypePrefix:
			if r.Prefix.Contains(ip) {
				return true
			}
		}
	}

	// If the IP is not explicitly allowed or blocked, we check the IP info for country, continent, and AS matches.
	record, err := r.db.Lookup(ip)
	if err == nil {
		defer record.Free()

		if contains(r.countrySet, toLower(record.CountryCode)) {
			return true
		}

		if contains(r.continentSet, toLower(record.ContinentCode)) {
			return true
		}

		for _, as := range r.ass {
			if as.Number != "" && as.Number == record.AsNumber {
				return true
			}
			if as.Domain != "" && as.Domain == record.ASDomain {
				return true
			}
			if as.Name != "" && strings.Contains(toLower(record.ASName), toLower(as.Name)) {
				return true
			}
		}
	} else if errors.Is(err, ipinfo.ErrAddrIsPrivate) {
		r.log.Debug().Str("ip", ip.String()).Msg("address is private")
	} else {
		r.log.Warn().Str("ip", ip.String()).Err(err).Msg("failed to lookup address")
	}

	return false
}
