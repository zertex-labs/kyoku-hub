/**
 * Mock scenarios for the download-job modal dev harness (/dev/download).
 *
 * Each scenario is a job plus a list of timed steps that mutate it, so the
 * harness can replay a whole run (walk → classify → download → decide) without
 * a backend. Domain rules being demonstrated are in docs/downloader-plan.md §3.1:
 *
 *   FIT      every track at/above target AND identity matches → stop, download
 *   PARTIAL  usable but ≥1 track below target → record a candidate, keep walking
 *   MISS     cannot reach the ladder / wrong release → skip, log the reason
 *   floor    album quality = min across tracks; summary carries the count
 */

import type {
  Candidate,
  CandidateTrack,
  Job,
  Scenario,
  ScenarioStep,
  SourceRow,
  LogLine,
} from '~/types/download-job'

const DISCOVERY = [
  'One More Time',
  'Aerodynamic',
  'Digital Love',
  'Harder, Better, Faster, Stronger',
  'Crescendolls',
  'Nightvision',
  'Superheroes',
  'High Life',
  'Something About Us',
  'Voyager',
  'Veridis Quo',
  'Short Circuit',
  'Face to Face',
  'Too Long',
]

const COVER =
  'https://cdn-images.dzcdn.net/images/cover/5718f7c81c27e0b2417e2a4c45224f8a/250x250-000000-80-0-0.jpg'

function clock() {
  let t = 0
  return () => {
    t += 0.4
    const whole = Math.floor(t)
    const tenth = Math.floor((t - whole) * 10)
    return `00:${String(whole).padStart(2, '0')}.${tenth}`
  }
}

function baseJob(trackCount = 14, mode: Job['mode'] = 'order'): Job {
  return {
    album: { title: 'Discovery', artist: 'Daft Punk', year: '2001', trackCount, cover: COVER },
    phase: 'walking',
    mode,
    sources: [
      { id: 'youtube', name: 'YouTube', status: 'pending', ceiling: 'max Opus 256' },
      { id: 'deezer', name: 'Deezer', status: 'pending', ceiling: 'max FLAC 16/44.1' },
      { id: 'tidal', name: 'Tidal', status: 'pending', ceiling: 'max FLAC 24/96' },
    ],
    log: [],
    progress: null,
    candidates: [],
    decision: null,
  }
}

function line(t: () => string, level: LogLine['level'], text: string): LogLine {
  return { t: t(), level, text }
}

function setSource(job: Job, id: string, patch: Partial<SourceRow>) {
  const row = job.sources.find((s) => s.id === id)
  if (row) Object.assign(row, patch)
}

/** Steps for a plain per-track download; `failAt` raises the track_failed decision. */
function downloadSteps(titles: string[], perTrack = 230): ScenarioStep[] {
  return titles.map((title, i) => ({
    after: perTrack,
    run: (job: Job) => {
      job.phase = 'downloading'
      job.progress = {
        done: i,
        total: titles.length,
        current: `t${String(i + 1).padStart(2, '0')} ${title}`,
      }
    },
  }))
}

/** Deterministic per-track mock: plausible bitrates/sizes, one optional defect. */
function tracksFor(
  titles: string[],
  opts: { hd?: boolean; defect?: number } = {},
): CandidateTrack[] {
  const { hd = false, defect } = opts
  return titles.map((title, i) => {
    const n = i + 1
    const bad = defect === n
    const duration = 180 + ((i * 23) % 130)
    const kbps = bad ? 320 : hd ? 2700 + ((i * 37) % 500) : 950 + ((i * 37) % 200)
    const sizeMb = Math.round(((duration * kbps) / 8 / 1024) * 10) / 10
    const parts = [bad ? 'MP3' : 'FLAC', `${hd && !bad ? 96000 : 44100} Hz`]
    if (!bad) parts.push(`${hd ? 24 : 16}-bit`)
    parts.push('stereo', `${kbps} kbps`, `${sizeMb} MB`)
    return { n, title, ok: !bad, facts: parts.join(' • ') }
  })
}

