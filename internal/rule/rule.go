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

// Lookup interface abstracts the address lookup dependency for testability. (but I don't care I load the dataset in the tests anyways XD)
type Lookuper interface {
	Lookup(addr netip.Addr) (*ipinfo.Record, error)
}

type Rule struct {
	lookuper Lookuper
	allowSet hashset.Set[netip.Addr]

	blockSet     hashset.Set[netip.Addr]
	countrySet   hashset.Set[string]
	continentSet hashset.Set[string]

	ass    []config.AS
	ranges []config.Range

	log     zerolog.Logger
	silence bool
}

func New(cr config.Rule, lookuper Lookuper, silence bool) *Rule {
	log := log.With().Str("rule", cr.Name).Logger()

	return &Rule{
		lookuper:     lookuper,
		allowSet:     newIpSet(cr.Allow),
		blockSet:     newIpSet(cr.Block),
		countrySet:   newStringSet(cr.Countries),
		continentSet: newStringSet(cr.Continents),
		ass:          cr.ASs,
		ranges:       cr.Ranges,
		log:          log,
		silence:      silence,
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

var noopLog = zerolog.Nop()

func (r *Rule) IsAllowed(ip netip.Addr) bool {
	log := noopLog

	// Avoid making an allocation for the logger if trace logging is not enabled or if silenced
	if zerolog.GlobalLevel() == zerolog.TraceLevel && !r.silence {
		log = r.log.With().Str("ip", ip.String()).Logger()
	}

	if contains(r.allowSet, ip) {
		log.Trace().Msg("explicitly allowed")
		return true
	}

	if contains(r.blockSet, ip) {
		log.Trace().Msg("explicitly blocked")
		return false
	}

	for _, r := range r.ranges {
		switch r.Type {
		case config.RangeTypeFromTo:
			if ip.Compare(r.From) >= 0 && ip.Compare(r.To) <= 0 {
				log.Trace().Msg("within range")
				return true
			}
		case config.RangeTypePrefix:
			if r.Prefix.Contains(ip) {
				log.Trace().Msg("within prefix")
				return true
			}
		}
	}

	// If the address is not explicitly allowed or blocked, we check the address info for country, continent, and AS matches.
	record, err := r.lookuper.Lookup(ip)
	if err == nil {
		defer record.Free()

		if contains(r.countrySet, toLower(record.CountryCode)) {
			log.Trace().Str("country", record.CountryCode).Msg("country match")
			return true
		}

		if contains(r.continentSet, toLower(record.ContinentCode)) {
			log.Trace().Str("continent", record.ContinentCode).Msg("continent match")
			return true
		}

		for _, as := range r.ass {
			if as.Number != "" && as.Number == record.AsNumber {
				log.Trace().Msg("as number match")
				return true
			}
			if as.Domain != "" && as.Domain == record.ASDomain {
				log.Trace().Msg("as domain match")
				return true
			}
			if as.Name != "" && strings.Contains(toLower(record.ASName), toLower(as.Name)) {
				log.Trace().Str("contained", as.Name).Msg("as name match")
				return true
			}
		}
	} else if errors.Is(err, ipinfo.ErrAddrIsPrivate) {
		log.Trace().Msg("address is private")
	} else {
		log.Warn().Err(err).Msg("failed to lookup address")
	}

	if record != nil {
		log.Trace().
			Str("country", record.CountryCode).
			Str("continent", record.ContinentCode).
			Str("as", record.AsNumber).
			Msg("denied address")
	}

	return false
}
