# purplepag.es

A specialized Nostr relay built with [khatru](https://khatru.nostr.technology/) that focuses on profile and relay list events.

## Features

- **Event Type Filtering**: Only accepts specific event kinds:
  - Kind 0: User metadata/profiles
  - Kind 3: Contact lists/follows
  - All NIP-51 relay list kinds (kinds 10000-10102, 30000-30030, 30063, 30267, 31924, 39089, 39092)

- **Automatic Relay Discovery**: Extracts relay URLs from kind:10002 events

- **Relay Syncing**: Continuously syncs with discovered relays on a rotating queue

- **REQ Analytics & Spam Detection**:
  - Tracks pubkey request popularity and co-occurrence patterns
  - Detects bot clusters via follow graph analysis (Tarjan's SCC algorithm)
  - Trust propagation from largest connected component
  - Manual spam purging with confirmation

- **LMDB + SQLite Storage**: LMDB for events, local SQLite for analytics and history

- **Statistics Dashboard**:
  - `/stats` - Relay statistics, event counts, discovered relays
  - `/stats/analytics` - REQ analytics, bot clusters, spam candidates
  - `/relays` - Detailed relay health and contribution stats
  - `/rankings` - Top profiles by follower count
  - `/profile` - View individual profiles

- **NIP-11 Relay Information**: Fully configurable relay metadata

## Installation

```bash
go build -o purplepages
```

## Configuration

The relay is configured via `config.json`:

```json
{
  "relay": {
    "name": "purplepag.es",
    "description": "A specialized relay for profile and relay list events",
    "pubkey": "",
    "contact": "",
    "icon": "",
    "supported_nips": [1, 11, 42, 51],
    "software": "https://github.com/purplepages/relay",
    "version": "0.1.0"
  },
  "server": {
    "host": "0.0.0.0",
    "port": 3335
  },
  "storage": {
    "path": "./data/events.lmdb"
  },
  "allowed_kinds": [0, 3, 10000, 10001, ...],
  "sync": {
    "enabled": true,
    "relays": [
      "wss://relay.damus.io",
      "wss://nos.lol",
      "wss://relay.nostr.band"
    ]
  }
}
```

### Configuration Options

- `relay.*`: NIP-11 relay information metadata
- `server.host`: Interface to bind to (default: 0.0.0.0)
- `server.port`: Port to listen on (default: 3335)
- `storage.path`: Path to LMDB event storage directory
- Analytics DB: A local `analytics.sqlite` file is created alongside the storage path directory
- `allowed_kinds`: Array of event kinds to accept
- `sync.enabled`: Enable/disable automatic sync on startup
- `sync.relays`: Array of relay URLs to sync from initially

## Usage

```bash
./purplepages
```

### Command Line Options

- `--port <port>`: Override port from config
- `--import <file.jsonl>`: Import events from JSONL file and exit

## Architecture

```
├── main.go                 # Entry point, relay initialization
├── config/
│   └── config.go           # Configuration loading and validation
├── storage/
│   ├── storage.go          # Storage backend abstraction
│   ├── relay_discovery.go  # Relay discovery + relay stats scanning
│   └── analytics.go        # REQ analytics & spam detection tables
├── analytics/
│   ├── tracker.go          # REQ event tracking with periodic flush
│   ├── cluster.go          # Bot cluster detection (Tarjan's SCC)
│   └── trust.go            # Trust propagation & spam identification
├── relay/
│   ├── queue.go            # Relay sync queue
│   └── sync_subscriber.go  # Persistent sync subscriber
├── internal/
│   └── relayutil/          # Relay URL normalization helpers
├── stats/
│   ├── stats.go            # In-memory statistics tracking
│   ├── handler.go          # /stats endpoint
│   ├── relays_handler.go   # /relays endpoint
│   └── analytics_handler.go # /stats/analytics endpoint
├── pages/
│   └── pages.go            # /rankings, /profile endpoints
└── sync/
    └── sync.go             # Initial sync from configured relays
```

## Spam Detection

The relay includes a spam detection system based on follow graph analysis:

1. **Seed trusted set**: Largest connected component of the follow graph
2. **Trust propagation**: Pubkeys followed by 10+ trusted users become trusted
3. **Bot cluster detection**: Strongly connected components with high internal density (>70%) and low external connections (<20%)
4. **Spam candidates**: Untrusted pubkeys in bot clusters or never requested by anyone

View and purge spam at `/stats/analytics`.

## Dependencies

- [khatru](https://fiatjaf.com/nostr/khatru) - Nostr relay framework
- [go-nostr](https://github.com/nbd-wtf/go-nostr) - Nostr protocol implementation
- [eventstore](https://github.com/fiatjaf/eventstore) - Event storage abstraction
- [sqlx](https://github.com/jmoiron/sqlx) - SQL extensions for Go
- [go-sqlite3](https://github.com/mattn/go-sqlite3) - SQLite driver for analytics storage

## License

MIT
