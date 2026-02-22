package bunny

import (
	"context"
	"fmt"
	"net/http"
	"time"

	"go.uber.org/zap"
)

// DNSRecordType represents the type of DNS record
type DNSRecordType int

const (
	// DNSRecordTypeA represents an A record
	DNSRecordTypeA DNSRecordType = 0
	// DNSRecordTypeAAAA represents an AAAA record
	DNSRecordTypeAAAA DNSRecordType = 1
	// DNSRecordTypeCNAME represents a CNAME record
	DNSRecordTypeCNAME DNSRecordType = 2
	// DNSRecordTypeTXT represents a TXT record
	DNSRecordTypeTXT DNSRecordType = 3
	// DNSRecordTypeMX represents an MX record
	DNSRecordTypeMX DNSRecordType = 4
	// DNSRecordTypeNS represents an NS record
	DNSRecordTypeNS DNSRecordType = 5
)

// String returns the string representation of the DNS record type
func (t DNSRecordType) String() string {
	switch t {
	case DNSRecordTypeA:
		return "A"
	case DNSRecordTypeAAAA:
		return "AAAA"
	case DNSRecordTypeCNAME:
		return "CNAME"
	case DNSRecordTypeTXT:
		return "TXT"
	case DNSRecordTypeMX:
		return "MX"
	case DNSRecordTypeNS:
		return "NS"
	default:
		return "UNKNOWN"
	}
}

// DNSZone represents a BunnyDNS zone
type DNSZone struct {
	ID          int64     `json:"Id"`
	Domain      string    `json:"Domain"`
	UserEnabled bool      `json:"UserEnabled"`
	SoaEmail    string    `json:"SoaEmail,omitempty"`
	Nameservers []string  `json:"Nameservers,omitempty"`
	CreatedAt   time.Time `json:"CreatedAt,omitempty"`
}

// DNSRecord represents a DNS record
type DNSRecord struct {
	ID           int64         `json:"Id"`
	Type         DNSRecordType `json:"Type"`
	Name         string        `json:"Name"`
	Value        string        `json:"Value"`
	TTL          int           `json:"TTL"`
	Priority     int           `json:"Priority,omitempty"`
	Weight       int           `json:"Weight,omitempty"`
	Flags        int           `json:"Flags,omitempty"`
	Tag          string        `json:"Tag,omitempty"`
	Port         int           `json:"Port,omitempty"`
	Enabled      bool          `json:"Enabled"`
	DisableLinks bool          `json:"DisableLinks,omitempty"`
}

// CreateDNSZoneRequest is the request to create a DNS zone
type CreateDNSZoneRequest struct {
	Domain   string `json:"Domain"`
	SoaEmail string `json:"SoaEmail,omitempty"`
}

// UpdateDNSZoneRequest is the request to update a DNS zone
type UpdateDNSZoneRequest struct {
	UserEnabled bool   `json:"UserEnabled"`
	SoaEmail    string `json:"SoaEmail,omitempty"`
}

// AddDNSRecordRequest is the request to add a DNS record
type AddDNSRecordRequest struct {
	Type         DNSRecordType `json:"Type"`
	Name         string        `json:"Name"`
	Value        string        `json:"Value"`
	TTL          int           `json:"TTL"`
	Priority     int           `json:"Priority,omitempty"`
	Weight       int           `json:"Weight,omitempty"`
	Flags        int           `json:"Flags,omitempty"`
	Tag          string        `json:"Tag,omitempty"`
	Port         int           `json:"Port,omitempty"`
	Enabled      bool          `json:"Enabled"`
	DisableLinks bool          `json:"DisableLinks,omitempty"`
}

// UpdateDNSRecordRequest is the request to update a DNS record
type UpdateDNSRecordRequest struct {
	Type         DNSRecordType `json:"Type"`
	Name         string        `json:"Name"`
	Value        string        `json:"Value"`
	TTL          int           `json:"TTL"`
	Priority     int           `json:"Priority,omitempty"`
	Weight       int           `json:"Weight,omitempty"`
	Flags        int           `json:"Flags,omitempty"`
	Tag          string        `json:"Tag,omitempty"`
	Port         int           `json:"Port,omitempty"`
	Enabled      bool          `json:"Enabled"`
	DisableLinks bool          `json:"DisableLinks,omitempty"`
}

