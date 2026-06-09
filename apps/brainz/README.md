# brainz

MusicBrainz data service. Source of truth for artist, release group, and release data.

## What it does

- Fetches artist/release data from MusicBrainz API (rate-limited to 1 req/s by default)
- Priority queue: processes artists by request count, falls back to oldest-fetched (cron-style)
- In-memory cache (ristretto) with adaptive TTL based on query frequency
- Pushes updates to subscribed hub instances over WebSocket

## Architecture

Two goroutines:
1. **Echo server** - REST + WS on same port
2. **Worker** - MB fetching, priority queue, DB writes, cache invalidation, WS push

## Config (env vars)

| Var | Default | Description |
|-----|---------|-------------|
| `DATABASE_URL` | - | Postgres connection string |
| `PORT` | `9000` | HTTP port |
| `FORCE_UPDATE_THRESHOLD` | `100` | Request count that triggers immediate processing |
| `WORKER_POLL_INTERVAL` | `5s` | Idle poll interval |
| `MB_RATE_LIMIT` | `1` | MusicBrainz requests per second |
| `CACHE_MAX_SIZE_MB` | `256` | Ristretto max memory |

## Cache TTL tiers
Each cache entry has a TTL depending on how frequently it is accessed(over the span of 1h). The more requested an entry is the longer it's TTL would be.

| Hits in last hour | TTL |
|---|---|
| 50+ | 6h |
| 10–49 | 2h |
| 1–9 | 30m |
| 0 | 10m |

## Database

Postgres. Stores artists, release groups, releases, hub registrations, subscriptions, and per-hub request counts.

_The MB team suggests hosting our own instance, but we've prompted to this method(at least for now)._
