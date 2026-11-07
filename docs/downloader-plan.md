# Downloader & Acquisition Backlog

Derived from a review of `feat/downloader` against SoulSync (github.com/Nezreka/SoulSync,
music side only). SoulSync is a co-located single-install Python monolith — its
architecture does not transfer, its failure catalogue does. Every item below is a
rule it learned the hard way, restated for this repo.

Scope: album/release-group downloads from streaming providers (Deezer, Tidal, ...).
Not movies, not playlists, not scrobbling.

Legend: [ ] todo, [~] in progress, [x] done. P0 = data loss / wrong output.

## Status

Built and tested (`go test ./packages/downloader/... ./apps/hub/backend/internal/download/`):

| Piece | Where |
|---|---|
| Comparable quality, ranking, effective target, floor | `packages/downloader/quality.go`, `album.go` |
| Per-account throttle (reserve-slot, cooldown) | `packages/downloader/throttle.go` |
| Deezer probe (per-track, account ceiling) + per-track download | `packages/downloader/deezer/probe.go`, `source.go` |
| Walk: search → probe → fit/partial/miss → download | `apps/hub/backend/internal/download/manager.go` |
| Decisions (`track_failed`), dedupe, cancel, ring-buffered log | same |
| Staging + atomic publish + tier-1 verify | `manager.go`, `verify.go`, `paths.go` |
| Post-processing chain (tags, cover) + MusicBrainz enrichment | `postprocess/`, `enrich.go` |
| Job API + SSE snapshots | `internal/handler/download.go`, `sources.go` |
| Modal wired to real jobs | `app/stores/download.ts`, `app/pages/download/rg/[id].vue` |

Not built yet, in the order they matter:

- **0c transfer.** The agent still does not receive bytes: the hub downloads and
  publishes into `DOWNLOAD_DIR`. Everything upstream of the transfer is real, so this is
  now a protocol change (`DownloadTicket` request/response + `TrackDone`), not a redesign.
- **MP3 tagging.** FLAC is tagged; a 320 lands with no ID3. Blocked on nothing but time.
- **Cover embedding.** A `cover.jpg` sidecar is written; nothing is embedded in the file.
- **5a library presence / upgrade**, **5c query relaxation**, **6 extension** — untouched.


---

## 0. Decision required: where do the bytes land?

`runDownload` writes to `DOWNLOAD_DIR` on the hub box (`apps/hub/backend/internal/handler/download.go:147`),
then calls `agentManager.RequestSyncAgent` (`:174`). The agent rescans its own library
roots. Nothing carries the file from hub disk to the agent's library:
`proto/fs/v1/fs.proto` defines only Challenge / AuthResult / SyncRequest / TrackEntry /
ScanResult, and `apps/fs` contains no download or transfer code.

So today a "successful" download is stamped with MUSICBRAINZ_RELEASEGROUPID and then
stays on the hub. That only looks like it works when `DOWNLOAD_DIR` happens to be the
library root (same box, or a bind mount).

Options, in increasing order of effort:

### 0a. Zero code — point DOWNLOAD_DIR at a mount of the agent's library
- [ ] Document it, and make the handler fail loudly when the destination is not
      writable / is inside the hub container's own layer.
- Pro: works today, no protocol change.
- Con: no mount across NAT, mount failures are silent and nasty, hub still needs the
  disk, hub becomes a single point for library writes.

### 0b. Hub streams the file to the agent over the existing WS
New envelope messages: `FileOffer{transfer_id, album_id, path, size, sha256}`,
`FileChunk{transfer_id, offset, bytes}`, `FileDone{transfer_id}`, `FileAck{offset}`.
- Pro: agent stays a dumb file service; credentials, session heal, throttling and
  provider code stay in one place (the hub); no secrets on user machines; one
  implementation to maintain per provider.
- Con: every byte crosses the hub twice (provider -> hub -> agent). A 600MB album
  costs 1.2GB of hub traffic; a remote hub/VPS pays for it. Needs chunking,
  backpressure, resume, checksums, timeouts — a real protocol, on a channel that
  currently carries a few KB per scan. Hub restart kills in-flight transfers.

### 0c. Hub resolves a per-track "download ticket", the agent transfers  <-- recommended
Hub keeps the provider session (ARL, license token, gw-light calls) and resolves one
short-lived stream URL per track; the agent fetches it from the CDN directly,
decrypts (the Blowfish key is derivable from the track id — no secret), tags and
writes. New messages: `DownloadTicketRequest{album_id, track_id}` ->
`DownloadTicket{track_id, url, mime, expires_at, quality}`, `TrackDone{track_id, path, bytes}`.
- Pro: bytes go provider -> library, never through the hub. Credentials never leave
  the hub. Quota and throttling stay centralized (one budget per provider account).
  URL expiry is a feature: the hub stays in the loop per track, so it can cancel,
  re-issue, retry, and observe progress. Landing next to the library makes staging
  + atomic rename + rescan local and cheap. Survives hub restarts (agent retries
  the ticket; hub has the job row).
