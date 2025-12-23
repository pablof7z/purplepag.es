# Event Storage Tracking Implementation Plan

> **For Claude:** REQUIRED SUB-SKILL: Use superpowers:executing-plans to implement this plan task-by-task.

**Goal:** Add event data size tracking to dashboard with drill-down detail page showing 30-day storage growth trends.

**Architecture:** Track daily snapshots of event table size in new `daily_storage_stats` table. Dashboard shows current size + growth as clickable card linking to detail page with Chart.js visualization. Backend-specific queries for PostgreSQL, SQLite3, and LMDB.

**Tech Stack:** Go, SQLite3/PostgreSQL/LMDB, Chart.js, HTML templates

---

## Task 1: Create Storage Stats Schema

**Files:**
- Modify: `storage/daily_stats.go:50` (add after InitDailyStatsSchema)

**Step 1: Add schema initialization method**

Add this method after the `InitDailyStatsSchema()` function in `storage/daily_stats.go`:

```go
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
	CREATE INDEX IF NOT EXISTS idx_storage_stats_date ON daily_storage_stats(date);
	`

	_, err := dbConn.Exec(schema)
	return err
}
```

**Step 2: Register schema initialization in main.go**

In `main.go`, add after line 95 (after InitDailyStatsSchema):

```go
	if err := store.InitStorageStatsSchema(); err != nil {
		log.Fatalf("Failed to initialize storage stats schema: %v", err)
	}
```

**Step 3: Test schema creation**

Run: `go build && ./purplepages`
Expected: Application starts without errors, new table created

**Step 4: Commit**

```bash
git add storage/daily_stats.go main.go
git commit -m "Add daily_storage_stats table schema

Creates table to track daily snapshots of event table size.

Generated with Claude Code"
```

---

## Task 2: Implement Event Size Calculation

**Files:**
- Modify: `storage/daily_stats.go:85` (add after RecordDailyStats)

**Step 1: Add helper method to check if LMDB backend**

Add this method after the `isPostgres()` helper in `storage/relay_discovery.go:39`:

```go
func (s *Storage) isLMDB() bool {
	_, ok := s.db.(*lmdb.LMDBBackend)
	return ok
}

func (s *Storage) getLMDBPath() string {
	if db, ok := s.db.(*lmdb.LMDBBackend); ok {
		return db.Path
	}
	return ""
}
```

**Step 2: Add method to get current event table size**

Add this method in `storage/daily_stats.go` after `RecordDailyStats`:

```go
func (s *Storage) GetCurrentEventSize(ctx context.Context) (int64, int64, error) {
	// For LMDB, get file size from disk
	if s.isLMDB() {
		path := s.getLMDBPath()
		if path == "" {
			return 0, 0, fmt.Errorf("LMDB path not available")
		}

		// LMDB creates data.mdb file
		dataPath := filepath.Join(path, "data.mdb")
		stat, err := os.Stat(dataPath)
		if err != nil {
			return 0, 0, fmt.Errorf("failed to stat LMDB file: %w", err)
		}

		// Get event count
		count, err := s.CountEvents(ctx)
		if err != nil {
			return stat.Size(), 0, nil
		}

		return stat.Size(), count, nil
	}

	dbConn := s.getDBConn()
	if dbConn == nil {
		return 0, 0, fmt.Errorf("database connection not available")
	}

	var tableSize int64
	var eventCount int64

	// Get event count first (works for both backends)
	err := dbConn.QueryRowContext(ctx, "SELECT COUNT(*) FROM event").Scan(&eventCount)
	if err != nil {
		return 0, 0, fmt.Errorf("failed to count events: %w", err)
	}

	// PostgreSQL: Use pg_total_relation_size
	if s.isPostgres() {
		err = dbConn.QueryRowContext(ctx, "SELECT pg_total_relation_size('event')").Scan(&tableSize)
		if err != nil {
			return 0, eventCount, fmt.Errorf("failed to get PostgreSQL table size: %w", err)
		}
		return tableSize, eventCount, nil
	}

	// SQLite: Calculate from page_count * page_size
	var pageCount, pageSize int64
	err = dbConn.QueryRowContext(ctx, "SELECT page_count FROM pragma_page_count()").Scan(&pageCount)
	if err != nil {
		return 0, eventCount, fmt.Errorf("failed to get page count: %w", err)
	}

	err = dbConn.QueryRowContext(ctx, "SELECT page_size FROM pragma_page_size()").Scan(&pageSize)
	if err != nil {
		return 0, eventCount, fmt.Errorf("failed to get page size: %w", err)
	}

	// For SQLite, we get the whole DB size, not just event table
	// This is acceptable as event table is the largest table
	tableSize = pageCount * pageSize

	return tableSize, eventCount, nil
}
```

**Step 3: Add imports to storage/daily_stats.go**

Add to imports at top of file:

```go
import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"time"
)
```

**Step 4: Add imports to storage/relay_discovery.go**

Add to imports (if not already present):

```go
	"github.com/fiatjaf/eventstore/lmdb"