function candidate(
  over: Partial<Candidate> &
    Pick<Candidate, 'id' | 'provider'> & { defect?: number; hd?: boolean; titles?: string[] },
): Candidate {
  const { defect, hd, titles, ...rest } = over
  const merged: Candidate = {
    title: 'Discovery',
    artist: 'Daft Punk',
    year: '2001',
    trackCount: 14,
    okTracks: 14,
    summary: '14/14 songs · FLAC 16/44.1',
    deltas: [],
    tone: 'fit',
    cover: COVER,
    ...rest,
  }
  if (!merged.tracks) {
    merged.tracks = tracksFor(titles ?? DISCOVERY.slice(0, merged.trackCount), { hd, defect })
  }
  return merged
}

/* ─────────────────────────── scenarios ─────────────────────────── */

function perfectFit(): { job: Job; steps: ScenarioStep[] } {
  const t = clock()
  const job = baseJob()
  const steps: ScenarioStep[] = []

  steps.push({
    after: 300,
    run: (j) => {
      j.log.push(line(t, 'info', 'walk started · source order · 3 enabled sources'))
      j.log.push(line(t, 'info', 'album: Discovery — Daft Punk (2001), 14 tracks, rg 1b022e01'))
      setSource(j, 'youtube', { status: 'running' })
    },
  })
  steps.push({
    after: 900,
    run: (j) => {
      setSource(j, 'youtube', {
        status: 'miss',
        summary: 'max Opus 256',
        reason: 'Cannot reach target FLAC 16/44.1 — Opus 256 is the ceiling. Not offered.',
      })
      j.log.push(line(t, 'warn', 'youtube: skipped — max Opus 256 cannot reach target FLAC'))
      setSource(j, 'deezer', { status: 'running' })
    },
  })
  steps.push({
    after: 950,
    run: (j) => {
      setSource(j, 'deezer', {
        status: 'fit',
        summary: '14/14 songs · FLAC 16/44.1',
        reason: 'Perfect fit: 14/14 tracks at FLAC 16/44.1, identity matches the release group.',
      })
      j.log.push(line(t, 'ok', 'deezer: fit — 14/14 FLAC 16/44.1 (probed 14 tracks)'))
      j.log.push(line(t, 'info', 'walk stopped at deezer (first fit)'))
    },
  })
  steps.push(...downloadSteps(DISCOVERY))
  steps.push({
    after: 500,
    run: (j) => {
      j.log.push(line(t, 'info', 'tags · Daft Punk — Discovery · album · 2001-03-12 · Electronic · Virgin'))
      j.log.push(line(t, 'info', 'cover · 500×500 image/jpeg · embedded in every track + cover.jpg'))
      j.log.push(line(t, 'info', 'mb · release group 8b8ed50b-ab91-4847-842b-6cc0a7b15659'))
      j.log.push(line(t, 'info', 't01 tags · ISRC GBDUW0000059 · mbid f1a6a40f · track 1/14'))
      j.log.push(line(t, 'info', 'mb · artist e134b52f · release 99cc17f3 · recordings 14/14'))
      j.log.push(line(t, 'ok', 'Done. 14/14 tracks · 14/14 FLAC 16/44.1 · 486 MB → Daft Punk/Discovery (2001)'))
      j.phase = 'done'
      j.progress = { done: 14, total: 14 }
      j.result = '14 tracks · 486 MB · FLAC 16/44.1'
    },
  })
  return { job, steps }
}

