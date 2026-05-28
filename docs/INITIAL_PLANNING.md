# Kyoku - Architecture Planning

## Overview

Kyoku is a music management platform consisting of multiple services in a Go monorepo, with a Vue SPA frontend.

## Monorepo Structure

```
kyoku/
├── apps/
│   ├── hub/
│   │   ├── frontend/    Nuxt (SPA mode) + Vue + Tailwind
│   │   └── backend/     Go (Echo) - API + WS server
│   ├── brainz/          Go (Echo) - MusicBrainz service
│   └── fs/              Go (Echo) - Local file agent
│
├── packages/            All Go, each with own go.mod
│   ├── musicbrainz-client/  MusicBrainz API client (rate-limited, 1 req/s)
│   ├── validation/      Tag validation logic
│   ├── downloader/      Download logic (Deezer/Tidal)
│   └── seek/            Soulseek client (future)
│
├── proto/               Protobuf definitions + generated Go code
│   ├── brainz.proto     Hub <-> Brainz messages
│   ├── fs.proto         Hub <-> Fs messages
│   └── common.proto     Shared types (Artist, Release, etc.)
│
├── go.work              Links all Go modules
└── Taskfile.yml         Build/dev/proto tasks
```

## Apps

### hub/backend (Go/Echo)

- Main API server for the frontend
- Manages users, libraries, preferences, download history
- Connects to brainz via WS for artist updates, fans out to browser clients
- Accepts inbound WS connections from fs agents
- Imports packages/validation, packages/downloader (and packages/seek in the future)
- Auth: MusicBrainz OAuth2 with PKCE, cookie-based sessions (30-day expiry)

### hub/frontend (Nuxt SPA)

- Plain SPA (`ssr: false`), no server-side rendering
- Communicates with hub backend via HTTP (REST) + WebSocket (real-time updates)
- All API calls go to the Go backend (no Nuxt server routes)

### brainz (Go/Echo)

- Source of truth for artist/release/release-group data
- Fetches from MusicBrainz API using `packages/musicbrainz-client` (rate-limited)
- Request-driven priority queue with configurable thresholds
- In-memory cache (ristretto) with adaptive TTL based on query frequency
- Exposes REST API for queries + WS endpoint for real-time update subscriptions
- Hub instances connect to it, not the other way around
- Single binary, 2 goroutines:
  1. **Echo server** - REST + WS on same port
  2. **Worker** - MusicBrainz fetching, priority queue, writes to DB, invalidates cache, signals server via Go channel to push updates to subscribed hubs

### fs (Go/Echo)

- Runs on user's machine or NAS, wherever the music files are
- Reads local files, exposes metadata (tags, album, artist, path, etc.)
- Connects outbound to hub backend via WS (no port exposure needed for user)
- In-memory cache: stores folder change-times, skips unchanged folders, rescans on mismatch
- Auth: keypair tied to user ID

## Packages

### musicbrainz-client

HTTP client for the MusicBrainz API. Features:
- Rate limiter: 1 request/second (global, via `golang.org/x/time/rate`)
- Retry on 503 with exponential backoff (1s, 2s, 4s)
- Proper User-Agent header (`Kyoku/<version> (<contact>)`)
- Used by both hub (for artist search) and brainz (for full data fetching)

### validation

Tag validation logic. Expects file metadata (from fs), checks if required tags (specified in config) are present.

### downloader

Download logic for Deezer and Tidal APIs (might support more in the future).

### seek

Soulseek client implementation. Future effort, not in initial scope. We have a working demo, but not enough to do in the initial dev builds.

## Communication

```
brainz (Go) <--(WS + protobuf)-- hub backend (Go)
hub backend (Go) <--(WS + protobuf)-- fs (Go)
hub backend (Go) <--(HTTP + WS JSON)-- Vue SPA
```

### Brainz <-> Hub

- Hub backend opens a persistent WS connection to brainz on startup
- Auth: keypair challenge-response on connect
  1. Hub auto-registers on first boot (sends public key to brainz)
  2. Brainz stores public key + assigns instanceId
  3. On every WS connect: brainz sends challenge (nonce), hub signs with private key, brainz verifies
  4. No per-message signing - connection is trusted for its lifetime