// DNSZoneListResponse is the response from listing DNS zones
type DNSZoneListResponse struct {
	Items []DNSZone `json:"Items"`
}

// DNSRecordsResponse is the response from getting DNS records
type DNSRecordsResponse struct {
	Items []DNSRecord `json:"Items"`
}

// CreateDNSZone creates a new DNS zone
// API: POST /dns
func (c *Client) CreateDNSZone(ctx context.Context, domain string, soaEmail string) (*DNSZone, error) {
	if domain == "" {
		return nil, fmt.Errorf("domain is required")
	}

	req := &CreateDNSZoneRequest{
		Domain:   domain,
		SoaEmail: soaEmail,
	}

	var zone DNSZone
	err := c.post(ctx, "/dns", req, &zone)
	if err != nil {
		return nil, err
	}

	c.logger.Info("DNS zone created",
		zap.Int64("zone_id", zone.ID),
		zap.String("domain", zone.Domain),
	)

	return &zone, nil
}

// GetDNSZone retrieves a DNS zone by domain
// API: GET /dns/{id} (by zone ID) or we can search by listing
func (c *Client) GetDNSZone(ctx context.Context, domain string) (*DNSZone, error) {
	if domain == "" {
		return nil, fmt.Errorf("domain is required")
	}

	// Bunny.net doesn't have a direct "get by domain" endpoint
	// We need to list zones and find the matching one
	zones, err := c.ListDNSZones(ctx)
	if err != nil {
		return nil, err
	}

	for _, zone := range zones {
		if zone.Domain == domain {
			return &zone, nil
		}
	}

	return nil, &APIError{
		StatusCode: http.StatusNotFound,
		Message:    fmt.Sprintf("DNS zone for domain %s not found", domain),
	}
}

// GetDNSZoneByID retrieves a DNS zone by ID
// API: GET /dns/{id}
func (c *Client) GetDNSZoneByID(ctx context.Context, zoneID int64) (*DNSZone, error) {
	if zoneID <= 0 {
		return nil, fmt.Errorf("zone ID must be positive")
	}

	var zone DNSZone
	path := fmt.Sprintf("/dns/%d", zoneID)
	err := c.get(ctx, path, &zone)
	if err != nil {
		return nil, err
	}

	return &zone, nil
}

// ListDNSZones lists all DNS zones
// API: GET /dns
func (c *Client) ListDNSZones(ctx context.Context) ([]DNSZone, error) {
	var resp DNSZoneListResponse
	err := c.get(ctx, "/dns", &resp)
	if err != nil {
		return nil, err
	}

	return resp.Items, nil
}

// UpdateDNSZone updates a DNS zone
// API: POST /dns/{id}
func (c *Client) UpdateDNSZone(ctx context.Context, zoneID int64, req *UpdateDNSZoneRequest) error {
	if zoneID <= 0 {
		return fmt.Errorf("zone ID must be positive")
	}
	if req == nil {
		return fmt.Errorf("request is required")
	}

	path := fmt.Sprintf("/dns/%d", zoneID)
	err := c.post(ctx, path, req, nil)
	if err != nil {
		return err
	}

	c.logger.Info("DNS zone updated", zap.Int64("zone_id", zoneID))
	return nil
}

// DeleteDNSZone deletes a DNS zone
// API: DELETE /dns/{id}
func (c *Client) DeleteDNSZone(ctx context.Context, zoneID int64) error {
	if zoneID <= 0 {
		return fmt.Errorf("zone ID must be positive")
	}

	path := fmt.Sprintf("/dns/%d", zoneID)
	err := c.delete(ctx, path)
	if err != nil {
		return err
	}

	c.logger.Info("DNS zone deleted", zap.Int64("zone_id", zoneID))
	return nil
}