function partialPick(): { job: Job; steps: ScenarioStep[] } {
  const t = clock()
  const job = baseJob()
  const steps: ScenarioStep[] = []

  steps.push({
    after: 300,
    run: (j) => {
      j.log.push(line(t, 'info', 'walk started · source order · 3 enabled sources'))
      setSource(j, 'youtube', { status: 'running' })
    },
  })
  steps.push({
    after: 800,
    run: (j) => {
      setSource(j, 'youtube', {
        status: 'miss',
        summary: 'max Opus 256',
        reason: 'Cannot reach target FLAC 16/44.1.',
      })
      j.log.push(line(t, 'warn', 'youtube: skipped — max Opus 256 cannot reach target FLAC'))
      setSource(j, 'deezer', { status: 'running' })
    },
  })
  steps.push({
    after: 1000,
    run: (j) => {
      setSource(j, 'deezer', {
        status: 'partial',
        summary: '14/14 songs · floor MP3 320',
        reason:
          'Partial: 13 of 14 tracks are FLAC, t7 “Superheroes” is only available at 320 kbps. Album floor: MP3 320.',
      })
      j.log.push(line(t, 'warn', 'deezer: partial — 13/14 FLAC, t7 “Superheroes” only 320'))
      j.log.push(line(t, 'info', 'recorded as candidate, walk continues'))
      setSource(j, 'tidal', { status: 'running' })
    },
  })
  steps.push({
    after: 900,
    run: (j) => {
      setSource(j, 'tidal', {
        status: 'miss',
        summary: 'no album',
        reason: 'No matching album for this release group in your region.',
      })
      j.log.push(line(t, 'warn', 'tidal: no matching album in region (releases: 0)'))
      j.log.push(line(t, 'info', 'walk finished — no perfect fit, 1 candidate kept'))
    },
  })
  steps.push({
    after: 700,
    run: (j) => {
      j.phase = 'results'
      j.candidates = [
        candidate({
          id: 'deezer',
          provider: 'Deezer',
          okTracks: 13,
          summary: '14/14 songs · floor MP3 320',
          tone: 'partial',
          deltas: ['t7 “Superheroes” → 320 kbps instead of FLAC 16/44.1'],
          defect: 7,
        }),
      ]
      j.log.push(line(t, 'info', 'waiting for you to pick a release'))
    },
  })
  return { job, steps }
}

function twoPartials(): { job: Job; steps: ScenarioStep[] } {
  const t = clock()
  const job = baseJob()
  const steps: ScenarioStep[] = []

  steps.push({
    after: 300,
    run: (j) => {
      j.log.push(line(t, 'info', 'walk started · source order · 3 enabled sources'))
      setSource(j, 'youtube', { status: 'running' })
    },
  })
  steps.push({
    after: 700,
    run: (j) => {
      setSource(j, 'youtube', { status: 'miss', summary: 'max Opus 256', reason: 'Cannot reach FLAC.' })
      j.log.push(line(t, 'warn', 'youtube: skipped — max Opus 256 cannot reach target FLAC'))
      setSource(j, 'deezer', { status: 'running' })
    },
  })
  steps.push({
    after: 900,
    run: (j) => {
      setSource(j, 'deezer', {
        status: 'partial',
        summary: '14/14 songs · floor MP3 320',
        reason: 'Partial: t7 only 320 kbps.',
      })
      j.log.push(line(t, 'warn', 'deezer: partial — 13/14 FLAC, t7 only 320'))
      setSource(j, 'tidal', { status: 'running' })
    },
  })
  steps.push({
    after: 950,
    run: (j) => {
      setSource(j, 'tidal', {
        status: 'partial',
        summary: '14/14 songs · FLAC 24/96',
        reason:
          'Partial: 14/14 FLAC 24/96, but the release date reads 2019 against the release group’s 2001 — probably a remaster, not this release.',
      })
      j.log.push(line(t, 'warn', 'tidal: partial — 14/14 FLAC 24/96 but year 2019 vs 2001 (remaster?)'))
      j.log.push(line(t, 'info', 'walk finished — no perfect fit, 2 candidates kept'))
    },
  })
  steps.push({
    after: 700,
    run: (j) => {
      j.phase = 'results'
      j.candidates = [
        candidate({
          id: 'tidal',
          provider: 'Tidal',
          okTracks: 14,
          summary: '14/14 songs · FLAC 24/96',
          tone: 'partial',
          deltas: [
            'Year 2019 vs release group 2001 — likely the 2019 remaster, different master',
            'Track count and titles match',
          ],
          hd: true,
        }),
        candidate({
          id: 'deezer',
          provider: 'Deezer',
          okTracks: 13,
          summary: '14/14 songs · floor MP3 320',
          tone: 'partial',
          deltas: ['t7 “Superheroes” → 320 kbps instead of FLAC 16/44.1'],
          defect: 7,
        }),
      ]
      j.log.push(line(t, 'info', 'waiting for you to pick a release'))
    },
  })
  return { job, steps }
}

