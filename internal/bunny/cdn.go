package bunny

import (
	"context"
	"fmt"
	"net/http"
	"strings"
	"time"

	"go.uber.org/zap"
)

// PullZone represents a BunnyCDN Pull Zone
type PullZone struct {
	ID                      int64      `json:"Id"`
	Name                    string     `json:"Name"`
	OriginURL               string     `json:"OriginUrl"`
	OriginHostHeader        string     `json:"OriginHostHeader,omitempty"`
	OriginShieldZoneCode    string     `json:"OriginShieldZoneCode,omitempty"`
	EnableGeoZoneASIA       bool       `json:"EnableGeoZoneASIA"`
	EnableGeoZoneEU         bool       `json:"EnableGeoZoneEU,omitempty"`
	EnableGeoZoneNA         bool       `json:"EnableGeoZoneNA,omitempty"`
	EnableGeoZoneSA         bool       `json:"EnableGeoZoneSA,omitempty"`
	EnableGeoZoneAF         bool       `json:"EnableGeoZoneAF,omitempty"`
	EnableOriginShield      bool       `json:"EnableOriginShield,omitempty"`
	EnableAutoSSL           bool       `json:"EnableAutoSSL"`
	EnableBrotliCompression bool       `json:"EnableBrotliCompression,omitempty"`
	CacheExpirationTime     int        `json:"CacheExpirationTime,omitempty"`
	BracketedForce          bool       `json:"BracketedForce,omitempty"`
	DisableCookies          bool       `json:"DisableCookies,omitempty"`
	EnableQueryStringBased  bool       `json:"EnableQueryStringBased,omitempty"`
	ZoneStatus              int        `json:"ZoneStatus,omitempty"`
	Hostnames               []Hostname `json:"Hostnames,omitempty"`
	Type                    int        `json:"Type,omitempty"`
	CreatedAt               time.Time  `json:"CreationDate,omitempty"`
	ModifiedAt              time.Time  `json:"ModifyDate,omitempty"`
}

// Hostname represents a hostname (custom domain) for a pull zone
type Hostname struct {
	ID                int64   `json:"Id"`
	Hostname          string  `json:"Hostname"`
	ForceSSL          bool    `json:"ForceSSL"`
	SSLCertificateID  *int64  `json:"SslCertificateId,omitempty"`
	Value             *string `json:"Value,omitempty"`
	VerificationError *string `json:"VerificationError,omitempty"`
}

// SSLCertificate represents an SSL certificate for a hostname
type SSLCertificate struct {
	ID             int64     `json:"Id"`
	Hostname       string    `json:"Hostname"`
	ValidationType string    `json:"ValidationType"`
	Status         string    `json:"Status"`
	ExpirationDate time.Time `json:"ExpirationDate,omitempty"`
	Issuer         string    `json:"Issuer,omitempty"`
	CreatedDate    time.Time `json:"CreatedDate,omitempty"`
}

// CreatePullZoneRequest is the request to create a pull zone
type CreatePullZoneRequest struct {
	Name                    string `json:"Name"`
	OriginURL               string `json:"OriginUrl"`
	OriginHostHeader        string `json:"OriginHostHeader,omitempty"`
	EnableGeoZoneASIA       bool   `json:"EnableGeoZoneASIA"`
	EnableGeoZoneEU         bool   `json:"EnableGeoZoneEU,omitempty"`
	EnableGeoZoneNA         bool   `json:"EnableGeoZoneNA,omitempty"`
	EnableGeoZoneSA         bool   `json:"EnableGeoZoneSA,omitempty"`
	EnableGeoZoneAF         bool   `json:"EnableGeoZoneAF,omitempty"`
	EnableOriginShield      bool   `json:"EnableOriginShield"`
	OriginShieldZoneCode    string `json:"OriginShieldZoneCode,omitempty"`
	EnableAutoSSL           bool   `json:"EnableAutoSSL"`
	EnableBrotliCompression bool   `json:"EnableBrotliCompression"`
	CacheExpirationTime     int    `json:"CacheExpirationTime"`
}