```

**Step 5: Test size calculation**

Run: `go build`
Expected: Compiles successfully without errors

**Step 6: Commit**

```bash
git add storage/daily_stats.go storage/relay_discovery.go
git commit -m "Add event table size calculation

Implements backend-specific size queries for PostgreSQL, SQLite3, and LMDB.

Generated with Claude Code"
```

---

## Task 3: Implement Daily Storage Snapshot

**Files:**
- Modify: `storage/daily_stats.go:52` (modify RecordDailyStats function)

**Step 1: Add storage snapshot to RecordDailyStats**

Replace the existing `RecordDailyStats` function in `storage/daily_stats.go` to add storage snapshot at the end:

```go
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
	if err != nil {
		return err
	}

	// Record storage snapshot once per day
	s.recordStorageSnapshot(ctx, date)

	return nil
}

func (s *Storage) recordStorageSnapshot(ctx context.Context, date string) {
	dbConn := s.getDBConn()
	if dbConn == nil {
		return
	}

	// Check if we already have a snapshot for today
	var exists int
	err := dbConn.QueryRowContext(ctx, s.rebind("SELECT 1 FROM daily_storage_stats WHERE date = ?"), date).Scan(&exists)
	if err == nil {
		// Already have snapshot for today
		return
	}

	// Get current event size
	tableSize, eventCount, err := s.GetCurrentEventSize(ctx)
	if err != nil {
		log.Printf("Failed to get event size for storage snapshot: %v", err)
		return
	}

	// Insert snapshot
	_, err = dbConn.ExecContext(ctx, s.rebind(`
		INSERT INTO daily_storage_stats (date, event_table_bytes, event_count, recorded_at)
		VALUES (?, ?, ?, ?)
		ON CONFLICT(date) DO NOTHING
	`), date, tableSize, eventCount, time.Now())
	if err != nil {
		log.Printf("Failed to record storage snapshot: %v", err)
	}
}
```

**Step 2: Add log import if not present**

Ensure `"log"` is in imports at top of `storage/daily_stats.go`.

**Step 3: Test snapshot recording**

Run: `go build`
Expected: Compiles successfully

**Step 4: Commit**

```bash
git add storage/daily_stats.go
git commit -m "Add daily storage snapshot recording

Records event table size once per day during stats tracking.

Generated with Claude Code"
```

---

## Task 4: Add Storage Stats Query Methods

**Files:**
- Modify: `storage/daily_stats.go` (add after GetTodayStats)

**Step 1: Add method to get storage stats**

Add these methods at the end of `storage/daily_stats.go`:

```go
type StorageStats struct {
	Date            string
	EventTableBytes int64
	EventCount      int64
	RecordedAt      time.Time
}

func (s *Storage) GetStorageStats(ctx context.Context, days int) ([]StorageStats, error) {
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

	var stats []StorageStats
	for rows.Next() {
		var stat StorageStats
		if err := rows.Scan(&stat.Date, &stat.EventTableBytes, &stat.EventCount, &stat.RecordedAt); err != nil {
			return nil, err
		}
		stats = append(stats, stat)
	}

	return stats, rows.Err()
}

