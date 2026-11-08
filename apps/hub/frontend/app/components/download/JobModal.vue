<script setup lang="ts">
/**
 * Download job modal (mock-driven for the /dev/download harness).
 *
 * Layout: split — left = job output, right = source list in chain order.
 * When a run ends with no FIT the left pane collapses (grid transition) and the
 * right pane becomes the results view; a chevron brings the split back with the
 * colours kept and the detail hidden (docs/downloader-plan.md §3.1).
 *
 * Results cards can expand into a per-track list: one line per track with the
 * actual delivered facts (codec · rate · depth · channels · bitrate · size), so
 * "13/14 FLAC" is checkable rather than a claim.
 *
 * Status colour is never the only signal: every row carries an icon and, when
 * partial, a "Partial" chip with a tooltip.
 */
import type { Candidate, CandidateTrack, Job, SourceStatus } from '~/types/download-job'

const props = defineProps<{
  open: boolean
  job: Job | null
}>()

const emit = defineEmits<{
  close: []
  decide: [optionId: string]
  'update:mode': [mode: 'best' | 'order']
}>()

const j = computed(() => props.job)
const isResults = computed(() => j.value?.phase === 'results')
const peek = ref(false)
const logEl = ref<HTMLElement | null>(null)
const expanded = ref<Set<string>>(new Set())

// A new results phase always opens full-screen, unless the user asked to peek.
watch(
  () => j.value?.phase,
  (phase) => {
    if (phase === 'results') {
      peek.value = false
      expanded.value = new Set()
    }
  },
)

watch(
  () => j.value?.log.length,
  async () => {
    await nextTick()
    if (logEl.value) logEl.value.scrollTop = logEl.value.scrollHeight
  },
)

const collapsed = computed(() => isResults.value && !peek.value)

const STATUS: Record<
  SourceStatus,
  { icon: string; text: string; border: string; dot: string; label: string }
> = {
  pending: {
    icon: 'lucide:clock',
    text: 'text-gray-600',
    border: 'border-white/5',
    dot: 'bg-gray-700',
    label: 'Queued',
  },
  running: {
    icon: 'lucide:loader-2',
    text: 'text-orange-300',
    border: 'border-orange-400/25',
    dot: 'bg-orange-400',
    label: 'Working',
  },
  fit: {
    icon: 'lucide:circle-check',
    text: 'text-green-300',
    border: 'border-green-400/20',
    dot: 'bg-green-400',
    label: 'Perfect fit',
  },
  partial: {
    icon: 'lucide:circle-alert',
    text: 'text-amber-300',
    border: 'border-amber-400/25',
    dot: 'bg-amber-400',
    label: 'Partial',
  },
  miss: {
    icon: 'lucide:circle-slash',
    text: 'text-gray-500',
    border: 'border-white/5',
    dot: 'bg-gray-600',
    label: 'Skipped',
  },
  failed: {
    icon: 'lucide:circle-x',
    text: 'text-red-300',
    border: 'border-red-400/25',
    dot: 'bg-red-400',
    label: 'Failed',
  },
}

const PARTIAL_TOOLTIP =
  'Partial — at least one track is below your target quality. The run keeps going; you will see exactly which tracks and what they fell back to in the results.'

const LOG_STYLE: Record<string, string> = {
  info: 'text-gray-400',
  ok: 'text-green-300/90',
  warn: 'text-amber-300/90',
  error: 'text-red-300/90',
}

const trackWord = (n: number) => (n === 1 ? 'track' : 'tracks')

// "2026 · 11 tracks", and just "11 tracks" when the provider gave no year —
// the naive template produced a naked "·" and glued the count to the word.
// One modal, three jobs. The kind changes the wording and which controls make
// sense — a re-tag has no sources and no probe budget, an inspect has both but
// never downloads.
const kind = computed(() => props.job?.kind ?? "download")
const isDownload = computed(() => kind.value === "download")
// The right-hand pane lists sources; a re-tag has none.
const hasSide = computed(() => kind.value !== "retag")

const subtitle = computed(() => {
  if (kind.value === "retag") {
    return props.job?.album.year
      ? `${props.job.album.year} · re-writing tags`
      : "re-writing tags"
  }
  // A card started job has no track count yet — "0 tracks" is worse than silence.
  const n = props.job?.album.trackCount ?? 0
  return [props.job?.album.year || "", n > 0 ? `${n} ${trackWord(n)}` : ""]
    .filter(Boolean)
    .join(" · ")
})

function toggleTracks(id: string) {
  const next = new Set(expanded.value)
  if (next.has(id)) next.delete(id)
  else next.add(id)
  expanded.value = next
}

