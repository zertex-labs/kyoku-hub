<script setup lang="ts">
const route = useRoute();
const router = useRouter();
const config = useRuntimeConfig();
const apiBase = config.public.apiBaseUrl as string;
const rgId = route.params.id as string;

const titleParam = (route.query.title as string) ?? "";
const artistParam = (route.query.artist as string) ?? "";
const searchQuery = [artistParam, titleParam].filter(Boolean).join(" ");

// Strict by default: results by another artist are noise. The switch exists for
// the case where the provider spells the artist differently and strictness hides
// everything.
const showOthers = ref(false);

// Fetch search results and connected agents in parallel
const [{ data: results, status }, { data: agentsRaw }] = await Promise.all([
  useFetch<SearchResult[]>(`${apiBase}/api/download/rg/${rgId}/search`, {
    // `artist`/`title` are the release group we came from: the hub drops results
    // by another artist and orders the rest by title distance. Provider search
    // alone returns adonä and Noir for "Ado DND".
    query: {
      q: searchQuery,
      artist: artistParam,
      title: titleParam,
      others: computed(() => (showOthers.value ? "1" : "0")),
    },
    watch: [showOthers],
    credentials: "include",
  }),
  useFetch<{ id: string; label: string; connected: boolean }[]>(`${apiBase}/api/agent/list`, {
    credentials: "include",
  }),
]);

// The editions of this release group on MusicBrainz. Seven of them can share a
// track count, so the picker is the only way to say which one this is; without a
// choice the resolver guesses.
type MBRelease = {
  id: string;
  title: string;
  date: string;
  country: string;
  status: string;
  trackCount: number;
};

const { data: mbReleases } = await useFetch<MBRelease[]>(
  `${apiBase}/api/download/rg/${rgId}/releases`,
  { credentials: "include" },
);

const chosenRelease = ref<string>("");

const connectedAgents = computed(() => (agentsRaw.value ?? []).filter((a) => a.connected));

// Auto-select the only connected agent if there's exactly one
const selectedAgentId = ref<string | null>(
  connectedAgents.value.length === 1 ? connectedAgents.value[0].id : null,
);

const byProvider = computed(() => {
  const map: Record<string, SearchResult[]> = {};
  for (const r of results.value ?? []) {
    (map[r.provider] ??= []).push(r);
  }
  return map;
});

const downloads = useDownloadStore();

// Cover Art Archive keys covers by release group, which is the release the user
// clicked. Not every group has one: the image hides itself on a 404 rather than
// leaving a broken frame.
const rgCover = `https://coverartarchive.org/release-group/${rgId}/front-500`;
const rgCoverFailed = ref(false);
// Cover Art Archive redirects to archive.org and can take seconds; an empty frame
// reads as "no artwork", so the square pulses until the image lands.
const rgCoverLoaded = ref(false);


// The walk runs on the server: clicking a card hands it the release-group
// metadata and the album the user pointed at, then the modal takes over.
async function startDownload(result: SearchResult) {
  await downloads.start(
    rgId,
    {
      title: titleParam || result.title,
      artist: artistParam || result.artist,
      year: result.year,
      trackCount: result.track_count,
      cover: result.cover_url,
      releaseId: chosenRelease.value || undefined,
    },
    {
      agentId: selectedAgentId.value,
      provider: result.provider,
      albumId: result.id,
    },
  );
}
</script>