function bestQuality(): { job: Job; steps: ScenarioStep[] } {
  const t = clock()
  const job = baseJob(14, 'best')
  const steps: ScenarioStep[] = []

  steps.push({
    after: 300,
    run: (j) => {
      j.log.push(line(t, 'info', 'walk started · best quality · probing all enabled sources'))
      setSource(j, 'youtube', { status: 'running' })
      setSource(j, 'deezer', { status: 'running' })
      setSource(j, 'tidal', { status: 'running' })
    },
  })
  steps.push({
    after: 900,
    run: (j) => {
      setSource(j, 'youtube', { status: 'miss', summary: 'max Opus 256', reason: 'Cannot reach FLAC.' })
      j.log.push(line(t, 'warn', 'youtube: skipped — max Opus 256 cannot reach target FLAC'))
    },
  })
  steps.push({
    after: 600,
    run: (j) => {
      setSource(j, 'deezer', {
        status: 'fit',
        summary: '14/14 songs · FLAC 16/44.1',
        reason: 'Perfect fit at target #1 (FLAC 16/44.1).',
      })
      j.log.push(line(t, 'ok', 'deezer: fit — 14/14 FLAC 16/44.1'))
    },
  })
  steps.push({
    after: 700,
    run: (j) => {
      setSource(j, 'tidal', {
        status: 'fit',
        summary: '14/14 songs · FLAC 24/96',
        reason: 'Perfect fit at target #0 (FLAC 24/96) — ranks above Deezer.',
      })
      j.log.push(line(t, 'ok', 'tidal: fit — 14/14 FLAC 24/96 (ranks above deezer: target #0 vs #1)'))
      j.log.push(line(t, 'info', 'best quality → showing all candidates'))
    },
  })
  steps.push({
    after: 600,
    run: (j) => {
      j.phase = 'results'
      j.candidates = [
        candidate({
          id: 'tidal',
          provider: 'Tidal',
          okTracks: 14,
          summary: '14/14 songs · FLAC 24/96',
          tone: 'fit',
          deltas: ['Target #0 — 24-bit/96 kHz, above your 16/44.1 rung'],
          hd: true,
        }),
        candidate({
          id: 'deezer',
          provider: 'Deezer',
          okTracks: 14,
          summary: '14/14 songs · FLAC 16/44.1',
          tone: 'fit',
          deltas: [],
        }),
      ]
    },
  })
  return { job, steps }
}

function nothingFound(): { job: Job; steps: ScenarioStep[] } {
  const t = clock()
  const job = baseJob()
  const steps: ScenarioStep[] = []

  steps.push({
    after: 300,
    run: (j) => {
      j.log.push(line(t, 'info', 'walk started · source order · 3 enabled sources'))
      setSource(j, 'youtube', { status: 'running' })
    },
  })
  const misses: Array<[string, string, string, string]> = [
    ['youtube', 'max Opus 256', 'Cannot reach target FLAC.', 'youtube: skipped — max Opus 256 cannot reach target FLAC'],
    ['deezer', 'no album', 'No album for this release group on Deezer.', 'deezer: no matching album (searched 4 variants)'],
    ['tidal', 'geo-blocked', 'Album exists but is geo-restricted to your region.', 'tidal: geo-restricted (error 2002)'],
  ]
  misses.forEach(([id, summary, reason, text], i) => {
    steps.push({
      after: i === 0 ? 800 : 900,
      run: (j) => {
        setSource(j, id, { status: 'miss', summary, reason })
        j.log.push(line(t, 'warn', text))
        const next = misses[i + 1]?.[0]
        if (next) setSource(j, next, { status: 'running' })
        else j.log.push(line(t, 'error', 'walk finished — nothing could satisfy the target'))
      },
    })
  })
  steps.push({
    after: 700,
    run: (j) => {
      j.phase = 'results'
      j.candidates = []
      j.log.push(line(t, 'info', 'no candidates — nothing to publish'))
    },
  })
  return { job, steps }
}