// UpdatePullZoneRequest is the request to update a pull zone
type UpdatePullZoneRequest struct {
	Name                    string `json:"Name,omitempty"`
	OriginURL               string `json:"OriginUrl,omitempty"`
	OriginHostHeader        string `json:"OriginHostHeader,omitempty"`
	EnableGeoZoneASIA       bool   `json:"EnableGeoZoneASIA,omitempty"`
	EnableGeoZoneEU         bool   `json:"EnableGeoZoneEU,omitempty"`
	EnableGeoZoneNA         bool   `json:"EnableGeoZoneNA,omitempty"`
	EnableGeoZoneSA         bool   `json:"EnableGeoZoneSA,omitempty"`
	EnableGeoZoneAF         bool   `json:"EnableGeoZoneAF,omitempty"`
	EnableOriginShield      bool   `json:"EnableOriginShield,omitempty"`
	OriginShieldZoneCode    string `json:"OriginShieldZoneCode,omitempty"`
	EnableAutoSSL           bool   `json:"EnableAutoSSL,omitempty"`
	EnableBrotliCompression bool   `json:"EnableBrotliCompression,omitempty"`
	CacheExpirationTime     int    `json:"CacheExpirationTime,omitempty"`
}

// AddHostnameRequest is the request to add a hostname to a pull zone
type AddHostnameRequest struct {
	Hostname string `json:"Hostname"`
}

// PullZoneListResponse is the response from listing pull zones
type PullZoneListResponse struct {
	Items []PullZone `json:"Items"`
}

// SSLCertificatesResponse is the response from listing SSL certificates
type SSLCertificatesResponse struct {
	Items []SSLCertificate `json:"Items"`
}

// CreatePullZone creates a new pull zone
// API: POST /pullzone
func (c *Client) CreatePullZone(ctx context.Context, domain, originIP string) (*PullZone, error) {
	if domain == "" {
		return nil, fmt.Errorf("domain is required")
	}
	if originIP == "" {
		return nil, fmt.Errorf("origin IP is required")
	}

	// Generate pull zone name: morden-example-com (replace dots with dashes)
	zoneName := generatePullZoneName(domain)

	req := &CreatePullZoneRequest{
		Name:                    zoneName,
		OriginURL:               fmt.Sprintf("http://%s", originIP),
		OriginHostHeader:        domain,
		EnableGeoZoneASIA:       true, // ONLY Asia+Oceania
		EnableGeoZoneEU:         false,
		EnableGeoZoneNA:         false,
		EnableGeoZoneSA:         false,
		EnableGeoZoneAF:         false,
		EnableOriginShield:      true,
		OriginShieldZoneCode:    "SG", // Singapore
		EnableAutoSSL:           true,
		EnableBrotliCompression: true,
		CacheExpirationTime:     1440, // 24 hours
	}

	var zone PullZone
	err := c.post(ctx, "/pullzone", req, &zone)
	if err != nil {
		return nil, err
	}

	c.logger.Info("Pull zone created",
		zap.Int64("zone_id", zone.ID),
		zap.String("name", zone.Name),
		zap.String("domain", domain),
	)

	return &zone, nil
}

// GetPullZone retrieves a pull zone by ID
// API: GET /pullzone/{id}
func (c *Client) GetPullZone(ctx context.Context, zoneID int64) (*PullZone, error) {
	if zoneID <= 0 {
		return nil, fmt.Errorf("zone ID must be positive")
	}

	var zone PullZone
	path := fmt.Sprintf("/pullzone/%d", zoneID)
	err := c.get(ctx, path, &zone)
	if err != nil {
		return nil, err
	}

	return &zone, nil
}

// GetPullZoneByName retrieves a pull zone by name
// Bunny.net doesn't have a direct "get by name" endpoint, so we list all zones
func (c *Client) GetPullZoneByName(ctx context.Context, name string) (*PullZone, error) {
	if name == "" {
		return nil, fmt.Errorf("name is required")
	}

	zones, err := c.ListPullZones(ctx)
	if err != nil {
		return nil, err
	}

	for _, zone := range zones {
		if zone.Name == name {
			return &zone, nil
		}
	}

	return nil, &APIError{
		StatusCode: http.StatusNotFound,
		Message:    fmt.Sprintf("Pull zone with name %s not found", name),
	}
}