func (s *Storage) GetStorageGrowth(ctx context.Context, days int) (float64, error) {
	stats, err := s.GetStorageStats(ctx, days)
	if err != nil || len(stats) < 2 {
		return 0, err
	}

	first := stats[0].EventTableBytes
	last := stats[len(stats)-1].EventTableBytes

	if first == 0 {
		return 0, nil
	}

	growth := float64(last-first) / float64(first) * 100
	return growth, nil
}
```

**Step 2: Test query methods**

Run: `go build`
Expected: Compiles successfully

**Step 3: Commit**

```bash
git add storage/daily_stats.go
git commit -m "Add storage stats query methods

Methods to fetch daily storage snapshots and calculate growth percentage.

Generated with Claude Code"
```

---

## Task 5: Create Storage Detail Page Handler

**Files:**
- Create: `stats/storage.go`

**Step 1: Create storage handler file**

Create new file `stats/storage.go`:

```go
package stats

import (
	"context"
	"encoding/json"
	"fmt"
	"html/template"
	"net/http"

	"github.com/pablof7z/purplepag.es/storage"
)

var storageTemplate = `<!DOCTYPE html>
<html lang="en">
<head>
    <meta charset="UTF-8">
    <meta name="viewport" content="width=device-width, initial-scale=1.0">
    <title>purplepag.es - Event Storage</title>
    <script src="https://cdn.jsdelivr.net/npm/chart.js"></script>
    <style>
        * { margin: 0; padding: 0; box-sizing: border-box; }
        body {
            font-family: 'SF Mono', 'Monaco', 'Inconsolata', 'Fira Code', monospace;
            background: #0d1117;
            min-height: 100vh;
            padding: 2rem;
            color: #c9d1d9;
        }
        .container { max-width: 1400px; margin: 0 auto; }
        header { margin-bottom: 2rem; border-bottom: 1px solid #21262d; padding-bottom: 1rem; }
        h1 { font-size: 1.5rem; font-weight: 600; color: #f0f6fc; margin-bottom: 0.25rem; }
        .subtitle { font-size: 0.875rem; color: #8b949e; }
        .back-link {
            display: inline-block;
            margin-bottom: 1rem;
            color: #58a6ff;
            text-decoration: none;
            font-size: 0.875rem;
        }
        .back-link:hover { text-decoration: underline; }
        .stats-grid {
            display: grid;
            grid-template-columns: repeat(auto-fit, minmax(250px, 1fr));
            gap: 1rem;
            margin-bottom: 2rem;
        }
        .stat-card {
            background: #161b22;
            border: 1px solid #21262d;
            border-radius: 6px;
            padding: 1rem;
        }
        .stat-label {
            font-size: 0.75rem;
            color: #8b949e;
            text-transform: uppercase;
            letter-spacing: 0.05em;
            margin-bottom: 0.5rem;
        }
        .stat-value {
            font-size: 2rem;
            font-weight: 600;
            color: #f0f6fc;
            font-variant-numeric: tabular-nums;
        }
        .stat-subtext {
            font-size: 0.75rem;
            color: #8b949e;
            margin-top: 0.25rem;
        }
        .growth-positive { color: #3fb950; }
        .growth-negative { color: #f85149; }
        .chart-section {
            background: #161b22;
            border: 1px solid #21262d;
            border-radius: 6px;
            padding: 1.5rem;
            margin-bottom: 1rem;
        }
        .chart-section h2 {
            font-size: 0.875rem;
            font-weight: 600;
            margin-bottom: 1rem;
            color: #f0f6fc;
        }
        .chart-container { position: relative; height: 300px; }
        .section {
            background: #161b22;
            border: 1px solid #21262d;
            border-radius: 6px;
            padding: 1rem;
            margin-bottom: 1rem;
        }
        .section h2 { font-size: 0.875rem; font-weight: 600; margin-bottom: 1rem; color: #f0f6fc; }
        .data-table { width: 100%; border-collapse: collapse; }
        .data-table th, .data-table td { padding: 0.5rem; text-align: left; border-bottom: 1px solid #21262d; }
        .data-table th { color: #8b949e; font-weight: 600; font-size: 0.625rem; text-transform: uppercase; }
        .data-table td { font-size: 0.75rem; }
        .data-table .num { font-variant-numeric: tabular-nums; color: #58a6ff; font-weight: 600; }
        .empty-state {
            text-align: center;
            padding: 3rem;
            color: #8b949e;
        }
        @media (max-width: 768px) {
            body { padding: 1rem; }
            .stat-value { font-size: 1.5rem; }
        }
    </style>
</head>
<body>
    <div class="container">
        <a href="/stats/dashboard" class="back-link">← Back to Dashboard</a>

        <header>
            <h1>Event Storage</h1>
            <div class="subtitle">30-Day Event Data Size Tracking</div>
        </header>

        <div class="stats-grid">
            <div class="stat-card">
                <div class="stat-label">Current Size</div>
                <div class="stat-value">{{.CurrentSizeFormatted}}</div>
            </div>
            <div class="stat-card">
                <div class="stat-label">30-Day Growth</div>
                <div class="stat-value {{if gt .GrowthPercent 0.0}}growth-positive{{else if lt .GrowthPercent 0.0}}growth-negative{{end}}">
                    {{if gt .GrowthPercent 0.0}}+{{end}}{{printf "%.1f" .GrowthPercent}}%
                </div>
            </div>
            <div class="stat-card">
                <div class="stat-label">Total Events</div>
                <div class="stat-value">{{.CurrentEventCount}}</div>
            </div>
        </div>

        {{if .HasData}}
        <div class="chart-section">
            <h2>Event Data Size (30 Days)</h2>
            <div class="chart-container">
                <canvas id="storageChart"></canvas>
            </div>
        </div>

        <div class="section">
            <h2>Daily Breakdown</h2>
            <table class="data-table">
                <thead>
                    <tr>
                        <th>Date</th>
                        <th>Size</th>
                        <th>Events</th>
                        <th>Bytes/Event</th>
                    </tr>
                </thead>
                <tbody>
                    {{range .StorageStats}}
                    <tr>
                        <td>{{.Date}}</td>
                        <td class="num">{{.SizeFormatted}}</td>
                        <td class="num">{{.EventCount}}</td>
                        <td class="num">{{.BytesPerEvent}}</td>
                    </tr>
                    {{end}}
                </tbody>
            </table>
        </div>
        {{else}}
        <div class="empty-state">
            <p>Collecting data... Check back tomorrow for storage trends.</p>
        </div>
        {{end}}
    </div>

    {{if .HasData}}
    <script>
        const data = {{.StorageDataJSON}};

        const ctx = document.getElementById('storageChart').getContext('2d');
        new Chart(ctx, {
            type: 'line',
            data: {
                labels: data.labels,
                datasets: [{
                    label: 'Event Data Size',
                    data: data.sizes,
                    borderColor: '#a371f7',
                    backgroundColor: 'rgba(163, 113, 247, 0.1)',
                    fill: true,
                    tension: 0.3
                }]
            },
            options: {
                responsive: true,
                maintainAspectRatio: false,
                plugins: {
                    legend: { display: false }
                },
                scales: {
                    x: {
                        grid: { color: '#21262d' },
                        ticks: { color: '#8b949e', maxRotation: 45, minRotation: 45, font: { family: 'monospace', size: 10 } }
                    },
                    y: {
                        grid: { color: '#21262d' },
                        ticks: {
                            color: '#8b949e',
                            font: { family: 'monospace', size: 10 },
                            callback: function(value) {
                                if (value >= 1073741824) return (value / 1073741824).toFixed(1) + ' GB';
                                if (value >= 1048576) return (value / 1048576).toFixed(1) + ' MB';
                                if (value >= 1024) return (value / 1024).toFixed(1) + ' KB';
                                return value + ' B';
                            }
                        },
                        beginAtZero: true
                    }
                }
            }
        });
    </script>
    {{end}}
</body>
</html>`

