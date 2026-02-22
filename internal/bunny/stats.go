package bunny

import (
	"context"
	"fmt"
	"time"
)

// StatsRequest represents a request for statistics
type StatsRequest struct {
	PullZoneID int64     `json:"PullZoneId"`
	FromDate   time.Time `json:"FromDate"`
	ToDate     time.Time `json:"ToDate"`
}

// PullZoneStats represents statistics for a pull zone
type PullZoneStats struct {
	PullZoneID       int64     `json:"PullZoneId"`
	PullZoneName     string    `json:"PullZoneName"`
	TotalRequests    int64     `json:"TotalRequests"`
	TotalBandwidth   int64     `json:"TotalBandwidth"`
	TotalCacheHits   int64     `json:"TotalCacheHits"`
	TotalCacheMisses int64     `json:"TotalCacheMisses"`
	CacheHitRate     float64   `json:"CacheHitRate"`
	StartDate        time.Time `json:"StartDate"`
	EndDate          time.Time `json:"EndDate"`
	Status           string    `json:"Status,omitempty"`
	Timestamp        time.Time `json:"Timestamp,omitempty"`
}

// BandwidthEntry represents a bandwidth consumer entry
type BandwidthEntry struct {
	ZoneID     int64     `json:"PullZoneId"`
	ZoneName   string    `json:"PullZoneName"`
	Hostname   string    `json:"Hostname,omitempty"`
	Bandwidth  int64     `json:"Bandwidth"`
	Requests   int64     `json:"Requests"`
	Percentage float64   `json:"Percentage"`
	Date       time.Time `json:"Date"`
}

// StatsResponse is the response from the stats API
type StatsResponse struct {
	TotalRequests  int64 `json:"TotalRequests"`
	TotalBandwidth int64 `json:"TotalBandwidth"`
	CacheHits      int64 `json:"CacheHits"`
	CacheMisses    int64 `json:"CacheMisses"`
}

// BandwidthResponse is the response from the bandwidth stats API
type BandwidthResponse struct {
	Items []BandwidthEntry `json:"Items"`
}

// TimestampedStats represents stats with a timestamp
type TimestampedStats struct {
	Timestamp      time.Time `json:"Timestamp"`
	TotalRequests  int64     `json:"TotalRequests"`
	TotalBandwidth int64     `json:"TotalBandwidth"`
	CacheHits      int64     `json:"CacheHits"`
	CacheMisses    int64     `json:"CacheMisses"`
}

// GetPullZoneStats retrieves statistics for a specific pull zone
// API: GET /pullzone/{id}/stats
func GetPullZoneStats(ctx context.Context, client *Client, pullZoneID int64, from, to time.Time) (*PullZoneStats, error) {
	if pullZoneID <= 0 {
		return nil, fmt.Errorf("pull zone ID must be positive")
	}

	// Format dates as required by Bunny.net API (YYYY-MM-DD)
	fromStr := from.Format("2006-01-02")
	toStr := to.Format("2006-01-02")

	path := fmt.Sprintf("/pullzone/%d/stats?DateStart=%s&DateEnd=%s", pullZoneID, fromStr, toStr)

	var resp StatsResponse
	err := client.get(ctx, path, &resp)
	if err != nil {
		return nil, err
	}

	// Get the pull zone name
	zone, err := client.GetPullZone(ctx, pullZoneID)
	var zoneName string
	if err == nil && zone != nil {
		zoneName = zone.Name
	}

	// Calculate cache hit rate
	var hitRate float64
	totalRequests := resp.CacheHits + resp.CacheMisses
	if totalRequests > 0 {
		hitRate = float64(resp.CacheHits) / float64(totalRequests) * 100
	}

	stats := &PullZoneStats{
		PullZoneID:       pullZoneID,
		PullZoneName:     zoneName,
		TotalRequests:    resp.TotalRequests,
		TotalBandwidth:   resp.TotalBandwidth,
		TotalCacheHits:   resp.CacheHits,
		TotalCacheMisses: resp.CacheMisses,
		CacheHitRate:     hitRate,
		StartDate:        from,
		EndDate:          to,
		Status:           "completed",
		Timestamp:        time.Now(),
	}

	return stats, nil
}

// GetPullZoneStatsHourly retrieves hourly statistics for a pull zone
// API: GET /pullzone/{id}/stats?hourly=true
func GetPullZoneStatsHourly(ctx context.Context, client *Client, pullZoneID int64, from, to time.Time) ([]TimestampedStats, error) {
	if pullZoneID <= 0 {
		return nil, fmt.Errorf("pull zone ID must be positive")
	}

	fromStr := from.Format("2006-01-02")
	toStr := to.Format("2006-01-02")

	path := fmt.Sprintf("/pullzone/%d/stats?DateStart=%s&DateEnd=%s&Hourly=true", pullZoneID, fromStr, toStr)

	var resp []TimestampedStats
	err := client.get(ctx, path, &resp)
	if err != nil {
		return nil, err
	}

	return resp, nil
}

