# hub/backend

Main API server. Manages users, libraries, and real-time updates. Connects outbound to brainz, accepts inbound WebSocket connections from fs agents and browser clients.

## What it does

- REST API for the frontend (users, artists, sessions)
- WebSocket hub: fans out brainz updates to subscribed browser clients
- Accepts and authenticates fs agent connections
- MusicBrainz OAuth2 login flow (PKCE)

## Config (env vars)

| Var | Required | Description |
|-----|----------|-------------|
| `DATABASE_URL` | yes | Postgres connection string |
| `BRAINZ_WS_URL` | yes | WebSocket URL of brainz instance |
| `BRAINZ_API_URL` | yes | HTTP URL of brainz instance |
| `MB_CLIENT_ID` | yes | MusicBrainz OAuth2 client ID |
| `MB_CLIENT_SECRET` | yes | MusicBrainz OAuth2 client secret |
| `MB_REDIRECT_URI` | yes | OAuth2 redirect URI |
| `DATA_DIR` | no | Where to store hub keypair (`./data`) |
| `PORT` | no | HTTP port (default `8080`) |

## Running with Docker

```sh
docker compose up -d
```

## Database

Postgres. Schema managed via migrations in `internal/db/migrations/`.

```sql
users, sessions, artists, user_artists
```
