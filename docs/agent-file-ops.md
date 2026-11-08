# File operations belong to the agent

The hub is hosted by us and shared; every user runs at least one fs agent next to
their own files. That single sentence invalidates an assumption the download
pipeline was built on: that the hub can open, write and move the files it
downloaded.

It cannot. `DOWNLOAD_DIR` is our disk, not the user's. So everything that touches
a file has to happen on the agent — download, tag, cover, publish, re-tag — and
the hub keeps only what it is good at: deciding what to fetch and what the tags
should say.

This is topology C from `downloader-plan.md`, which was chosen early and then not
built. With a shared hub it stops being a preference.

## The split

| question | who answers it | why |
| --- | --- | --- |
| does the user own this album | agent (scan) | it can read the files |
| what does the provider offer for it | hub (probe) | needs the account credentials |
| which edition, which recording ids | hub (MusicBrainz) | one cache and one rate budget, shared by every user |
| what bytes go in the file | hub (computed) | tags are a decision, not an observation |
| fetching the audio | agent | the bytes must land on the user's disk |
| writing tags, cover, folder layout | agent | the files are there |
| telling Navidrome | agent | same host, credentials stay local |

## Consequences

**Deezer credentials stay on the hub.** The probe and the ticket resolution need
the ARL; the CDN fetch needs only the resolved URL. Bytes flow agent → Deezer,
never through the hub. That was the point of the topology and it finally matters:
a shared hub carrying every user's audio would be a bandwidth bill, not a product.

**MusicBrainz belongs to the hub.** One rate budget, one cache, deduplicated
across users: the same album fetched by two people is one set of requests. A
per-agent implementation would multiply requests by the number of users for no
gain.

**The agent stays dumb about metadata.** It receives the tag manifest — title,
artist, album, dates, ids, cover bytes — and writes frames. It does not decide
what the album is called, which edition it is, or whether a tag is worth keeping.

**Except when it must read.** A re-tag needs what the file already says (the ids
that must survive a failed lookup, the track number, the ISRC). The agent reports
those on the way in, the hub resolves on top of them, and the manifest comes back.

## The contract

Agent → hub, when a job is picked up:

```json
POST /api/agent/jobs/{id}/claim
{ "files": [{ "path": "Ado/残夢/01 - Nukegara.flac", "isrc": "JPPO02402979",
              "track": 1, "disc": 1, "release_track_id": "..." }] }
```

Hub → agent, the manifest (also over HTTP, so a 100 MB job is not a websocket
frame):

```json
{
  "album_dir": "Ado/残夢",
  "staging_dir": ".staging/8f2c…",
  "cover": "base64…",
  "tracks": [
    {
      "dest": "Ado/残夢/01 - Nukegara.flac",
      "url": "https://cdnt…",         // short-lived, provider-specific
      "headers": { "…": "…" },        // whatever the CDN needs
      "expected": { "ext": ".flac", "bytes": 41123456 },
      "tags": { "TITLE": "Nukegara", "MUSICBRAINZ_TRACKID": "…", "…": "…" }
    }
  ]
}
```

Agent → hub, progress: `track.progress`, `track.done`, `track.failed`,
`album.published`, with the same envelope the scan already uses.

The manifest is deliberately provider-agnostic: the agent never learns what
Deezer is, it learns "these bytes, this URL, these tags".

## Phasing, with the dev stack intact

Each step leaves the current single-machine setup working, by keeping a
**hub-local agent** for development: the same code path, embedded in the hub.

1. **Share the tag writer.** Move `internal/download/postprocess` to
   `packages/tagwriter` so both sides can write Picard-compatible tags. No
   behaviour change.
2. **Manifest + claim endpoints.** Hub computes the manifest it would have
   written itself; agent fetches it. Nothing consumes it yet.
3. **Agent downloads and tags.** `packages/tagwriter` + a fetch step in the
   agent; the hub stops writing files when the job targets an agent, and keeps
   writing them itself when it targets the embedded one.
4. **Publish and re-tag move too.** Same manifest shape, `mode: "retag"`.
5. **Navidrome trigger** on `album.published` (see `navidrome.md`).
6. **Delete the hub-local path** once the agent is the only writer.

Steps 1–3 are the load-bearing ones; 4–6 are smaller and become easy after them.

## What this costs

Honest estimates for a codebase this size:

- extracting the tag writer and its tests: mechanical, an afternoon;
- manifest/claim plumbing (proto, endpoints, agent client, job state): the bulk,
  a day or two;
- agent-side fetch with progress and verification: a day;
- re-tag through the agent: half a day, because the manifest is already there.

The risk is not size, it is the temptation to keep a second path "for
convenience". Two writers means two sets of tag bugs, and the ones we spent
today chasing (blanked ids, missing artwork, half-tagged albums) are exactly what
a divergent second path produces. The hub-local agent should run the *same* code,
not a parallel copy of it.
