# LMDB + PostgreSQL Separation Design

**Date:** 2025-01-23
**Status:** Implemented

## Problem

Production relay not keeping up due to PostgreSQL eventstore backend limitations:
- Missing proper tag indexing
- Complex filter queries slow/impossible
- Fighting library abstractions with workarounds
- At scale (>1M events, >1k concurrent users) performance critical

## Constraints

- Prefer libraries over custom implementations (maintenance burden)
- Need complex filter query support
- Performance is critical
- Already built event_tags workaround showing library limitations

## Solution: LMDB for Events, PostgreSQL for Analytics Only

### Architecture

**LMDB (via eventstore library):**
- All event storage and client queries
- Fast concurrent reads/writes
- Uses existing fiatjaf/eventstore/lmdb backend
- No changes to relay query logic

**PostgreSQL (separate connection):**
- Analytics tables ONLY: daily_stats, req_analytics, bot_clusters, trust_graph
- Heavy graph algorithms run in separate analytics worker process
- No event storage

### Key Design Decisions

1. **Not dual-write for queries** - LMDB handles all event queries, PostgreSQL only for analytics
2. **Separate database connection** - New `analyticsDB` field in Storage struct, not extracted from eventstore
3. **Same binary, different modes** - `./purplepages` = relay, `./purplepages analytics` = worker
4. **Clean separation** - Events and analytics completely separated

### Implementation

**Storage struct:**
```go
type Storage struct {
    db             eventstore.Store  // LMDB for events
    archiveEnabled bool
    analyticsDB    *sqlx.DB          // Separate PostgreSQL
}
```

**Config:**
```json
{
  "storage": {
    "backend": "lmdb",
    "path": "./data/events.lmdb",
    "analytics_db_url": "postgresql://..."
  }
}
```

**Process separation:**
- Relay: handles client REQs, queries LMDB
- Analytics worker: runs graph algorithms against PostgreSQL

### Migration Path

1. Create separate analytics database
2. Export events from PostgreSQL eventstore
3. Update config to use LMDB + analytics_db_url
4. Import events to LMDB
5. Run relay and analytics worker separately

### Expected Performance

**Before:**
- Tag queries: 500ms - 2s
- Write contention during analytics
- Hourly analytics blocking relay

**After:**
- Event queries: <50ms
- No write contention
- Analytics isolated in separate process

## Trade-offs

**Pros:**
- Fast event queries (LMDB optimized for this)
- No more fighting eventstore limitations
- Clean separation of concerns
- Library-based (fiatjaf/eventstore)

**Cons:**
- Two databases to manage (but clean separation)
- Migration required for production
- LMDB file pre-allocates space (16GB)

## Rejected Alternatives

1. **PostgreSQL only, own the schema** - Rejected due to maintenance burden
2. **Dual-write for query optimization** - User corrected: PostgreSQL for analytics only
3. **Different relay implementation** - Would require rewrite in different language
