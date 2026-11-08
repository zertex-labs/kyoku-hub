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

// Per-card action menu. Rendered in a teleport at fixed coordinates: the card
// clips its children (rounded cover), so a menu inside it gets cut off, and a
// card's stacking context makes z-index unreliable against the next card.
const menuRg = ref<ReleaseGroup | null>(null);
const menuPos = ref({ x: 0, y: 0 });
const downloads = useDownloadStore();

function openMenuFor(rg: ReleaseGroup, event: MouseEvent) {
  const card = (event.currentTarget as HTMLElement).closest(".group") as HTMLElement | null;
  const rect = (card ?? (event.currentTarget as HTMLElement)).getBoundingClientRect();
  menuPos.value = { x: rect.right, y: rect.top + 26 };
  menuRg.value = menuRg.value?.mb_release_group_id === rg.mb_release_group_id ? null : rg;
}

function closeMenu() {
  menuRg.value = null;
}

// An installed album can be looked at two ways: is there a better copy than the
// one I have, and are the tags right. Both run as jobs, so both reuse the modal
// the download page already uses.
function checkQuality(rg: ReleaseGroup) {
  closeMenu();
  downloads.start(rg.mb_release_group_id, albumFor(rg), { kind: "inspect" });
}

function retag(rg: ReleaseGroup) {
  closeMenu();
  downloads.start(rg.mb_release_group_id, albumFor(rg), { kind: "retag" });
}

function albumFor(rg: ReleaseGroup) {
  return {
    title: rg.title,
    artist: artist.value?.name ?? "",
    year: rg.first_release_date?.slice(0, 4) ?? "",
    trackCount: (rg as any).track_count ?? 0,
  };
}

function downloadReleaseGroup(rg: ReleaseGroup) {
  navigateTo(
    `/download/rg/${rg.mb_release_group_id}?title=${encodeURIComponent(rg.title)}&artist=${encodeURIComponent(artist.value?.name ?? "")}`,
  );
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
              <!-- Actions for an album that is already on disk: fade in on hover. -->
              <button
                v-if="rg.fs_state && rg.fs_state !== 'MISSING'"
                class="absolute top-1 right-1 z-10 w-5 h-4 rounded bg-black/70 backdrop-blur-sm border border-white/15 text-gray-300 hover:text-white hover:border-white/30 flex items-center justify-center transition-opacity"
                :class="
                  menuRg?.mb_release_group_id === rg.mb_release_group_id
                    ? 'opacity-100'
                    : 'opacity-0 group-hover:opacity-100'
                "
                title="Album actions"
                @click.stop="openMenuFor(rg, $event)"
              >
                <Icon name="lucide:ellipsis" class="w-3 h-3" />
              </button>

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
                  <!-- Not in the library: one action, the download. -->
                  <button
                    v-if="!rg.fs_state || rg.fs_state === 'MISSING'"
                    class="w-5 h-4 rounded bg-white/5 text-gray-400 hover:text-white hover:bg-white/10 flex items-center justify-center transition-colors"
                    title="Download"
                    @click.prevent="downloadReleaseGroup(rg)"
                  >
                    <Icon name="lucide:download" class="w-3 h-3" />
                  </button>

                  <!-- On disk: the state, with the same 24px footprint so a row of
                       cards does not read as a row of different-sized chips. -->
                  <component
                    :is="rg.fs_state === 'BAD' ? 'button' : 'span'"
                    v-else
                    class="w-5 h-4 rounded flex items-center justify-center transition-colors"
                    :class="{
                      'bg-green-500/20 text-green-400': rg.fs_state === 'PERFECT',
                      'bg-yellow-500/20 text-yellow-400': rg.fs_state === 'MB_MISS',
                      'bg-red-500/20 text-red-400 hover:bg-red-500/30 cursor-pointer':
                        rg.fs_state === 'BAD',
                    }"
                    :title="
                      rg.fs_state === 'PERFECT'
                        ? 'Complete'
                        : rg.fs_state === 'BAD'
                          ? 'Incomplete — click for details'
                          : 'Missing tags'
                    "
                    @click.prevent="rg.fs_state === 'BAD' && (badModalRg = rg)"
                  >
                    <Icon
                      :name="
                        rg.fs_state === 'PERFECT'
                          ? 'lucide:check'
                          : rg.fs_state === 'MB_MISS'
                            ? 'lucide:help-circle'
                            : 'lucide:alert-circle'
                      "
                      class="w-3 h-3"
                    />
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

    <!-- Card actions, teleported: inside the card the rounded cover clips it and
         the card's stacking context decides who paints on top. -->
    <Teleport to="body">
      <div
        v-if="menuRg"
        class="fixed inset-0 z-[70]"
        @click="closeMenu"
        @contextmenu.prevent="closeMenu"
      />
      <div
        v-if="menuRg"
        class="fixed z-[71] w-48 rounded-md border border-white/10 bg-[#17171b] shadow-xl py-1"
        :style="{ left: `${menuPos.x}px`, top: `${menuPos.y}px`, transform: 'translateX(-100%)' }"
        @click.stop
      >
        <button
          class="w-full text-left px-2.5 py-1.5 text-[11px] text-gray-300 hover:bg-white/5 hover:text-white transition-colors"
          @click="checkQuality(menuRg)"
        >
          Check for better quality
        </button>
        <button
          class="w-full text-left px-2.5 py-1.5 text-[11px] text-gray-300 hover:bg-white/5 hover:text-white transition-colors"
          @click="retag(menuRg)"
        >
          Re-tag from MusicBrainz
        </button>
      </div>
    </Teleport>

    <!-- The same modal the download page uses: an inspect or a re-tag started
         from a card reports here, with its log and its sources. -->
    <DownloadJobModal
      :open="downloads.open"
      :job="downloads.job"
      @close="downloads.close()"
      @decide="downloads.decide"
    />

    <!-- BAD Tags Modal -->
    <ModalsTagIssues
      :rg-id="badModalRg?.mb_release_group_id ?? null"
      :rg-title="badModalRg?.title"
      @close="badModalRg = null"
    />
  </div>
</template>
