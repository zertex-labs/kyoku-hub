<script setup lang="ts">
/**
 * Download flow harness — /dev/download
 *
 * Click a scenario to open the job modal and replay a full run against mock data:
 * walk → classify (fit / partial / miss / failed) → download or results view.
 * No backend, no auth, no real requests. Choices are only half-simulated: picking
 * a release replays a download, the other decision options just log and continue.
 *
 * Dev only: the route is exempted from the auth middleware behind import.meta.dev.
 */
import type { Job, ScenarioStep } from '~/types/download-job'
import { downloadStepsFor, scenarios } from '~/utils/mock-download'

definePageMeta({ layout: false })

useSeoMeta({ title: "Download flow harness" })

const open = ref(false)
const active = ref<Job | null>(null)
const activeKey = ref<string | null>(null)
const playing = ref(true)
const finished = ref(false)

let timer: ReturnType<typeof setTimeout> | null = null
let steps: ScenarioStep[] = []
let cursor = 0

const activeScenario = computed(() => scenarios.find((s) => s.key === activeKey.value) ?? null)

function clearTimer() {
  if (timer) {
    clearTimeout(timer)
    timer = null
  }
}

function schedule(delay: number) {
  clearTimer()
  timer = setTimeout(() => {
    if (playing.value) advance()
  }, Math.max(0, delay))
}

/** Run the next queued step. Called by the timer, by "Step", and after a decision. */
function advance() {
  clearTimer()
  if (!active.value) return
  if (active.value.decision) return // waiting on the user
  if (cursor >= steps.length) {
    finished.value = true
    return
  }

  const step = steps[cursor++]
  step.run(active.value)

  if (active.value.decision) return
  if (cursor >= steps.length) {
    finished.value = true
    return
  }
  if (playing.value) schedule(steps[cursor].after)
}

function load(key: string) {
  const scenario = scenarios.find((s) => s.key === key)
  if (!scenario) return

  clearTimer()
  const built = scenario.build()

  active.value = reactive(built.job) as Job
  steps = built.steps
  cursor = 0
  activeKey.value = key
  playing.value = true
  finished.value = false
  open.value = true

  schedule(steps[0]?.after ?? 0)
}

function restart() {
  if (activeKey.value) load(activeKey.value)
}

function togglePlay() {
  playing.value = !playing.value
  if (playing.value) {
    if (active.value?.decision) return // still blocked on the user
    if (cursor < steps.length) schedule(steps[cursor].after)
    else advance()
  } else {
    clearTimer()
  }
}

function stepOnce() {
  playing.value = false
  advance()
}

function closeModal() {
  // The real job keeps running when the modal closes; here the run pauses so it
  // can be resumed from the controls.
  open.value = false
  clearTimer()
}

function onMode(mode: 'best' | 'order') {
  if (!active.value) return
  active.value.mode = mode
  active.value.log.push({
    t: '—',
    level: 'info',
    text: `mode → ${mode === 'best' ? 'best quality' : 'source order'}`,
  })
}

function labelFor(id: string) {
  if (id === 'quit') return 'quit'
  if (id.startsWith('use:')) return `use ${id.slice(4)}`
  return active.value?.decision?.options.find((o) => o.id === id)?.label ?? id
}

function onDecide(id: string) {
  const job = active.value
  if (!job) return

  job.log.push({ t: '—', level: 'ok', text: `you chose: ${labelFor(id)}` })

  if (id === 'quit') {
    job.decision = null
    job.phase = 'done'
    job.progress = null
    job.result = 'cancelled — nothing published'
    finished.value = true
    return
  }

  if (id.startsWith('use:')) {
    const provider = id.slice(4)
    const chosen = job.candidates.find((c) => c.id === provider)
    job.candidates = chosen ? [chosen] : []
    job.phase = 'downloading'
    job.result = undefined
    job.progress = { done: 0, total: job.album.trackCount }
    job.log.push({
      t: '—',
      level: 'info',
      text: `picked ${chosen?.provider ?? provider} — ${chosen?.summary ?? ''}`,
    })

    steps = [
      ...downloadStepsFor(job.album.trackCount),
      {
        after: 400,
        run: (j) => {
          j.log.push({
            t: '—',
            level: 'info',
            text: 'tagging · writing cover · publishing',
          })
          j.log.push({ t: '—', level: 'ok', text: 'published' })
          j.phase = 'done'
          j.progress = { done: j.album.trackCount, total: j.album.trackCount }
          j.result = `${j.album.trackCount} tracks published`
        },
      },
    ]
    cursor = 0
    playing.value = true
    schedule(steps[0].after)
    return
  }

  // track_failed and friends: log the choice, then finish with an honest result.
  job.decision = null
  const choice = id
  const total = job.album.trackCount

  setTimeout(() => {
    if (!active.value) return
    const j = active.value
    if (choice === 'restart') {
      j.log.push({ t: '—', level: 'info', text: 'restarting the album on Tidal from scratch' })
      j.result = `restarted on tidal — ${total} tracks`
    } else if (choice === 'mix') {
      j.log.push({
        t: '—',
        level: 'warn',
        text: 'fetching t07 from another source — album now mixes two sources (two masters)',
      })
      j.result = `${total}/${total} tracks · mixed sources`
    } else if (choice === 'skip') {
      j.result = `${total - 1}/${total} tracks · incomplete`
    } else {
      j.result = 'cancelled — nothing published'
    }
    j.phase = 'done'
    j.progress = { done: total - 1, total }
    finished.value = true
  }, 500)
}
</script>

