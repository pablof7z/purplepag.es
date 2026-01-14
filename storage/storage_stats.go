package storage

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/fiatjaf/eventstore/lmdb"
	"github.com/nbd-wtf/go-nostr"
)

type DailyStorageStats struct {
	Date             string
	EventTableBytes  int64
	EventCount       int64
	RecordedAt       time.Time
	BytesPerEvent    int64
}

func (s *Storage) InitStorageStatsSchema() error {
	dbConn := s.getDBConn()
	if dbConn == nil {
		return nil
	}

	schema := `
	CREATE TABLE IF NOT EXISTS daily_storage_stats (
		date TEXT PRIMARY KEY,
		event_table_bytes INTEGER NOT NULL,
		event_count INTEGER NOT NULL,
		recorded_at TIMESTAMP NOT NULL
	);
	`

	_, err := dbConn.Exec(schema)
	return err
}

// GetEventTableSize returns the size of the event table in bytes
func (s *Storage) GetEventTableSize(ctx context.Context) (int64, error) {
	if lmdbBackend, ok := s.db.(*lmdb.LMDBBackend); ok {
		dataPath := filepath.Join(lmdbBackend.Path, "data.mdb")
		stat, err := os.Stat(dataPath)
		if err != nil {
			return 0, fmt.Errorf("failed to stat LMDB data file: %w", err)
		}
		return stat.Size(), nil
	}

	return 0, fmt.Errorf("unsupported storage backend for size")
}

// GetTotalEventCount returns the total number of events in the event table
func (s *Storage) GetTotalEventCount(ctx context.Context) (int64, error) {
	if counter, ok := s.db.(interface {
		CountEvents(context.Context, nostr.Filter) (int64, error)
	}); ok {
		return counter.CountEvents(ctx, nostr.Filter{})
	}

	return 0, fmt.Errorf("unsupported storage backend for count")
}

// RecordDailyStorageSnapshot records a daily snapshot of storage stats
func (s *Storage) RecordDailyStorageSnapshot(ctx context.Context) error {
	dbConn := s.getDBConn()
	if dbConn == nil {
		return fmt.Errorf("analytics database not available")
	}

	size, err := s.GetEventTableSize(ctx)
	if err != nil {
		return fmt.Errorf("failed to get event table size: %w", err)
	}

	count, err := s.GetTotalEventCount(ctx)
	if err != nil {
		return fmt.Errorf("failed to get event count: %w", err)
	}

	today := time.Now().Format("2006-01-02")
	now := time.Now()

	_, err = dbConn.ExecContext(ctx, s.rebind(`
		INSERT INTO daily_storage_stats (date, event_table_bytes, event_count, recorded_at)
		VALUES (?, ?, ?, ?)
		ON CONFLICT(date) DO UPDATE SET
			event_table_bytes = excluded.event_table_bytes,
			event_count = excluded.event_count,
			recorded_at = excluded.recorded_at
	`), today, size, count, now)

	if err != nil {
		return err
	}

	// Cleanup old records (older than 30 days)
	cutoffDate := time.Now().AddDate(0, 0, -30).Format("2006-01-02")
	_, err = dbConn.ExecContext(ctx, s.rebind(`
		DELETE FROM daily_storage_stats WHERE date < ?
	`), cutoffDate)

	return err
}

// GetDailyStorageStats returns daily storage stats for the last N days
func (s *Storage) GetDailyStorageStats(ctx context.Context, days int) ([]DailyStorageStats, error) {
	dbConn := s.getDBConn()
	if dbConn == nil {
		return nil, nil
	}

	cutoffDate := time.Now().AddDate(0, 0, -days).Format("2006-01-02")

	rows, err := dbConn.QueryContext(ctx, s.rebind(`
		SELECT date, event_table_bytes, event_count, recorded_at
		FROM daily_storage_stats
		WHERE date >= ?
		ORDER BY date ASC
	`), cutoffDate)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var results []DailyStorageStats
	for rows.Next() {
		var stat DailyStorageStats
		if err := rows.Scan(&stat.Date, &stat.EventTableBytes, &stat.EventCount, &stat.RecordedAt); err != nil {
			return nil, err
		}
		// Calculate bytes per event
		if stat.EventCount > 0 {
			stat.BytesPerEvent = stat.EventTableBytes / stat.EventCount
		}
		results = append(results, stat)
	}

	return results, rows.Err()
}

// GetCurrentStorageInfo returns the current storage size and event count
func (s *Storage) GetCurrentStorageInfo(ctx context.Context) (*DailyStorageStats, error) {
	size, err := s.GetEventTableSize(ctx)
	if err != nil {
		return nil, err
	}

	count, err := s.GetTotalEventCount(ctx)
	if err != nil {
		return nil, err
	}

	stat := &DailyStorageStats{
		Date:            time.Now().Format("2006-01-02"),
		EventTableBytes: size,
		EventCount:      count,
		RecordedAt:      time.Now(),
	}

	if count > 0 {
		stat.BytesPerEvent = size / count
	}

	return stat, nil
}

// GetStorageGrowth returns the growth percentage over the last N days
// Returns 0 if there's insufficient data
func (s *Storage) GetStorageGrowth(ctx context.Context, days int) (float64, error) {
	stats, err := s.GetDailyStorageStats(ctx, days)
	if err != nil {
		return 0, err
	}

	if len(stats) < 2 {
		// Not enough data to calculate growth
		return 0, nil
	}

	firstSize := float64(stats[0].EventTableBytes)
	lastSize := float64(stats[len(stats)-1].EventTableBytes)

	if firstSize == 0 {
		return 0, nil
	}

	growth := ((lastSize - firstSize) / firstSize) * 100
	return growth, nil
}
