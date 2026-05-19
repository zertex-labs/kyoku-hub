<script setup lang="ts">
const config = useRuntimeConfig();
const apiBase = config.public.apiBaseUrl as string;

const searchStore = useMBAristSearchStore();
const { r$, loadedArtists, hasMore } = storeToRefs(searchStore);

const showAddModal = ref(false);
const searchQuery = ref("");
const searching = ref(false);
const loadingMore = ref(false);
const adding = ref(false);

// Fetch tracked artists reactively
const { data: trackedArtists, refresh: refreshArtists } = await useFetch(
  `${apiBase}/api/artist/list`,
  { credentials: "include" },
);

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
    await $fetch(`${apiBase}/api/artist/add`, {
      method: "POST",
      body: { mbid: artist.id },
      credentials: "include",
    });
    await refreshArtists();
    showAddModal.value = false;
    searchQuery.value = "";
    loadedArtists.value = [];
  } catch (e: any) {
    // TODO: show error toast
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
</script>

<template>
  <div>
    <div class="flex items-center justify-between mb-6">
      <h1 class="text-lg font-semibold text-white">Artists</h1>
      <button
        @click="openModal"
        class="flex items-center gap-2 px-4 py-2 bg-orange-500 hover:bg-orange-400 text-white text-sm font-medium rounded-lg transition-colors"
      >
        <Icon name="lucide:plus" class="w-4 h-4" />
        Add Artist
      </button>
    </div>

    <!-- Artists grid -->
    <div
      v-if="trackedArtists?.length"
      class="grid grid-cols-3 sm:grid-cols-4 md:grid-cols-5 lg:grid-cols-6 gap-3"
    >
      <NuxtLink
        v-for="artist in trackedArtists"
        :key="artist.id"
        class="group relative rounded-lg overflow-hidden aspect-[3/4] cursor-pointer bg-white/5"
      >
        <NuxtImg
          v-if="artist.imageUrl"
          :src="artist.imageUrl"
          :alt="artist.name"
          class="w-full h-full object-cover transition-transform group-hover:scale-105"
        />
        <div v-else class="w-full h-full flex items-center justify-center">
          <Icon name="lucide:music" class="w-8 h-8 text-gray-600" />
        </div>
        <div class="absolute inset-0 bg-gradient-to-t from-black/90 via-black/40 to-transparent" />
        <div class="absolute bottom-0 left-0 right-0 p-2">
          <p class="text-xs font-medium text-white truncate">{{ artist.name }}</p>
          <p v-if="artist.disambiguation" class="text-[11px] text-gray-500 truncate">
            {{ artist.disambiguation }}
          </p>
        </div>
      </NuxtLink>
    </div>

    <!-- Empty state -->
    <div v-else class="flex flex-col items-center justify-center min-h-[40vh] gap-3 text-center">
      <Icon name="lucide:music" class="w-10 h-10 text-gray-600" />
      <p class="text-gray-400 text-sm">No artists yet. Add one to get started.</p>
    </div>

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
