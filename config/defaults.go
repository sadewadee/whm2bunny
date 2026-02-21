package config

const (
	// DefaultServerPort is the default port for the HTTP server
	DefaultServerPort = 9090

	// DefaultOriginShieldRegion is the default origin shield region
	DefaultOriginShieldRegion = "SG"

	// DefaultSOAEmail is the default SOA email
	DefaultSOAEmail = "hostmaster@mordenhost.com"
)

// DefaultConfig returns default configuration values
var DefaultConfig = map[string]interface{}{
	"server.port":           DefaultServerPort,
	"origin_shield_region":  DefaultOriginShieldRegion,
	"soa_email":            DefaultSOAEmail,
}