// AddDNSRecord adds a DNS record to a zone
// API: POST /dns/{id}/records
func (c *Client) AddDNSRecord(ctx context.Context, zoneID int64, req *AddDNSRecordRequest) (*DNSRecord, error) {
	if zoneID <= 0 {
		return nil, fmt.Errorf("zone ID must be positive")
	}
	if req == nil {
		return nil, fmt.Errorf("request is required")
	}
	if req.Name == "" {
		return nil, fmt.Errorf("record name is required")
	}
	if req.Value == "" {
		return nil, fmt.Errorf("record value is required")
	}

	path := fmt.Sprintf("/dns/%d/records", zoneID)

	var record DNSRecord
	err := c.post(ctx, path, req, &record)
	if err != nil {
		return nil, err
	}

	c.logger.Info("DNS record added",
		zap.Int64("zone_id", zoneID),
		zap.Int64("record_id", record.ID),
		zap.String("type", req.Type.String()),
		zap.String("name", req.Name),
	)

	return &record, nil
}

// GetDNSRecords retrieves all DNS records for a zone
// API: GET /dns/{id}/records
func (c *Client) GetDNSRecords(ctx context.Context, zoneID int64) ([]DNSRecord, error) {
	if zoneID <= 0 {
		return nil, fmt.Errorf("zone ID must be positive")
	}

	var resp DNSRecordsResponse
	path := fmt.Sprintf("/dns/%d/records", zoneID)
	err := c.get(ctx, path, &resp)
	if err != nil {
		return nil, err
	}

	return resp.Items, nil
}

// GetDNSRecord retrieves a single DNS record
// API: GET /dns/{id}/records/{recordId}
func (c *Client) GetDNSRecord(ctx context.Context, zoneID int64, recordID int64) (*DNSRecord, error) {
	if zoneID <= 0 {
		return nil, fmt.Errorf("zone ID must be positive")
	}
	if recordID <= 0 {
		return nil, fmt.Errorf("record ID must be positive")
	}

	var record DNSRecord
	path := fmt.Sprintf("/dns/%d/records/%d", zoneID, recordID)
	err := c.get(ctx, path, &record)
	if err != nil {
		return nil, err
	}

	return &record, nil
}

// UpdateDNSRecord updates a DNS record
// API: POST /dns/{id}/records/{recordId}
func (c *Client) UpdateDNSRecord(ctx context.Context, zoneID int64, recordID int64, req *UpdateDNSRecordRequest) error {
	if zoneID <= 0 {
		return fmt.Errorf("zone ID must be positive")
	}
	if recordID <= 0 {
		return fmt.Errorf("record ID must be positive")
	}
	if req == nil {
		return fmt.Errorf("request is required")
	}

	path := fmt.Sprintf("/dns/%d/records/%d", zoneID, recordID)
	err := c.post(ctx, path, req, nil)
	if err != nil {
		return err
	}

	c.logger.Info("DNS record updated",
		zap.Int64("zone_id", zoneID),
		zap.Int64("record_id", recordID),
	)

	return nil
}

// DeleteDNSRecord deletes a DNS record
// API: DELETE /dns/{id}/records/{recordId}
func (c *Client) DeleteDNSRecord(ctx context.Context, zoneID int64, recordID int64) error {
	if zoneID <= 0 {
		return fmt.Errorf("zone ID must be positive")
	}
	if recordID <= 0 {
		return fmt.Errorf("record ID must be positive")
	}

	path := fmt.Sprintf("/dns/%d/records/%d", zoneID, recordID)
	err := c.delete(ctx, path)
	if err != nil {
		return err
	}

	c.logger.Info("DNS record deleted",
		zap.Int64("zone_id", zoneID),
		zap.Int64("record_id", recordID),
	)

	return nil
}

// ImportDNSRecords imports DNS records from a zone file or server
// API: POST /dns/{id}/importRecords
func (c *Client) ImportDNSRecords(ctx context.Context, zoneID int64, domain string) error {
	if zoneID <= 0 {
		return fmt.Errorf("zone ID must be positive")
	}
	if domain == "" {
		return fmt.Errorf("domain is required")
	}

	// Bunny.net uses this to auto-import existing records
	req := map[string]string{
		"Domain": domain,
	}

	path := fmt.Sprintf("/dns/%d/importRecords", zoneID)
	err := c.post(ctx, path, req, nil)
	if err != nil {
		return err
	}

	c.logger.Info("DNS records imported",
		zap.Int64("zone_id", zoneID),
		zap.String("domain", domain),
	)

	return nil
}
