# Navidrome support

How kyoku can know what a Navidrome library already holds, and how a download can
end up in it.

**Where this runs matters more than what it calls.** The integration belongs in
the *fs agent*, not the hub: the agent is the component people deploy next to
Navidrome, on the host that has the files. The hub only needs to know the
outcome.

## Who holds what

Three components, three jobs:

| component | where it lives | what it does |
| --- | --- | --- |
| hub | anywhere | decides what to fetch, stores jobs and state |
| fs agent | next to the files, usually the Navidrome host | reads tags, writes tags, knows the paths, talks to Navidrome |
| Navidrome | the media host | plays the library, scans folders it watches |

So the integration is **agent-side**: the agent already scans the library, already
has a hub connection, and is the only component that can both rewrite a file and
tell Navidrome about it. Credentials stay on the media host; Navidrome never has
to be reachable from the hub (which in a homelab is often behind a different
network anyway).

The one thing that does not hold today: the hub is still what downloads and
writes files (`DOWNLOAD_DIR`). Hosting the agent next to Navidrome therefore gets
you correct *reads* immediately, and correct *writes* only when either

- the hub's `DOWNLOAD_DIR` is a volume the agent also mounts (the current
  deployment assumption), or
- the download moves to the agent (topology C in `downloader-plan.md`: the hub
  resolves a ticket, the agent fetches the bytes). That is the real end state and
  the reason this integration is worth putting on the agent now.

## What Navidrome actually exposes

Two APIs, same credentials, both HTTP. Verified against Navidrome `master`
(2026-11).

### Native API — for reading

Undocumented but stable, and richer than Subsonic.

| thing | detail |
| --- | --- |
| Login | `POST /auth/login` `{"username","password"}` → JWT |
| Auth header | `X-ND-Authorization: Bearer <jwt>` |
| Albums | `GET /api/album?_start=0&_end=200&_sort=name` |
| Songs of an album | `GET /api/song?album_id=<id>&_start=0&_end=500` |
| Pagination | `_start` / `_end`, plus `_sort` and `_order` |

Album fields that matter (`model/album.go`):

```
id, name, albumArtist, year, date, releaseDate, originalDate,
songCount, duration, size, genres[], releaseTypes[],
mbzAlbumId, mbzReleaseGroupId, mbzAlbumArtistId, mbzAlbumType
```

`mbzReleaseGroupId` is the whole game: it maps a Navidrome album onto a kyoku
release group with no name matching at all.

Song fields that matter (`model/mediafile.go`):

```
title, track, discNumber, duration, size, suffix, bitRate, sampleRate,
bitDepth, channels, codec, isrc, mbzRecordingID, mbzReleaseTrackId,
mbzAlbumId, mbzReleaseGroupId, mbzArtistId
```

That is enough to grade quality without touching a file: `suffix=flac`,
`bitDepth=24`, `sampleRate=96000` is FLAC 24/96, `suffix=mp3&bitRate=320` is MP3
320. The same ladder kyoku already uses for probing applies to an owned album,
so "check for better" can compare against what Navidrome has without reading it.

### Subsonic API — for the scan trigger

Native API has no scan route; Subsonic does.

```
GET /rest/startScan.view?u=<user>&t=<md5(password+salt)>&s=<salt>&v=1.16.1&c=kyoku&f=json
```

- Admin only.
- Current master accepts `target=` params for a **selective** scan, so a publish
  can ask Navidrome to look at one folder instead of the whole library.
- `getScanStatus.view` reports whether it is still running.

Older Navidrome ignores `target` and does a full scan — correct, just slower. No
version detection needed: ask for the target, live with the fallback.

## Design

Two directions, one component

### Read: the agent reports, the hub stores

The agent already walks the configured `SCAN_DIRS` and reports every track with
its release-group id — that is the "what do I already have" answer, and it needs
no Navidrome API at all. What the API adds is what the files cannot say: albums
Navidrome knows about that the agent's scan roots do not cover, and the
per-library folder mapping needed to ask for a scan of one directory.

Optional, therefore: a `library_albums` table holding what Navidrome reports,
filled by the agent when it is configured with Navidrome credentials.