// ListPullZones lists all pull zones
// API: GET /pullzone
func (c *Client) ListPullZones(ctx context.Context) ([]PullZone, error) {
	var resp PullZoneListResponse
	err := c.get(ctx, "/pullzone", &resp)
	if err != nil {
		return nil, err
	}

	return resp.Items, nil
}

// UpdatePullZone updates a pull zone
// API: POST /pullzone/{id}
func (c *Client) UpdatePullZone(ctx context.Context, zoneID int64, req *UpdatePullZoneRequest) error {
	if zoneID <= 0 {
		return fmt.Errorf("zone ID must be positive")
	}
	if req == nil {
		return fmt.Errorf("request is required")
	}

	path := fmt.Sprintf("/pullzone/%d", zoneID)
	err := c.post(ctx, path, req, nil)
	if err != nil {
		return err
	}

	c.logger.Info("Pull zone updated", zap.Int64("zone_id", zoneID))
	return nil
}

// DeletePullZone deletes a pull zone
// API: DELETE /pullzone/{id}
func (c *Client) DeletePullZone(ctx context.Context, zoneID int64) error {
	if zoneID <= 0 {
		return fmt.Errorf("zone ID must be positive")
	}

	path := fmt.Sprintf("/pullzone/%d", zoneID)
	err := c.delete(ctx, path)
	if err != nil {
		return err
	}

	c.logger.Info("Pull zone deleted", zap.Int64("zone_id", zoneID))
	return nil
}

// PurgePullZoneCache purges the cache for a pull zone
// API: POST /pullzone/{id}/purgeCache
func (c *Client) PurgePullZoneCache(ctx context.Context, zoneID int64) error {
	if zoneID <= 0 {
		return fmt.Errorf("zone ID must be positive")
	}

	path := fmt.Sprintf("/pullzone/%d/purgeCache", zoneID)
	err := c.post(ctx, path, struct{}{}, nil)
	if err != nil {
		return err
	}

	c.logger.Info("Pull zone cache purged", zap.Int64("zone_id", zoneID))
	return nil
}

// PurgePullZoneCacheByURL purges specific URLs from a pull zone cache
// API: POST /pullzone/{id}/purgeCache
func (c *Client) PurgePullZoneCacheByURL(ctx context.Context, zoneID int64, urls []string) error {
	if zoneID <= 0 {
		return fmt.Errorf("zone ID must be positive")
	}
	if len(urls) == 0 {
		return fmt.Errorf("at least one URL is required")
	}

	path := fmt.Sprintf("/pullzone/%d/purgeCache", zoneID)
	req := map[string]interface{}{
		"Urls": urls,
	}

	err := c.post(ctx, path, req, nil)
	if err != nil {
		return err
	}

	c.logger.Info("Pull zone cache purged by URL",
		zap.Int64("zone_id", zoneID),
		zap.Int("url_count", len(urls)),
	)
	return nil
}

// AddPullZoneHostname adds a hostname to a pull zone
// API: POST /pullzone/{id}/addHostname
func (c *Client) AddPullZoneHostname(ctx context.Context, zoneID int64, hostname string) error {
	if zoneID <= 0 {
		return fmt.Errorf("zone ID must be positive")
	}
	if hostname == "" {
		return fmt.Errorf("hostname is required")
	}

	req := &AddHostnameRequest{
		Hostname: hostname,
	}

	path := fmt.Sprintf("/pullzone/%d/addHostname", zoneID)
	err := c.post(ctx, path, req, nil)
	if err != nil {
		return err
	}

	c.logger.Info("Hostname added to pull zone",
		zap.Int64("zone_id", zoneID),
		zap.String("hostname", hostname),
	)
	return nil
}

// SetPullZoneHostnames sets the hostnames for a pull zone (replaces existing)
// API: POST /pullzone/{id}/setHostnames
func (c *Client) SetPullZoneHostnames(ctx context.Context, zoneID int64, hostnames []string) error {
	if zoneID <= 0 {
		return fmt.Errorf("zone ID must be positive")
	}
	if len(hostnames) == 0 {
		return fmt.Errorf("at least one hostname is required")
	}

	req := map[string]interface{}{
		"Hostnames": hostnames,
	}

	path := fmt.Sprintf("/pullzone/%d/setHostnames", zoneID)
	err := c.post(ctx, path, req, nil)
	if err != nil {
		return err
	}

	c.logger.Info("Pull zone hostnames set",
		zap.Int64("zone_id", zoneID),
		zap.Int("hostname_count", len(hostnames)),
	)
	return nil
}

