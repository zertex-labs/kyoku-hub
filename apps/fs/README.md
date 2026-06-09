# fs

Local file agent. Runs on the user's machine or NAS, reads music files, connects outbound to hub over WebSocket. **Does not need to be publicly accessible.**

## What it does

- Walks configured music directories, extracts ID3/Vorbis/MP4 tags (including MusicBrainz IDs)
- Caches folder mtimes in SQLite — skips unchanged directories on subsequent scans
- Exposes `GET /api/fs/data` (quick scan) and `GET /api/fs/data?full=1` (force rescan)
- Authenticates with hub via ED25519 keypair challenge-response

## Config (env vars)

| Var | Required | Default | Description |
|-----|----------|---------|-------------|
| `SCAN_DIRS` | yes | - | Comma-separated list of directories to scan |
| `HUB_WS_URL` | yes | - | WebSocket URL of hub (`ws://hub:8080/ws/agent`) |
| `HUB_API_URL` | yes | - | HTTP URL of hub (`http://hub:8080/api`) |
| `DATA_DIR` | no | `./data` | Where to store `agent.json` (keypair) and `cache.db` |
| `PORT` | no | `8000` | Local HTTP port |
| `AGENT_TOKEN` | first boot only | - | One-time registration token from hub UI |
| `AGENT_LABEL` | no | - | Human-readable label shown in hub UI |

## Running with Docker

```sh
AGENT_TOKEN=<token> \
AGENT_LABEL="My NAS" \
MUSIC_DIR=/path/to/music \
  docker compose up -d
```

The `MUSIC_DIR` host path is mounted read-only at `/music` inside the container. Agent identity and cache are persisted in `./data`.

## Endpoints

| Method | Path | Description |
|--------|------|-------------|
| GET | `/health` | Health check |
| GET | `/api/fs/data` | Quick scan (skips unchanged dirs) |
| GET | `/api/fs/data?full=1` | Full rescan, repopulates cache |

## Supported formats

`.mp3` `.flac` `.ogg` `.oga` `.m4a` `.m4b` `.m4p` `.mp4` `.alac`