// GetTopBandwidthConsumers retrieves the top bandwidth consumers across all pull zones
// API: GET /statistics
func GetTopBandwidthConsumers(ctx context.Context, client *Client, limit int) ([]BandwidthEntry, error) {
	if limit <= 0 {
		limit = 10 // Default limit
	}
	if limit > 100 {
		limit = 100 // Max limit
	}

	path := fmt.Sprintf("/statistics?limit=%d&orderBy=Bandwidth", limit)

	var resp BandwidthResponse
	err := client.get(ctx, path, &resp)
	if err != nil {
		return nil, err
	}

	// Limit results
	if len(resp.Items) > limit {
		resp.Items = resp.Items[:limit]
	}

	return resp.Items, nil
}

// GetTotalBandwidth retrieves total bandwidth usage across all pull zones
// API: GET /statistics
func GetTotalBandwidth(ctx context.Context, client *Client) (int64, error) {
	// This returns a simplified stats object
	type totalBandwidthResponse struct {
		TotalBandwidth int64 `json:"TotalBandwidth"`
	}

	var resp totalBandwidthResponse
	err := client.get(ctx, "/statistics", &resp)
	if err != nil {
		return 0, err
	}

	return resp.TotalBandwidth, nil
}

// GetPullZoneBandwidth retrieves bandwidth statistics for a specific pull zone
// API: GET /pullzone/{id}/stats
func (c *Client) GetPullZoneBandwidth(ctx context.Context, pullZoneID int64, from, to time.Time) (*PullZoneStats, error) {
	return GetPullZoneStats(ctx, c, pullZoneID, from, to)
}

// GetAccountStatistics retrieves account-wide statistics
// API: GET /billing
// Note: Bunny.net uses the billing endpoint for general account stats
func (c *Client) GetAccountStatistics(ctx context.Context) (*AccountStats, error) {
	type billingResponse struct {
		BillingID     string  `json:"BillingId"`
		Balance       float64 `json:"Balance"`
		PaidThisMonth float64 `json:"PaidThisMonth"`
		DueThisMonth  float64 `json:"DueThisMonth"`
	}

	var resp billingResponse
	err := c.get(ctx, "/billing", &resp)
	if err != nil {
		return nil, err
	}

	stats := &AccountStats{
		BillingID:     resp.BillingID,
		Balance:       resp.Balance,
		PaidThisMonth: resp.PaidThisMonth,
		DueThisMonth:  resp.DueThisMonth,
	}

	return stats, nil
}

// AccountStats represents account billing and usage statistics
type AccountStats struct {
	BillingID     string  `json:"BillingId"`
	Balance       float64 `json:"Balance"`
	PaidThisMonth float64 `json:"PaidThisMonth"`
	DueThisMonth  float64 `json:"DueThisMonth"`
}

// GetZoneStats retrieves DNS zone statistics
// API: GET /dns/{id}/stats
func (c *Client) GetZoneStats(ctx context.Context, zoneID int64) (*ZoneStats, error) {
	if zoneID <= 0 {
		return nil, fmt.Errorf("zone ID must be positive")
	}

	zone, err := c.GetDNSZoneByID(ctx, zoneID)
	if err != nil {
		return nil, err
	}

	stats := &ZoneStats{
		ZoneID:   int(zoneID),
		ZoneName: zone.Domain,
	}

	return stats, nil
}

// ZoneStats represents statistics for a DNS zone
type ZoneStats struct {
	ZoneID   int    `json:"ZoneId"`
	ZoneName string `json:"ZoneName"`
}

// GetDailyPullZoneStats retrieves daily statistics for a pull zone
// API: GET /pullzone/{id}/stats?daily=true
func (c *Client) GetDailyPullZoneStats(ctx context.Context, pullZoneID int64, from, to time.Time) ([]TimestampedStats, error) {
	if pullZoneID <= 0 {
		return nil, fmt.Errorf("pull zone ID must be positive")
	}

	fromStr := from.Format("2006-01-02")
	toStr := to.Format("2006-01-02")

	path := fmt.Sprintf("/pullzone/%d/stats?DateStart=%s&DateEnd=%s&Daily=true", pullZoneID, fromStr, toStr)

	var resp []TimestampedStats
	err := c.get(ctx, path, &resp)
	if err != nil {
		return nil, err
	}

	return resp, nil
}