type StorageHandler struct {
	storage *storage.Storage
}

func NewStorageHandler(storage *storage.Storage) *StorageHandler {
	return &StorageHandler{storage: storage}
}

type StorageStatDisplay struct {
	Date          string
	SizeFormatted string
	EventCount    int64
	BytesPerEvent int64
}

type StorageData struct {
	CurrentSizeFormatted string
	CurrentEventCount    int64
	GrowthPercent        float64
	HasData              bool
	StorageStats         []StorageStatDisplay
	StorageDataJSON      template.JS
}

func formatBytes(bytes int64) string {
	const unit = 1024
	if bytes < unit {
		return fmt.Sprintf("%d B", bytes)
	}
	div, exp := int64(unit), 0
	for n := bytes / unit; n >= unit; n /= unit {
		div *= unit
		exp++
	}
	return fmt.Sprintf("%.1f %cB", float64(bytes)/float64(div), "KMGT"[exp])
}

func (h *StorageHandler) HandleStorage() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		ctx := context.Background()

		// Get current size
		currentSize, currentCount, err := h.storage.GetCurrentEventSize(ctx)
		if err != nil {
			currentSize = 0
			currentCount = 0
		}

		// Get 30-day history
		stats, err := h.storage.GetStorageStats(ctx, 30)
		hasData := err == nil && len(stats) > 0

		// Calculate growth
		growth, _ := h.storage.GetStorageGrowth(ctx, 30)

		// Format for display
		statDisplays := make([]StorageStatDisplay, len(stats))
		labels := make([]string, len(stats))
		sizes := make([]int64, len(stats))

		for i, stat := range stats {
			bytesPerEvent := int64(0)
			if stat.EventCount > 0 {
				bytesPerEvent = stat.EventTableBytes / stat.EventCount
			}

			statDisplays[i] = StorageStatDisplay{
				Date:          stat.Date,
				SizeFormatted: formatBytes(stat.EventTableBytes),
				EventCount:    stat.EventCount,
				BytesPerEvent: bytesPerEvent,
			}

			labels[i] = stat.Date
			sizes[i] = stat.EventTableBytes
		}

		chartData := map[string]interface{}{
			"labels": labels,
			"sizes":  sizes,
		}
		chartJSON, _ := json.Marshal(chartData)

		data := StorageData{
			CurrentSizeFormatted: formatBytes(currentSize),
			CurrentEventCount:    currentCount,
			GrowthPercent:        growth,
			HasData:              hasData,
			StorageStats:         statDisplays,
			StorageDataJSON:      template.JS(chartJSON),
		}

		tmpl, err := template.New("storage").Parse(storageTemplate)
		if err != nil {
			http.Error(w, "Internal server error", http.StatusInternalServerError)
			return
		}

		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		if err := tmpl.Execute(w, data); err != nil {
			http.Error(w, "Internal server error", http.StatusInternalServerError)
		}
	}
}
```

**Step 2: Test storage handler**

Run: `go build`
Expected: Compiles successfully

**Step 3: Commit**

```bash
git add stats/storage.go
git commit -m "Add storage detail page handler

