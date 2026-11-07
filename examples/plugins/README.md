# kyoku plugins

Post-download steps that run on their own process. Drop a directory into
`$PLUGINS_DIR` (default `~/.config/kyoku/plugins`) and restart the hub.

    plugins/replaygain/
      plugin.yaml      # what it is, which hooks it wants
      replaygain.sh    # any executable: shell, python, node, a Go binary

## plugin.yaml

```yaml
name: replaygain
description: EBU R128 album gain via loudgain
command: ["./replaygain.sh"]   # argv; relative paths resolve against this dir
hooks:
  - album.beforePublish
  - track.tagged
timeout: 120s                  # default 60s, whole process group is killed
required: false                # true = a failure blocks the publish
extensions: [".flac", ".mp3"]  # optional filter, omit for every format
```

## Hooks

| Hook | When | Payload |
|---|---|---|
| `track.downloaded` | file on disk, before kyoku tags it | `track` |
| `track.tagged` | after tags and artwork are written | `track` |
| `album.beforePublish` | every file staged, before the move into the library | `files[]` |
| `album.published` | album is in the library | `files[]` with final paths |

`album.probed` exists in the code but is not dispatched yet.

A plugin only runs for hooks it lists. Files it creates in `album.dir` during
`album.beforePublish` are published with the album; it must not move, rename or
delete the audio files themselves.

## Protocol

Host → plugin: one JSON document on stdin.

```json
{
  "hook": "album.beforePublish",
  "plugin": "replaygain",
  "app": { "name": "kyoku-hub", "version": "0.1.0" },
  "album": {
    "title": "Raven Sky", "artist": "Ado", "year": "2026-10-15",
    "genre": "Asian Music", "label": "Universal Music LLC",
    "releaseType": "single", "releaseGroupId": "8b8ed50b-…",
    "dir": "/music/.staging/<job>", "trackCount": 1,
    "coverPath": "/music/.staging/<job>/cover.jpg"
  },
  "files": [ { "path": "…/01 - Raven Sky.flac", "ext": ".flac" } ]
}
```

Track hooks carry a `track` object instead of `files`: `path`, `ext`, `title`,
`number`, `trackTotal`, `disc`, `discTotal`, `isrc`, `musicbrainzTrackId`,
`musicbrainzArtistId`, `musicbrainzAlbumId`.

Plugin → host: optional JSON on stdout, anything on stderr.

```json
{ "message": "applied album gain -7.4 dB", "files": ["01 - Raven Sky.lac"] }
```

`message` is logged next to the plugin name. Exit codes: `0` ok, `3` skip,
anything else failure.

## Environment

Parsing JSON in `sh` is the fastest way to make a plugin unpleasant to write, so
the fields a shell script usually needs are mirrored as variables:

| Variable | Value |
|---|---|
| `KYOKU_HOOK` | the hook being dispatched |
| `KYOKU_PLUGIN` | plugin name |
| `KYOKU_APP_VERSION` | host version |
| `KYOKU_ALBUM_DIR` | album directory (staging before publish, library after) |
| `KYOKU_ALBUM_TITLE`, `KYOKU_ALBUM_ARTIST` | album metadata |
| `KYOKU_RELEASE_GROUP_ID` | MusicBrainz release group |
| `KYOKU_TRACK_PATH`, `KYOKU_TRACK_TITLE`, `KYOKU_TRACK_ISRC` | track hooks only |

Plus `PATH`, `HOME`, `TMPDIR`, `LANG`, `LC_ALL`. Nothing else is inherited.

## Shipped examples

- `hello/` — the minimum: prints what it received, writes one file.
- `replaygain/` — real ReplayGain 2.0 via
  [rsgain](https://github.com/complexlogic/rsgain):

      rsgain easy -m MAX -S -q "$KYOKU_ALBUM_DIR"

  `-S` makes it idempotent (a retried download does not rescan), and because it
  runs on the whole album folder the files get `REPLAYGAIN_ALBUM_GAIN` as well as
  the per-track values. It runs at `album.beforePublish`, so kyoku's own tagging
  has already happened and rsgain's keys are never overwritten — kyoku's writer
  only owns the keys it writes.