- Hub subscribes to artist IDs it cares about (aggregated for all the users in that instance)
- Hub sends incremental subscribe/unsubscribe as users add/remove artists
- Hub forwards "request update" messages to brainz (incrementing priority)
- Brainz persists subscriptions in a junction table (see [Brainz Schema](#brainz-database-schema))
- On update: brainz queries which hubs are subscribed to that artist, pushes to those
- Brainz pushes updates only to hubs subscribed to that artist
- On reconnect: hub calls `GET /updates?after=<last_seen_timestamp>`, brainz looks up the hub's persisted subscriptions and returns updates for those artists since the timestamp

### Hub <-> Fs

- Fs initiates WS connection to hub (fs needs outbound internet access, but does NOT need to be publicly accessible - works behind NAT/firewalls(hopefully))
- Hub accepts the connection on a WS endpoint (e.g. `/ws/agent`)
- Auth: keypair challenge-response tied to user ID
- Once connected, communication is bidirectional over the same WS
- On reconnect: hub requests current state from fs over the WS connection (fresh data, no queuing)

### Hub <-> Frontend

- HTTP REST for data fetching
- WebSocket for real-time updates (artist changes from brainz, download progress, etc.)

## Databases

| App    | Engine    | Stores                                                                                      |
| ------ | --------- | ------------------------------------------------------------------------------------------- |
| brainz | Postgres  | Artists, release-groups, releases, hub registrations, subscriptions, request counts         |
| hub    | Postgres  | Users, sessions, tracked artists (with sync status), user-artist links                     |
| fs     | In-memory | Folder change-time cache                                                                    |

### Hub Database Schema

```sql
CREATE TABLE users (
    id TEXT PRIMARY KEY,
    username TEXT NOT NULL,
    mb_user_id TEXT UNIQUE NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE TABLE sessions (
    id TEXT PRIMARY KEY,
    user_id TEXT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    expires_at TIMESTAMPTZ NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

-- Shared artist state (one row per unique MBID across all users)
CREATE TABLE artists (
    mb_artist_id TEXT PRIMARY KEY,
    name TEXT NOT NULL,
    disambiguation TEXT DEFAULT '',
    status TEXT NOT NULL DEFAULT 'queued',  -- queued | processing | synced
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    synced_at TIMESTAMPTZ
);

-- Which users track which artists
CREATE TABLE user_artists (
    user_id TEXT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    mb_artist_id TEXT NOT NULL REFERENCES artists(mb_artist_id) ON DELETE CASCADE,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    PRIMARY KEY (user_id, mb_artist_id)
);

CREATE INDEX idx_sessions_user_id ON sessions(user_id);
```

### Brainz Database Schema

```sql
-- Artists (source of truth for MB data)
CREATE TABLE artists (
    mb_artist_id TEXT PRIMARY KEY,
    name TEXT NOT NULL,
    sort_name TEXT DEFAULT '',
    disambiguation TEXT DEFAULT '',
    type TEXT DEFAULT '',              -- Person, Group, Orchestra, etc.
    country TEXT DEFAULT '',
    last_fetched_at TIMESTAMPTZ,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

-- Release Groups (albums, singles, EPs, etc.)
CREATE TABLE release_groups (
    mb_release_group_id TEXT PRIMARY KEY,
    mb_artist_id TEXT NOT NULL REFERENCES artists(mb_artist_id) ON DELETE CASCADE,
    title TEXT NOT NULL,
    primary_type TEXT DEFAULT '',       -- Album, Single, EP, Broadcast, Other
    disambiguation TEXT DEFAULT '',
    first_release_date TEXT DEFAULT '', -- TEXT: MB uses partial dates ("2022", "2022-01", "2022-01-26")
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX idx_release_groups_artist ON release_groups(mb_artist_id);

-- Releases (specific editions/versions of a release group)
CREATE TABLE releases (
    mb_release_id TEXT PRIMARY KEY,
    mb_release_group_id TEXT NOT NULL REFERENCES release_groups(mb_release_group_id) ON DELETE CASCADE,
    title TEXT NOT NULL,
    status TEXT DEFAULT '',             -- Official, Promotion, Bootleg, Pseudo-Release
    country TEXT DEFAULT '',
    date TEXT DEFAULT '',               -- partial date
    barcode TEXT DEFAULT '',
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX idx_releases_release_group ON releases(mb_release_group_id);

-- Hub instances registered with brainz
CREATE TABLE hub_instances (
    id TEXT PRIMARY KEY,
    public_key TEXT NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

-- Hub subscriptions (which hubs care about which artists)
CREATE TABLE hub_subscriptions (
    hub_instance_id TEXT NOT NULL REFERENCES hub_instances(id) ON DELETE CASCADE,
    mb_artist_id TEXT NOT NULL REFERENCES artists(mb_artist_id) ON DELETE CASCADE,
    subscribed_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    PRIMARY KEY (hub_instance_id, mb_artist_id)
);

-- Request counts per hub per artist (resets to 0 after sync)
CREATE TABLE artist_requests (
    mb_artist_id TEXT NOT NULL REFERENCES artists(mb_artist_id) ON DELETE CASCADE,
    hub_instance_id TEXT NOT NULL REFERENCES hub_instances(id) ON DELETE CASCADE,
    request_count INT NOT NULL DEFAULT 0,
    last_requested_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    PRIMARY KEY (mb_artist_id, hub_instance_id)
);
```

## Brainz Worker & Priority Queue

The worker goroutine processes artists in priority order:

```sql
-- Pick next artist to fetch:
-- 1. Force immediate if total requests >= threshold (configurable)
-- 2. Otherwise highest request count first
-- 3. If no pending requests, oldest last_fetched_at (cron-style refresh)
SELECT a.mb_artist_id, COALESCE(SUM(r.request_count), 0) AS total_requests
FROM artists a
LEFT JOIN artist_requests r ON r.mb_artist_id = a.mb_artist_id
GROUP BY a.mb_artist_id
ORDER BY
    CASE WHEN COALESCE(SUM(r.request_count), 0) >= $1 THEN 0 ELSE 1 END,
    COALESCE(SUM(r.request_count), 0) DESC,
    a.last_fetched_at ASC NULLS FIRST
LIMIT 1;
```

After sync completes for an artist:
1. Update `artists.last_fetched_at = NOW()`
2. Reset `artist_requests.request_count = 0` for that artist
3. Invalidate cache entries for that artist
4. Push update to subscribed hubs via WS

### Configuration

| Key | Default | Description |
|-----|---------|-------------|
| `FORCE_UPDATE_THRESHOLD` | 100 | Total request count that forces immediate processing |
| `WORKER_POLL_INTERVAL` | 5s | How often the worker checks for work when idle |
| `MB_RATE_LIMIT` | 1/s | MusicBrainz API rate limit |

## Brainz In-Memory Cache (Ristretto)

Brainz uses [ristretto](https://github.com/dgraph-io/ristretto) as an in-memory cache layer in front of Postgres.

### Cache Keys

| Pattern | Value | Invalidated on |
|---------|-------|----------------|
| `artist:{mbid}` | Compressed JSON (artist metadata) | Artist sync |
| `release-groups:{mbid}` | Compressed JSON (all release groups for artist) | Artist sync |
| `releases:{release_group_id}` | Compressed JSON (all releases for RG) | Artist sync |

### Adaptive TTL

TTL is based on query frequency (how often brainz receives REST requests for a given artist):

| Hits in last hour | TTL |
|---|---|
| 50+ | 6 hours |
| 10-49 | 2 hours |
| 1-9 | 30 minutes |
| 0 | 10 minutes |

### Configuration

| Key | Default | Description |
|-----|---------|-------------|
| `CACHE_MAX_SIZE_MB` | 256 | Maximum cache memory usage |
| `CACHE_HIT_WINDOW` | 1h | Time window for counting query hits |

TTL tiers are configured as a sorted list of `{minHits, ttl}` pairs (evaluated top-down, first match wins):

```go
// Default configuration
CacheTTLTiers: []CacheTier{
    {MinHits: 50, TTL: 6 * time.Hour},
    {MinHits: 10, TTL: 2 * time.Hour},
    {MinHits: 1,  TTL: 30 * time.Minute},
    {MinHits: 0,  TTL: 10 * time.Minute},
}
```

### Cache Invalidation

- **On artist sync**: worker evicts `artist:{mbid}`, `release-groups:{mbid}`, and all `releases:*` for that artist's release groups
- **On TTL expiry**: ristretto automatically evicts
- **On memory pressure**: ristretto's admission policy evicts least valuable entries

## Artist Sync Status Flow

```
User adds artist
       │
       ▼
Hub: INSERT artists (status='queued')
Hub: INSERT user_artists
Hub: Send subscribe to brainz via WS
       │
       ▼
Brainz: receives subscription
Brainz: worker picks up artist (via priority queue)
Brainz: pushes status update to hub (status='processing')
       │
       ▼
Hub: UPDATE artists SET status='processing'
Hub: fans out to connected browser clients via WS
       │
       ▼
Brainz: fetches from MusicBrainz API (release groups, releases)
Brainz: stores in DB, invalidates cache
Brainz: pushes completion + data to hub (status='synced')
       │
       ▼
Hub: UPDATE artists SET status='synced', synced_at=NOW()
Hub: fans out to connected browser clients via WS
Frontend: shows artist data, removes "queued"/"processing" indicator
```

## Auth Patterns

### Hub <-> Browser (user auth)

- MusicBrainz OAuth2 with PKCE (S256)
- Cookie-based sessions (`kyoku_session`, 30-day expiry, HttpOnly, SameSite=Lax)
- Session stored in Postgres, validated on each request via auth middleware

### Hub <-> Brainz (instance auth)

- Hub generates keypair on first boot
- Registers public key with brainz, receives instanceId
- Challenge-response on every WS connection (sign nonce with private key)
- No secrets transmitted over the wire

### Hub <-> Fs (user-bound auth)

- Same keypair challenge-response pattern
- Key is tied to a specific user ID in hub's database
- Only the user who registered the fs agent can receive data from it

## Deployment

All services are independently deployable:

- **brainz**: standalone Go binary + Postgres. Can be hosted centrally (shared) or self-hosted.
- **hub backend**: standalone Go binary + Postgres. Points to a brainz instance (hosted or self-hosted).
- **hub frontend**: static SPA, served from anywhere (CDN, Nginx, etc.). Points to hub backend API.
- **fs**: standalone Go binary. Runs on user's machine/NAS. Connects outbound to hub.

Users can mix and match:

- Use the hosted brainz + self-hosted hub + local fs
- Self-host everything
- Use hosted everything + local fs

## Tooling

| Tool           | Purpose                                                 |
| -------------- | ------------------------------------------------------- |
| `go.work`      | Go workspace linking all modules                        |
| `buf`          | Protobuf linting, generation, breaking change detection |
| `Taskfile.yml` | Build, dev, proto generation tasks                      |
| Echo           | HTTP/WS framework for all Go services                   |
| Postgres       | Database for brainz and hub                             |
| Ristretto      | In-memory cache for brainz                              |
| Zap            | Structured logging (hub backend)                        |

## Hub Backend Internal Fan-out

For pushing real-time updates to connected browser clients:

```go
// In hub backend memory
artistSubscribers map[string]map[*websocket.Conn]struct{} // artistId -> set of browser WS connections
```

- User opens page -> subscribes to artists in their library
- User closes tab -> WS closes -> remove from all subscription sets
- Brainz pushes update for artist X -> hub looks up subscribers -> sends to those connections

Scaling: single instance handles ~10-50k concurrent WS connections. If horizontal scaling is needed later, we're doomed (in reality can add something like redis queues)

## Future Considerations

- **packages/seek**: Soulseek protocol implementation, not in initial scope
- **kyoku-p2p**: Custom P2P protocol, modern alternative to soulseek, not in scope
- **Horizontal scaling**: Redis pub/sub between hub instances if single instance can't handle WS load
- **Proto versioning**: buf handles breaking change detection as schemas evolve