Handler and template for /stats/storage page showing 30-day event size trends.

Generated with Claude Code"
```

---

## Task 6: Add Storage Card to Dashboard

**Files:**
- Modify: `stats/dashboard.go:362` (DashboardData struct)
- Modify: `stats/dashboard.go:373` (HandleDashboard function)

**Step 1: Update DashboardData struct**

In `stats/dashboard.go`, modify the `DashboardData` struct (around line 362) to add storage fields:

```go
type DashboardData struct {
	TodayREQs             int64
	TodayUniqueIPs        int64
	TodayEventsServed     int64
	StorageSizeFormatted  string
	StorageGrowthPercent  float64
	DailyStatsJSON        template.JS
	HourlyStatsJSON       template.JS
	TopIPs                []TopIPDisplay
	UsersTracked          int64
	ArchivedVersions      int64
}
```

**Step 2: Fetch storage data in HandleDashboard**

In `stats/dashboard.go`, add storage data fetching in the `HandleDashboard` function (around line 403, before creating topIPDisplays):

```go
		// Fetch storage stats
		currentSize, _, err := h.storage.GetCurrentEventSize(ctx)
		storageSizeFormatted := "N/A"
		if err == nil {
			storageSizeFormatted = formatStorageSize(currentSize)
		}

		storageGrowth, _ := h.storage.GetStorageGrowth(ctx, 30)
