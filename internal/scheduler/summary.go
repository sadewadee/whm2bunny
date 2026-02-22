package scheduler

import (
	"context"
	"fmt"
	"os"
	"sort"
	"time"

	"github.com/mordenhost/whm2bunny/config"
	"github.com/mordenhost/whm2bunny/internal/bunny"
	"github.com/mordenhost/whm2bunny/internal/notifier"
	"github.com/mordenhost/whm2bunny/internal/state"
	"github.com/robfig/cron/v3"
	"go.uber.org/zap"
)

// Scheduler manages cron jobs for daily and weekly summaries
type Scheduler struct {
	cron          *cron.Cron
	bunnyClient   *bunny.Client
	notifier      *notifier.TelegramNotifier
	config        *config.Config
	logger        *zap.Logger
	snapshotStore *state.SnapshotStore
	running       bool
	mu            chan struct{}
}

// NewScheduler creates a new scheduler instance
func NewScheduler(
	cfg *config.Config,
	bunnyClient *bunny.Client,
	telegramNotifier *notifier.TelegramNotifier,
	snapshotStore *state.SnapshotStore,
	logger *zap.Logger,
) *Scheduler {
	return &Scheduler{
		cron:          cron.New(cron.WithSeconds()), // Use seconds precision
		bunnyClient:   bunnyClient,
		notifier:      telegramNotifier,
		config:        cfg,
		logger:        logger,
		snapshotStore: snapshotStore,
		running:       false,
		mu:            make(chan struct{}, 1),
	}
}

// Start starts the scheduler cron jobs
func (s *Scheduler) Start() error {
	s.mu <- struct{}{}
	defer func() { <-s.mu }()

	if s.running {
		s.logger.Info("Scheduler already running")
		return nil
	}

	// Check if summary is enabled
	if !s.config.Telegram.Summary.Enabled {
		s.logger.Info("Telegram summary is disabled, scheduler not starting")
		return nil
	}

	// Get timezone for scheduling
	loc, err := s.getTimezone()
	if err != nil {
		return fmt.Errorf("failed to get timezone: %w", err)
	}

	// Parse daily schedule
	dailySchedule := s.config.Telegram.Summary.Schedule
	if dailySchedule == "" {
		dailySchedule = "0 9 * * *" // Default: 9:00 AM daily
	}

	// Convert standard 5-field cron to 6-field (with seconds)
	dailyScheduleWithSec := "0 " + dailySchedule

	// Add daily summary job
	_, err = s.cron.AddFunc(dailyScheduleWithSec, func() {
		s.runDailySummary(context.Background())
	})
	if err != nil {
		return fmt.Errorf("failed to add daily summary job: %w", err)
	}
	s.logger.Info("Added daily summary job",
		zap.String("schedule", dailyScheduleWithSec),
		zap.String("timezone", loc.String()))

	// Parse weekly schedule
	weeklySchedule := s.config.Telegram.Summary.WeeklySchedule
	if weeklySchedule == "" {
		weeklySchedule = "0 9 * * 1" // Default: 9:00 AM on Monday
	}

	// Convert standard 5-field cron to 6-field (with seconds)
	weeklyScheduleWithSec := "0 " + weeklySchedule

	// Add weekly summary job
	_, err = s.cron.AddFunc(weeklyScheduleWithSec, func() {
		s.runWeeklySummary(context.Background())
	})
	if err != nil {
		return fmt.Errorf("failed to add weekly summary job: %w", err)
	}
	s.logger.Info("Added weekly summary job",
		zap.String("schedule", weeklyScheduleWithSec),
		zap.String("timezone", loc.String()))

	// Add bandwidth alert check job - run every hour (at minute 0)
	_, err = s.cron.AddFunc("0 0 * * * *", func() {
		s.checkBandwidthAlerts(context.Background())
	})
	if err != nil {
		return fmt.Errorf("failed to add bandwidth alert job: %w", err)
	}
	s.logger.Info("Added bandwidth alert check job", zap.String("schedule", "0 0 * * * *"))

	// Start the cron scheduler
	s.cron.Start()
	s.running = true

	s.logger.Info("Scheduler started",
		zap.String("timezone", loc.String()),
		zap.String("daily_schedule", dailySchedule),
		zap.String("weekly_schedule", weeklySchedule))

	return nil
}

