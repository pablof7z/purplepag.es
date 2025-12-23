# LMDB + PostgreSQL Architecture Migration

## Overview

Separate event storage from analytics for better performance:
- **LMDB**: Fast event storage and queries (via eventstore library)
- **PostgreSQL**: Analytics data only (stats, graphs, bot detection)

## Architecture

```
┌─────────────────────┐
│  Client REQs        │
└──────┬──────────────┘
       │
       v
┌─────────────────────┐
│  Relay Process      │
│  - Event validation │
│  - LMDB queries     │
└──────┬──────────────┘
       │
       ├──> LMDB (events)
       │
       └──> PostgreSQL (lightweight tracking)

┌─────────────────────┐
│ Analytics Worker    │
│  - Graph algorithms │
│  - Bot detection    │
└──────┬──────────────┘
       │
       └──> PostgreSQL (heavy analytics)
```

## Migration Steps

### 1. Setup PostgreSQL for Analytics

```bash
# Create database
createdb purplepages_analytics

# Initialize schema (run relay once to create tables)
./purplepages
```

### 2. Export Events from Current PostgreSQL

```bash
# Export all events to JSONL
psql purplepages -c "COPY (SELECT json_build_object(
  'id', id,
  'pubkey', pubkey,
  'created_at', created_at,
  'kind', kind,
  'tags', tags::json,
  'content', content,
  'sig', sig
) FROM event) TO STDOUT" > events-export.jsonl
```

### 3. Update Config

```json
{
  "storage": {
    "backend": "lmdb",
    "path": "./data/events.lmdb",
    "analytics_db_url": "postgresql://localhost/purplepages_analytics?sslmode=disable"
  }
}
```

### 4. Import Events to LMDB

```bash
# Import events
./purplepages --import events-export.jsonl
```

### 5. Start Relay and Analytics Worker

```bash
# Terminal 1: Relay
./purplepages

# Terminal 2: Analytics worker
./purplepages analytics
```

## Production Setup

### Systemd Services

**Relay:**
```ini
[Unit]
Description=PurplePages Relay
After=network.target

[Service]
Type=simple
User=purplepages
WorkingDirectory=/opt/purplepages
ExecStart=/opt/purplepages/purplepages
Restart=always

[Install]
WantedBy=multi-user.target
```

**Analytics:**
```ini
[Unit]
Description=PurplePages Analytics Worker
After=network.target postgresql.service
Wants=postgresql.service

[Service]
Type=simple
User=purplepages
WorkingDirectory=/opt/purplepages
ExecStart=/opt/purplepages/purplepages analytics
Restart=always
MemoryMax=2G

[Install]
WantedBy=multi-user.target
```

## Performance Expectations

**Before (PostgreSQL eventstore):**
- Complex tag queries: 500ms - 2s
- Write contention during analytics
- Slow hourly graph algorithms blocking queries

**After (LMDB + PostgreSQL):**
- Event queries: <50ms
- No write contention (LMDB handles concurrent writes)
- Analytics runs in separate process
- Relay stays responsive during heavy analytics

## Rollback Plan

If you need to rollback:

1. Stop relay
2. Change config back to `"backend": "postgresql"`
3. Events are still in PostgreSQL (not deleted during migration)
4. Restart relay

## Notes

- LMDB file grows to MapSize (16GB) immediately - this is normal
- Analytics DB is separate - can be on different server if needed
- Both relay and analytics worker can share same analytics DB
- LMDB is append-only - use backups for safety