function sourceFailed(): { job: Job; steps: ScenarioStep[] } {
  const t = clock()
  const job = baseJob()

  const steps: ScenarioStep[] = [
    {
      after: 300,
      run: (j) => {
        j.log.push(line(t, 'info', 'walk started · source order · 3 enabled sources'))
        setSource(j, 'youtube', { status: 'running' })
      },
    },
    {
      after: 700,
      run: (j) => {
        setSource(j, 'youtube', { status: 'miss', summary: 'max Opus 256', reason: 'Cannot reach FLAC.' })
        j.log.push(line(t, 'warn', 'youtube: skipped — max Opus 256 cannot reach target FLAC'))
        setSource(j, 'deezer', { status: 'running' })
      },
    },
    {
      after: 800,
      run: (j) => {
        setSource(j, 'deezer', {
          status: 'failed',
          summary: 'auth',
          reason: 'ARL rejected: Deezer returned no user. Paste a fresh ARL in Settings → Connections.',
        })
        j.log.push(line(t, 'error', 'deezer: auth failed — ARL expired (gw-light getUserData: USER_ID 0)'))
        j.log.push(line(t, 'info', 'deezer is dead for the rest of this run'))
        setSource(j, 'tidal', { status: 'running' })
      },
    },
    {
      after: 900,
      run: (j) => {
        setSource(j, 'tidal', {
          status: 'fit',
          summary: '14/14 songs · FLAC 24/96',
          reason: 'Perfect fit — 24/96, above target.',
        })
        j.log.push(line(t, 'ok', 'tidal: fit — 14/14 FLAC 24/96'))
        j.log.push(line(t, 'info', 'walk stopped at tidal (first fit)'))
      },
    },
    ...downloadSteps(DISCOVERY, 200),
    {
      after: 500,
      run: (j) => {
        j.log.push(line(t, 'ok', 'published to /music/Daft Punk/Discovery (2001)'))
        j.phase = 'done'
        j.progress = { done: 14, total: 14 }
        j.result = '14 tracks · 1.2 GB · FLAC 24/96'
      },
    },
  ]
  return { job, steps }
}

function trackFailed(): { job: Job; steps: ScenarioStep[] } {
  const t = clock()
  const job = baseJob()

  const steps: ScenarioStep[] = [
    {
      after: 300,
      run: (j) => {
        j.log.push(line(t, 'info', 'walk started · source order · 3 enabled sources'))
        setSource(j, 'youtube', { status: 'miss', summary: 'max Opus 256', reason: 'Cannot reach FLAC.' })
        setSource(j, 'deezer', { status: 'running' })
      },
    },
    {
      after: 900,
      run: (j) => {
        setSource(j, 'deezer', {
          status: 'fit',
          summary: '14/14 songs · FLAC 16/44.1',
          reason: 'Perfect fit: 14/14 tracks at FLAC 16/44.1.',
        })
        j.log.push(line(t, 'ok', 'deezer: fit — 14/14 FLAC 16/44.1'))
        j.log.push(line(t, 'info', 'walk stopped at deezer (first fit)'))
      },
    },
    ...downloadSteps(DISCOVERY.slice(0, 7)),
    {
      after: 700,
      run: (j) => {
        j.log.push(line(t, 'error', 't07 “Superheroes”: 404 from CDN (track pulled since probe)'))
        j.log.push(line(t, 'warn', '13 of 14 tracks transferred — waiting on you'))
        j.decision = {
          kind: 'track_failed',
          title: 't07 “Superheroes” failed',
          body: 'The track was available when probed and is gone now. 13 tracks are already staged — skipping leaves the album incomplete, fetching it elsewhere mixes masters.',
          options: [
            { id: 'skip', label: 'Skip track', tone: 'primary' },
            { id: 'restart', label: 'Restart on Tidal' },
            { id: 'mix', label: 'Fetch from Tidal', tone: 'ghost' },
            { id: 'cancel', label: 'Cancel', tone: 'danger' },
          ],
        }
      },
    },
    {
      after: 600,
      run: (j) => {
        j.log.push(line(t, 'ok', 'published 13 tracks — album marked incomplete (1 missing)'))
        j.phase = 'done'
        j.progress = { done: 13, total: 14 }
        j.result = '13/14 tracks · 451 MB · incomplete'
      },
    },
  ]
  return { job, steps }
}