// Stop stops the scheduler cron jobs gracefully
func (s *Scheduler) Stop() {
	s.mu <- struct{}{}
	defer func() { <-s.mu }()

	if !s.running {
		return
	}

	ctx := s.cron.Stop()
	select {
	case <-ctx.Done():
		// All jobs completed
	case <-time.After(10 * time.Second):
		s.logger.Warn("Scheduler stop timed out, forcing stop")
	}

	s.running = false
	s.logger.Info("Scheduler stopped")
}

// getTimezone returns the configured timezone or default to Asia/Jakarta
func (s *Scheduler) getTimezone() (*time.Location, error) {
	tz := s.config.Telegram.Summary.Timezone
	if tz == "" {
		tz = "Asia/Jakarta"
	}
	return time.LoadLocation(tz)
}

// runDailySummary generates and sends the daily summary
func (s *Scheduler) runDailySummary(ctx context.Context) {
	s.logger.Info("Running daily summary")

	// Get yesterday's date range
	now := time.Now()
	loc, err := s.getTimezone()
	if err != nil {
		s.logger.Error("Failed to get timezone", zap.Error(err))
		loc = time.UTC
	}
	nowInLoc := now.In(loc)
	yesterday := nowInLoc.AddDate(0, 0, -1)

	from := time.Date(yesterday.Year(), yesterday.Month(), yesterday.Day(), 0, 0, 0, 0, loc)
	to := time.Date(yesterday.Year(), yesterday.Month(), yesterday.Day(), 23, 59, 59, 0, loc)

	// Get all pull zones
	zones, err := s.bunnyClient.ListPullZones(ctx)
	if err != nil {
		s.logger.Error("Failed to list pull zones for daily summary", zap.Error(err))
		return
	}

	// Collect stats for all zones
	var totalBandwidth int64
	var totalRequestsVal int64
	var totalCacheHits int64
	var totalCacheMisses int64
	zoneStats := make([]bunny.BandwidthEntry, 0, len(zones))

	for _, zone := range zones {
		stats, err := bunny.GetPullZoneStats(ctx, s.bunnyClient, zone.ID, from, to)
		if err != nil {
			s.logger.Warn("Failed to get stats for zone",
				zap.Int64("zone_id", zone.ID),
				zap.String("zone_name", zone.Name),
				zap.Error(err))
			continue
		}

		totalBandwidth += stats.TotalBandwidth
		totalRequestsVal += stats.TotalRequests
		totalCacheHits += stats.TotalCacheHits
		totalCacheMisses += stats.TotalCacheMisses

		zoneStats = append(zoneStats, bunny.BandwidthEntry{
			ZoneID:    zone.ID,
			ZoneName:  zone.Name,
			Hostname:  zone.Name,
			Bandwidth: stats.TotalBandwidth,
			Requests:  stats.TotalRequests,
			Date:      from,
		})

		// Store snapshot for comparison
		if s.snapshotStore != nil {
			snapshot := state.BandwidthSnapshot{
				Timestamp:   time.Now(),
				ZoneID:      zone.ID,
				ZoneName:    zone.Name,
				Bandwidth:   stats.TotalBandwidth,
				Requests:    stats.TotalRequests,
				CacheHits:   stats.TotalCacheHits,
				CacheMisses: stats.TotalCacheMisses,
			}
			_ = s.snapshotStore.AddSnapshot(snapshot)
		}
	}

	// Sort by bandwidth (descending)
	sort.Slice(zoneStats, func(i, j int) bool {
		return zoneStats[i].Bandwidth > zoneStats[j].Bandwidth
	})

	// Calculate cache hit rate
	cacheHitRate := 0.0
	totalRequestsCalc := totalCacheHits + totalCacheMisses
	if totalRequestsCalc > 0 {
		cacheHitRate = float64(totalCacheHits) / float64(totalRequestsCalc) * 100
	}

	// Get top N zones
	topN := s.config.Telegram.Summary.IncludeTopBandwidth
	if topN <= 0 {
		topN = 5
	}
	if topN > len(zoneStats) {
		topN = len(zoneStats)
	}

	// Build summary message
	message := s.formatDailySummary(yesterday, totalBandwidth, totalRequestsVal, cacheHitRate, zoneStats[:topN])

	// Send notification
	if s.notifier != nil && s.notifier.IsEnabled() {
		if err := s.notifier.SendRaw(ctx, message); err != nil {
			s.logger.Error("Failed to send daily summary", zap.Error(err))
		} else {
			s.logger.Info("Daily summary sent successfully")
		}
	}
}

