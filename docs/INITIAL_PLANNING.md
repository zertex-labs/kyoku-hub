# Kyoku - Architecture Planning

## Overview

Kyoku is a music management platform consisting of multiple services in a Go monorepo, with a Vue SPA frontend.

## Monorepo Structure

```
kyoku/
├── apps/
│   ├── hub/
│   │   ├── frontend/    Vite + Vue SPA (TypeScript)
│   │   └── backend/     Go (Echo) - API + WS server
│   ├── brainz/          Go (Echo) - MusicBrainz service
│   └── fs/              Go (Echo) - Local file agent
│
├── packages/            All Go, each with own go.mod
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

### hub/frontend (Vite + Vue SPA)

- Plain SPA, no SSR
- Communicates with hub backend via HTTP (REST) + WebSocket (real-time updates)

### brainz (Go/Echo)

- Source of truth for artist/release/release-group data
- Fetches from MusicBrainz API
- Cron-based update system with request-driven priority
- Exposes REST API for queries + WS endpoint for real-time update subscriptions
- Hub instances connect to it, not the other way around

### fs (Go/Echo)

- Runs on user's machine or NAS, wherever the music files are
- Reads local files, exposes metadata (tags, album, artist, path, etc.)
- Connects outbound to hub backend via WS (no port exposure needed for user)
- In-memory cache: stores folder change-times, skips unchanged folders, rescans on mismatch
- Auth: keypair tied to user ID

## Packages

### validation

Tag validation logic. Expects file metadata (from fs), checks if required tags (specified in config) are present.

### downloader

Download logic for Deezer and Tidal APIs(might support more in the future).

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
- Brainz persists subscriptions in a junction table (see [Brainz Schema](#brainz-schema-partial))
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
| brainz | Postgres  | Artists, releases, release-groups, hub registrations (public keys), request counts/priority |
| hub    | Postgres  | Users, libraries, fs agent keys, download history, preferences                              |
| fs     | In-memory | Folder change-time cache                                                                    |

### Brainz Schema (partial)

```sql
hub_subscriptions
├── hub_instance_id  (FK -> hub_instances.id)
├── artist_id        (FK -> artists.id)
└── subscribed_at
-- UNIQUE(hub_instance_id, artist_id)
```

## Auth Patterns

### Hub <-> Brainz (instance auth)

- Hub generates keypair on first boot
- Registers public key with brainz, receives instanceId
- Challenge-response on every WS connection (sign nonce with private key)
- No secrets transmitted over the wire

### Hub <-> Fs (user-bound auth)

- Same keypair challenge-response pattern
- Key is tied to a specific user ID in hub's database
- Only the user who registered the fs agent can receive data from it

## Brainz Update Strategy

- Users "request" artist updates via hub frontend
- Hub forwards request counts to brainz
- Brainz aggregates requests across ALL connected hub instances
- Higher aggregate request count = higher priority in cron queue
- 100+ total requests across all hubs = force immediate update
- Cron cycles through artists by priority otherwise

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

## Hub Backend Internal Fan-out

For pushing real-time updates to connected browser clients:

```go
// In hub backend memory
artistSubscribers map[string]map[*websocket.Conn]struct{} // artistId -> set of browser WS connections
```

- User opens page -> subscribes to artists in their library
- User closes tab -> WS closes -> remove from all subscription sets
- Brainz pushes update for artist X -> hub looks up subscribers -> sends to those connections

Scaling: single instance handles ~10-50k concurrent WS connections. If horizontal scaling is needed later, we're doomed(in reality can add something like redis queues)

## Future Considerations

- **packages/seek**: Soulseek protocol implementation, not in initial scope
- **kyoku-p2p**: Custom P2P protocol, modern alternative to soulseek, not in scope
- **Horizontal scaling**: Redis pub/sub between hub instances if single instance can't handle WS load
- **Proto versioning**: buf handles breaking change detection as schemas evolve
