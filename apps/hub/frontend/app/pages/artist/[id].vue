<script setup lang="ts">
const route = useRoute();
const config = useRuntimeConfig();
const apiBase = config.public.apiBaseUrl as string;
const id = route.params.id as string;

const MAX_DESCRIPTION_LENGTH = 350;

const coverErrors = ref<Set<string>>(new Set());
const coverLoaded = ref<Set<string>>(new Set());

function onCoverLoad(rgId: string) {
  coverLoaded.value = new Set([...coverLoaded.value, rgId]);
}

function onCoverError(rgId: string) {
  coverLoaded.value = new Set([...coverLoaded.value, rgId]);
  coverErrors.value = new Set([...coverErrors.value, rgId]);
}

const showBioModal = ref(false);

const { data: artist } = await useFetch<BrainzArtist>(`${apiBase}/api/artist/${id}`, {
  credentials: "include",
});

const { data: releaseGroups, refresh: refreshReleaseGroups } = await useFetch<ReleaseGroup[]>(
  `${apiBase}/api/artist/${id}/release-groups`,
  { credentials: "include" },
);

const description = computed(() => artist.value?.description || "");
const truncatedDescription = computed(() => {
  if (description.value.length <= MAX_DESCRIPTION_LENGTH) return description.value;
  const truncated = description.value.slice(0, MAX_DESCRIPTION_LENGTH);
  const lastSpace = truncated.lastIndexOf(" ");
  return (lastSpace > 0 ? truncated.slice(0, lastSpace) : truncated) + "...";
});

// Group release groups by primary_type + secondary_types
function getReleaseGroupType(rg: ReleaseGroup): string {
  const primary = rg.primary_type || "Other";
  if (rg.secondary_types && rg.secondary_types.trim()) {
    return `${primary} + ${rg.secondary_types}`;
  }
  return primary;
}

const groupedReleaseGroups = computed(() => {
  const groups: Record<string, ReleaseGroup[]> = {};
  for (const rg of releaseGroups.value || []) {
    const type = getReleaseGroupType(rg);
    if (!groups[type]) groups[type] = [];
    groups[type].push(rg);
  }
  // Sort each group by date desc
  for (const type in groups) {
    groups[type].sort((a, b) =>
      (b.first_release_date || "").localeCompare(a.first_release_date || ""),
    );
  }
  return groups;
});

const typeOrder = ["Album", "Single", "EP", "Broadcast", "Other"];
const sortedTypes = computed(() =>
  Object.keys(groupedReleaseGroups.value).sort((a, b) => {
    const aBase = a.split(" + ")[0];
    const bBase = b.split(" + ")[0];
    const aIdx = typeOrder.indexOf(aBase) === -1 ? 99 : typeOrder.indexOf(aBase);
    const bIdx = typeOrder.indexOf(bBase) === -1 ? 99 : typeOrder.indexOf(bBase);
    if (aIdx !== bIdx) return aIdx - bIdx;
    // Pure type before type + secondary
    const aHasSecondary = a.includes(" + ");
    const bHasSecondary = b.includes(" + ");
    if (!aHasSecondary && bHasSecondary) return -1;
    if (aHasSecondary && !bHasSecondary) return 1;
    return a.localeCompare(b);
  }),
);

// FS issues modal (the component fetches issue details on demand)
const badModalRg = ref<ReleaseGroup | null>(null);

// SSE: real-time updates for this artist
// Download state: rg_id -> phase string or null
const downloadState = ref<Map<string, string>>(new Map());

async function downloadReleaseGroup(rg: ReleaseGroup) {
  if (downloadState.value.get(rg.mb_release_group_id)) return;
  const artistName = artist.value?.name ?? "";
  downloadState.value = new Map([...downloadState.value, [rg.mb_release_group_id, "queued"]]);
  try {
    await $fetch(`${apiBase}/api/download/rg/${rg.mb_release_group_id}`, {
      method: "POST",
      credentials: "include",
      body: { title: rg.title, artist_name: artistName },
    });
  } catch {
    downloadState.value = new Map(
      [...downloadState.value].filter(([k]) => k !== rg.mb_release_group_id),
    );
  }
}

const { on, off } = useSSE();