// runWeeklySummary generates and sends the weekly summary
func (s *Scheduler) runWeeklySummary(ctx context.Context) {
	s.logger.Info("Running weekly summary")

	// Get last week's date range
	now := time.Now()
	loc, err := s.getTimezone()
	if err != nil {
		s.logger.Error("Failed to get timezone", zap.Error(err))
		loc = time.UTC
	}
	nowInLoc := now.In(loc)

	// Find last Monday
	weekday := nowInLoc.Weekday()
	daysSinceMonday := (int(weekday) - 1 + 7) % 7
	lastMonday := nowInLoc.AddDate(0, 0, -daysSinceMonday-7)
	lastSunday := lastMonday.AddDate(0, 0, 6)

	from := time.Date(lastMonday.Year(), lastMonday.Month(), lastMonday.Day(), 0, 0, 0, 0, loc)
	to := time.Date(lastSunday.Year(), lastSunday.Month(), lastSunday.Day(), 23, 59, 59, 0, loc)

	// Get previous week for comparison
	prevMonday := lastMonday.AddDate(0, 0, -7)
	prevSunday := lastMonday.AddDate(0, 0, -1)
	prevFrom := time.Date(prevMonday.Year(), prevMonday.Month(), prevMonday.Day(), 0, 0, 0, 0, loc)
	prevTo := time.Date(prevSunday.Year(), prevSunday.Month(), prevSunday.Day(), 23, 59, 59, 0, loc)

	// Get all pull zones
	zones, err := s.bunnyClient.ListPullZones(ctx)
	if err != nil {
		s.logger.Error("Failed to list pull zones for weekly summary", zap.Error(err))
		return
	}

	// Collect stats for all zones (current week)
	var totalBandwidth int64
	var totalRequestsVal int64
	var totalCacheHits int64
	var totalCacheMisses int64
	zoneStats := make([]bunny.BandwidthEntry, 0, len(zones))

	// Previous week totals for comparison
	var prevTotalBandwidth int64

	for _, zone := range zones {
		stats, err := bunny.GetPullZoneStats(ctx, s.bunnyClient, zone.ID, from, to)
		if err != nil {
			s.logger.Warn("Failed to get stats for zone",
				zap.Int64("zone_id", zone.ID),
				zap.String("zone_name", zone.Name),
				zap.Error(err))
			continue
		}

		totalBandwidth += stats.TotalBandwidth
		totalRequestsVal += stats.TotalRequests
		totalCacheHits += stats.TotalCacheHits
		totalCacheMisses += stats.TotalCacheMisses

		zoneStats = append(zoneStats, bunny.BandwidthEntry{
			ZoneID:    zone.ID,
			ZoneName:  zone.Name,
			Hostname:  zone.Name,
			Bandwidth: stats.TotalBandwidth,
			Requests:  stats.TotalRequests,
			Date:      from,
		})

		// Get previous week stats for comparison
		prevStats, errPrev := bunny.GetPullZoneStats(ctx, s.bunnyClient, zone.ID, prevFrom, prevTo)
		if errPrev == nil {
			prevTotalBandwidth += prevStats.TotalBandwidth
		}
	}

	// Sort by bandwidth (descending)
	sort.Slice(zoneStats, func(i, j int) bool {
		return zoneStats[i].Bandwidth > zoneStats[j].Bandwidth
	})

	// Calculate cache hit rate
	cacheHitRate := 0.0
	totalRequestsCalc := totalCacheHits + totalCacheMisses
	if totalRequestsCalc > 0 {
		cacheHitRate = float64(totalCacheHits) / float64(totalRequestsCalc) * 100
	}

	// Calculate bandwidth change
	bandwidthChange := 0.0
	if prevTotalBandwidth > 0 {
		bandwidthChange = float64(totalBandwidth-prevTotalBandwidth) / float64(prevTotalBandwidth) * 100
	}

	// Get top N zones
	topN := s.config.Telegram.Summary.IncludeTopBandwidth
	if topN <= 0 {
		topN = 10
	}
	if topN > len(zoneStats) {
		topN = len(zoneStats)
	}

	// Get week number
	_, weekNum := from.ISOWeek()

	// Build summary message
	message := s.formatWeeklySummary(weekNum, from.Year(), totalBandwidth, totalRequestsVal, cacheHitRate, bandwidthChange, zoneStats[:topN])

	// Send notification
	if s.notifier != nil && s.notifier.IsEnabled() {
		if err := s.notifier.SendRaw(ctx, message); err != nil {
			s.logger.Error("Failed to send weekly summary", zap.Error(err))
		} else {
			s.logger.Info("Weekly summary sent successfully")
		}
	}
}