function single(): { job: Job; steps: ScenarioStep[] } {
  const t = clock()
  const job = baseJob(1)
  job.album.title = 'One More Time'
  job.album.year = '2000'
  job.album.trackCount = 1

  const steps: ScenarioStep[] = [
    {
      after: 300,
      run: (j) => {
        j.log.push(line(t, 'info', 'walk started · source order · 3 enabled sources'))
        j.log.push(line(t, 'info', 'single: “One More Time” — 1 track. The floor is the only track.'))
        setSource(j, 'youtube', { status: 'miss', summary: 'max Opus 256', reason: 'Cannot reach FLAC.' })
        setSource(j, 'deezer', { status: 'running' })
      },
    },
    {
      after: 900,
      run: (j) => {
        setSource(j, 'deezer', {
          status: 'partial',
          summary: '1/1 songs · MP3 320',
          reason:
            'Partial: the only track is available at 320 kbps on this account — no FLAC for this single. Album floor: MP3 320.',
        })
        j.log.push(line(t, 'warn', 'deezer: partial — 1/1 track, only 320 kbps on this account'))
        j.log.push(line(t, 'info', 'no redundancy on a single: one low track makes the whole release low'))
        setSource(j, 'tidal', { status: 'running' })
      },
    },
    {
      after: 900,
      run: (j) => {
        setSource(j, 'tidal', { status: 'miss', summary: 'no album', reason: 'Single not on Tidal.' })
        j.log.push(line(t, 'warn', 'tidal: no matching release'))
        j.log.push(line(t, 'info', 'walk finished — no perfect fit, 1 candidate kept'))
      },
    },
    {
      after: 700,
      run: (j) => {
        j.phase = 'results'
        j.candidates = [
          candidate({
            id: 'deezer',
            provider: 'Deezer',
            title: 'One More Time',
            year: '2000',
            trackCount: 1,
            okTracks: 0,
            summary: '1/1 songs · MP3 320',
            tone: 'partial',
            deltas: ['t1 “One More Time” → 320 kbps, no FLAC on this account'],
            defect: 1,
            titles: ['One More Time'],
          }),
        ]
      },
    },
  ]
  return { job, steps }
}

