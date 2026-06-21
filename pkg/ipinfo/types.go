package ipinfo

import "sync"

type Record struct {
	Country       string `maxminddb:"country" json:"country"`
	CountryCode   string `maxminddb:"country_code" json:"country_code"`
	Continent     string `maxminddb:"continent" json:"continent"`
	ContinentCode string `maxminddb:"continent_code" json:"continent_code"`
	AsNumber      string `maxminddb:"asn" json:"asn"`
	ASName        string `maxminddb:"as_name" json:"as_name"`
	ASDomain      string `maxminddb:"as_domain" json:"as_domain"`

	pool *sync.Pool
}

// Free releases the Record after you're done using it. If the Record was
// allocated from the Datasets LookupRecordPool, it is reset and returned to the
// pool. If it was not pooled, Free does nothing. (optional to call)
func (r *Record) Free() {
	if r == nil || r.pool == nil {
		return
	}

	r.Country = ""
	r.CountryCode = ""
	r.Continent = ""
	r.ContinentCode = ""
	r.AsNumber = ""
	r.ASName = ""
	r.ASDomain = ""

	r.pool.Put(r)
}
