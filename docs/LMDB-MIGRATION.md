# LMDB Migration (Legacy Postgres Export)

## Overview

Purplepag.es now uses LMDB for events and a local SQLite analytics database (`analytics.sqlite`).
If you are migrating from an older PostgreSQL eventstore, export events to JSONL and import them into LMDB.

## Migration Steps

### 1. Export Events from Old PostgreSQL (optional)

```bash
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

### 2. Update Config

```json
{
  "storage": {
    "path": "./data/events.lmdb"
  }
}
```

### 3. Import Events to LMDB

```bash
./purplepages --import events-export.jsonl
```

### 4. Start Relay and Analytics Worker

```bash
# Terminal 1: Relay
./purplepages

# Terminal 2: Analytics worker
./purplepages analytics
```

## Notes

- The analytics DB (`analytics.sqlite`) is created alongside the LMDB directory.
- LMDB pre-allocates to its configured MapSize (16GB by default) — this is normal.
- PostgreSQL is no longer used after migration; keep it only as a backup if desired.