// checkBandwidthAlerts checks for bandwidth spikes and sends alerts
func (s *Scheduler) checkBandwidthAlerts(ctx context.Context) {
	s.logger.Debug("Checking bandwidth alerts")

	if s.snapshotStore == nil {
		return
	}

	// Get alert threshold
	threshold := float64(s.config.Telegram.Summary.BandwidthAlertThreshold)
	if threshold <= 0 {
		threshold = 50 // Default: 50%
	}

	// Get all pull zones
	zones, err := s.bunnyClient.ListPullZones(ctx)
	if err != nil {
		s.logger.Error("Failed to list pull zones for bandwidth check", zap.Error(err))
		return
	}

	// Check each zone for bandwidth spikes
	now := time.Now()
	loc, _ := s.getTimezone()
	nowInLoc := now.In(loc)

	// Current 24 hours
	currentFrom := time.Date(nowInLoc.Year(), nowInLoc.Month(), nowInLoc.Day(), 0, 0, 0, 0, loc)
	currentTo := nowInLoc

	// Previous 24 hours
	previousFrom := currentFrom.AddDate(0, 0, -1)
	previousTo := currentFrom

	for _, zone := range zones {
		// Get current stats
		currentStats, err := bunny.GetPullZoneStats(ctx, s.bunnyClient, zone.ID, currentFrom, currentTo)
		if err != nil {
			continue
		}

		// Get previous stats from snapshot store
		snapshots := s.snapshotStore.GetSnapshotsByZone(zone.ID, previousFrom)
		var previousBandwidth int64
		for _, snap := range snapshots {
			previousBandwidth += snap.Bandwidth
		}

		// If no previous snapshot, try to get from API
		if previousBandwidth == 0 {
			prevStats, errPrev := bunny.GetPullZoneStats(ctx, s.bunnyClient, zone.ID, previousFrom, previousTo)
			if errPrev == nil {
				previousBandwidth = prevStats.TotalBandwidth
			}
		}

		// Calculate percentage increase
		if previousBandwidth > 0 {
			percentIncrease := float64(currentStats.TotalBandwidth-previousBandwidth) / float64(previousBandwidth) * 100

			if percentIncrease >= threshold {
				s.logger.Warn("Bandwidth spike detected",
					zap.Int64("zone_id", zone.ID),
					zap.String("zone_name", zone.Name),
					zap.Float64("increase", percentIncrease))

				// Send alert
				message := s.formatBandwidthAlert(zone.Name, currentStats.TotalBandwidth, previousBandwidth, percentIncrease)
				if s.notifier != nil && s.notifier.IsEnabled() {
					_ = s.notifier.SendRaw(ctx, message)
				}
			}
		}
	}
}