<template>
  <div class="max-w-6xl">
    <!-- Back -->
    <button
      class="inline-flex items-center gap-1.5 text-sm text-gray-400 hover:text-white transition-colors mb-6"
      @click="router.back()"
    >
      <Icon name="lucide:arrow-left" class="w-4 h-4" />
      Back
    </button>

    <div>
        <!-- Header -->
        <div class="mb-6 flex items-start justify-between gap-6">
          <div>
            <h1 class="text-xl font-semibold text-white">Choose a release to download</h1>
            <p v-if="titleParam" class="text-sm text-gray-400 mt-1">
              {{ titleParam }}<span v-if="artistParam" class="text-gray-600"> · {{ artistParam }}</span>
            </p>

                <div v-if="connectedAgents.length > 0" class="mt-5">
          <p class="text-xs font-semibold text-gray-400 uppercase tracking-wider mb-2">
            Download destination
          </p>
          <div class="flex flex-wrap gap-2">
            <button
              v-for="agent in connectedAgents"
              :key="agent.id"
              class="inline-flex items-center gap-1.5 text-xs px-2.5 py-1.5 rounded-md border transition-colors"
              :class="
                selectedAgentId === agent.id
                  ? 'border-orange-500/50 bg-orange-500/10 text-orange-300'
                  : 'border-white/10 bg-white/5 text-gray-400 hover:text-white hover:border-white/20'
              "
              @click="selectedAgentId = agent.id"
            >
              <span
                class="w-1.5 h-1.5 rounded-full flex-shrink-0"
                :class="selectedAgentId === agent.id ? 'bg-orange-400' : 'bg-green-400'"
              />
              {{ agent.label || "Unnamed agent" }}
            </button>
            <button
              v-if="connectedAgents.length > 1"
              class="inline-flex items-center gap-1.5 text-xs px-2.5 py-1.5 rounded-md border transition-colors"
              :class="
                selectedAgentId === null
                  ? 'border-orange-500/50 bg-orange-500/10 text-orange-300'
                  : 'border-white/10 bg-white/5 text-gray-400 hover:text-white hover:border-white/20'
              "
              @click="selectedAgentId = null"
            >
              All agents
            </button>
          </div>
        </div>

              <div v-if="(mbReleases?.length ?? 0) > 1" class="mt-5">
                <p class="text-xs font-semibold text-gray-400 uppercase tracking-wider mb-2">
                  Release edition
                </p>
                <select
                  v-model="chosenRelease"
                  class="w-full max-w-md bg-black/30 border border-white/10 rounded-md px-2.5 py-1.5 text-xs text-gray-200 outline-none focus:border-orange-500/50"
                >
                  <option value="">Auto — match by track count</option>
                  <option v-for="r in mbReleases" :key="r.id" :value="r.id">
                    {{ r.date || "date unknown" }} · {{ r.country || "—" }} ·
                    {{ r.status || "release" }} · {{ r.trackCount }} tracks
                  </option>
                </select>
                <p class="text-[11px] text-gray-600 mt-1">
                  The edition owns the label, the date and the MusicBrainz id.
                </p>
              </div>
          </div>
          <div
            v-if="rgCover"
            class="relative w-[250px] h-[250px] shrink-0 rounded-lg overflow-hidden border border-white/10 bg-white/[0.03]"
          >
            <!-- The image has to be in the DOM to load at all: an v-else-if behind
                 the skeleton never mounts, so it never fires @load. -->
            <img
              v-if="!rgCoverFailed"
              :src="rgCover"
              :alt="titleParam"
              class="w-full h-full object-cover"
              @load="rgCoverLoaded = true"
              @error="rgCoverFailed = true"
            />
            <div v-else class="w-full h-full flex items-center justify-center">
              <Icon name="lucide:disc-3" class="w-6 h-6 text-gray-700" />
            </div>
            <div
              v-if="!rgCoverLoaded && !rgCoverFailed"
              class="absolute inset-0 animate-pulse bg-white/[0.06]"
            />
          </div>
        </div>

    <!-- Loading -->
    <div v-if="status === 'pending'" class="space-y-6">
      <div class="h-3 w-16 bg-white/5 rounded animate-pulse" />
      <div class="grid grid-cols-2 sm:grid-cols-3 lg:grid-cols-4 gap-3">
        <div v-for="i in 8" :key="i" class="bg-white/5 rounded-lg overflow-hidden animate-pulse">
          <div class="w-full aspect-square bg-white/[0.03]" />
          <div class="p-2.5 space-y-2">
            <div class="h-2.5 w-3/4 bg-white/5 rounded" />
            <div class="h-2 w-1/2 bg-white/5 rounded" />
          </div>
        </div>
      </div>
    </div>

    <!-- Error -->
    <p v-else-if="status === 'error'" class="text-sm text-red-400">
      Failed to search providers. Check your credentials and try again.
    </p>

    <!-- Empty -->
    <div v-else-if="!results?.length" class="text-sm text-gray-500 space-y-1">
      <p v-if="artistParam">
        No releases by <span class="text-gray-300">{{ artistParam }}</span> found<span v-if="titleParam">
          for "{{ titleParam }}"</span
        >.
      </p>
      <p v-else>No results found for "{{ searchQuery }}".</p>
      <p class="text-xs text-gray-600">
        Deezer files some releases under their licensed title, so the album may be there under a
        different name — but this artist itself came back empty, which usually means they are not
        in the catalogue.
      </p>
    </div>

    <!-- Results by provider -->
    <div v-else class="space-y-8">
      <div
        v-if="status !== 'pending' && !Object.keys(byProvider).length"
        class="rounded-lg border border-white/5 bg-white/[0.02] px-4 py-6 text-sm text-gray-400"
      >
        <p>
          No releases by
          <span class="text-white">{{ artistParam || "this artist" }}</span>
          found for "{{ titleParam || searchQuery }}".
        </p>
        <button
          class="mt-2 text-xs text-orange-400 hover:text-orange-300 transition-colors"
          @click="showOthers = true"
        >
          Show results from other artists anyway
        </button>
      </div>

      <div v-for="(items, provider) in byProvider" :key="provider">
        <h2 class="text-xs font-semibold text-gray-400 uppercase tracking-wider mb-3 capitalize">
          {{ provider }}
        </h2>
        <!-- Wrapping grid: every release stays visible. A horizontal strip clipped
             the last card and put a scrollbar under the row for no reason. -->
        <div class="grid grid-cols-2 sm:grid-cols-3 lg:grid-cols-4 gap-3">
          <div
            v-for="result in items"
            :key="result.id"
            class="bg-white/5 hover:bg-white/[0.08] rounded-lg overflow-hidden transition-colors"
          >
            <!-- Cover -->
            <div
              class="w-full aspect-square bg-white/[0.03] flex items-center justify-center overflow-hidden"
            >
              <img
                v-if="result.cover_url"
                :src="result.cover_url"
                :alt="result.title"
                class="w-full h-full object-cover"
              />
              <Icon v-else name="lucide:disc" class="w-10 h-10 text-gray-700" />
            </div>
            <!-- Info -->
            <div class="p-2.5 space-y-1.5">
              <p class="text-xs font-medium text-white truncate" :title="result.title">
                {{ result.title }}
              </p>
              <p class="text-[11px] text-gray-500 truncate">{{ result.artist }}</p>
              <div class="flex items-center justify-between">
                <span class="text-[10px] text-gray-600">{{ result.year || "—" }}</span>
                <span class="text-[10px] text-gray-600">{{ result.track_count }}t</span>
              </div>
              <div class="flex items-center justify-between">
                <span
                  class="text-[10px] font-semibold px-1 py-0.5 rounded"
                  :class="
                    result.quality === 'LOSSLESS' || result.quality === 'HI_RES_LOSSLESS'
                      ? 'bg-green-500/20 text-green-400'
                      : 'bg-white/5 text-gray-500'
                  "
                >
                  {{
                    result.quality === "HI_RES_LOSSLESS"
                      ? "HiRes"
                      : result.quality === "LOSSLESS"
                        ? "FLAC"
                        : result.quality
                  }}
                </span>
                <button
                  class="text-[10px] px-1.5 py-0.5 rounded transition-colors"
                  :class="
                    downloads.starting
                      ? 'bg-white/5 text-gray-600 cursor-not-allowed'
                      : 'bg-orange-500/20 text-orange-400 hover:bg-orange-500/30'
                  "
                  :disabled="downloads.starting"
                  @click="startDownload(result)"
                >
                  <Icon
                    v-if="downloads.starting"
                    name="lucide:loader-circle"
                    class="w-3 h-3 animate-spin"
                  />
                  <Icon v-else name="lucide:download" class="w-3 h-3" />
                </button>
              </div>
            </div>
          </div>
        </div>
      </div>
    </div>

      </div>

    <!-- Reopen affordance: the job outlives the modal -->
    <button
      v-if="downloads.job && !downloads.open"
      class="fixed bottom-6 left-1/2 -translate-x-1/2 z-40 flex items-center gap-2 px-3.5 py-2 rounded-lg bg-[#1a1a1f] border border-white/10 shadow-xl text-xs text-gray-300 hover:text-white hover:border-white/20 transition-colors"
      @click="downloads.open = true"
    >
      <Icon
        :name="
          downloads.job.phase === 'done'
            ? 'lucide:check-circle'
            : downloads.job.phase === 'failed'
              ? 'lucide:circle-x'
              : 'lucide:loader-circle'
        "
        class="w-4 h-4"
        :class="{
          'text-green-400': downloads.job.phase === 'done',
          'text-red-400': downloads.job.phase === 'failed',
          'text-orange-400 animate-spin':
            downloads.job.phase !== 'done' && downloads.job.phase !== 'failed',
        }"
      />
      {{
        downloads.job.phase === "done"
          ? "Download finished"
          : downloads.job.phase === "failed"
            ? "Download failed"
            : downloads.job.phase === "results"
              ? "Waiting for you"
              : "Downloading"
      }}
    </button>

    <p v-if="downloads.error" class="text-xs text-red-400 mt-4">{{ downloads.error }}</p>

    <DownloadJobModal
      :open="downloads.open"
      :job="downloads.job"
      @close="downloads.close()"
      @decide="downloads.decide"
    />
  </div>
</template>
