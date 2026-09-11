package domain

// DNSConfig represents DNS resolution settings for compiled client profiles.
type DNSConfig struct {
	ID                 int64    `json:"id" yaml:"id"`
	Enabled            bool     `json:"enabled" yaml:"enabled"`
	Listen             string   `json:"listen,omitempty" yaml:"listen,omitempty"`
	IPv6               bool     `json:"ipv6" yaml:"ipv6"`
	DefaultNameservers []string `json:"default_nameservers,omitempty" yaml:"default_nameservers,omitempty"`
	Nameservers        []string `json:"nameservers,omitempty" yaml:"nameservers,omitempty"`
	Fallback           []string `json:"fallback,omitempty" yaml:"fallback,omitempty"`
	FallbackFilter     string   `json:"fallback_filter,omitempty" yaml:"fallback_filter,omitempty"`
	EnhancedMode       string   `json:"enhanced_mode,omitempty" yaml:"enhanced_mode,omitempty"`
	FakeIPRange        string   `json:"fake_ip_range,omitempty" yaml:"fake_ip_range,omitempty"`
	RawYAML            string   `json:"raw_yaml,omitempty" yaml:"raw_yaml,omitempty"`
}

// DefaultDNSConfig returns a safe DNS configuration.
func DefaultDNSConfig() *DNSConfig {
	return &DNSConfig{
		Enabled:      true,
		Listen:       "0.0.0.0:1053",
		IPv6:         false,
		EnhancedMode: "fake-ip",
		FakeIPRange:  "198.18.0.1/16",
		Nameservers: []string{
			"223.5.5.5",
			"119.29.29.29",
			"tls://dns.alidns.com",
		},
		Fallback: []string{
			"https://1.1.1.1/dns-query",
			"https://dns.google/dns-query",
		},
	}
}
