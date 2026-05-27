<script setup lang="ts">
const route = useRoute();
const config = useRuntimeConfig();
const apiBase = config.public.apiBaseUrl as string;
const id = route.params.id as string;

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

interface BrainzArtist {
  name: string;
  sort_name: string;
  disambiguation: string;
  type: string;
  country: string;
  last_fetched_at: string;
  created_at: string;
}

interface ReleaseGroup {
  mb_release_group_id: string;
  title: string;
  primary_type: string;
  secondary_types: string;
  disambiguation: string;
  first_release_date: string;
}

const { data: artist } = await useFetch<BrainzArtist>(`${apiBase}/api/artist/${id}`, {
  credentials: "include",
});

const { data: releaseGroups, refresh: refreshReleaseGroups } = await useFetch<ReleaseGroup[]>(
  `${apiBase}/api/artist/${id}/release-groups`,
  { credentials: "include" },
);

// Get description from the artist store (hub stores it)
const artistStore = useArtistStore();
await artistStore.fetchArtists();
const trackedArtist = computed(() =>
  artistStore.artists.find((a) => a.mb_artist_id === id),
);

const description = computed(() => trackedArtist.value?.description || "");
const truncatedDescription = computed(() => {
  if (description.value.length <= 200) return description.value;
  return description.value.slice(0, 200) + "...";
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
    groups[type].sort((a, b) => (b.first_release_date || "").localeCompare(a.first_release_date || ""));
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

// SSE: real-time updates for this artist
const { on, off } = useSSE();

function handleArtistUpdated(data: any) {
  if (data.mb_artist_id !== id) return;
  // Update artist info in-place
  if (artist.value) {
    artist.value.name = data.name;
    artist.value.disambiguation = data.disambiguation;
  }
  // Update tracked artist in store (for description)
  const tracked = artistStore.artists.find((a) => a.mb_artist_id === id);
  if (tracked) {
    tracked.description = data.description;
    tracked.image_url = data.image_url;
    tracked.release_group_count = data.release_group_count;
    tracked.status = data.status;
    tracked.synced_at = new Date().toISOString();
  }
  // Refetch release groups from backend
  refreshReleaseGroups();
}

on("artist:updated", handleArtistUpdated);

onUnmounted(() => {
  off("artist:updated", handleArtistUpdated);
});
</script>

<template>
  <div v-if="artist">
    <!-- Back -->
    <NuxtLink to="/" class="inline-flex items-center gap-1 text-sm text-gray-400 hover:text-white transition-colors mb-6">
      <Icon name="lucide:arrow-left" class="w-4 h-4" />
      Back
    </NuxtLink>

    <!-- Header -->
    <div class="mb-6">
      <h1 class="text-3xl font-bold text-white">{{ artist.name }}</h1>
      <p v-if="artist.disambiguation" class="text-sm text-gray-400 mt-1">{{ artist.disambiguation }}</p>
      <p class="text-sm text-gray-500 mt-1">{{ releaseGroups?.length || 0 }} Release Groups</p>

      <!-- Links -->
      <div class="flex items-center gap-2 mt-3">
        <a
          :href="`https://www.last.fm/music/${encodeURIComponent(artist.name)}`"
          target="_blank"
          class="inline-flex items-center gap-1.5 px-3 py-1.5 text-xs text-gray-300 border border-white/10 rounded-md hover:bg-white/5 transition-colors"
        >
          <svg class="w-3.5 h-3.5" viewBox="0 0 256 256" fill="currentColor"><path d="M128 0C57.3 0 0 57.3 0 128s57.3 128 128 128 128-57.3 128-128S198.7 0 128 0zm58.5 176.3c-16.8 0-28.9-8.9-36.9-30.5l-2.6-7.1-12.3-34.5c-5.8-16.4-16.9-24.8-33.1-24.8-19.6 0-33.2 14.7-33.2 35.7s13.6 35.7 33.2 35.7c13.2 0 22.4-5.4 28.8-16.8l14.4 8c-9.5 16.2-24.2 24.8-43.8 24.8-28.4 0-49.3-20.5-49.3-51.5S72.6 63.7 101 63.7c22.5 0 36.3 12.6 44.7 37.7l12.9 36.3c5.6 15.7 11.5 22.5 24.4 22.5 10.7 0 18.3-7.3 18.3-17.8 0-3.2-.6-6.2-2-9.1l-14 4.3-6.3-16.4 24.6-7.8c7.6 7.4 11.7 17.4 11.7 29.7-.2 19.8-13.8 33.2-28.8 33.2z"/></svg>
          Last.fm
        </a>
        <a
          :href="`https://musicbrainz.org/artist/${id}`"
          target="_blank"
          class="inline-flex items-center gap-1.5 px-3 py-1.5 text-xs text-gray-300 border border-white/10 rounded-md hover:bg-white/5 transition-colors"
        >
          <svg class="w-3.5 h-3.5" viewBox="0 0 256 256" fill="currentColor"><path d="M128 0C57.3 0 0 57.3 0 128s57.3 128 128 128 128-57.3 128-128S198.7 0 128 0zm-7.5 233.3c-3.2.5-6.4.8-9.7 1l-5.5-29.7-10.9 28.5c-6.7-.7-13.2-2-19.5-3.8l8.7-43.3-17.7 38.5c-5.4-2.3-10.6-5-15.5-8.1l27.2-54.4-2.2-11.5-14.4 57.8c-4.6-3.5-8.9-7.4-12.8-11.6l12.3-62.3-7.1 55.7c-3.5-4.3-6.6-8.9-9.3-13.8L56 118.8l1.7 42.5c-2.2-4.8-4-9.8-5.4-15l-1.4-34.6 1.2 22c-.8-4.7-1.2-9.5-1.2-14.4 0-2 .1-3.9.2-5.8l7.5-19.2-8.2 12.6c3-19.5 11.2-37.2 23.2-51.7l14.1-5.5-12 1.2c12.3-13.5 28-23.8 45.6-29.7l6.9 3.2-3.4-4.5c5.8-1.6 11.8-2.8 18-3.4l11.8 16.2-5.4-16.4c3.5-.1 7-.1 10.5.1l5.1 23.4 1.2-23c6.7.8 13.2 2.2 19.4 4.1L179 49.1l2.1-16.9c5.4 2.2 10.5 4.8 15.4 7.8l-7.3 32.4 13.3-27.2c4.5 3.2 8.7 6.8 12.6 10.7l-16 38.2 20.5-31.3c3.3 3.9 6.2 8.1 8.8 12.5l-29.8 50.8 33-42c2 4.4 3.7 9 5.1 13.7l-38.3 48.8 40-38.4c1 4.6 1.7 9.3 2.1 14.1l-50.4 37.4 49.5-26.2c0 3.5-.2 7-.5 10.4l-62.2 29 58.5-17.2c-1 5.4-2.4 10.6-4.2 15.6l-64.2 15.7 56.6-4.5c-2.3 5.6-5 10.9-8.2 15.9l-62.7 6.5 50.6 4.5c-3.2 4.5-6.8 8.7-10.7 12.6l-49.4-4.4 38 11.9c-4.3 3.9-9 7.4-13.9 10.5l-37.8-14.5 26.3 18.8c-4.5 2.5-9.2 4.7-14.1 6.5L120.5 233z"/></svg>
          MusicBrainz
        </a>
      </div>
    </div>

    <!-- About the Artist -->
    <div v-if="description" class="border-t border-white/5 pt-5 mb-8">
      <h2 class="text-xs font-semibold text-gray-400 uppercase tracking-wider mb-3">About the Artist</h2>
      <p class="text-sm text-gray-300 leading-relaxed">
        {{ truncatedDescription }}
        <button
          v-if="description.length > 200"
          @click="showBioModal = true"
          class="text-orange-400 hover:text-orange-300 ml-1 transition-colors"
        >
          Read more
        </button>
      </p>
    </div>

    <!-- Release Groups by Type -->
    <div class="border-t border-white/5 pt-5">
      <h2 class="text-lg font-semibold text-white mb-4">
        Albums by {{ artist.name }}
      </h2>

      <div v-for="type in sortedTypes" :key="type" class="mb-8">
        <div class="flex items-center gap-2 mb-3">
          <h3 class="text-sm font-semibold text-white">{{ type }}</h3>
          <span class="text-xs text-gray-500 bg-white/5 px-1.5 py-0.5 rounded">{{ groupedReleaseGroups[type].length }}</span>
        </div>

        <div class="grid grid-cols-2 sm:grid-cols-3 md:grid-cols-4 lg:grid-cols-5 xl:grid-cols-6 gap-3">
          <div
            v-for="rg in groupedReleaseGroups[type]"
            :key="rg.mb_release_group_id"
            class="group rounded-lg overflow-hidden bg-white/5 hover:bg-white/[0.08] transition-colors"
          >
            <!-- Cover -->
            <div class="aspect-square bg-white/[0.03] flex items-center justify-center overflow-hidden relative">
              <template v-if="!coverErrors.has(rg.mb_release_group_id)">
                <div
                  v-if="!coverLoaded.has(rg.mb_release_group_id)"
                  class="absolute inset-0 bg-white/5 animate-pulse flex items-center justify-center"
                >
                  <Icon name="lucide:disc" class="w-10 h-10 text-gray-700 animate-spin" style="animation-duration: 3s" />
                </div>
                <NuxtImg
                  :src="`https://coverartarchive.org/release-group/${rg.mb_release_group_id}/front-250`"
                  :alt="rg.title"
                  :class="coverLoaded.has(rg.mb_release_group_id) ? 'w-full h-full object-cover' : 'w-0 h-0 absolute opacity-0'"
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
              <p class="text-[11px] text-gray-500 mt-0.5">{{ rg.first_release_date?.slice(0, 4) || '—' }}</p>
            </div>
          </div>
        </div>
      </div>

      <p v-if="!releaseGroups?.length" class="text-gray-500 text-sm">No release groups available yet.</p>
    </div>

    <!-- Bio Modal -->
    <Teleport to="body">
      <Transition
        enter-active-class="transition duration-150 ease-out"
        enter-from-class="opacity-0"
        enter-to-class="opacity-100"
        leave-active-class="transition duration-100 ease-in"
        leave-from-class="opacity-100"
        leave-to-class="opacity-0"
      >
        <div v-if="showBioModal" class="fixed inset-0 z-50 flex items-center justify-center px-4">
          <div class="absolute inset-0 bg-black/60 backdrop-blur-sm" @click="showBioModal = false" />
          <div class="relative w-full max-w-lg max-h-[80vh] bg-[#1a1a1f] border border-white/10 rounded-xl shadow-2xl flex flex-col">
            <div class="flex items-center justify-between px-5 py-4 border-b border-white/5 shrink-0">
              <h2 class="text-sm font-semibold text-white">About {{ artist.name }}</h2>
              <button @click="showBioModal = false" class="text-gray-500 hover:text-white transition-colors">
                <Icon name="lucide:x" class="w-5 h-5" />
              </button>
            </div>
            <div class="p-5 overflow-y-auto flex-1 min-h-0">
              <p class="text-sm text-gray-300 leading-relaxed whitespace-pre-line">{{ description }}</p>
            </div>
          </div>
        </div>
      </Transition>
    </Teleport>
  </div>
</template>
