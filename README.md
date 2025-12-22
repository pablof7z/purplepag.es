# purplepag.es

A specialized Nostr relay built with [khatru](https://khatru.nostr.technology/) that focuses on profile and relay list events.

## Features

- **Event Type Filtering**: Only accepts specific event kinds:
  - Kind 0: User metadata/profiles
  - Kind 3: Contact lists/follows
  - All NIP-51 relay list kinds (kinds 10000-10102, 30000-30030, 30063, 30267, 31924, 39089, 39092)

- **Automatic Relay Discovery**: Extracts relay URLs from kind:10002 events and automatically discovers new relays

- **Intelligent Relay Syncing**: Continuously syncs with discovered relays every 30 seconds, tracking:
  - Success rates for each relay
  - Events contributed by each relay
  - Connection statistics

- **Negentropy Support**: Built-in support for efficient event synchronization using negentropy protocol

- **SQLite3 Storage**: Fast, reliable database storage with no content size limits

- **Beautiful Statistics Dashboard**:
  - `/stats` - View relay statistics, event counts, and discovered relays
  - `/relays` - Detailed view of all discovered relays with success rates and contributions

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
    "port": 3334
  },
  "storage": {
    "backend": "lmdb",
    "path": "./data"
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
- `storage.backend`: Storage backend ("sqlite3" or "lmdb")
- `storage.path`: Path to storage file/directory
- `allowed_kinds`: Array of event kinds to accept
- `sync.enabled`: Enable/disable automatic sync on startup
- `sync.relays`: Array of relay URLs to sync from initially

## Usage

```bash
./purplepages
```

The relay will:
1. Load configuration from `config.json`
2. Initialize LMDB storage
3. Start syncing events from configured relays (if enabled)
4. Begin accepting WebSocket connections

## Supported Event Kinds

### Standard Profile & Social Graph
- **Kind 0**: User metadata (profiles)
- **Kind 3**: Contact lists (follows)

### NIP-51 Lists (Replaceable)
- **Kind 10000**: Mute list
- **Kind 10001**: Pinned notes
- **Kind 10002**: Read/write relays
- **Kind 10003**: Bookmarks
- **Kind 10004**: Communities
- **Kind 10005**: Public chats
- **Kind 10006**: Blocked relays
- **Kind 10007**: Search relays
- **Kind 10009**: Simple groups
- **Kind 10012**: Relay feeds
- **Kind 10015**: Interests
- **Kind 10020**: Media follows
- **Kind 10030**: Emojis
- **Kind 10050**: DM relays
- **Kind 10101**: Good wiki authors
- **Kind 10102**: Good wiki relays

### NIP-51 Sets (Parameterized Replaceable)
- **Kind 30000**: Follow sets
- **Kind 30002**: Relay sets
- **Kind 30003**: Bookmark sets
- **Kind 30004**: Curation sets (articles)
- **Kind 30005**: Curation sets (videos)
- **Kind 30007**: Kind mute sets
- **Kind 30015**: Interest sets
- **Kind 30030**: Emoji sets
- **Kind 30063**: Release artifact sets
- **Kind 30267**: App curation sets
- **Kind 31924**: Calendar events
- **Kind 39089**: Starter packs
- **Kind 39092**: Media starter packs

## Architecture

```
├── main.go              # Entry point, relay initialization
├── config/
│   └── config.go       # Configuration loading and validation
├── storage/
│   └── storage.go      # LMDB storage backend wrapper
└── sync/
    └── sync.go         # Negentropy-based event synchronization
```

## Dependencies

- [khatru](https://github.com/fiatjaf/khatru) - Nostr relay framework
- [go-nostr](https://github.com/nbd-wtf/go-nostr) - Nostr protocol implementation
- [go-negentropy](https://github.com/illuzen/go-negentropy) - Negentropy set reconciliation
- [eventstore](https://github.com/fiatjaf/eventstore) - Event storage abstraction with LMDB backend

## License

MIT
