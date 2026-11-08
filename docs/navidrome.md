# Navidrome support

How kyoku can know what a Navidrome library already holds, and how a download can
end up in it, without mounting anybody's storage.

## The problem with pointing the fs agent at the folder

It looks like the cheap answer and it hides three things:

1. **The agent has to be able to write.** Re-tagging rewrites files in place. If
   the agent mounts Navidrome's music volume read-only (the safe way to mount
   someone else's library), the whole re-tag feature stops working there.
2. **It assumes the same machine or a shared mount.** In a homelab the hub, the
   agent and Navidrome are usually three containers on two hosts. NFS/SSHFS is
   more moving parts than an HTTP call, and a dead mount looks exactly like an
   empty library.
3. **It answers the wrong question.** "Is this album in my library?" is a
   question Navidrome has already answered, with MusicBrainz ids, for every
   album it scanned. Re-deriving it by walking files duplicates work and gets
   different answers when the two tag readers disagree.

The folder is still the right place for *writes* (kyoku publishes into a folder
Navidrome watches). It is the wrong place for *reads*.

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

Two directions, one credential.

### Read: a library connector

Not an fs agent. A `library_albums` table holding what Navidrome reports:

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

### Write: publish, then ask for a scan

After `album.published`:

1. kyoku publishes into `DOWNLOAD_DIR`, which is a folder Navidrome watches.
2. Hub calls `startScan?target=<absolute path>` (admin credentials).
3. Nothing else. No rescan of the whole library, no waiting for a poll interval.

If the folder is *not* on Navidrome's side (different host without a shared
volume), the targeted scan returns "not under any library" and the log says so —
that is a configuration mistake worth surfacing, not hiding.

### Credentials

One Navidrome account with admin rights (scan is admin-only). Stored like the
Deezer ARL: an encrypted row in `provider_credentials`, never sent to the
frontend, never logged. A non-admin account works for reads only — say so in the
settings form rather than failing at scan time.

## Settings

Settings → Connections, next to Deezer:

```
Navidrome URL      https://music.example.lan
Username           kyoku
Password           ••••••••
[Test connection]   → "412 albums · admin · scanned 3 minutes ago"
```

The test button is one `/api/album?_start=0&_end=1` plus `getScanStatus`, which
answers everything the user needs to know before trusting it.

## Phases

| phase | what | why first |
| --- | --- | --- |
| 1 | Navidrome client + login + paged album sync into `library_albums`, shown as "in Navidrome" on the album grid | answers the actual question; read-only, cannot break anything |
| 2 | Ownership union in `GetArtistReleaseGroups` / `CountOwned` | makes the grid correct instead of duplicated |
| 3 | Per-album song fetch → quality, so "check for better" compares against Navidrome | turns a boolean into a decision |
| 4 | `startScan` after publish, targeted when supported | closes the loop |
| 5 | (optional) Subsonic-only fallback for other servers | portability, not needed for a homelab |

Rough size: client and auth ~200 lines with tests, sync job ~200, ownership union
~60, settings form ~80, scan trigger ~60. Phase 1–2 is a day's work; nothing here
touches the download pipeline.

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