- Con: one round trip per track (fine at 50-200ms). Agent needs the decrypt + tag
  code from `packages/downloader` (x/crypto + go-flac only, no auth). Two binaries
  to keep in version sync (shared Go module — acceptable). Agent needs egress to
  the CDN (it already has egress to the hub). Errors happen on a remote box, so a
  minimal job/log report back to the hub is required.

### 0d. Agent runs the whole provider
Hub sends `DownloadRequest{provider, album_id, quality, staging_dir}`.
- Pro: simplest split conceptually, fully local retries, parallel per agent.
- Con: provider credentials must reach user machines (solvable — the agent already
  has an ed25519 keypair, so the hub can encrypt a scoped grant to it — but it is
  still a secret on a NAS). Rate limits become distributed: N agents sharing one
  account blow the provider quota in a way no per-process throttle can see. Every
  agent inherits provider breakage and needs updating when Deezer changes.

Recommendation: 0c. It is the only option that keeps credentials and quota in one
place *and* keeps bytes off the hub, and it fits the keypair/WS model that already
exists. Do 0a now as a stopgap, but decide 0c vs 0b before the `Provider` interface
grows more `destDir` arguments.

---

## 1. P0 — correctness fixes in the current branch

Data-loss or wrong-output class. Each is small and independent.

- [ ] **Disc-safe filenames.** `packages/downloader/deezer/deezer.go:141` builds
      `%02d - Title.ext` into one flat album dir, so disc 2 track 1 overwrites disc 1
      track 1. `DiskNumber` is parsed in `getTracksForAlbum` and then thrown away.
      Tidal (`tidal/tidal.go:156`) uses the array index — no collision, but the track
      number is wrong on every multi-disc release.
      Fix: shared `TrackFileName(disc, track, title, ext)` in `packages/downloader`,
      `%02d-%02d - Title.ext`; use the real `TrackPosition`/`TrackNumber` on both
      providers. Acceptance: a 2-disc album yields 2N distinct files.
- [ ] **Tag every container, not just FLAC.** `deezer.go:148` tags only
      `if ext == ".flac"`; `tags.go` is FLAC/Vorbis only. Deezer Free/Premium users get
      untagged MP3 — and the whole fs-agent pipeline is tag validation, so those files
      land as permanently invalid tracks. Tidal downloads are not tagged at all.
      Fix: `packages/downloader/tag` with a container switch (FLAC/OGG vorbis,
      MP3 ID3v2.4, M4A atoms later); download loops call it unconditionally.
      Also this is where cover art embedding belongs (see 5b).
- [ ] **Verify the bytes after transfer.** `deezer.go:180 downloadTrack` and
      `tidal/tidal.go:166 downloadFile` return nil as soon as the writes finish.
      Tier 1: minimum size (~100KB), container header parse, decoded duration vs
      provider/`TRACK_TOKEN` duration (±2s). Tier 2 (opt-in): full FLAC decode
      checking STREAMINFO MD5. A truncated CDN response is currently a success.
- [ ] **Progress is user-private.** `download.go:150,163` uses `sse.Broadcast`, while
      `internal/sse/sse.go:41` says Broadcast is never for user-private data. Use
      `SendToUser(userID, ...)`; include `download_id` in the payload.
- [ ] **Cache providers per (user, provider).** `download.go:60 getProviders` builds a
      new `deezer.New(...)` + `http.Client` on every call, so no connection reuse, no
      shared session heal, and a token refreshed at runtime is discarded.
- [ ] **One throttle budget per provider account.** `deezer.go publicAPICallInner`
      sleeps 5s and retries once on body code 4/700, per request, per goroutine. Two
      concurrent users multiply that. Port the reservation model: lock-protected
      "reserve next slot, then sleep", ~1 call / 125ms sustained, plus a cooldown on
      code 4. Deezer signals quota as **HTTP 200 with a body error**, so status-code
      checks never see it.

## 2. P1 — the contract: quality as a value, sources as plugins

- [ ] **`packages/downloader/quality.go`** — the thing that makes the rest cheap:
      ```go
      type AudioQuality struct{ Format string; Bitrate, SampleRate, BitDepth int }
      type QualityTarget struct{ Format string; MinBitrate, MinSampleRate, BitDepth int }
      func Rank(a AudioQuality, targets []QualityTarget) (idx int, score float64)
      ```
      Plus `QualityLabel()` and `AudioQualityFrom{Extension,Deezer,Tidal,Qobuz}`.
      Today `SearchResult.Quality` is a bare string with three vocabularies
      ("FLAC" from Deezer, "LOSSLESS"/"HI_RES" from Tidal, "FLAC"/"MP3_320" in
      `Config`), and `SearchResult` cannot be compared across providers at all.
