<script setup lang="ts">
const route = useRoute();
const router = useRouter();
const config = useRuntimeConfig();
const apiBase = config.public.apiBaseUrl as string;
const rgId = route.params.id as string;

const titleParam = (route.query.title as string) ?? "";
const artistParam = (route.query.artist as string) ?? "";
const searchQuery = [artistParam, titleParam].filter(Boolean).join(" ");

// Fetch search results and connected agents in parallel
const [{ data: results, status }, { data: agentsRaw }] = await Promise.all([
  useFetch<SearchResult[]>(`${apiBase}/api/download/rg/${rgId}/search`, {
    query: { q: searchQuery },
    credentials: "include",
  }),
  useFetch<{ id: string; label: string; connected: boolean }[]>(`${apiBase}/api/agent/list`, {
    credentials: "include",
  }),
]);

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

const activeDownload = ref<string | null>(null);
const downloadPhase = ref("");
const downloadTrack = ref(0);
const downloadTotal = ref(0);

async function startDownload(result: SearchResult) {
  if (activeDownload.value) return;
  activeDownload.value = result.id;
  downloadPhase.value = "queued";
  try {
    await $fetch(`${apiBase}/api/download/rg/${rgId}`, {
      method: "POST",
      credentials: "include",
      body: {
        provider: result.provider,
        album_id: result.id,
        agent_id: selectedAgentId.value ?? undefined,
      },
    });
  } catch {
    activeDownload.value = null;
    downloadPhase.value = "";
  }
}

const { on, off } = useSSE();

function handleProgress(data: any) {
  if (data.rg_id !== rgId) return;
  downloadPhase.value = data.phase;
  downloadTrack.value = data.track ?? 0;
  downloadTotal.value = data.total ?? 0;
  if (data.phase === "complete") {
    setTimeout(() => router.back(), 1500);
  } else if (data.phase === "error") {
    activeDownload.value = null;
  }
}

on("download:progress", handleProgress);
onUnmounted(() => off("download:progress", handleProgress));
</script>

<template>
  <div class="max-w-4xl">
    <!-- Back -->
    <button
      class="inline-flex items-center gap-1.5 text-sm text-gray-400 hover:text-white transition-colors mb-6"
      @click="router.back()"
    >
      <Icon name="lucide:arrow-left" class="w-4 h-4" />
      Back
    </button>

    <!-- Header -->
    <div class="mb-6">
      <h1 class="text-xl font-semibold text-white">Choose a release to download</h1>
      <p v-if="titleParam" class="text-sm text-gray-400 mt-1">
        {{ titleParam }}<span v-if="artistParam" class="text-gray-600"> · {{ artistParam }}</span>
      </p>
    </div>

    <!-- Agent picker (only when multiple connected) -->
    <div v-if="connectedAgents.length > 0" class="mb-6">
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
          {{ agent.label }}
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

    <!-- Loading -->
    <div v-if="status === 'pending'" class="space-y-6">
      <div class="h-3 w-16 bg-white/5 rounded animate-pulse" />
      <div class="flex gap-3">
        <div
          v-for="i in 5"
          :key="i"
          class="w-40 flex-shrink-0 bg-white/5 rounded-lg overflow-hidden animate-pulse"
        >
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
      <p>No results found for "{{ searchQuery }}".</p>
      <p class="text-xs text-gray-600">Check the hub backend logs for Tidal API errors.</p>
    </div>

    <!-- Results by provider -->
    <div v-else class="space-y-8">
      <div v-for="(items, provider) in byProvider" :key="provider">
        <h2 class="text-xs font-semibold text-gray-400 uppercase tracking-wider mb-3 capitalize">
          {{ provider }}
        </h2>
        <div class="flex gap-3 overflow-x-auto pb-2">
          <div
            v-for="result in items"
            :key="result.id"
            class="flex-shrink-0 w-40 bg-white/5 hover:bg-white/[0.08] rounded-lg overflow-hidden transition-colors"
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
                    activeDownload
                      ? 'bg-white/5 text-gray-600 cursor-not-allowed'
                      : 'bg-orange-500/20 text-orange-400 hover:bg-orange-500/30'
                  "
                  :disabled="!!activeDownload"
                  @click="startDownload(result)"
                >
                  <Icon
                    v-if="activeDownload === result.id"
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

    <!-- Sticky progress bar -->
    <Transition
      enter-active-class="transition duration-200 ease-out"
      enter-from-class="opacity-0 translate-y-2"
      enter-to-class="opacity-100 translate-y-0"
      leave-active-class="transition duration-150 ease-in"
      leave-from-class="opacity-100 translate-y-0"
      leave-to-class="opacity-0 translate-y-2"
    >
      <div
        v-if="activeDownload"
        class="fixed bottom-6 left-1/2 -translate-x-1/2 z-50 flex items-center gap-3 px-4 py-3 rounded-lg bg-[#1a1a1f] border border-white/10 shadow-xl text-sm"
      >
        <Icon
          v-if="downloadPhase !== 'complete'"
          name="lucide:loader-circle"
          class="w-4 h-4 text-orange-400 animate-spin flex-shrink-0"
        />
        <Icon v-else name="lucide:check-circle" class="w-4 h-4 text-green-400 flex-shrink-0" />
        <span v-if="downloadPhase === 'complete'" class="text-green-400">Download complete</span>
        <span v-else-if="downloadPhase === 'queued'" class="text-gray-300">Queued…</span>
        <span v-else class="text-gray-300">
          Downloading track {{ downloadTrack }}/{{ downloadTotal }}
          <span v-if="downloadPhase" class="text-gray-500"> · {{ downloadPhase }}</span>
        </span>
      </div>
    </Transition>
  </div>
</template>
