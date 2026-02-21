package bunny

// PullZone represents a BunnyCDN Pull Zone
type PullZone struct {
	ID       int
	Name     string
	Hostname string
	OriginURL string
}

// CreatePullZoneOptions contains options for creating a pull zone
type CreatePullZoneOptions struct {
	Name            string
	OriginURL       string
	EnableGeoZoneASIA bool
}
