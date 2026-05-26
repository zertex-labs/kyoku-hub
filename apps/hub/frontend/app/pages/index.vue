<script setup lang="ts">
const artistStore = useArtistStore();
const { artists: trackedArtists } = storeToRefs(artistStore);
await artistStore.fetchArtists();

const searchStore = useMBAristSearchStore();
const { r$, loadedArtists, hasMore } = storeToRefs(searchStore);

const activities = ref<ArtistActivity[]>([]);

const showAddModal = ref(false);
const searchQuery = ref("");
const searching = ref(false);
const loadingMore = ref(false);
const adding = ref(false);
const nameFilter = ref("");

const filteredArtists = computed(() => {
  if (!nameFilter.value.trim()) return trackedArtists.value;
  const q = nameFilter.value.toLowerCase();
  return trackedArtists.value.filter((a) => a.name.toLowerCase().includes(q));
});

function timeAgo(date: string): string {
  const seconds = Math.floor((Date.now() - new Date(date).getTime()) / 1000);
  if (seconds < 60) return `${seconds}s ago`;
  const minutes = Math.floor(seconds / 60);
  if (minutes < 60) return `${minutes}m ago`;
  const hours = Math.floor(minutes / 60);
  if (hours < 24) return `${hours}h ago`;
  const days = Math.floor(hours / 24);
  if (days < 30) return `${days}d ago`;
  const months = Math.floor(days / 30);
  return `${months}mo ago`;
}

async function handleSearch() {
  if (!searchQuery.value.trim()) return;
  searching.value = true;
  try {
    r$.value.artistName.$value = searchQuery.value.trim();
    await searchStore.doSearch();
  } catch (e) {
    console.error("Search failed:", e);
  } finally {
    searching.value = false;
  }
}

async function handleLoadMore() {
  loadingMore.value = true;
  try {
    await searchStore.loadMore();
  } catch (e) {
    console.error("Load more failed:", e);
  } finally {
    loadingMore.value = false;
  }
}

async function addArtist(artist: { id: string; name: string; disambiguation?: string }) {
  if (adding.value) return;
  adding.value = true;
  try {
    await artistStore.addArtist(artist.id);
    showAddModal.value = false;
    searchQuery.value = "";
    loadedArtists.value = [];
  } catch (e: any) {
    console.error("Failed to add artist:", e.data?.statusMessage || e.message);
  } finally {
    adding.value = false;
  }
}

function openModal() {
  showAddModal.value = true;
  searchQuery.value = "";
  loadedArtists.value = [];
}

function closeModal() {
  showAddModal.value = false;
  searchQuery.value = "";
  loadedArtists.value = [];
}

async function removeArtist(mbArtistId: string) {
  try {
    await artistStore.removeArtist(mbArtistId);
  } catch (e: any) {
    console.error("Failed to remove artist:", e.data?.statusMessage || e.message);
  }
}
</script>