function epTwoTracks(): { job: Job; steps: ScenarioStep[] } {
  const t = clock()
  const job = baseJob(2)
  job.album.title = 'Aerodynamic'
  job.album.year = '2001'
  job.album.trackCount = 2

  const steps: ScenarioStep[] = [
    {
      after: 300,
      run: (j) => {
        j.log.push(line(t, 'info', 'walk started · source order · 3 enabled sources'))
        setSource(j, 'youtube', { status: 'miss', summary: 'max Opus 256', reason: 'Cannot reach FLAC.' })
        setSource(j, 'deezer', { status: 'running' })
      },
    },
    {
      after: 900,
      run: (j) => {
        setSource(j, 'deezer', {
          status: 'partial',
          summary: '2/2 songs · floor MP3 320',
          reason: 'Partial: t2 (Slumdrum Edit) only at 320 kbps. Album floor: MP3 320.',
        })
        j.log.push(line(t, 'warn', 'deezer: partial — 1/2 FLAC, t2 only 320'))
        setSource(j, 'tidal', { status: 'running' })
      },
    },
    {
      after: 900,
      run: (j) => {
        setSource(j, 'tidal', {
          status: 'partial',
          summary: '2/2 songs · FLAC 24/96',
          reason: 'Partial: 2/2 FLAC 24/96, but the year is 2019 (remaster) against the release group’s 2001.',
        })
        j.log.push(line(t, 'warn', 'tidal: partial — 2/2 FLAC 24/96 but year 2019 vs 2001'))
        j.log.push(line(t, 'info', 'walk finished — no perfect fit, 2 candidates kept'))
      },
    },
    {
      after: 700,
      run: (j) => {
        j.phase = 'results'
        j.candidates = [
          candidate({
            id: 'tidal',
            provider: 'Tidal',
            title: 'Aerodynamic',
            trackCount: 2,
            summary: '2/2 songs · FLAC 24/96',
            tone: 'partial',
            deltas: ['Year 2019 vs 2001 — likely a remaster'],
            hd: true,
            titles: ['Aerodynamic', 'Aerodynamic (Slumdrum Edit)'],
          }),
          candidate({
            id: 'deezer',
            provider: 'Deezer',
            title: 'Aerodynamic',
            trackCount: 2,
            okTracks: 1,
            summary: '2/2 songs · floor MP3 320',
            tone: 'partial',
            deltas: ['t2 “Aerodynamic (Slumdrum Edit)” → 320 kbps instead of FLAC'],
            defect: 2,
            titles: ['Aerodynamic', 'Aerodynamic (Slumdrum Edit)'],
          }),
        ]
      },
    },
  ]
  return { job, steps }
}

/** Track titles for the mock download of `count` tracks. */
export function downloadStepsFor(count: number, perTrack = 220): ScenarioStep[] {
  const titles =
    count <= DISCOVERY.length
      ? DISCOVERY.slice(0, count)
      : Array.from({ length: count }, (_, i) => `Track ${i + 1}`)
  return downloadSteps(titles, perTrack)
}

export const scenarios: Scenario[] = [
  {
    key: 'fit',
    label: 'Perfect fit',
    hint: 'YouTube skipped, Deezer fits → downloads with no prompt at all.',
    build: perfectFit,
  },
  {
    key: 'partial-pick',
    label: 'Partial → results',
    hint: '13/14 FLAC on Deezer, nothing else → results view with one candidate.',
    build: partialPick,
  },
  {
    key: 'partial-two',
    label: 'Two partials',
    hint: 'Deezer has a bad track, Tidal has the wrong master → compare both.',
    build: twoPartials,
  },
  {
    key: 'best',
    label: 'Best quality (2 fits)',
    hint: 'Both fit; Tidal 24/96 outranks Deezer 16/44.1 on the target ladder.',
    build: bestQuality,
  },
  {
    key: 'nothing',
    label: 'Nothing found',
    hint: 'Every source misses → empty results with the reasons.',
    build: nothingFound,
  },
  {
    key: 'failed',
    label: 'Source fails',
    hint: 'ARL expired on Deezer (red), Tidal still fits → download proceeds.',
    build: sourceFailed,
  },
  {
    key: 'track-failed',
    label: 'Track dies mid-download',
    hint: 'Fit, then t07 disappears from the CDN → track_failed decision.',
    build: trackFailed,
  },
  {
    key: 'single',
    label: 'Single (1 track)',
    hint: 'No redundancy: one 320 track makes the whole release partial.',
    build: single,
  },
  {
    key: 'ep',
    label: 'EP (2 tracks)',
    hint: 'Floor rule on a 2-track release; two partials to compare.',
    build: epTwoTracks,
  },
]