```sql
CREATE TABLE library_albums (
  user_id    TEXT NOT NULL,
  source     TEXT NOT NULL,             -- 'navidrome' now, 'plex' later
  rg_id      TEXT,                      -- mbzReleaseGroupId, may be empty
  album_id   TEXT NOT NULL,             -- Navidrome's id
  title      TEXT NOT NULL,
  artist     TEXT NOT NULL,
  year       INT,
  song_count INT NOT NULL,
  quality    TEXT,                      -- graded from the songs, e.g. FLAC 24/96
  path       TEXT,                      -- libraryPath, for display
  synced_at  TIMESTAMPTZ NOT NULL
);
```

The sync is a paged walk of `/api/album` plus one `/api/song?album_id=` per album
for quality. A 5 000-album library is 5 001 requests; at 10 concurrent that is a
few seconds, and it is a background job with progress in the same job modal the
downloader uses (`inspect` already proved a non-download job works there).

Ownership then becomes a union: an album is *owned* if the fs agent saw it **or**
Navidrome reports it. `CountOwned` and the album grid read from that union.

**Matching when the ids are missing.** Navidrome happily ingests albums tagged by
tools that write no MusicBrainz ids, and so does kyoku. For those, fall back to
`downloader.FoldArtist` + `TitleRank` (already written for the release picker),
and mark the row `matched_by = 'artist_title'` so the UI can be honest that it is
a guess.

### Write: publish, then ask Navidrome to look

1. A publish or a re-tag changes files the agent can see.
2. The hub sends the changed paths along with its usual "rescan" request
   (`SyncRequest.changed`), instead of a bare `full=false`.
3. The agent maps each path onto a Navidrome library (`GET /api/library` gives
   id + root) and calls `startScan` with `target=<libraryID>:<relative path>`.
4. No target match, or an older Navidrome that ignores targets: fall back to a
   full `startScan`, and say which one happened in the job log.

`startScan` is admin-only, so the agent needs an admin account. Reads work with
any account, which is what "Test connection" should report.

### Credentials

Agent config, not hub config: `NAVIDROME_URL`, `NAVIDROME_USER`,
`NAVIDROME_PASS`. They never leave the media host, which is also where they are
useful — the hub has no reason to hold them, and cannot reach the instance in
most deployments anyway.

## Agent config

```bash
NAVIDROME_URL=https://music.example.lan
NAVIDROME_USER=kyoku
NAVIDROME_PASS=…
# optional: only scan the folder that changed, when Navidrome supports targets
NAVIDROME_TARGET_SCAN=true
```

The agent logs the mapping it derived on the first request ("library 1 · /music ·
target Music/aespa/LEMONADE") and then stays quiet.

## Phases

| phase | what | why first |
| --- | --- | --- |
| 1 | `SyncRequest.changed` paths: hub sends what it touched, agent keeps them | tiny, and both later phases need it |
| 2 | Agent-side Navidrome client: login, `GET /api/library`, `startScan` with a target, full-scan fallback | closes the loop: tag a file, Navidrome sees it |
| 3 | Agent reports Navidrome's album list too (`/api/album` paged), for libraries whose roots the agent does not scan | covers the "my agent only sees part of the library" case |
| 4 | Per-album songs → quality, so "check for better" compares against Navidrome | turns a boolean into a decision |
| 5 | Move publish and tagging to the agent (topology C) | the real fix for remote agents; make it unnecessary to share volumes |

Rough size: paths ~40 lines, agent client ~200 with tests, album sync ~200,
quality ~80. Phases 1–2 are the useful half and do not touch the download
pipeline.

## Open questions

- **Multiple libraries / users.** Navidrome has per-user library access. The
  connector reads with the configured account, so it sees what that account sees;
  per-kyoku-user Navidrome accounts would be a later refinement.
- **Deletions.** A file removed from Navidrome should stop counting as owned. The
  sync replaces rows per `source`, so a missing album disappears — but only on a
  full sync, not on a targeted scan.
- **Quality ladder.** Navidrome's `bitDepth`/`sampleRate` is authoritative for its
  files; kyoku's own probe is authoritative for what a provider offers. Both map
  onto `downloader.AudioQuality`, so the comparison is like-for-like.
