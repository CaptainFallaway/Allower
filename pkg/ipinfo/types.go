package ipinfo

type Record struct {
	Country       string `maxminddb:"country" json:"country"`
	CountryCode   string `maxminddb:"country_code" json:"country_code"`
	Continent     string `maxminddb:"continent" json:"continent"`
	ContinentCode string `maxminddb:"continent_code" json:"continent_code"`
	AsNumber      string `maxminddb:"asn" json:"asn"`
	ASName        string `maxminddb:"as_name" json:"as_name"`
	ASDomain      string `maxminddb:"as_domain" json:"as_domain"`
}
