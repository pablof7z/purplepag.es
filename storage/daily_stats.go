package storage

import (
	"context"
	"time"
)

type DailyStats struct {
	Date         string
	TotalREQs    int64
	UniqueIPs    int64
	EventsServed int64
}

type HourlyStats struct {
	Hour         string // Format: "2006-01-02 15"
	TotalREQs    int64
	UniqueIPs    int64
	EventsServed int64
}

func (s *Storage) InitDailyStatsSchema() error {
	dbConn := s.getDBConn()
	if dbConn == nil {
		return nil
	}

	schema := `
	CREATE TABLE IF NOT EXISTS daily_requests (
		date TEXT NOT NULL,
		ip TEXT NOT NULL,
		request_count INTEGER NOT NULL DEFAULT 0,
		events_served INTEGER NOT NULL DEFAULT 0,
		PRIMARY KEY (date, ip)
	);
	CREATE INDEX IF NOT EXISTS idx_daily_requests_date ON daily_requests(date);

	CREATE TABLE IF NOT EXISTS hourly_requests (
		hour TEXT NOT NULL,
		ip TEXT NOT NULL,
		request_count INTEGER NOT NULL DEFAULT 0,
		events_served INTEGER NOT NULL DEFAULT 0,
		PRIMARY KEY (hour, ip)
	);
	CREATE INDEX IF NOT EXISTS idx_hourly_requests_hour ON hourly_requests(hour);
	`

	_, err := dbConn.Exec(schema)
	return err
}

func (s *Storage) RecordDailyStats(ctx context.Context, ip string, eventsServed int64) error {
	dbConn := s.getDBConn()
	if dbConn == nil {
		return nil
	}

	now := time.Now()
	date := now.Format("2006-01-02")
	hour := now.Format("2006-01-02 15")

	// Record daily stats
	_, err := dbConn.ExecContext(ctx, s.rebind(`
		INSERT INTO daily_requests (date, ip, request_count, events_served)
		VALUES (?, ?, 1, ?)
		ON CONFLICT(date, ip) DO UPDATE SET
			request_count = daily_requests.request_count + 1,
			events_served = daily_requests.events_served + excluded.events_served
	`), date, ip, eventsServed)
	if err != nil {
		return err
	}

	// Record hourly stats
	_, err = dbConn.ExecContext(ctx, s.rebind(`
		INSERT INTO hourly_requests (hour, ip, request_count, events_served)
		VALUES (?, ?, 1, ?)
		ON CONFLICT(hour, ip) DO UPDATE SET
			request_count = hourly_requests.request_count + 1,
			events_served = hourly_requests.events_served + excluded.events_served
	`), hour, ip, eventsServed)

	return err
}

func (s *Storage) GetDailyStats(ctx context.Context, days int) ([]DailyStats, error) {
	dbConn := s.getDBConn()
	if dbConn == nil {
		return nil, nil
	}

	// Calculate the cutoff date in Go (works for both SQLite and PostgreSQL)
	cutoffDate := time.Now().AddDate(0, 0, -days).Format("2006-01-02")

	rows, err := dbConn.QueryContext(ctx, s.rebind(`
		SELECT
			date,
			SUM(request_count) as total_reqs,
			COUNT(DISTINCT ip) as unique_ips,
			SUM(events_served) as events_served
		FROM daily_requests
		WHERE date >= ?
		GROUP BY date
		ORDER BY date ASC
	`), cutoffDate)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var results []DailyStats
	for rows.Next() {
		var stat DailyStats
		if err := rows.Scan(&stat.Date, &stat.TotalREQs, &stat.UniqueIPs, &stat.EventsServed); err != nil {
			return nil, err
		}
		results = append(results, stat)
	}

	return results, rows.Err()
}

func (s *Storage) GetHourlyStats(ctx context.Context, hours int) ([]HourlyStats, error) {
	dbConn := s.getDBConn()
	if dbConn == nil {
		return nil, nil
	}

	// Calculate the cutoff hour in Go (works for both SQLite and PostgreSQL)
	cutoffHour := time.Now().Add(-time.Duration(hours) * time.Hour).Format("2006-01-02 15")

	rows, err := dbConn.QueryContext(ctx, s.rebind(`
		SELECT
			hour,
			SUM(request_count) as total_reqs,
			COUNT(DISTINCT ip) as unique_ips,
			SUM(events_served) as events_served
		FROM hourly_requests
		WHERE hour >= ?
		GROUP BY hour
		ORDER BY hour ASC
	`), cutoffHour)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var results []HourlyStats
	for rows.Next() {
		var stat HourlyStats
		if err := rows.Scan(&stat.Hour, &stat.TotalREQs, &stat.UniqueIPs, &stat.EventsServed); err != nil {
			return nil, err
		}
		results = append(results, stat)
	}

	return results, rows.Err()
}

type TopIP struct {
	IP           string
	TotalREQs    int64
	EventsServed int64
}

func (s *Storage) GetTopIPs(ctx context.Context, limit int) ([]TopIP, error) {
	dbConn := s.getDBConn()
	if dbConn == nil {
		return nil, nil
	}

	rows, err := dbConn.QueryContext(ctx, s.rebind(`
		SELECT
			ip,
			SUM(request_count) as total_reqs,
			SUM(events_served) as events_served
		FROM daily_requests
		GROUP BY ip
		ORDER BY events_served DESC
		LIMIT ?
	`), limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var results []TopIP
	for rows.Next() {
		var ip TopIP
		if err := rows.Scan(&ip.IP, &ip.TotalREQs, &ip.EventsServed); err != nil {
			return nil, err
		}
		results = append(results, ip)
	}

	return results, rows.Err()
}

func (s *Storage) GetEventsServedLast24Hours(ctx context.Context, ip string) (int64, error) {
	dbConn := s.getDBConn()
	if dbConn == nil {
		return 0, nil
	}

	// Calculate the cutoff hour in Go (works for both SQLite and PostgreSQL)
	cutoffHour := time.Now().Add(-24 * time.Hour).Format("2006-01-02 15")

	var total int64
	err := dbConn.QueryRowContext(ctx, s.rebind(`
		SELECT COALESCE(SUM(events_served), 0)
		FROM hourly_requests
		WHERE ip = ?
		  AND hour >= ?
	`), ip, cutoffHour).Scan(&total)

	return total, err
}

func (s *Storage) GetTodayStats(ctx context.Context) (*DailyStats, error) {
	dbConn := s.getDBConn()
	if dbConn == nil {
		return nil, nil
	}

	today := time.Now().Format("2006-01-02")
	var stat DailyStats
	stat.Date = today

	err := dbConn.QueryRowContext(ctx, s.rebind(`
		SELECT
			COALESCE(SUM(request_count), 0),
			COALESCE(COUNT(DISTINCT ip), 0),
			COALESCE(SUM(events_served), 0)
		FROM daily_requests
		WHERE date = ?
	`), today).Scan(&stat.TotalREQs, &stat.UniqueIPs, &stat.EventsServed)

	if err != nil {
		return nil, err
	}

	return &stat, nil
}