<template>
  <div class="min-h-screen bg-[#0d0d0f] text-gray-100">
    <div class="fixed inset-0 pointer-events-none">
      <div
        class="absolute bottom-0 left-0 right-0 h-[40%] bg-gradient-to-t from-purple-950/20 to-transparent"
      />
    </div>

    <div class="relative max-w-3xl mx-auto px-5 py-10">
      <header class="mb-8">
        <p class="text-[11px] uppercase tracking-wider text-orange-400/80 font-semibold mb-1">
          dev · mock data
        </p>
        <h1 class="text-xl font-semibold text-white">Download flow harness</h1>
        <p class="text-sm text-gray-500 mt-2 max-w-xl">
          Pick a scenario to replay a whole run: walk the source chain, classify each source
          (fit / partial / miss / failed), then either download silently or stop at the results
          view. Nothing talks to a backend.
        </p>
      </header>

      <div class="grid gap-2 sm:grid-cols-2 mb-6">
        <button
          v-for="s in scenarios"
          :key="s.key"
          class="text-left rounded-lg border px-3 py-2.5 transition-colors"
          :class="
            activeKey === s.key
              ? 'border-orange-400/40 bg-orange-400/[0.07]'
              : 'border-white/10 bg-white/[0.02] hover:border-white/20 hover:bg-white/[0.04]'
          "
          @click="load(s.key)"
        >
          <p class="text-xs font-medium text-white">{{ s.label }}</p>
          <p class="text-[11px] text-gray-500 mt-0.5 leading-snug">{{ s.hint }}</p>
        </button>
      </div>

      <div v-if="active" class="rounded-lg border border-white/10 bg-white/[0.02] p-3">
        <div class="flex flex-wrap items-center gap-2">
          <button
            class="text-[11px] px-2.5 py-1.5 rounded-md border border-white/10 text-gray-300 hover:text-white hover:border-white/20 transition-colors"
            @click="open = true"
          >
            <Icon name="lucide:panel-top-open" class="w-3 h-3 inline -mt-0.5 mr-1" />
            Open modal
          </button>
          <button
            class="text-[11px] px-2.5 py-1.5 rounded-md border border-white/10 text-gray-300 hover:text-white hover:border-white/20 transition-colors"
            @click="togglePlay"
          >
            {{ playing ? "Pause" : "Play" }}
          </button>
          <button
            class="text-[11px] px-2.5 py-1.5 rounded-md border border-white/10 text-gray-300 hover:text-white hover:border-white/20 transition-colors"
            @click="stepOnce"
          >
            Step
          </button>
          <button
            class="text-[11px] px-2.5 py-1.5 rounded-md border border-white/10 text-gray-300 hover:text-white hover:border-white/20 transition-colors"
            @click="restart"
          >
            <Icon name="lucide:rotate-ccw" class="w-3 h-3 inline -mt-0.5 mr-1" />
            Restart
          </button>

          <span class="ml-auto text-[10px] text-gray-600">
            {{ finished ? "finished" : playing ? "running" : "paused" }} ·
            {{ active.log.length }} log lines
          </span>
        </div>

        <p v-if="activeScenario" class="text-[11px] text-gray-600 mt-2">
          {{ activeScenario.hint }}
        </p>
      </div>

      <p class="text-[10px] text-gray-700 mt-6 leading-relaxed">
        Mock only. Picking a release replays a download; the other decision options log the
        choice and finish, they do not re-run the branch.
      </p>
    </div>

    <DownloadJobModal
      :open="open"
      :job="active"
      @close="closeModal"
      @decide="onDecide"
      @update:mode="onMode"
    />
  </div>
</template>
