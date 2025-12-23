# Event Data Size Tracking Design

**Date:** 2025-12-22
**Feature:** Dashboard disk space monitoring for event data

## Overview

Track and visualize the growth of event data storage over time, helping monitor data accumulation and plan capacity.

## Scope

Track **event table size only** (not entire database, not stats tables, not logs).

## User Flow

1. **Dashboard** (`/stats/dashboard`): New card "Event Data Size" on top row
   - Shows current event table size (formatted: GB/MB)
   - Shows 30-day growth percentage (e.g., "+15% this month")
   - Card is clickable

2. **Storage Detail Page** (`/stats/storage`): Click card to navigate
   - 30-day line chart showing event table size trend
   - Data table with daily breakdown (date, size, event count, bytes/event)
   - Password-protected (same auth as dashboard)

## Database Schema

New table for daily snapshots:

```sql
CREATE TABLE daily_storage_stats (
    date TEXT PRIMARY KEY,           -- YYYY-MM-DD format
    event_table_bytes INTEGER,       -- Size of event table in bytes
    event_count INTEGER,             -- Number of events (for context)
    recorded_at TIMESTAMP            -- When snapshot was taken
);
```

**Why this structure:**
- One snapshot per day (date as primary key)
- Store both size and event count to calculate "bytes per event" metric
- Works across all backends (LMDB, SQLite3, PostgreSQL)
- Minimal storage overhead (~20-30 bytes/day = ~10KB/year)

## Data Collection

### Backend-Specific Size Queries

**PostgreSQL:**
```sql
SELECT pg_total_relation_size('event')
```

**SQLite3:**
```sql
-- Get page_count from table info
-- Get page_size from PRAGMA
-- Calculate: page_count * page_size
```

**LMDB:**
```go
// Get file size from filesystem
stat, err := os.Stat(lmdbPath)
size := stat.Size()
```

### Snapshot Frequency

- Daily snapshots (taken once per day)
- Integrate with existing stats tracking mechanism
- Store last 30 days of history

## Implementation Components

### New Files

1. **`stats/storage.go`** - Storage page handler
   - `NewStorageHandler(store)` - Constructor
   - `HandleStorage()` - HTTP handler
   - `GetStorageStats(ctx, days)` - Fetch daily snapshots
   - `GetCurrentEventSize(ctx)` - Get current event table size

2. **`stats/storage.html`** - Storage page template
   - Chart.js line chart (30-day trend)
   - Data table (daily breakdown)
   - Same styling as dashboard

3. **Migration file** - Create `daily_storage_stats` table
   - SQL migration for table creation
   - Add to migration system

### Modified Files

1. **`stats/dashboard.go`**
   - Add method to fetch current storage stats
   - Pass storage data to template

2. **`stats/dashboard.html`**
   - Add new card in top row
   - Make card clickable (link to `/stats/storage`)
   - Show current size + growth percentage

3. **`stats/tracker.go`**
   - Add daily snapshot function
   - Call snapshot function in daily stats recording
   - Handle all three backend types

4. **`main.go`**
   - Register `/stats/storage` route
   - Apply `requireStatsAuth` middleware
   - Initialize storage handler

## Data Flow

```
Daily (via stats tracker):
  1. Calculate event table size (backend-specific)
  2. Get event count
  3. Insert into daily_storage_stats

Dashboard Load:
  1. Fetch current event size
  2. Fetch 30-day growth (first vs last snapshot)
  3. Render card with size + growth %

Storage Page Load:
  1. Fetch last 30 days from daily_storage_stats
  2. Calculate bytes/event for each day
  3. Render chart + table
```

## Display Formatting

- **Sizes**: Auto-format (bytes → KB → MB → GB)
  - < 1MB: show in KB
  - < 1GB: show in MB
  - ≥ 1GB: show in GB
- **Growth**: Percentage with +/- indicator
- **Chart**: Y-axis auto-scales based on data range

## Error Handling

- If no historical data exists: show current size only, message "Collecting data..."
- If size query fails: log error, show "N/A" on dashboard
- Backend-specific errors: handle gracefully with fallbacks

## Future Enhancements (not in scope)

- Growth rate projection
- Alerts/thresholds
- Comparison with other relays
- Detailed table-level breakdown