```

**Step 3: Add formatStorageSize helper**

Add this helper function at the end of `stats/dashboard.go`:

```go
func formatStorageSize(bytes int64) string {
	const unit = 1024
	if bytes < unit {
		return fmt.Sprintf("%d B", bytes)
	}
	div, exp := int64(unit), 0
	for n := bytes / unit; n >= unit; n /= unit {
		div *= unit
		exp++
	}
	return fmt.Sprintf("%.1f %cB", float64(bytes)/float64(div), "KMGT"[exp])
}
```

**Step 4: Add fmt import**

Add to imports at top of `stats/dashboard.go` if not present:

```go
	"fmt"
```

**Step 5: Update data struct initialization**

In `HandleDashboard`, update the `DashboardData` initialization (around line 425) to include storage fields:

```go
		data := DashboardData{
			TodayREQs:            todayStats.TotalREQs,
			TodayUniqueIPs:       todayStats.UniqueIPs,
			TodayEventsServed:    todayStats.EventsServed,
			StorageSizeFormatted: storageSizeFormatted,
			StorageGrowthPercent: storageGrowth,
			DailyStatsJSON:       template.JS(dailyStatsJSON),
			HourlyStatsJSON:      template.JS(hourlyStatsJSON),
			TopIPs:               topIPDisplays,
			UsersTracked:         usersTracked,
			ArchivedVersions:     archivedVersions,
		}
```

**Step 6: Add storage card to template**

In `stats/dashboard.go`, modify the `dashboardTemplate` variable. Find the stats-grid div (around line 138) and add a new card:

```html
        <div class="stats-grid">
            <div class="stat-card">
                <div class="stat-label">Today's REQs</div>
                <div class="stat-value">{{.TodayREQs}}</div>
            </div>
            <div class="stat-card">
                <div class="stat-label">Today's Unique IPs</div>
                <div class="stat-value">{{.TodayUniqueIPs}}</div>
            </div>
            <div class="stat-card">
                <div class="stat-label">Today's Events Served</div>
                <div class="stat-value">{{.TodayEventsServed}}</div>
            </div>
            <a href="/stats/storage" style="text-decoration: none; color: inherit;">
                <div class="stat-card" style="cursor: pointer; transition: border-color 0.2s;" onmouseover="this.style.borderColor='#58a6ff'" onmouseout="this.style.borderColor='#21262d'">
                    <div class="stat-label">Event Data Size</div>
                    <div class="stat-value">{{.StorageSizeFormatted}}</div>
                    <div style="font-size: 0.75rem; color: {{if gt .StorageGrowthPercent 0.0}}#3fb950{{else if lt .StorageGrowthPercent 0.0}}#f85149{{else}}#8b949e{{end}}; margin-top: 0.25rem;">
                        {{if gt .StorageGrowthPercent 0.0}}+{{end}}{{printf "%.1f" .StorageGrowthPercent}}% (30d)
                    </div>
                </div>
            </a>
        </div>
```

**Step 7: Test dashboard card**

Run: `go build`
Expected: Compiles successfully

**Step 8: Commit**

```bash
git add stats/dashboard.go
git commit -m "Add storage card to dashboard

Clickable card showing current event data size and 30-day growth.

