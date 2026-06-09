# kyoku-hub

Music management platform, trying to allow downloading, tagging and managing your music library in a single place. 

## Structure

```
kyoku-hub/
├── apps/
│   ├── hub/
│   │   ├── backend/     Go (Echo) - main API + WebSocket server
│   │   └── frontend/    Nuxt (SPA) + Vue + Tailwind
│   ├── brainz/          Go (Echo) - MusicBrainz data service
│   └── fs/              Go (Echo) - local file agent (runs on NAS/user machine)
│
├── packages/
│   ├── musicbrainz-client/  rate-limited MB API client (1 req/s)
│   ├── validation/          tag validation logic
│   ├── downloader/          Deezer/Tidal download logic
│   └── seek/                Soulseek client (future)
│
├── proto/               Protobuf definitions + generated Go code
├── go.work              Go workspace linking all modules
└── Taskfile.yml         Build / dev / proto generation tasks
```

## Services

| Service | Port | Description |
|---------|------|-------------|
| hub/backend | 8080 | Main API, WebSocket hub, user auth |
| hub/frontend | 3000 | Vue SPA |
| brainz | - | MusicBrainz sync, internal only |
| fs | 8000 | File agent, deployed per-user |

## Communication

```
brainz  <──(WS + protobuf)──  hub/backend  <──(HTTP + WS JSON)──  frontend
                                    ▲
                               (WS + protobuf)
                                    │
                                   fs (outbound, behind NAT)
```

## Auth

- **User → Hub**: MusicBrainz OAuth2 (PKCE), cookie sessions (30-day)
- **Hub → Brainz**: keypair challenge-response on each WS connect
- **Fs → Hub**: keypair challenge-response, key tied to user ID. Registered once with an ID and immutable after that

## Databases

| App | Engine | Notes |
|-----|--------|-------|
| hub/backend | Postgres | users, sessions, artists, user-artist links |
| brainz | Postgres | artists, release groups, releases, hub subscriptions |
| fs | SQLite | folder mtime cache (non-critical, loss = full rescan) |

## Development

```sh
# proto generation
task proto

# run all services
task dev
```

See each app's README for individual setup.
