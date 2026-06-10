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

// FS issues modal
const fsStore = useFsStore();
const badModalRg = ref<ReleaseGroup | null>(null);
const badModalIssues = computed(() =>
  badModalRg.value ? fsStore.getIssues(badModalRg.value.mb_release_group_id) : [],
);

function openBadModal(rg: ReleaseGroup) {
  badModalRg.value = rg;
}

// SSE: real-time updates for this artist
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

function handleFsSynced(data: any) {
  fsStore.applySync(data);
  refreshReleaseGroups();
}

on("artist:updated", handleArtistUpdated);
on("fs:synced", handleFsSynced);

onUnmounted(() => {
  off("artist:updated", handleArtistUpdated);
  off("fs:synced", handleFsSynced);
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
                <component
                  :is="rg.fs_state === 'BAD' ? 'button' : 'span'"
                  v-if="rg.fs_state"
                  class="text-[10px] font-semibold px-1 py-0.5 rounded"
                  :class="{
                    'bg-green-500/20 text-green-400': rg.fs_state === 'PERFECT',
                    'bg-yellow-500/20 text-yellow-400': rg.fs_state === 'MB_MISS',
                    'bg-red-500/20 text-red-400 hover:bg-red-500/30 cursor-pointer': rg.fs_state === 'BAD',
                    'bg-white/5 text-gray-600': rg.fs_state === 'MISSING',
                  }"
                  :title="rg.fs_state"
                  @click.prevent="rg.fs_state === 'BAD' && openBadModal(rg)"
                >
                  {{ rg.fs_state === 'PERFECT' ? '✓' : rg.fs_state === 'MB_MISS' ? 'MB?' : rg.fs_state === 'BAD' ? '!' : '–' }}
                </component>
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

    <!-- BAD Tags Modal -->
    <Teleport to="body">
      <Transition
        enter-active-class="transition duration-150 ease-out"
        enter-from-class="opacity-0"
        enter-to-class="opacity-100"
        leave-active-class="transition duration-100 ease-in"
        leave-from-class="opacity-100"
        leave-to-class="opacity-0"
      >
        <div v-if="badModalRg" class="fixed inset-0 z-50 flex items-center justify-center px-4">
          <div class="absolute inset-0 bg-black/60 backdrop-blur-sm" @click="badModalRg = null" />
          <div
            class="relative w-full max-w-xl max-h-[80vh] bg-[#1a1a1f] border border-white/10 rounded-xl shadow-2xl flex flex-col"
          >
            <div class="flex items-center justify-between px-5 py-4 border-b border-white/5 shrink-0">
              <div>
                <h2 class="text-sm font-semibold text-white">Tag Issues</h2>
                <p class="text-xs text-gray-500 mt-0.5">{{ badModalRg.title }}</p>
              </div>
              <button @click="badModalRg = null" class="text-gray-500 hover:text-white transition-colors">
                <Icon name="lucide:x" class="w-5 h-5" />
              </button>
            </div>
            <div class="overflow-y-auto flex-1 min-h-0">
              <div v-if="badModalIssues.length === 0" class="p-5 text-sm text-gray-500">
                No issue details available. Run sync to see tag problems.
              </div>
              <div v-else class="divide-y divide-white/5">
                <div v-for="issue in badModalIssues" :key="issue.rpath" class="px-5 py-3">
                  <p class="text-xs text-gray-300 truncate font-mono mb-1.5">{{ issue.rpath }}</p>
                  <div class="flex flex-wrap gap-1">
                    <span
                      v-for="tag in issue.missing"
                      :key="tag"
                      class="text-[11px] px-1.5 py-0.5 rounded bg-red-500/15 text-red-400 font-medium"
                    >
                      {{ tag }}
                    </span>
                  </div>
                </div>
              </div>
            </div>
          </div>
        </div>
      </Transition>
    </Teleport>
  </div>
</template>
