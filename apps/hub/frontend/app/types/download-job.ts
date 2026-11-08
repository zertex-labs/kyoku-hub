/**
 * Types for the download job modal + the dev harness that drives it with mocks.
 *
 * The backend contract they mirror lives in docs/downloader-plan.md §3.1:
 * one status enum per source, two decision kinds, a durable `awaiting_pick`.
 */

export type SourceStatus = 'pending' | 'running' | 'fit' | 'partial' | 'miss' | 'failed'

export interface SourceRow {
  id: string
  name: string
  status: SourceStatus
  /** Short line under the name in the results view; also the tooltip text. */
  reason?: string
  /** e.g. "11/12 FLAC · t7 → 320" */
  summary?: string
  /** e.g. "max FLAC 16/44.1" — what this source can ever deliver. */
  ceiling?: string
}

export type LogLevel = 'info' | 'ok' | 'warn' | 'error'

export interface LogLine {
  t: string
  level: LogLevel
  text: string
}

export interface CandidateTrack {
  n: number
  title: string
  /** false = below target (still present), e.g. a 320 kbps track in a FLAC album */
  ok: boolean
  /** Rendered by the server: "FLAC 16-bit/44.1kHz". One string, no client-side formatting. */
  facts: string
}

export interface Candidate {
  id: string
  provider: string
  title: string
  artist: string
  year: string
  trackCount: number
  okTracks: number
  /** "11/12 FLAC" */
  summary: string
  /** ["t7 “Superheroes” → 320 kbps instead of FLAC"] */
  deltas: string[]
  /** Per-track detail for the results expander. */
  tracks?: CandidateTrack[]
  tone: 'fit' | 'partial'
  cover?: string
}

export interface DecisionOption {
  id: string
  label: string
  tone?: 'primary' | 'danger' | 'ghost'
}

export interface Decision {
  kind: 'pick_release' | 'track_failed'
  title: string
  body?: string
  options: DecisionOption[]
}

export type JobPhase = 'walking' | 'downloading' | 'working' | 'results' | 'done' | 'failed'

export interface JobAlbum {
  title: string
  artist: string
  year: string
  trackCount: number
  cover?: string
  /** MusicBrainz release (an edition), when the user picked one. */
  releaseId?: string
}

export interface Job {
  /** Present on real server jobs; absent on the mock harness. */
  id?: string
  rg_id?: string
  /** What the job does. Absent means a download (the original kind). */
  kind?: JobKind
  /** Where the album sits on disk, once published or re-tagged. */
  dir?: string
  album: JobAlbum
  phase: JobPhase
  mode: 'best' | 'order'
  sources: SourceRow[]
  log: LogLine[]
  progress: { done: number; total: number; current?: string } | null
  candidates: Candidate[]
  decision: Decision | null
  /** Shown in the footer once the run ends: "14 tracks · 486 MB". */
  result?: string
}

export interface ScenarioStep {
  /** Milliseconds after the previous step. */
  after: number
  run: (job: Job) => void
}

export interface Scenario {
  key: string
  label: string
  hint: string
  build: () => { job: Job; steps: ScenarioStep[] }
}
