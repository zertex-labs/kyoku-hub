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
const syncing = ref(false);

async function triggerSync() {
  if (syncing.value) return;
  syncing.value = true;
  try {
    await $fetch(`${useRuntimeConfig().public.apiBaseUrl}/api/fs/sync`, {
      method: "POST",
      credentials: "include",
    });
  } catch (e) {
    console.error("Sync failed:", e);
  } finally {
    syncing.value = false;
  }
}

// SSE: real-time artist updates
const { on, off } = useSSE();

function handleArtistStatus(data: { mb_artist_id: string; status: string }) {
  const artist = trackedArtists.value.find((a) => a.mb_artist_id === data.mb_artist_id);
  if (artist) {
    artist.status = data.status;
  }
}

function handleArtistUpdated(data: any) {
  const artist = trackedArtists.value.find((a) => a.mb_artist_id === data.mb_artist_id);
  if (artist) {
    artist.name = data.name;
    artist.disambiguation = data.disambiguation;
    artist.image_url = data.image_url;
    artist.description = data.description;
    artist.release_group_count = data.release_group_count;
    artist.status = data.status;
    artist.synced_at = new Date().toISOString();
  }
}

function handleArtistFailed(data: { mb_artist_id: string }) {
  // Remove the failed artist from the local list
  const idx = trackedArtists.value.findIndex((a) => a.mb_artist_id === data.mb_artist_id);
  if (idx !== -1) {
    trackedArtists.value.splice(idx, 1);
  }
}

on("artist:status", handleArtistStatus);
on("artist:updated", handleArtistUpdated);
on("artist:failed", handleArtistFailed);

onUnmounted(() => {
  off("artist:status", handleArtistStatus);
  off("artist:updated", handleArtistUpdated);
  off("artist:failed", handleArtistFailed);
});

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
    await artistStore.addArtist(artist.id, artist.name, artist.disambiguation ?? "");
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
      <!-- Header: title + filter + sync + add -->
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
          @click="triggerSync"
          :disabled="syncing"
          class="w-7 h-7 flex items-center justify-center rounded-md border border-white/10 bg-white/[0.03] hover:bg-white/[0.06] text-gray-400 hover:text-white disabled:opacity-40 disabled:cursor-not-allowed transition-colors"
          title="Sync library"
        >
          <Icon name="lucide:refresh-cw" class="w-3.5 h-3.5" :class="{ 'animate-spin': syncing }" />
        </button>
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
              <BrandIcon name="musicbrainz" class="w-3.5 h-3.5" />
            </a>
            <a
              :href="`https://www.last.fm/music/${encodeURIComponent(artist.name)}`"
              target="_blank"
              class="flex-shrink-0 text-gray-600 hover:text-[#d51007] transition-colors"
              title="View on Last.fm"
            >
              <BrandIcon name="lastfm" class="w-3.5 h-3.5" />
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
