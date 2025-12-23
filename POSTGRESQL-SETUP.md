# PostgreSQL Configuration Options

## Option 1: Single PostgreSQL Database (Simplest)

**Use when:** You want everything in one database

```json
{
  "storage": {
    "backend": "postgresql",
    "path": "postgres://user:password@localhost/purplepages",
    "analytics_db_url": ""
  }
}
```

**Database schema:**
```sql
-- Events table (created by eventstore)
CREATE TABLE event (
  id text PRIMARY KEY,
  pubkey text,
  created_at integer,
  kind integer,
  tags jsonb,
  content text,
  sig text,
  tagvalues text[] GENERATED ALWAYS AS (tags_to_tagvalues(tags)) STORED
);

-- Analytics tables (created by relay)
CREATE TABLE daily_stats (...);
CREATE TABLE req_analytics (...);
CREATE TABLE trust_graph (...);
-- etc.
```

**Pros:**
- Simplest setup
- Single connection pool
- Cheaper (one database)

**Cons:**
- Heavy analytics can impact event queries
- No isolation between operations


## Option 2: Separate PostgreSQL Databases (Recommended for Production)

**Use when:** You want to isolate analytics from event queries

```json
{
  "storage": {
    "backend": "postgresql",
    "path": "postgres://user:password@localhost/purplepages_events",
    "analytics_db_url": "postgres://user:password@localhost/purplepages_analytics"
  }
}
```

**Database schemas:**

`purplepages_events` database:
```sql
-- Only the event table
CREATE TABLE event (...);
```

`purplepages_analytics` database:
```sql
-- Only analytics tables
CREATE TABLE daily_stats (...);
CREATE TABLE req_analytics (...);
CREATE TABLE trust_graph (...);
CREATE TABLE bot_clusters (...);
CREATE TABLE discovered_relays (...);
```

**Pros:**
- Analytics can't slow down event queries
- Separate connection pools
- Can scale/optimize independently
- Can run analytics worker on different host

**Cons:**
- Two databases to manage
- Slightly more complex setup


## Option 3: Same PostgreSQL Server, Different Databases

**Same as Option 2 but both DBs on same server:**

```json
{
  "storage": {
    "backend": "postgresql",
    "path": "postgres://user:password@localhost:5432/events",
    "analytics_db_url": "postgres://user:password@localhost:5432/analytics"
  }
}
```

This gives you logical separation with operational simplicity.


## Setup Commands

### Single Database Setup
```bash
createdb purplepages
./purplepages  # Tables created automatically
```

### Separate Databases Setup
```bash
createdb purplepages_events
createdb purplepages_analytics
./purplepages  # Both schemas created automatically
```

### Run Analytics Worker (if using separate analytics DB)
```bash
./purplepages analytics
```


## Connection String Format

```
postgres://username:password@host:port/database?options
```

**Examples:**
```
postgres://relay:secret@localhost/purplepages
postgres://relay:secret@db.example.com:5432/events
postgres://relay:secret@localhost/events?sslmode=disable
postgres://relay:secret@localhost/events?sslmode=require
```


## Performance Notes

- PostgreSQL uses JSONB + GIN index for tag queries (very fast)
- No `event_tags` duplication needed
- Native tag indexing via `tagvalues` generated column
- Analytics queries don't touch event table when separated