function progressPercent(job: Job) {
  if (!job.progress || job.progress.total === 0) return 0
  return Math.round((job.progress.done / job.progress.total) * 100)
}

function useCandidate(c: Candidate) {
  emit('decide', `use:${c.id}`)
}

useBodyScrollLock(computed(() => props.open))

function onKeydown(e: KeyboardEvent) {
  if (e.key === 'Escape' && props.open) emit('close')
}

onMounted(() => window.addEventListener('keydown', onKeydown))
onUnmounted(() => window.removeEventListener('keydown', onKeydown))
</script>

<template>
  <Teleport to="body">
    <Transition
      enter-active-class="transition duration-150 ease-out"
      enter-from-class="opacity-0"
      enter-to-class="opacity-100"
      leave-active-class="transition duration-100 ease-in"
      leave-from-class="opacity-100"
      leave-to-class="opacity-0"
    >
      <div
        v-if="open && j"
        class="fixed inset-0 z-50 flex items-center justify-center p-4"
        role="dialog"
        aria-modal="true"
      >
        <div class="absolute inset-0 bg-black/70 backdrop-blur-sm" @click="emit('close')" />

        <div
          class="relative w-full max-w-5xl h-[78vh] max-h-[calc(100dvh-2rem)] min-h-[320px] bg-[#141417] border border-white/10 rounded-xl shadow-2xl flex flex-col overflow-hidden"
        >
          <!-- ── Header ─────────────────────────────────────────────── -->
          <header class="flex items-center gap-3 px-4 py-3 border-b border-white/5 shrink-0">
            <button
              v-if="isResults"
              class="w-7 h-7 rounded-md border border-white/10 text-gray-400 hover:text-white hover:border-white/20 transition-colors flex items-center justify-center shrink-0"
              :title="peek ? 'Back to results' : 'Back to output'"
              @click="peek = !peek"
            >
              <Icon
                :name="peek ? 'lucide:chevron-right' : 'lucide:chevron-left'"
                class="w-4 h-4"
              />
            </button>

            <div class="min-w-0 flex-1">
              <p class="text-sm font-semibold text-white truncate">
                {{ j.album.title }}
                <span class="text-gray-600 font-normal">·</span>
                <span class="text-gray-400 font-normal">{{ j.album.artist }}</span>
              </p>
              <p class="text-[11px] text-gray-600">
                {{ subtitle }}
                <span v-if="j.result" class="text-gray-500"> · {{ j.result }}</span>
              </p>
            </div>

            <!-- probe budget toggle: a choice made when the job starts -->
            <div
              v-if="!j.id && isDownload"
              class="hidden sm:flex items-center rounded-lg border border-white/10 overflow-hidden text-[11px] shrink-0"
            >
              <button
                class="px-2.5 py-1.5 transition-colors"
                :class="
                  j.mode === 'best'
                    ? 'bg-white/10 text-white'
                    : 'text-gray-500 hover:text-gray-300'
                "
                @click="emit('update:mode', 'best')"
              >
                Best quality
              </button>
              <button
                class="px-2.5 py-1.5 transition-colors border-l border-white/10"
                :class="
                  j.mode === 'order'
                    ? 'bg-white/10 text-white'
                    : 'text-gray-500 hover:text-gray-300'
                "
                @click="emit('update:mode', 'order')"
              >
                Source order
              </button>
            </div>

            <span
              v-else-if="isDownload"
              class="hidden sm:inline text-[11px] px-2.5 py-1 rounded-md bg-white/5 text-gray-400"
              title="Chosen when the download started"
            >
              {{ j.mode === "best" ? "Best quality" : "Source order" }}
            </span>

            <button
              class="w-7 h-7 rounded-md text-gray-500 hover:text-white transition-colors flex items-center justify-center shrink-0"
              :title="isDownload ? 'Close (the download keeps running)' : 'Close'"
              @click="emit('close')"
            >
              <Icon name="lucide:x" class="w-4 h-4" />
            </button>
          </header>

          <!-- ── Body: log pane collapses, sources expand ───────────── -->
          <div
            class="flex-1 min-h-0 grid transition-[grid-template-columns] duration-300 ease-out"
            :style="{
              gridTemplateColumns: hasSide
                ? collapsed
                  ? '0fr 1fr'
                  : '1fr 320px'
                : '1fr 0fr',
            }"
          >
            <!-- left: output -->
            <section class="min-w-0 overflow-hidden border-r border-white/5">
              <div class="h-full flex flex-col">
                <div
                  class="flex items-center justify-between px-4 py-2 border-b border-white/5 shrink-0"
                >
                  <p class="text-[10px] uppercase tracking-wider text-gray-500 font-semibold">
                    Output
                  </p>
                  <p class="text-[10px] text-gray-600">{{ j.log.length }} lines</p>
                </div>
                <div
                  ref="logEl"
                  class="quiet-scroll flex-1 overflow-y-auto overscroll-contain px-4 py-3 font-mono text-[11px] leading-relaxed space-y-0.5"
                >
                  <p v-for="(l, i) in j.log" :key="i" :class="LOG_STYLE[l.level]">
                    <span class="text-gray-700 select-none">{{ l.t }}</span>
                    <span class="mx-1.5 text-gray-800">|</span>{{ l.text }}
                  </p>
                  <p v-if="!j.log.length" class="text-gray-700">waiting…</p>
                </div>
              </div>
            </section>

            <!-- right: sources / results -->
            <section class="min-w-0 overflow-hidden bg-white/[0.015]">
              <!-- sources: while walking, and in the compact peek view after results.
                   Peek = colours only, no candidate detail — the column is 320px. -->
              <div v-if="!isResults || !collapsed" class="h-full flex flex-col">
                <div class="px-3 py-2 border-b border-white/5 shrink-0">
                  <p class="text-[10px] uppercase tracking-wider text-gray-500 font-semibold">
                    Sources
                  </p>
                </div>
                <div class="quiet-scroll flex-1 overflow-y-auto p-2.5 space-y-1.5">
                  <div
                    v-for="s in j.sources"
                    :key="s.id"
                    class="rounded-lg border px-2.5 py-2 transition-colors"
                    :class="STATUS[s.status].border"
                    :title="s.reason || s.ceiling || undefined"
                  >
                    <div class="flex items-center gap-2">
                      <Icon
                        :name="STATUS[s.status].icon"
                        class="w-3.5 h-3.5 shrink-0"
                        :class="[
                          STATUS[s.status].text,
                          { 'animate-spin': s.status === 'running' },
                        ]"
                      />
                      <span
                        class="text-xs font-medium truncate"
                        :class="
                          s.status === 'pending' || s.status === 'miss'
                            ? 'text-gray-500'
                            : 'text-white'
                        "
                      >
                        {{ s.name }}
                      </span>

                      <span
                        v-if="s.status === 'partial'"
                        class="text-[9px] px-1.5 py-0.5 rounded border border-amber-400/25 bg-amber-400/10 text-amber-300 shrink-0"
                        :title="PARTIAL_TOOLTIP"
                      >
                        Partial
                      </span>

                      <span
                        v-if="s.status === 'failed'"
                        class="text-[9px] px-1.5 py-0.5 rounded border border-red-400/25 bg-red-400/10 text-red-300 shrink-0"
                      >
                        Failed
                      </span>

                      <span
                        v-if="s.status === 'miss'"
                        class="text-[9px] px-1.5 py-0.5 rounded border border-white/10 bg-white/5 text-gray-500 shrink-0"
                      >
                        Skipped
                      </span>
                    </div>
                    <!-- A skipped source is the one case where the reason is the
                         whole story ("could not probe any track: ARL expired"), so
                         it belongs on the row, not behind a hover. -->
                    <p
                      v-if="s.status === 'partial' || s.status === 'failed' || s.status === 'miss'"
                      class="text-[10px] text-gray-500 mt-0.5 pl-[1.375rem] truncate"
                      :class="s.status === 'miss' ? 'text-gray-600' : ''"
                    >
                      {{ s.summary || s.reason }}
                    </p>
                  </div>
                </div>

                <!-- peek mode: the way back to the full results -->
                <div v-if="isResults" class="border-t border-white/5 p-2.5 shrink-0">
                  <button
                    class="w-full text-[11px] px-3 py-2 rounded-md bg-white/5 text-gray-300 hover:bg-white/10 hover:text-white transition-colors flex items-center justify-center gap-1.5"
                    @click="peek = false"
                  >
                    <Icon name="lucide:maximize-2" class="w-3 h-3" />
                    {{
                      j.candidates.length
                        ? `Show ${j.candidates.length} candidate${j.candidates.length === 1 ? "" : "s"}`
                        : "Show why nothing matched"
                    }}
                  </button>
                </div>
              </div>

              <!-- results view -->
              <div v-else class="h-full flex flex-col">
                <!-- colours kept from the walk -->
                <div
                  class="flex flex-wrap items-center gap-x-3 gap-y-1 px-4 py-2 border-b border-white/5 shrink-0"
                >
                  <div
                    v-for="s in j.sources"
                    :key="s.id"
                    class="flex items-center gap-1.5"
                    :title="s.reason || undefined"
                  >
                    <span class="w-1.5 h-1.5 rounded-full" :class="STATUS[s.status].dot" />
                    <span class="text-[10px]" :class="STATUS[s.status].text">{{ s.name }}</span>
                  </div>
                </div>

                <div class="quiet-scroll flex-1 overflow-y-auto overscroll-contain p-4">
                  <div class="mb-3">
                    <h3 class="text-sm font-semibold text-white">
                      {{ j.candidates.length ? "Pick a release" : "Nothing worked" }}
                    </h3>
                    <p class="text-[11px] text-gray-500 mt-0.5">
                      {{
                        j.candidates.length
                          ? "No source delivered a perfect match. These are the closest — check the deltas before picking."
                          : "No source could deliver this album at any quality on your ladder."
                      }}
                    </p>
                  </div>

                  <!-- No candidates: the reasons have to live here, the log is collapsed -->
                  <ul v-if="!j.candidates.length" class="space-y-1.5 mb-3">
                    <li
                      v-for="s in j.sources"
                      :key="s.id"
                      class="flex gap-2 text-[11px] leading-snug"
                    >
                      <span class="w-16 shrink-0 text-gray-500">{{ s.name }}</span>
                      <span class="text-gray-600">{{ s.reason || s.summary || "no result" }}</span>
                    </li>
                  </ul>

                  <div class="space-y-2">
                    <div
                      v-for="c in j.candidates"
                      :key="c.id"
                      class="rounded-lg border transition-colors"
                      :class="
                        c.tone === 'fit'
                          ? 'border-green-400/20 bg-green-400/[0.04]'
                          : 'border-amber-400/25 bg-amber-400/[0.04]'
                      "
                    >
                      <div class="flex gap-3 p-3">
                        <div
                          class="w-14 h-14 rounded-md overflow-hidden bg-white/5 shrink-0 flex items-center justify-center"
                        >
                          <img
                            v-if="c.cover"
                            :src="c.cover"
                            :alt="c.title"
                            class="w-full h-full object-cover"
                            @error="($event.target as HTMLImageElement).style.display = 'none'"
                          />
                        </div>

                        <div class="min-w-0 flex-1">
                          <div class="flex items-center gap-1.5">
                            <span class="text-[10px] uppercase tracking-wide text-gray-500">{{
                              c.provider
                            }}</span>
                            <span
                              class="text-[9px] px-1.5 py-0.5 rounded border"
                              :class="
                                c.tone === 'fit'
                                  ? 'border-green-400/25 bg-green-400/10 text-green-300'
                                  : 'border-amber-400/25 bg-amber-400/10 text-amber-300'
                              "
                            >
                              {{ c.tone === "fit" ? "Fit" : "Partial" }}
                            </span>
                          </div>
                          <p class="text-xs text-white mt-0.5 truncate">
                            {{ c.title }}
                            <span class="text-gray-600">·</span>
                            <span class="text-gray-400">{{ c.year }}</span>
                            <span class="text-gray-600">·</span>
                            <span class="text-gray-400"
                              >{{ c.trackCount }} {{ trackWord(c.trackCount) }}</span
                            >
                          </p>
                          <p class="text-[11px] text-gray-400 mt-0.5">{{ c.summary }}</p>
                          <ul v-if="c.deltas.length" class="mt-1 space-y-0.5">
                            <li
                              v-for="(d, i) in c.deltas"
                              :key="i"
                              class="text-[10px] text-amber-300/80 leading-snug"
                            >
                              · {{ d }}
                            </li>
                          </ul>

                          <!-- per-track detail toggle -->
                          <button
                            v-if="c.tracks?.length"
                            class="mt-1.5 inline-flex items-center gap-1 text-[10px] text-gray-500 hover:text-white transition-colors"
                            @click="toggleTracks(c.id)"
                          >
                            <Icon
                              :name="expanded.has(c.id) ? 'lucide:chevron-up' : 'lucide:chevron-down'"
                              class="w-3 h-3"
                            />
                            {{
                              expanded.has(c.id)
                                ? "Hide tracks"
                                : `Show ${c.trackCount} ${trackWord(c.trackCount)}`
                            }}
                          </button>
                        </div>

                        <button
                          class="self-center shrink-0 text-[11px] font-medium px-3 py-1.5 rounded-md border transition-colors"
                          :class="
                            c.tone === 'fit'
                              ? 'border-green-400/30 text-green-300 hover:bg-green-400/10'
                              : 'border-amber-400/30 text-amber-300 hover:bg-amber-400/10'
                          "
                          @click="useCandidate(c)"
                        >
                          Use this
                        </button>
                      </div>

                      <!-- expanded: one line per track, with the delivered facts -->
                      <div
                        v-if="expanded.has(c.id) && c.tracks?.length"
                        class="px-3 pb-3"
                      >
                        <div
                          class="border-t border-white/5 pt-2 space-y-1 font-mono text-[10.5px] leading-relaxed"
                        >
                          <div
                            v-for="t in c.tracks"
                            :key="t.n"
                            class="flex flex-wrap items-baseline gap-x-1.5"
                          >
                            <Icon
                              :name="t.ok ? 'lucide:check' : 'lucide:triangle-alert'"
                              class="w-3 h-3 shrink-0 self-center"
                              :class="t.ok ? 'text-green-400' : 'text-amber-400'"
                            />
                            <span class="text-gray-600 tabular-nums">[{{ t.n }}]</span>
                            <span
                              class="font-medium"
                              :class="t.ok ? 'text-gray-100' : 'text-amber-200'"
                              >{{ t.title }}</span
                            >
                            <span class="text-gray-700">→</span>
                            <span :class="t.ok ? 'text-gray-500' : 'text-amber-300/90'">{{
                              t.facts
                            }}</span>
                          </div>
                        </div>
                      </div>
                    </div>
                  </div>
                </div>

                <div
                  class="flex items-center justify-between px-4 py-3 border-t border-white/5 shrink-0"
                >
                  <p class="text-[10px] text-gray-600">
                    {{
                      j.candidates.length
                        ? "Nothing is picked automatically. This waits until you choose."
                        : "Quit closes this run. Nothing was downloaded."
                    }}
                  </p>
                  <button
                    class="text-[11px] px-3 py-1.5 rounded-md border border-white/10 text-gray-400 hover:text-white hover:border-white/20 transition-colors"
                    @click="emit('decide', 'quit')"
                  >
                    Quit
                  </button>
                </div>
              </div>
            </section>
          </div>

          <!-- ── Decision bar (track_failed etc.) ───────────────────── -->
          <div
            v-if="j.decision"
            class="border-t border-amber-400/20 bg-amber-400/[0.05] px-4 py-3 shrink-0"
          >
            <p class="text-xs font-medium text-amber-200">{{ j.decision.title }}</p>
            <p v-if="j.decision.body" class="text-[11px] text-amber-200/60 mt-0.5">
              {{ j.decision.body }}
            </p>
            <div class="flex flex-wrap gap-2 mt-2.5">
              <button
                v-for="o in j.decision.options"
                :key="o.id"
                class="text-[11px] px-3 py-1.5 rounded-md border transition-colors"
                :class="{
                  'border-orange-400/40 bg-orange-400/10 text-orange-200 hover:bg-orange-400/20':
                    o.tone === 'primary',
                  'border-red-400/25 text-red-300 hover:bg-red-400/10': o.tone === 'danger',
                  'border-white/10 text-gray-400 hover:text-white hover:border-white/20':
                    o.tone === 'ghost' || !o.tone,
                }"
                @click="emit('decide', o.id)"
              >
                {{ o.label }}
              </button>
            </div>
          </div>

          <!-- ── Footer: progress ───────────────────────────────────── -->
          <footer
            v-else-if="j.progress"
            class="border-t border-white/5 px-4 py-2.5 shrink-0 flex items-center gap-3"
          >
            <div class="flex-1 h-1 rounded-full bg-white/5 overflow-hidden">
              <div
                class="h-full bg-orange-400 transition-[width] duration-300"
                :style="{ width: progressPercent(j) + '%' }"
              />
            </div>
            <p class="text-[10px] text-gray-500 tabular-nums shrink-0">
              {{ j.progress.done }}/{{ j.progress.total }}
            </p>
            <p class="text-[10px] text-gray-600 truncate max-w-[45%]">
              {{ j.progress.current ?? j.phase }}
            </p>
          </footer>
        </div>
      </div>
    </Transition>
  </Teleport>
</template>

<style scoped>
/* A bright default scrollbar over a dark panel reads as a bug: keep it a hint. */
.quiet-scroll {
  scrollbar-width: thin;
  scrollbar-color: rgba(255, 255, 255, 0.1) transparent;
}
.quiet-scroll::-webkit-scrollbar {
  width: 8px;
  height: 8px;
}
.quiet-scroll::-webkit-scrollbar-thumb {
  background: rgba(255, 255, 255, 0.1);
  border-radius: 8px;
}
.quiet-scroll::-webkit-scrollbar-thumb:hover {
  background: rgba(255, 255, 255, 0.18);
}
.quiet-scroll::-webkit-scrollbar-track {
  background: transparent;
}
</style>