// formatDailySummary formats the daily summary message
func (s *Scheduler) formatDailySummary(date time.Time, bandwidth, requests int64, cacheHitRate float64, topZones []bunny.BandwidthEntry) string {
	hostname := s.getHostname()
	dateStr := date.Format("Jan 2, 2006")
	bandwidthGB := float64(bandwidth) / (1024 * 1024 * 1024)

	message := fmt.Sprintf("📊 <b>Daily Summary</b> - %s\n\n📈 <b>Total Bandwidth:</b> %.2f GB\n📈 <b>Total Requests:</b> %s\n📈 <b>Cache Hit Rate:</b> %.1f%%\n\n🔝 <b>Top %d Domains:</b>",
		dateStr,
		bandwidthGB,
		formatNumber(requests),
		cacheHitRate,
		len(topZones),
	)

	totalBandwidthFloat := float64(bandwidth)
	for i, zone := range topZones {
		percentage := 0.0
		if totalBandwidthFloat > 0 {
			percentage = float64(zone.Bandwidth) / totalBandwidthFloat * 100
		}
		message += fmt.Sprintf("\n%d. %s - %.2f GB (%.0f%%)",
			i+1,
			zone.ZoneName,
			float64(zone.Bandwidth)/(1024*1024*1024),
			percentage,
		)
	}

	message += fmt.Sprintf("\n\n🖥️ <b>Server:</b> %s", hostname)

	return message
}

// formatWeeklySummary formats the weekly summary message
func (s *Scheduler) formatWeeklySummary(weekNum, year int, bandwidth, requests int64, cacheHitRate, bandwidthChange float64, topZones []bunny.BandwidthEntry) string {
	hostname := s.getHostname()
	bandwidthGB := float64(bandwidth) / (1024 * 1024 * 1024)

	changeStr := ""
	if bandwidthChange > 0 {
		changeStr = fmt.Sprintf("+%.0f%%", bandwidthChange)
	} else if bandwidthChange < 0 {
		changeStr = fmt.Sprintf("%.0f%%", bandwidthChange)
	} else {
		changeStr = "0%"
	}

	message := fmt.Sprintf("📊 <b>Weekly Summary</b> - Week %d, %d\n\n📈 <b>Total Bandwidth:</b> %.2f GB\n📈 <b>Total Requests:</b> %s\n📈 <b>Avg Cache Hit Rate:</b> %.1f%%\n📈 <b>Bandwidth Change:</b> %s vs last week\n\n🔝 <b>Top %d Domains:</b>",
		weekNum,
		year,
		bandwidthGB,
		formatNumber(requests),
		cacheHitRate,
		changeStr,
		len(topZones),
	)

	totalBandwidthFloat := float64(bandwidth)
	for i, zone := range topZones {
		percentage := 0.0
		if totalBandwidthFloat > 0 {
			percentage = float64(zone.Bandwidth) / totalBandwidthFloat * 100
		}
		message += fmt.Sprintf("\n%d. %s - %.2f GB (%.0f%%)",
			i+1,
			zone.ZoneName,
			float64(zone.Bandwidth)/(1024*1024*1024),
			percentage,
		)
	}

	message += fmt.Sprintf("\n\n🖥️ <b>Server:</b> %s", hostname)

	return message
}

// formatBandwidthAlert formats the bandwidth alert message
func (s *Scheduler) formatBandwidthAlert(domain string, current, previous int64, percentIncrease float64) string {
	hostname := s.getHostname()
	currentGB := float64(current) / (1024 * 1024 * 1024)
	previousGB := float64(previous) / (1024 * 1024 * 1024)

	return fmt.Sprintf(`⚠️ <b>Bandwidth Alert</b>

🌐 <b>Domain:</b> %s
📈 <b>Increase:</b> %.0f%% in last 24 hours
📊 <b>Current:</b> %.2f GB/day
📊 <b>Previous:</b> %.2f GB/day

🖥️ <b>Server:</b> %s`,
		domain,
		percentIncrease,
		currentGB,
		previousGB,
		hostname,
	)
}

// getHostname returns the server hostname
func (s *Scheduler) getHostname() string {
	hostname, err := os.Hostname()
	if err != nil {
		return "unknown"
	}
	return hostname
}

// formatNumber formats a large number with K/M/B suffixes
func formatNumber(n int64) string {
	if n >= 1_000_000_000 {
		return fmt.Sprintf("%.2fB", float64(n)/1_000_000_000)
	}
	if n >= 1_000_000 {
		return fmt.Sprintf("%.2fM", float64(n)/1_000_000)
	}
	if n >= 1_000 {
		return fmt.Sprintf("%.2fK", float64(n)/1_000)
	}
	return fmt.Sprintf("%d", n)
}