- [ ] **Source chain — ordered sources, draggable.** This is a separate axis from the
      quality targets, and both are needed. Model it on SoulSync's `SourcePolicy`
      (`core/downloads/source_policy.py`): `mode` (single | chain), `search_mode`
      (`priority` = probe in order, first satisfying hit wins | `best_quality` = ask
      all, rank candidates), `chain` (ordered source names), `rank_candidates_by_quality`.

      UI (Settings → Sources), same two-pane idea as SoulSync:
        left  = registry sources that are not enabled, each with a "Setup" action and
                a capability line ("needs ARL", "device auth", "no auth")
        right = enabled sources, drag to reorder — label this "preferred on ties"
                (quality rank wins first, this order decides equal-quality candidates);
                each row shows its *ceiling* ("max FLAC 16/44.1" for Deezer, "max Opus
                256" for YouTube) and what it would request for the current top target.
                The quality ladder editor sits directly above this list — the two
                together are the whole preference model.
      The drag order is a manual override on top of a **computed default**: sort by
      ceiling vs the user's top target, descending. Deezer before YouTube is then not
      folklore, it is the default the ladder produces — and a user who prefers Deezer
      even at 16/44.1 can drag it above Tidal and it sticks.

      Two rules make the order actually mean something (without them, position alone
      dictates quality and YouTube first = Opus for every album):
        1. skip any source whose ceiling cannot satisfy the top target
           (Deezer can never do 24-bit; YouTube can never do lossless)
        2. within a source, request the *cheapest tier that satisfies* the top target
           (Deezer: 128 / 320 / FLAC; Tidal: LOW / HIGH / LOSSLESS / HI_RES) — do not
           default to max
      Probe budget: priority mode costs at most one search call per source until a hit,
      so cap the chain length walk (e.g. stop after N misses) and reuse the search
      result when the user then clicks download.

      Storage: per-user (`users.sources jsonb` or a `user_sources` table with
      `position`), seeded from the registry on first login.

- [ ] **Ranking rule — one comparator, two search budgets.** Chain order must *not*
      outrank quality. Single sort key for any candidate (album, from one source):
      ```
      (targetIndex, -tierScore, chainIndex)
      targetIndex = index of the best-ranked QualityTarget the candidate satisfies
      tierScore   = AudioQuality.tier_score() within that bucket (96k vs 192k)
      chainIndex  = user's drag position, tie-break only
      ```
      So: Tidal 24/96 (target #0) beats Deezer FLAC 16/44.1 (target #3) regardless of
      chain position — the user already expressed that preference in the ladder. When
      two sources satisfy the *same* target (Deezer FLAC 16/44.1 vs Tidal LOSSLESS
      16/44.1), the achieved target ties and the drag decides. That is the whole
      purpose of the drag; `priority` vs `best_quality` then differ only in how many
      sources get probed (stop at first satisfying hit, vs ask all), not in how
      results are compared.
- [ ] **Classify against the source's ceiling, not rung 0.** The ladder is a preference
      order, not a set of requirements. Deezer's ceiling is FLAC 16/44.1, which satisfies
      rung 3 — so a uniform FLAC 16 album is a **FIT** and downloads without asking.
      Measuring against rung 0 would make every Deezer album PARTIAL and turn the results
      view into a toll booth. `effectiveTarget(ceiling, targets)` = the highest rung the
      ceiling satisfies; an unknown ceiling falls back to rung 0 (strict) because the one
      failure worth preventing is silently accepting a 320 as if it were perfect.
      Cross-source quality is still the ranking (§ ranking rule), not this verdict.
- [ ] **Rank albums, not tracks, and use the album floor.** A candidate's quality is
      the *weakest* track the source can deliver for the album (or its album-level
      claim, verified on probe) — never the best track. If Tidal yields 11 tracks at
      24/96 and one at 320, that album is a 320 candidate. Otherwise a single missing
      track silently mixes two sources inside one album: two masters, two tag sets,
      two loudness targets.
- [ ] **Probe results can veto the chain.** "Satisfies the target" is only known after
      the probe (Tidal LOSSLESS per country, Deezer geo-restrictions, account tier).
      Treat chain order as "who to ask first, who wins ties", and let the probe
      downgrade or disqualify a source per album.
- [ ] **Equal candidates, no preference: say so.** When neither source is preferred and
      both land on the same target, fall back to the seeded order (ceiling sort, then
      registration order) — deterministic, but surface it in the UI so a drag between
      two equals visibly changes the winner. Optional per-source free-text note
      ("prefer Tidal masters for this label") so taste survives the next reorder.

- [ ] **The ladder needs a bottom rung or fallback is dead weight.** If the target list ends
      at FLAC, a YouTube offer can never be PARTIAL — it is MISS, and the fallback wave never
      produces a card (the "try YouTube last" branch becomes unreachable). Add an explicit
      final entry ("any audio", or lossy >= 128) if users are expected to accept downgrades;
      otherwise the UI should say fallback sources exist for logging only.

- [ ] **Provider capabilities + registry (plugins).** Replace `map[string]Provider`
      with a registry of `Provider` implementations declaring
      `Name() string`, `Configured() bool`, `Available() bool`, `CheckConnection()`,
      `Qualities() []AudioQuality`, `SearchAlbums()`, `Album()` — so a new source is
      one package + one registration line, and the settings UI is generated from the
      registry instead of hardcoded (`connections.go:52` reports deezer AND tidal,
      but only deezer can be upserted at `:70`; the UI renders both).
- [ ] **Expose the tier actually granted.** Deezer falls back FLAC -> 320 -> 128
      inside `getStreamURL` and never tells the caller. Store `requested` vs `actual`
      on the job and surface it — `SearchResult.Quality` is a promise the download
      does not keep.

### 2.1 How an album's quality is actually determined

Neither provider hands you "this album is FLAC". Both must be asked per track, and the
answer is an *estimate* until the bytes land:

    Deezer   NO album-level quality anywhere. `/album/{id}` returns nb_tracks,
             record_type, release_date, label — no bitrate, no filesize; `/track/{id}`
             has none either (verified against the live API). The only source of truth
             is the private gw-light `song.getData`, per track, as FILESIZE_FLAC /
             _MP3_320 / _MP3_128, where "0" means unavailable on this account. So a
             Deezer probe is N authenticated calls per album (~1.5s for 12 tracks at the
             existing throttle).
    Tidal    advertises `audioQuality` on the album AND on each track item
             (LOSSLESS / HI_RES_LOSSLESS / HIGH / LOW). Treat that as a claim: the only
             ground truth is `streamUrl?soundQuality=`, which will quietly hand back a
             lower tier.

- [ ] **Search by catalogue title, not cover title.** A provider keeps the name it
      was licensed under: Deezer files 椎名林檎 as "Sheena Ringo" and 三毒史 as
      "triviṣa-itihāsa", so the exact pair returns zero results for an album that
      is right there. When the direct search comes back empty, resolve the artist
      and list their discography (`downloader.ArtistSource`), and do not re-filter
      those by name — a lexical compare across scripts would throw away the hit.
      Track counts are missing from Deezer's artist listing, so they are filled in
      per album for the rows that will be shown (12 calls, ~2s, throttled).

- [ ] **Probe per album, never per search result.** The probe costs N calls on Deezer,
      so it runs for the one album the user is about to download, after the click — not
      for the 10 covers on the search page. Cache by `(provider, album_id, account)` with
      a TTL (days, not minutes); an album's track qualities barely move.
- [ ] **Floor + count, never a majority vote.** `floor` = the minimum across probed
      tracks; `okTracks` = how many meet the top target. "11/12 FLAC" is materially worse
      than "12/12" and must not be rounded up by a vote. This is what §3.1 classifies as
      PARTIAL and what the results expander renders per track.
- [ ] **Unknown is not zero.** A track whose probe errored (gw-light non-200, geo,
      timeout) is `unknown`, not "no FLAC". Do not demote the album for it: mark the
      candidate `unverified` and let the transfer decide. Otherwise one flaky call turns
      a perfect album yellow.
- [ ] **Singles are the rule, not an edge case.** For a 1- or 2-track release the floor
      is the min of one or two, so a single 320 track makes the whole release PARTIAL.
      That is correct — a single has no redundancy — and it is why singles will land in
      the results view far more often than albums.
- [ ] **A FIT can be revoked by reality.** If the probe said 14/14 FLAC and track 7 comes
      back at 320 during transfer, the silent path was wrong: revoke the FIT, convert the
      run to the results path, and let the user choose with the truth in hand. The rule
      is "silent only when perfect", so anything that stops being perfect stops being
      silent.


## 3. P2 — job model (queue, cancel, staging)

- [ ] `downloads` table: `id, user_id, agent_id, rg_id, provider, provider_album_id,
      requested_quality jsonb, status, tracks_total, tracks_done, error_code,
      error_detail, staging_dir, candidates jsonb, created_at, started_at, finished_at`.
      Status: `queued | probing | awaiting_pick | downloading | verifying |
      publishing | done | failed | cancelled`.
- [ ] `download_decisions` table: `id, download_id, kind, payload jsonb, options jsonb,
      chosen, chosen_at, expires_at, default_choice`. Kinds: `pick_release | track_failed`
      only. Persisted, not in-memory — a job waiting on the results view must survive the
      tab closing.
- [ ] In-memory registry keyed `(provider, download_id)`, mirroring SoulSync's
      `DownloadEngine` — per-source locks, shallow-copy reads, atomic patches.
- [ ] Replace `go h.runDownload(...)` (`download.go:142`): dedupe on
      `(user, rg, provider, album)` so a double click does not run two goroutines into
      one directory; cap concurrency per user and per agent; `POST /api/download/:id/cancel`;
      `GET /api/download/jobs`; `POST /api/download/:id/decide`.
- [ ] Staging + atomic publish: write to `<downloadDir>/.staging/<id>/`, move to the
      final path only when every track verified, quarantine the batch otherwise.
      Media servers / the fs agent must never see half an album. Note the staging tree
      has to live *inside* the final filesystem root — a sibling dir is a different
      filesystem under bind mounts.
- [ ] Persist a per-track failure reason (`error_code` + human text) — the difference
      between "ARL expired", "geo-restricted", "no lossless on this account" and
      "provider does not have the album" is what makes the UI actionable.

### 3.1 Interactive flow (download modal)

Prerequisite: **blocked on P1**. The fit/partial classification, the album floor and the
per-source summary all need `AudioQuality`/`QualityTarget` — a `Quality string` cannot
answer any of them.

Layout: the modal is split — **left = job output (log)**, **right = source list in chain
order**. Clicking Download starts the walk immediately; nothing waits on the user.

One status enum per source drives the colour and nothing else:

    pending   none      queued behind the running source
    running   spinner   being probed, then downloaded
    fit       green     every track at/above the top target AND identity matches
    partial   yellow    usable, but at least one track is below the top target
    miss      gray      cannot reach the ladder, or identity is wrong (reason logged)
    failed    red       auth / geo / quota / network (reason logged)

Rules:
  - **FIT stops the walk** (green) and downloads immediately. No prompt.
  - **PARTIAL never stops the walk** (yellow). Recorded as a candidate, then on to the
    next source. No prompt, no interruption — the run is unattended until it ends.
  - **MISS is logged and skipped.** A gray row never offers anything.
  - The results view opens when the walk ends **with no FIT** (Source order mode) — or
    always in **Best quality** mode, where the user explicitly asked for a comparison. The
    left log pane collapses (slide) and the right source pane expands to full screen.

- [ ] **Source status is a single enum on the job**, streamed as one `download:source`
      event per transition (`{download_id, provider, status, reason, quality_summary}`).
      The pane is a projection of those events — same list in both layouts, no derived
      colour logic in the component.
- [ ] **Delete the mid-chain prompt.** Only two decision kinds remain: `pick_release`
      (end of walk) and `track_failed` (after acceptance). `partial_fit` and
      `match_ambiguous` disappear — a deviation becomes a yellow row, and the judgement
      moves to the results view where the user can compare instead of answering blind.
- [ ] **No separate fallback wave.** Whether YouTube is gray or yellow is decided by the
      bottom rung of the target ladder: add a lossy rung ("any audio" / lossy >= 128) and
      YouTube is PARTIAL and offerable; omit it and YouTube is MISS and logged only. One
      rule instead of two special-purpose waves.
- [ ] **Results view opens full width.** The log pane collapses to `0fr`, the source pane
      takes the whole body. Per candidate: cover, provider, Fit/Partial chip, title · year ·
      track count, the one-line summary, the deltas, `Show N tracks`, and `Use this`; a
      Quit in the footer. Nothing is auto-picked and Quit publishes nothing.
- [ ] **Peek mode is colours only.** The chevron brings the log back; the right column then
      renders the same compact source list as the walk (dot, name, Partial/Failed chip, one
      truncated summary line) plus a single `Show N candidates` button — no cards, no
      deltas, no expander. 320px is enough for state, not for a comparison. This is the
      "keep colours, don't show info" rule from the original sketch.
- [ ] **Per-track expander** (full-width view only): `Show N tracks` expands the card into
      one line per track, in the style of a build log —
      `✓ [6] Nightvision → FLAC • 44100 Hz • 16-bit • stereo • 1135 kbps • 40.9 MB`.
      Green check = at/above target, amber alert = below target. Omit fields a format
      cannot have (no `16-bit` on an MP3); show hi-res specs as delivered, so a 24/96 card
      is checkable against a 16/44.1 one. This is the album floor made auditable: the
      summary says "14/14", the expansion says which track is the 320.
- [ ] **Summary copy convention** (learned the hard way):
      `14/14 songs · FLAC 16/44.1` (fit) / `14/14 songs · floor MP3 320` (partial) /
      `1/1 songs · MP3 320` (single). Lead with completeness, put quality second, and carry
      the count-at-target in the deltas instead of the headline — `0/1 FLAC` or `13/14 FLAC`
      on a 1-track release reads as nonsense.

- [ ] **Retain candidates on the job** — provider, provider album id, per-track quality
      map, identity confidence, year — so the results view renders without re-probing.
      This is why `ProbeAlbum` returns per-track quality: deltas are the thing a user can
      choose between, "quality mismatch" is not. Re-probe only past a TTL.
- [ ] **After a pick, slide back to the split layout.** FIT and `track_failed` decisions
      already live there; the per-track progress and log belong in that pane, and a
      full-screen results view with a download running behind it has nowhere to report.
- [ ] **Durable wait.** While results are up, `status = awaiting_pick` in Postgres with
      the candidate rows. A closed tab must not discard probe work; the job stays pending
      until picked, dismissed or cancelled, and reopening the modal rejoins it.
- [ ] **`track_failed` options** (after acceptance, one track dies mid-download):
      [Skip track, album incomplete] / [Restart album on <source>] /
      [Fetch just this track from <other source> — mixes sources] / [Cancel].
- [ ] **Never mix sources inside one album by default.** Cross-source track fetch is the
      third, explicitly labelled option above — never the default; one folder with two
      masters, two tag sets and two loudness targets is worse than one missing track.
- [x] **State stream over the existing SSE channel.** Built as **one** `download:job`
      event carrying the whole `JobView` snapshot (via `SendToUser`), not five granular
      events: the client already takes a job object, so a snapshot cannot desync and needs
      no reducer. Cost is a few KB per track transition, which at album scale is nothing.
      The per-job log ring buffer (200 lines) lives in the snapshot, so a reopened modal
      replays what it missed — SSE itself does not replay.
- [ ] **Decision audit log.** `kind`, `options`, `chosen`, `chosen_at` per row is what
      makes "why did it pick Deezer" answerable after the fact.
- [ ] **Mode toggle** in the modal: `[ Best quality ] [ Source order ]`, default
      Best quality, one line of helper text. It changes probe count and whether the
      results view can contain a FIT, not the comparison rule:
        Source order  walk the chain, stop at the first FIT; results view only if no FIT
        Best quality  probe all enabled, rank (targetIndex, -tierScore, chainIndex);
                      results view lists everything, FIT first



## 4. P3 — secrets and provider health

- [ ] Encrypt `provider_credentials.credentials_json` at rest (key from
      `KYOKU_SECRET_KEY` env or a file next to the DB). An ARL is a password.
- [ ] Redaction sentinel on the connections API: GET returns `"__redacted__"`, PUT
      treats that as "keep existing" (`apps/hub/backend/internal/handler/connections.go`).
      Currently `List` only returns booleans, which is already good — keep it that way
      and add `user`, `tier`, `checked_at` instead of the secret.
- [ ] Sign-in probe returning three outcomes: `ok | failed | unreachable`. Probe from
      a **fresh session** that holds only the ARL — a live session's `sid` answers for
      the old login no matter which ARL is set, so it cannot test a new one.
      `refreshCSRFToken` (`deezer.go`) currently collapses both into "ARL expired", so
      a Deezer outage tells users to paste a new cookie.
- [ ] Re-auth backoff 30s doubling to 30min, shared by all callers, and never retry a
      *write* (playlist create/add) after a token heal.
- [ ] SSE `provider:health` event so the UI can badge a dead connection.

## 5. P4 — features worth adding later

- [ ] **5a. Library presence + upgrade.** The download page should say "you own this
      at MP3 320" and offer an upgrade. You already have `fs_release_group_states` per
      rg and per-track validated tags — this is a join, not new scanning. Needs the
      quality type from 2 to compare owned vs targets.
- [ ] **5b. Cover art.** `SearchResult.CoverURL` already exists (250px Deezer/Tidal).
      Embed at 640px and write `cover.jpg` beside the album. Belongs in the tag
      package from 1.
- [ ] **5c. Match quality on the download page.** Query relaxation (strip
      `(feat ...)`, balance-aware bracket groups, truncate long titles) and a
      word-boundary version-marker filter (`remix`, `live`, `acoustic`,
      `instrumental`, `sped up`, `type beat`, `8d audio`) so a studio-track search
      stops offering edits. Then score results against the release group (track count
      vs `nb_tracks`, year, artist similarity) and show a "likely match" badge.
- [ ] **5d. Wanted/failed retry.** A failed download becomes a row with
      `next_retry_at` + reason instead of a one-shot SSE error. Backoff per row.
      This is the backbone for the browser extension in 6.
- [ ] **5e. AcoustID verification (optional, last).** Only worth it as a
      post-import job that flags "provider master != MB release group", never in the
      download path — provider IDs are authoritative at release-group granularity.

---

## 6. Browser extension — quick-add to "wanted"

Goal: from any page (label site, blog, Bandcamp, Discogs, Last.fm, YouTube, a store),
one click adds the artist to your tracked artists, with no copy-paste and no manual
MusicBrainz lookup. The hub does the rest: brainz syncs the discography, release groups
appear on the artist page, and you pick what to download.

### 6.1 Hub prerequisites

"Wanted" = flag an artist so it lands in the artist db and the existing sync loop fills
in their discography; the user then downloads whatever they choose. That already exists:
`POST /api/artist/add {mbid, name, disambiguation}` (`apps/hub/backend/internal/handler/artist.go:71`)
upserts the artist, links it to the user, DB-rate-limits adds (`maxAddsPerHour`) and
sends the brainz subscription. So **no new table and no resolver worker** — the
extension is a remote control for the Add Artist modal.

- [ ] **Bearer auth for extension calls.** `mw.AuthRequired` reads the session cookie
      only, and the cookie is SameSite=Lax so it is not sent cross-origin. Add an
      `api_tokens` table (`id, user_id, token_hash, label, created_at, last_used_at,
      revoked_at`), reuse the `agent_tokens` shape already in `database.go:65`, and let
      `AuthRequired` also accept `Authorization: Bearer <token>`. Scope it to
      `/api/artist/*` so a leaked token cannot do anything else. Token shown once in
      Settings → Account.
- [ ] **Reuse the search endpoint.** `GET /api/artist/search?artistName=` (`artist.go:45`,
      currently unauthenticated, proxies brainz with pagination) is exactly the
      name → MBID resolver the popup needs. No new resolution logic.
- [ ] **Provenance (cheap, optional).** `user_artists.source TEXT DEFAULT 'ui'` +
      `source_url TEXT`, set to `'extension'` by the bearer path — so "where did this
      artist come from" is answerable, and the UI can badge extension-added artists.
- [ ] **CORS / host permission.** No server change needed if the request is made from
      the MV3 service worker with `host_permissions` for the hub origin; otherwise add
      the `chrome-extension://<id>` origin for `/api/artist/*` only.


### 6.2 Extension (Manifest V3, bundled code only, no remote scripts)

- [ ] Permissions: `activeTab`, `scripting`, `storage`, `contextMenus`,
      `optional_host_permissions` for the hub origin. Do not ask for `<all_urls>` at
      install — the picker works fine under `activeTab`.
- [ ] Onboarding: hub base URL + token, "test connection".
- [ ] Resolve artist name, in this order, first hit wins:
      1. **Site adapter** (~15 lines each, takes precedence over a remembered
         selector): bandcamp (`#name-section .from-merch`, `og:site_name`), discogs
         (`[itemprop=byArtist]`), last.fm (`h1.header-new-title` + "Artist - ..."),
         beatport (JSON blobs), musicbrainz (h1 on `/artist/*`), youtube/ytmusic
         (channel name in `ytd-channel-name` / `og:title`).
      2. **JSON-LD**: `script[type="application/ld+json"]` -> `MusicGroup`/`Person`
         `.name`, or `byArtist.name`, or `Album.byArtist`.
      3. **Microdata**: `[itemprop=byArtist] [itemprop=name]`.
      4. **Heuristics**: `og:title` / `og:site_name` / `<title>` split on
         ` - `, ` | `, ` — `, `:`; strip boilerplate fragments ("Listen to", "on
         Spotify", "| Discogs", "music, videos, stats").
      5. **H1/H2** text.
- [ ] Popup shows each candidate with its source ("from JSON-LD", "from title
      heuristic") and a confidence dot, the chosen one in an editable input, plus the
      raw text in small print. Never auto-submit a low-confidence guess.
- [ ] **"Pick field" mode** (the uBlock-style bit):
      - [ ] Content script injects a full-viewport overlay div
            (`z-index: 2147483647`, `pointer-events: none`) and a 2px orange outline
            that follows `mousemove` (capture phase, use
            `e.composedPath()[0]` so shadow DOM works; skip the overlay itself and
            `html`/`body`).
      - [ ] Tooltip next to the cursor: `tag#id.classes` + first 60 chars of its
            text, so it is obvious what you are about to select.
      - [ ] Click captures the element: `preventDefault` + `stopPropagation`, esc
            cancels.
      - [ ] Selector generation: unique `#id` -> unique attribute
            (`[itemprop="byArtist"]`, `[data-testid=...]`) -> unique class chain ->
            `:nth-of-type` path from the nearest id ancestor. Validate with
            `root.querySelectorAll(sel).length === 1` before accepting; if >1, append
            `:nth-of-type(n)` and re-validate.
      - [ ] Persist in `chrome.storage.local` keyed by **origin** (default) or by a
            path glob the user can widen (`/artist/*`), with a 200-char sample text
            for later debugging: `{origin, pathGlob, selector, sample, updatedAt}`.
      - [ ] Popup shows "remembered for this site: `<selector>`" with **Forget** and
            **Re-pick**. A saved selector never overrides a site adapter unless the
            user explicitly re-picked on that site.
- [ ] Resolve against the **live DOM at popup-open time**, never cached text — SPAs
      (Spotify, YouTube, Discogs) re-render under you.
- [ ] **MBID shortcut:** if the page is a `musicbrainz.org/artist/<mbid>` page or links
      to one, skip name resolution entirely — validate the MBID with
      `validation.IsMBID` client-side and add it directly. Zero ambiguity.
- [ ] Submit flow: name (or MBID) -> `GET /api/artist/search` -> the popup lists the top
      matches with name + disambiguation + score, exactly like the Add Artist modal ->
      pick -> `POST /api/artist/add {mbid, name, disambiguation}`. Pre-select the first
      result only when it is unambiguous (single result, or score gap above the next).
- [ ] Submit states: "Added: <artist>" / "Already tracking <artist>" / "Slow down
      (rate limited)" from the existing 429 / "Pick the right artist" when several
      matches are plausible.
- [ ] Extras: `Ctrl/Cmd+Shift+K` shortcut for the popup, context menu "Add to kyoku"
      on a selection or a link (uses `info.selectionText` / `linkUrl`), badge showing
      the artist already tracked or not on the current page.


### 6.3 Deliberately out of scope (v1)
- Pushing playback/queue state, scrobbling, or recognising the currently playing
  track automatically (needs per-site tap-ins and stays fragile).
- Sync/merge of selectors across devices (`storage.sync` has tight quotas and the
  selectors are per-browser-cosmetic — revisit once people ask).

---

## 7. Decisions (settled)

- **Transfer topology: 0c** — hub resolves a per-track ticket, agent fetches from the
  CDN and writes. Secrets never leave the hub; bytes never cross it.
- **Quality is a comparable value** (`AudioQuality` + ranked `QualityTarget`), and the
  **source chain is a separate, draggable, per-user order**. Ranking is
  `(targetIndex, -tierScore, chainIndex)` — quality rank always wins, the drag only
  decides candidates that land on the *same* target. Album-level comparison uses the
  album floor, never the best track. `priority`/`best_quality` are probe budgets, not
  two comparators.
- **"Wanted" = add the artist to the artist db.** Reuse `POST /api/artist/add`; the
  brainz sync loop fills the discography. No wanted/release table.
- **The download modal is an unattended walk with one decision at the end.** Split layout:
  left = job log, right = source list in chain order, one status enum per source driving
  the colour (pending / running-spinner / fit-green / partial-yellow / miss-gray /
  failed-red). FIT stops the walk and downloads; PARTIAL is recorded and the walk
  continues without a prompt; MISS is logged and skipped. Only when the walk ends with no
  FIT does the left pane slide away and the right pane go full screen with the retained
  candidates and their per-track deltas (Use / Quit, nothing auto-picked). Two decision
  kinds total: `pick_release`, `track_failed`. No fallback wave — whether a source can be
  offered at all is decided by the bottom rung of the target ladder. Mode toggle is
  `[ Best quality ] [ Source order ]` — probe budget, not comparison rule.
- **P1 (quality model) is the blocking prerequisite** for the modal, the fit/partial
  classification and the album floor. Nothing in 3.1 works on a `Quality string`.
- **Album quality is derived, not advertised.** Deezer exposes no quality field at all
  (per-track gw-light `song.getData` only, N calls); Tidal advertises `audioQuality` but
  the stream request is the truth. Floor + count, `unknown ≠ zero`, and a FIT can be
  revoked when the transfer disagrees with the probe (§2.1).
- **Deezer only for the first slice; Tidal is parked.** Tidal has no tagging and no token
  rotation, so it cannot be a second member of the walk yet. The registry stays a registry
  (one `Register()` line brings it back), the connections UI shows Deezer, and the chain
  editor can ship with a single enabled source without lying about it.
- **Disk layout (pinned).** `<library_root>/<albumartist>/<album> (<year>)/<NN> - <title>.<ext>`,
  disc prefix only when disc > 1 (`1-01 Title.flac`), `cover.jpg` beside the tracks.
  `library_root` is a new fs-agent config field, not one of the scan dirs.
- **FIT identity (pinned).** `nb_tracks` exact + normalized title match (containment
  tolerated, so "(Remastered)" does not fail on its own) + artist containment + year
  within ±1. Anything less is PARTIAL at best, MISS when a count/title/artist fails.
- **The agent verifies, the hub does not re-read the file.** Tier-1 checks (min size,
  header parse, decoded duration vs the ticket) run where the bytes are; a bad track
  comes back as `TrackFailed` with its reason.
- **Tickets are lazy per track.** The hub holds the session and issues one short-lived
  stream URL when the agent asks for that track, one retry on 403. Expiry stops being a
  scheduling problem, and the hub stays in the loop for cancel/retry.
- **Preferences are per user**; one active job per (user, release group); max 2 concurrent
  jobs per agent; a second user requesting the same rg runs their own job.
- **`awaiting_pick` never expires** — the probe was already paid for — and surfaces as a
  badge on the release-group page plus a downloads popover.
- **The UI has three states, not two.** Full-width results (log closed), peek (log back,
  the right column reduced to colours + one line + `Show N candidates`), and the walk.
  Per-track expansion exists only in the full-width view, because that is the only place
  its line fits.

## 8. What is still unknown (does not block the slice)

The seven questions that blocked implementation were answered: disk layout, FIT threshold,
agent-side verification, lazy tickets, per-user preferences, `awaiting_pick` lifetime, and
concurrency all sit in §7 as decisions. What remains can be settled by running it:

- **Probe latency on a big album.** 30 tracks is 30 gw-light calls at 8/s ≈ 4s before the
  modal can say anything. If that feels bad, the fix is to stream `download:source`
  progress during the probe rather than to parallelise (the quota is shared).
- **Cache TTL.** Per `(provider, album_id, account)`, days. Nobody knows yet whether album
  quality moves often enough to matter; start at 7 days and watch.
- **Whether PARTIAL-only runs are common in practice.** If most real albums land PARTIAL
  because three tracks are 320, the results view becomes the default experience and the
  thresholds in §2.1 need revisiting. The decision audit log answers this.
- **Tidal's return.** Blocked on tagging + token rotation, not on this design.

## 9. Dev harness (throwaway)

`/dev/download` replays every case against mock data, no backend, no auth:
`app/pages/dev/download.vue` + `app/components/download/JobModal.vue` +
`app/utils/mock-download.ts` + `app/types/download-job.ts`, with a three-line
`import.meta.dev` bypass in `middleware/auth.global.ts`.

Scenarios: perfect fit · partial → results · two partials · best quality (2 fits) ·
nothing found · source fails · track dies mid-download · single (1 track) · EP (2 tracks).
Controls: Open modal / Pause / Step / Restart.

The modal is now the real component: the download page feeds it server snapshots from
`useDownloadStore`, and the harness feeds it the same shape from mocks. Delete
`app/pages/dev/`, `app/utils/mock-download.ts` and the auth-middleware bypass once the
mock scenarios stop being useful for edge cases the backend cannot reproduce on demand
(nothing found, two partials, a track dying mid-album).

## 10. Post-processing and plugins

In-process processors (tags, cover) run for every download; user plugins are Go
binaries that receive a JSON payload per hook and run confined by a Landlock
ruleset. Full reference — manifest, hooks, payload, sandbox, config API — is in
[`docs/plugins.md`](plugins.md).

The short version: hooks are `track.downloaded`, `track.tagged`,
`album.beforePublish`, `album.published`; a plugin declares which it serves and
is verified with `--describe` before it is enabled; config is typed in the
manifest, stored encrypted and served to the plugin typed; the sandbox allows
read-only system paths plus the album directory and one temp dir, and denies all
TCP. No upload endpoint: a plugin is installed by copying a directory onto the
server.