<template>
  <div class="flex flex-col-reverse lg:flex-row gap-6">
    <!-- Activity Feed -->
    <div class="flex-1 min-w-0">
      <h2 class="text-sm font-semibold text-gray-400 uppercase tracking-wider mb-4">Activity</h2>
      <div class="flex flex-col gap-3">
        <ArtistActivity
          v-if="activities.length > 0"
          v-for="activity in activities"
          :key="activity.content"
          :activity
        />
        <p v-else class="text-gray-500 text-sm">No activity yet.</p>
      </div>
    </div>

    <!-- Artists -->
    <aside class="w-full lg:w-[45%] shrink-0">
      <!-- Header: title + filter + add -->
      <div class="flex items-center gap-2 mb-4">
        <h2 class="text-sm font-semibold text-gray-400 uppercase tracking-wider">Artists</h2>
        <div class="flex-1" />
        <input
          v-model="nameFilter"
          type="text"
          placeholder="Filter..."
          class="w-24 h-7 bg-white/5 border border-white/10 rounded-md px-2 text-xs text-white placeholder-gray-600 focus:outline-none focus:border-orange-400/50 transition-colors"
        />
        <button
          @click="openModal"
          class="w-7 h-7 flex items-center justify-center rounded-md border border-white/10 bg-white/[0.03] hover:bg-white/[0.06] text-gray-400 hover:text-white transition-colors"
          title="Add Artist"
        >
          <Icon name="lucide:plus" class="w-3.5 h-3.5" />
        </button>
      </div>

      <!-- Artist list -->
      <div
        v-if="filteredArtists.length"
        class="flex flex-col gap-px rounded-lg border border-white/10 overflow-hidden"
      >
        <NuxtLink
          v-for="artist in filteredArtists"
          :key="artist.mb_artist_id"
          :to="`/artist/${artist.mb_artist_id}`"
          class="group flex items-center gap-3 bg-white/[0.03] px-3 py-2 hover:bg-white/[0.06] transition-colors"
        >
          <!-- Name + disambiguation + links -->
          <div class="flex items-center gap-2 truncate flex-1">
            <p class="text-sm font-medium text-white truncate">
              {{ artist.name }}
              <span v-if="artist.disambiguation && artist.disambiguation !== artist.name" class="text-gray-500 font-normal text-xs">({{ artist.disambiguation }})</span>
            </p>
            <a
              :href="`https://musicbrainz.org/artist/${artist.mb_artist_id}`"
              target="_blank"
              class="flex-shrink-0 text-gray-600 hover:text-white transition-colors"
              title="View on MusicBrainz"
            >
              <svg class="w-3.5 h-3.5" viewBox="0 0 256 256" fill="currentColor" xmlns="http://www.w3.org/2000/svg">
                <path d="M128 0C57.3 0 0 57.3 0 128s57.3 128 128 128 128-57.3 128-128S198.7 0 128 0zm-7.5 233.3c-3.2.5-6.4.8-9.7 1l-5.5-29.7-10.9 28.5c-6.7-.7-13.2-2-19.5-3.8l8.7-43.3-17.7 38.5c-5.4-2.3-10.6-5-15.5-8.1l27.2-54.4-2.2-11.5-14.4 57.8c-4.6-3.5-8.9-7.4-12.8-11.6l12.3-62.3-7.1 55.7c-3.5-4.3-6.6-8.9-9.3-13.8L56 118.8l1.7 42.5c-2.2-4.8-4-9.8-5.4-15l-1.4-34.6 1.2 22c-.8-4.7-1.2-9.5-1.2-14.4 0-2 .1-3.9.2-5.8l7.5-19.2-8.2 12.6c3-19.5 11.2-37.2 23.2-51.7l14.1-5.5-12 1.2c12.3-13.5 28-23.8 45.6-29.7l6.9 3.2-3.4-4.5c5.8-1.6 11.8-2.8 18-3.4l11.8 16.2-5.4-16.4c3.5-.1 7-.1 10.5.1l5.1 23.4 1.2-23c6.7.8 13.2 2.2 19.4 4.1L179 49.1l2.1-16.9c5.4 2.2 10.5 4.8 15.4 7.8l-7.3 32.4 13.3-27.2c4.5 3.2 8.7 6.8 12.6 10.7l-16 38.2 20.5-31.3c3.3 3.9 6.2 8.1 8.8 12.5l-29.8 50.8 33-42c2 4.4 3.7 9 5.1 13.7l-38.3 48.8 40-38.4c1 4.6 1.7 9.3 2.1 14.1l-50.4 37.4 49.5-26.2c0 3.5-.2 7-.5 10.4l-62.2 29 58.5-17.2c-1 5.4-2.4 10.6-4.2 15.6l-64.2 15.7 56.6-4.5c-2.3 5.6-5 10.9-8.2 15.9l-62.7 6.5 50.6 4.5c-3.2 4.5-6.8 8.7-10.7 12.6l-49.4-4.4 38 11.9c-4.3 3.9-9 7.4-13.9 10.5l-37.8-14.5 26.3 18.8c-4.5 2.5-9.2 4.7-14.1 6.5L120.5 233z"/>
              </svg>
            </a>
            <a
              :href="`https://www.last.fm/music/${encodeURIComponent(artist.name)}`"
              target="_blank"
              class="flex-shrink-0 text-gray-600 hover:text-[#d51007] transition-colors"
              title="View on Last.fm"
            >
              <svg class="w-3.5 h-3.5" viewBox="0 0 256 256" fill="currentColor" xmlns="http://www.w3.org/2000/svg">
                <path d="M128 0C57.3 0 0 57.3 0 128s57.3 128 128 128 128-57.3 128-128S198.7 0 128 0zm58.5 176.3c-16.8 0-28.9-8.9-36.9-30.5l-2.6-7.1-12.3-34.5c-5.8-16.4-16.9-24.8-33.1-24.8-19.6 0-33.2 14.7-33.2 35.7s13.6 35.7 33.2 35.7c13.2 0 22.4-5.4 28.8-16.8l14.4 8c-9.5 16.2-24.2 24.8-43.8 24.8-28.4 0-49.3-20.5-49.3-51.5S72.6 63.7 101 63.7c22.5 0 36.3 12.6 44.7 37.7l12.9 36.3c5.6 15.7 11.5 22.5 24.4 22.5 10.7 0 18.3-7.3 18.3-17.8 0-3.2-.6-6.2-2-9.1l-14 4.3-6.3-16.4 24.6-7.8c7.6 7.4 11.7 17.4 11.7 29.7-.2 19.8-13.8 33.2-28.8 33.2z"/>
              </svg>
            </a>
          </div>

          <!-- Release groups -->
          <template v-if="artist.status === 'synced'">
            <span class="text-[11px] text-orange-400 font-semibold">{{ Math.round(artist.release_group_count * 0.75) }}/{{ artist.release_group_count }}</span>
            <div class="w-16 h-1 rounded-full bg-white/10 overflow-hidden">
              <div class="h-full rounded-full bg-orange-500" style="width: 75%" />
            </div>
          </template>
          <span v-else class="text-[11px] text-gray-500 italic">syncing...</span>

          <!-- Last synced -->
          <span class="text-[11px] text-blue-400 w-14 text-right" title="Last synced">{{ artist.synced_at ? timeAgo(artist.synced_at) : '—' }}</span>

          <!-- Date added -->
          <span class="text-[11px] text-gray-500 w-14 text-right" title="Date added">{{ timeAgo(artist.created_at) }}</span>

          <!-- Request button -->
          <button
            class="p-0.5 text-gray-600 hover:text-orange-400 transition-colors"
            title="Request sync"
          >
            <Icon name="lucide:refresh-cw" class="w-3 h-3" />
          </button>

          <!-- Status icon -->
          <div :title="artist.status" class="flex-shrink-0">
            <Icon
              v-if="artist.status === 'synced'"
              name="lucide:check-circle"
              class="w-3.5 h-3.5 text-green-500"
            />
            <Icon
              v-else-if="artist.status === 'queued'"
              name="lucide:clock"
              class="w-3.5 h-3.5 text-yellow-500"
            />
            <Icon
              v-else
              name="lucide:loader-2"
              class="w-3.5 h-3.5 text-blue-400 animate-spin"
            />
          </div>

          <!-- Remove button -->
          <button
            @click.prevent="removeArtist(artist.mb_artist_id)"
            class="p-0.5 text-gray-600 opacity-0 group-hover:opacity-100 hover:text-red-400 transition-all"
          >
            <Icon name="lucide:x" class="w-3 h-3" />
          </button>
        </NuxtLink>
      </div>

      <!-- Empty state -->
      <p v-else class="text-gray-500 text-sm">
        {{ nameFilter.trim() ? 'No matches. Try a different filter or add a new artist.' : 'No artists yet. Add one to get started.' }}
      </p>
    </aside>

    <!-- Add Artist Modal -->
    <Teleport to="body">
      <Transition
        enter-active-class="transition duration-150 ease-out"
        enter-from-class="opacity-0"
        enter-to-class="opacity-100"
        leave-active-class="transition duration-100 ease-in"
        leave-from-class="opacity-100"
        leave-to-class="opacity-0"
      >
        <div v-if="showAddModal" class="fixed inset-0 z-50 flex items-center justify-center px-4">
          <!-- Backdrop -->
          <div class="absolute inset-0 bg-black/60 backdrop-blur-sm" @click="closeModal" />

          <!-- Modal -->
          <div
            class="relative w-full max-w-md bg-[#1a1a1f] border border-white/10 rounded-xl shadow-2xl overflow-hidden"
          >
            <!-- Header -->
            <div class="flex items-center justify-between px-5 py-4 border-b border-white/5">
              <h2 class="text-sm font-semibold text-white">Add Artist</h2>
              <button @click="closeModal" class="text-gray-500 hover:text-white transition-colors">
                <Icon name="lucide:x" class="w-5 h-5" />
              </button>
            </div>

            <!-- Search -->
            <div class="p-5">
              <form @submit.prevent="handleSearch" class="flex gap-2">
                <input
                  v-model="searchQuery"
                  type="text"
                  placeholder="Search for an artist..."
                  class="flex-1 bg-white/5 border border-white/10 rounded-lg px-3 py-2 text-sm text-white placeholder-gray-600 focus:outline-none focus:border-orange-400/50 transition-colors"
                  autofocus
                />
                <button
                  type="submit"
                  :disabled="!searchQuery.trim() || searching"
                  class="px-4 py-2 bg-orange-500 hover:bg-orange-400 disabled:opacity-40 disabled:cursor-not-allowed text-white text-sm font-medium rounded-lg transition-colors"
                >
                  <Icon v-if="searching" name="lucide:loader-2" class="w-4 h-4 animate-spin" />
                  <span v-else>Search</span>
                </button>
              </form>

              <!-- Results -->
              <div
                v-if="loadedArtists.length > 0"
                class="mt-4 max-h-64 overflow-y-auto flex flex-col gap-1"
              >
                <button
                  v-for="artist in loadedArtists"
                  :key="artist.id"
                  @click="addArtist(artist)"
                  class="flex flex-col items-start w-full px-3 py-2.5 rounded-lg text-left hover:bg-white/5 transition-colors"
                >
                  <span class="text-sm text-white">{{ artist.name }}</span>
                  <span v-if="artist.disambiguation" class="text-xs text-gray-500">
                    {{ artist.disambiguation }}
                  </span>
                </button>

                <!-- Load more -->
                <button
                  v-if="hasMore"
                  @click="handleLoadMore"
                  :disabled="loadingMore"
                  class="mt-2 w-full py-2 text-sm text-gray-400 border border-white/10 rounded-lg disabled:animate-pulse transition-colors"
                  :class="
                    loadingMore
                      ? 'opacity-60 cursor-not-allowed'
                      : 'hover:text-white hover:bg-white/5'
                  "
                >
                  {{ loadingMore ? "Loading..." : "Load more" }}
                </button>
              </div>

              <!-- No results -->
              <p
                v-else-if="
                  !searching && searchQuery && loadedArtists.length === 0 && r$.artistName.$dirty
                "
                class="mt-4 text-sm text-gray-500 text-center"
              >
                No results found.
              </p>
            </div>
          </div>
        </div>
      </Transition>
    </Teleport>
  </div>
</template>
