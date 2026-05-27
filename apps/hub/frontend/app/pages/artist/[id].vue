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

// Get description from the artist store (hub stores it)
const artistStore = useArtistStore();
await artistStore.fetchArtists();
const trackedArtist = computed(() => artistStore.artists.find((a) => a.mb_artist_id === id));

const description = computed(() => trackedArtist.value?.description || "");
const truncatedDescription = computed(() => {
  // make sure to not cut of the final word
  let finalLength = MAX_DESCRIPTION_LENGTH;
  let finalChar = description.value[finalLength];
  while (finalChar !== " ") {
    finalLength++;
    finalChar = description.value[finalLength];
  }
  return description.value.substring(0, finalLength) + "...";
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

// SSE: real-time updates for this artist
const { on, off } = useSSE();

function handleArtistUpdated(data: any) {
  console.log(data);
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
              <p class="text-[11px] text-gray-500 mt-0.5">
                {{ rg.first_release_date?.slice(0, 4) || "—" }}
              </p>
            </div>
          </div>
        </div>
      </div>

      <p v-if="!releaseGroups?.length" class="text-gray-500 text-sm">
        No release groups available yet.
      </p>
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
          <div
            class="absolute inset-0 bg-black/60 backdrop-blur-sm"
            @click="showBioModal = false"
          />
          <div
            class="relative w-full max-w-lg max-h-[80vh] bg-[#1a1a1f] border border-white/10 rounded-xl shadow-2xl flex flex-col"
          >
            <div
              class="flex items-center justify-between px-5 py-4 border-b border-white/5 shrink-0"
            >
              <h2 class="text-sm font-semibold text-white">About {{ artist.name }}</h2>
              <button
                @click="showBioModal = false"
                class="text-gray-500 hover:text-white transition-colors"
              >
                <Icon name="lucide:x" class="w-5 h-5" />
              </button>
            </div>
            <div class="p-4 overflow-y-auto flex-1 min-h-0 mt-1 mb-2">
              <p class="text-sm text-gray-300 leading-relaxed whitespace-pre-line max-h-[200px]">
                {{ description }}
              </p>
            </div>
          </div>
        </div>
      </Transition>
    </Teleport>
  </div>
</template>