// RemovePullZoneHostname removes a hostname from a pull zone
// API: POST /pullzone/{id}/removeHostname
func (c *Client) RemovePullZoneHostname(ctx context.Context, zoneID int64, hostname string) error {
	if zoneID <= 0 {
		return fmt.Errorf("zone ID must be positive")
	}
	if hostname == "" {
		return fmt.Errorf("hostname is required")
	}

	req := &AddHostnameRequest{
		Hostname: hostname,
	}

	path := fmt.Sprintf("/pullzone/%d/removeHostname", zoneID)
	err := c.post(ctx, path, req, nil)
	if err != nil {
		return err
	}

	c.logger.Info("Hostname removed from pull zone",
		zap.Int64("zone_id", zoneID),
		zap.String("hostname", hostname),
	)
	return nil
}

// GetSSLCertificate retrieves SSL certificate for a pull zone hostname
// API: GET /pullzone/{id}/certificates
func (c *Client) GetSSLCertificate(ctx context.Context, zoneID int64) (*SSLCertificate, error) {
	if zoneID <= 0 {
		return nil, fmt.Errorf("zone ID must be positive")
	}

	var resp SSLCertificatesResponse
	path := fmt.Sprintf("/pullzone/%d/certificates", zoneID)
	err := c.get(ctx, path, &resp)
	if err != nil {
		return nil, err
	}

	if len(resp.Items) == 0 {
		return nil, &APIError{
			StatusCode: http.StatusNotFound,
			Message:    "No SSL certificate found for this pull zone",
		}
	}

	return &resp.Items[0], nil
}

// AddCertificate adds a custom SSL certificate to a pull zone
// API: POST /pullzone/{id}/addCertificate
func (c *Client) AddCertificate(ctx context.Context, zoneID int64, hostname, certificate, certificateKey string) error {
	if zoneID <= 0 {
		return fmt.Errorf("zone ID must be positive")
	}
	if hostname == "" {
		return fmt.Errorf("hostname is required")
	}
	if certificate == "" {
		return fmt.Errorf("certificate is required")
	}
	if certificateKey == "" {
		return fmt.Errorf("certificate key is required")
	}

	req := map[string]string{
		"Hostname":       hostname,
		"Certificate":    certificate,
		"CertificateKey": certificateKey,
	}

	path := fmt.Sprintf("/pullzone/%d/addCertificate", zoneID)
	err := c.post(ctx, path, req, nil)
	if err != nil {
		return err
	}

	c.logger.Info("SSL certificate added",
		zap.Int64("zone_id", zoneID),
		zap.String("hostname", hostname),
	)
	return nil
}

// ForceSSLCertificate forces SSL certificate issuance
// API: GET /pullzone/{id}/forceCertificate
func (c *Client) ForceSSLCertificate(ctx context.Context, zoneID int64) error {
	if zoneID <= 0 {
		return fmt.Errorf("zone ID must be positive")
	}

	path := fmt.Sprintf("/pullzone/%d/forceCertificate", zoneID)
	err := c.get(ctx, path, nil)
	if err != nil {
		return err
	}

	c.logger.Info("SSL certificate forced", zap.Int64("zone_id", zoneID))
	return nil
}

// generatePullZoneName generates a pull zone name from a domain
// e.g., "example.com" -> "morden-example-com"
func generatePullZoneName(domain string) string {
	// Convert domain to lowercase and replace dots with dashes
	name := strings.ToLower(domain)
	name = strings.ReplaceAll(name, ".", "-")
	// Prefix with morden-
	return "morden-" + name
}

// GetPullZoneStats returns statistics for a pull zone
// This is delegated to the stats package methods
func (c *Client) GetPullZoneStats(ctx context.Context, pullZoneID int64, from, to time.Time) (*PullZoneStats, error) {
	return GetPullZoneStats(ctx, c, pullZoneID, from, to)
}
