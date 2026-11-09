# kyoku

Find music, download it, tag it, and keep it in your own library.

The hub is the app you use — search, providers, jobs, settings. Your files never
touch it: every read and every write goes through an **fs agent** running on the
machine that holds your music. That split is the architecture, not a deployment
detail.

## How it fits together

```
   you ──► hub (ours, or yours)  ──► deezer ──┐
             │                              │ ticket: url + tags
             │                              ▼
             └──► fs agent (your machine) ──► your library
                        │
                        └──► navidrome, plex, whatever reads the folder

   brainz (one instance, shared) ──► musicbrainz.org
```

| piece | what it is | where it runs |
| --- | --- | --- |
| **hub** | the API and web app: search, provider chain, download jobs, tagger settings | our instance, or yours |
| **fs agent** | scans your library, fetches audio, writes tags, publishes into folders | always yours, at least one |
| **brainz** | MusicBrainz catalogue and a single rate budget for every hub | one instance, shared |

The hub resolves *what* to fetch and *what the tags should say*. The agent does
everything that touches a file. Audio never passes through the hub — the agent
fetches from the provider CDN directly.

## What it does today

- **Follow artists** and see, per release group, what you own and what is
  incomplete — answered by your agent's own scan, not by guessing.
- **Search a provider** (Deezer), drop results by a different artist, and fall
  back to MusicBrainz aliases and the artist's discography when a catalogue files
  things under another script or a licensed title.
- **Pick an edition.** A release group often has several; the picker passes the
  MusicBrainz release id, and the chosen edition outranks the provider's metadata
  (Deezer calls 放生会 "Carnival"; the file gets the real title).
- **See quality before you commit.** Per-track probing: `16/16 songs · FLAC
  16/44.1`, classified FIT / PARTIAL / MISS against your quality ladder and
  source order.
- **Download, tag and publish** — through the agent, with a live job log.
- **Tags that other tools read.** Picard's key names per container: Vorbis
  comments for FLAC, ID3v2.4 for MP3, recording id in `MUSICBRAINZ_TRACKID` /
  `UFID`, cover art embedded and written as `cover.jpg`.
- **Re-tag, or check for a better copy**, from an album card. Re-tagging reads
  what the file already says first, so a throttled MusicBrainz cannot blank the
  ids it carries.
- **Settings** that are yours: required tags, artwork, genre source, and the album
  folder template (`{artist}/[{year}] {album}` by default, Picard's shape).

## In progress

- **Plugins run on the agent.** The plugin host, install directory and config
  currently live on the hub; they move to the agent so a plugin can read and write
  the files it is asked to process. See `docs/plugins.md`.
- **Navidrome integration** — rescan after a publish and reading the library's own
  album list. Designed, not built: `docs/navidrome.md`.
- **More providers.** The chain and quality model are provider-agnostic; only
  Deezer is wired up. Tidal is parked.

## Repository layout

```
kyoku-hub/
├── apps/
│   ├── hub/
│   │   ├── backend/     Go (Echo) — API, WebSocket hub, download orchestration
│   │   └── frontend/    Nuxt 4 SPA — Vue + Tailwind
│   ├── brainz/          Go (Echo) — MusicBrainz catalogue service
│   └── fs/              Go (Echo) — the agent that touches your files
│
├── packages/
│   ├── downloader/      quality model, matching, provider chain, Deezer source
│   ├── tagwriter/       Picard-compatible FLAC / MP3 writers + readers
│   ├── validation/      configurable required-tag checks
│   ├── plugin-api/      wire contract between host and plugin
│   ├── plugin-sdk/      what a Go plugin is written against
│   ├── musicbrainz-client/
│   └── logger/
│
├── proto/               Protobuf definitions + generated Go
├── docs/                design notes (see below)
├── go.work              Go workspace
└── Taskfile.yml
```

## Services

| Service | Port | Description |
| --- | --- | --- |
| hub backend | 8080 | API, WebSocket hub, auth, jobs |
| hub frontend | 3000 | Vue SPA (dev server) |
| brainz | 8081 | MusicBrainz catalogue, internal |
| fs agent | 8000 | File agent — health + scan endpoint, dials the hub |

## Communication

```
brainz  ◄──(WS + protobuf)──  hub backend  ◄──(HTTP + SSE)──  frontend
                                   ▲
                              (WS + protobuf)
                                   │
                              fs agent (dials out, works behind NAT)
```

A job crosses that line twice: the hub sends a `WriteAlbum` manifest (URLs,
headers, tags, paths relative to the agent's library root), and the agent sends
back progress events until it reports `done` or `failed`.

## Auth

- **User → hub**: MusicBrainz OAuth2 (PKCE), cookie sessions (30 days).
- **Hub → brainz**: keypair challenge–response on each connect.
- **fs agent → hub**: keypair challenge–response, key bound to one user. Registered
  once with a token and immutable after that.
- **Provider accounts** (Deezer ARL): stored encrypted at the hub — it resolves
  the tickets. Navidrome and plugin secrets stay on the agent.

## Data

Where each piece lives, in one table: `docs/data-map.md`. Short version:

| App | Engine | Holds |
| --- | --- | --- |
| hub | Postgres | users, sessions, artists, agents, download jobs, user settings |
| brainz | Postgres | artists, release groups, releases, hub subscriptions |
| fs agent | SQLite | folder mtime cache (throw it away and rescan) |

## Running it

```sh
task deps     # postgres containers for hub + brainz
task dev      # hub :8080, brainz :8081, agent :8000, frontend :3000
task proto    # regenerate protobuf code
```

The agent needs, at minimum:

```sh
SCAN_DIRS=/music                  # what it watches
LIBRARY_DIR=/music                # where downloads land (defaults to SCAN_DIRS[0])
HUB_WS_URL=ws://localhost:8080/ws/agent
HUB_API_URL=http://localhost:8080/api
AGENT_TOKEN=…                     # first run only, from the hub UI
```

## Docs

| file | what it covers |
| --- | --- |
| `docs/downloader-plan.md` | quality model, provider chain, job model, tagging, post-processing |
| `docs/agent-file-ops.md` | why the agent does the file work, and the manifest contract |
| `docs/data-map.md` | where every piece of data lives |
| `docs/plugins.md` | plugin contract, sandbox, and where plugin state lives |
| `docs/navidrome.md` | what a Navidrome integration would take |