Generated with Claude Code"
```

---

## Task 7: Register Storage Route

**Files:**
- Modify: `main.go` (around line 367 and 398)

**Step 1: Initialize storage handler**

In `main.go`, add storage handler initialization (around line 367, after dashboardHandler):

```go
	dashboardHandler := stats.NewDashboardHandler(store)
	storageHandler := stats.NewStorageHandler(store)
```

**Step 2: Register route**

In `main.go`, add route registration (around line 398, after dashboard route):

```go
	mux.HandleFunc("/stats/dashboard", requireStatsAuth(dashboardHandler.HandleDashboard()))
	mux.HandleFunc("/stats/storage", requireStatsAuth(storageHandler.HandleStorage()))
```

**Step 3: Test route registration**

Run: `go build`
Expected: Compiles successfully

**Step 4: Commit**

```bash
git add main.go
git commit -m "Register storage detail page route

Add /stats/storage route with auth middleware.

Generated with Claude Code"
```

---

## Task 8: Add CountEvents Helper (if missing)

**Files:**
- Modify: `storage/daily_stats.go` (add method if not exists)

**Step 1: Check if CountEvents exists**

Run: `grep -n "CountEvents" storage/*.go`

**Step 2: If not found, add CountEvents method**

Add to `storage/daily_stats.go`:

```go
func (s *Storage) CountEvents(ctx context.Context) (int64, error) {
	dbConn := s.getDBConn()
	if dbConn == nil {
		// For LMDB, we need to query through the eventstore interface
		// This is a rough estimate
		return 0, fmt.Errorf("count not available for LMDB without SQL")
	}

	var count int64
	err := dbConn.QueryRowContext(ctx, "SELECT COUNT(*) FROM event").Scan(&count)
	return count, err
}
```

**Step 3: Test build**

Run: `go build`
Expected: Compiles successfully

**Step 4: Commit (only if added)**

```bash
git add storage/daily_stats.go
git commit -m "Add CountEvents helper method

Helper to count total events in database.

Generated with Claude Code"
```

---

## Task 9: Final Integration Test

**Step 1: Build application**

Run: `go build`
Expected: No compilation errors

**Step 2: Run application**

Run: `./purplepages`
Expected: Application starts, all schemas initialize successfully

**Step 3: Visit dashboard**

Open browser to: `http://localhost:PORT/stats/dashboard`
Expected: New "Event Data Size" card visible in top row

**Step 4: Click storage card**

Click on storage card
Expected: Navigate to `/stats/storage` page

**Step 5: Verify storage page**

Check storage page shows:
- Current size card
- 30-day growth card
- Total events card
- Empty state message (if no data yet)

**Step 6: Wait for first snapshot**

Wait 24 hours or trigger manually by making requests
Expected: Storage stats populate with first data point

**Step 7: Commit if any fixes needed**

```bash
git add .
git commit -m "Final integration fixes for storage tracking

Generated with Claude Code"
```

---

## Verification Checklist

- [ ] `daily_storage_stats` table created successfully
- [ ] Schema initialization registered in main.go
- [ ] Event size calculation works for all backends (PostgreSQL, SQLite3, LMDB)
- [ ] Daily snapshot recorded during stats tracking
- [ ] Storage stats query methods return correct data
- [ ] Storage detail page renders correctly
- [ ] Dashboard card displays current size and growth
- [ ] Card click navigates to storage detail page
- [ ] Route registered with auth middleware
- [ ] Application compiles without errors
- [ ] Application runs without crashes
- [ ] Empty state shows when no data available
- [ ] Chart displays after data accumulates

---

## Notes

- LMDB size is entire database file, not just event table (acceptable approximation)
- SQLite size is entire database, not just event table (acceptable approximation)
- PostgreSQL size is event table specifically (most accurate)
- Snapshots taken once per day during first request of the day
- Growth percentage based on first vs last snapshot in 30-day window
- Chart auto-scales Y-axis with formatted byte labels (B/KB/MB/GB)