function handleArtistUpdated(data: any) {
  if (data.mb_artist_id !== id) return;
  if (artist.value) {
    artist.value.name = data.name;
    artist.value.disambiguation = data.disambiguation;
    artist.value.image_url = data.image_url;
    artist.value.description = data.description;
  }
  refreshReleaseGroups();
}

function handleFsSynced() {
  refreshReleaseGroups();
}

function handleDownloadProgress(data: any) {
  const rgId = data.rg_id as string;
  if (!rgId) return;
  if (data.phase === "complete" || data.phase === "error") {
    downloadState.value = new Map(
      [...downloadState.value].filter(([k]) => k !== rgId),
    );
    if (data.phase === "complete") refreshReleaseGroups();
  } else {
    downloadState.value = new Map([...downloadState.value, [rgId, data.phase as string]]);
  }
}

on("artist:updated", handleArtistUpdated);
on("fs:synced", handleFsSynced);
on("download:progress", handleDownloadProgress);

onUnmounted(() => {
  off("artist:updated", handleArtistUpdated);
  off("fs:synced", handleFsSynced);
  off("download:progress", handleDownloadProgress);
});
</script>

<template>
  <div v-if="artist">
    <!-- Back -->
    <NuxtLink
      to="/"
      class="inline-flex items-center gap-1 text-sm text-gray-400 hover:text-white transition-colors mb-6"
    >
      <Icon name="lucide:arrow-left" class="w-4 h-4" />
      Back
    </NuxtLink>

    <!-- Header -->
    <div class="mb-6">
      <h1 class="text-3xl font-bold text-white">{{ artist.name }}</h1>
      <p v-if="artist.disambiguation" class="text-sm text-gray-400 mt-1">
        {{ artist.disambiguation }}
      </p>
      <p class="text-sm text-gray-500 mt-1">{{ releaseGroups?.length || 0 }} Release Groups</p>

      <!-- Links -->
      <div class="flex items-center gap-2 mt-3">
        <a
          :href="`https://www.last.fm/music/${encodeURIComponent(artist.name)}`"
          target="_blank"
          class="inline-flex items-center gap-1.5 px-3 py-1.5 text-xs text-gray-300 border border-white/10 rounded-md hover:bg-white/5 transition-colors"
        >
          <BrandIcon name="lastfm" class="w-3.5 h-3.5" />
          Last.fm
        </a>
        <a
          :href="`https://musicbrainz.org/artist/${id}`"
          target="_blank"
          class="inline-flex items-center gap-1.5 px-3 py-1.5 text-xs text-gray-300 border border-white/10 rounded-md hover:bg-white/5 transition-colors"
        >
          <BrandIcon name="musicbrainz" class="w-3.5 h-3.5" />
          MusicBrainz
        </a>
      </div>
    </div>

    <!-- About the Artist -->
    <div v-if="description" class="border-t border-white/5 pt-5 mb-8">
      <h2 class="text-xs font-semibold text-gray-400 uppercase tracking-wider mb-3">
        About the Artist
      </h2>
      <p class="text-sm text-gray-300 leading-relaxed">
        {{ truncatedDescription }}
        <button
          v-if="description.length > MAX_DESCRIPTION_LENGTH"
          @click="showBioModal = true"
          class="text-orange-400 hover:text-orange-300 ml-1 transition-colors"
        >
          Read more
        </button>
      </p>
    </div>

    <!-- Release Groups by Type -->
    <div class="border-t border-white/5 pt-5">
      <h2 class="text-lg font-semibold text-white mb-4">Albums by {{ artist.name }}</h2>

      <div v-for="type in sortedTypes" :key="type" class="mb-8">
        <div class="flex items-center gap-2 mb-3">
          <h3 class="text-sm font-semibold text-white">{{ type }}</h3>
          <span class="text-xs text-gray-500 bg-white/5 px-1.5 py-0.5 rounded">
            {{ groupedReleaseGroups[type].length }}
          </span>
        </div>

        <div
          class="grid grid-cols-2 sm:grid-cols-3 md:grid-cols-4 lg:grid-cols-5 xl:grid-cols-6 gap-3"
        >
          <div
            v-for="rg in groupedReleaseGroups[type]"
            :key="rg.mb_release_group_id"
            class="group rounded-lg overflow-hidden bg-white/5 hover:bg-white/[0.08] transition-colors"
          >
            <!-- Cover -->
            <div
              class="aspect-square bg-white/[0.03] flex items-center justify-center overflow-hidden relative"
            >
              <template v-if="!coverErrors.has(rg.mb_release_group_id)">
                <div
                  v-if="!coverLoaded.has(rg.mb_release_group_id)"
                  class="absolute inset-0 bg-white/5 animate-pulse flex items-center justify-center"
                >
                  <Icon
                    name="lucide:disc"
                    class="w-10 h-10 text-gray-700 animate-spin"
                    style="animation-duration: 3s"
                  />
                </div>
                <NuxtImg
                  :src="`https://coverartarchive.org/release-group/${rg.mb_release_group_id}/front-250`"
                  :alt="rg.title"
                  :class="
                    coverLoaded.has(rg.mb_release_group_id)
                      ? 'w-full h-full object-cover'
                      : 'w-0 h-0 absolute opacity-0'
                  "
                  loading="lazy"
                  @load="onCoverLoad(rg.mb_release_group_id)"
                  @error="onCoverError(rg.mb_release_group_id)"
                />
              </template>
              <Icon v-else name="lucide:disc" class="w-10 h-10 text-gray-700" />
            </div>
            <!-- Info -->
            <div class="p-2.5">
              <p class="text-xs font-medium text-white truncate">{{ rg.title }}</p>
              <div class="flex items-center justify-between mt-0.5">
                <p class="text-[11px] text-gray-500">
                  {{ rg.first_release_date?.slice(0, 4) || "—" }}
                </p>
                <div class="flex items-center gap-1">
                  <!-- Download button: show when MISSING and not already downloading -->
                  <template v-if="rg.fs_state === 'MISSING'">
                    <button
                      v-if="!downloadState.get(rg.mb_release_group_id)"
                      class="text-[10px] px-1 py-0.5 rounded bg-white/5 text-gray-400 hover:text-white hover:bg-white/10 transition-colors"
                      title="Download from Tidal"
                      @click.prevent="downloadReleaseGroup(rg)"
                    >
                      <Icon name="lucide:download" class="w-3 h-3" />
                    </button>
                    <span
                      v-else
                      class="text-[10px] px-1 py-0.5 rounded bg-orange-500/10 text-orange-400 capitalize"
                      :title="downloadState.get(rg.mb_release_group_id)"
                    >
                      <Icon name="lucide:loader-circle" class="w-3 h-3 animate-spin" />
                    </span>
                  </template>

                  <component
                    :is="rg.fs_state === 'BAD' ? 'button' : 'span'"
                    v-if="rg.fs_state"
                    class="text-[10px] font-semibold px-1 py-0.5 rounded"
                    :class="{
                      'bg-green-500/20 text-green-400': rg.fs_state === 'PERFECT',
                      'bg-yellow-500/20 text-yellow-400': rg.fs_state === 'MB_MISS',
                      'bg-red-500/20 text-red-400 hover:bg-red-500/30 cursor-pointer':
                        rg.fs_state === 'BAD',
                      'bg-white/5 text-gray-600': rg.fs_state === 'MISSING',
                    }"
                    :title="rg.fs_state"
                    @click.prevent="rg.fs_state === 'BAD' && (badModalRg = rg)"
                  >
                    {{
                      rg.fs_state === "PERFECT"
                        ? "✓"
                        : rg.fs_state === "MB_MISS"
                          ? "MB?"
                          : rg.fs_state === "BAD"
                            ? "!"
                            : "–"
                    }}
                  </component>
                </div>
              </div>
            </div>
          </div>
        </div>
      </div>

      <p v-if="!releaseGroups?.length" class="text-gray-500 text-sm">
        No release groups available yet.
      </p>
    </div>

    <!-- Bio Modal -->
    <ModalsArtistBio
      :open="showBioModal"
      :name="artist.name"
      :description="description"
      @close="showBioModal = false"
    />

    <!-- BAD Tags Modal -->
    <ModalsTagIssues
      :rg-id="badModalRg?.mb_release_group_id ?? null"
      :rg-title="badModalRg?.title"
      @close="badModalRg = null"
    />
  </div>
</template>
