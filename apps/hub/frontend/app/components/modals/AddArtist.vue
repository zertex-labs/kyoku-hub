<script setup lang="ts">
const props = defineProps<{ open: boolean }>();
const emit = defineEmits<{ close: [] }>();

const artistStore = useArtistStore();
const searchStore = useMBAristSearchStore();
const { r$, loadedArtists, hasMore } = storeToRefs(searchStore);

const searchQuery = ref("");
const searching = ref(false);
const loadingMore = ref(false);
const adding = ref(false);

// Fresh state every time the modal opens.
watch(
  () => props.open,
  (open) => {
    if (open) {
      searchQuery.value = "";
      loadedArtists.value = [];
    }
  },
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
    await artistStore.addArtist(artist.id, artist.name, artist.disambiguation ?? "");
    emit("close");
  } catch (e: any) {
    console.error("Failed to add artist:", e.data?.statusMessage || e.message);
  } finally {
    adding.value = false;
  }
}
</script>

<template>
  <ModalsBase :open="open" title="Add Artist" @close="emit('close')">
    <div class="p-5">
      <form class="flex gap-2" @submit.prevent="handleSearch">
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
        class="mt-4 max-h-64 overflow-y-auto overscroll-contain flex flex-col gap-1"
      >
        <button
          v-for="artist in loadedArtists"
          :key="artist.id"
          class="flex flex-col items-start w-full px-3 py-2.5 rounded-lg text-left hover:bg-white/5 transition-colors"
          @click="addArtist(artist)"
        >
          <span class="text-sm text-white">{{ artist.name }}</span>
          <span v-if="artist.disambiguation" class="text-xs text-gray-500">
            {{ artist.disambiguation }}
          </span>
        </button>

        <!-- Load more -->
        <button
          v-if="hasMore"
          :disabled="loadingMore"
          class="mt-2 w-full py-2 text-sm text-gray-400 border border-white/10 rounded-lg disabled:animate-pulse transition-colors"
          :class="
            loadingMore ? 'opacity-60 cursor-not-allowed' : 'hover:text-white hover:bg-white/5'
          "
          @click="handleLoadMore"
        >
          {{ loadingMore ? "Loading..." : "Load more" }}
        </button>
      </div>

      <!-- No results -->
      <p
        v-else-if="!searching && searchQuery && loadedArtists.length === 0 && r$.artistName.$dirty"
        class="mt-4 text-sm text-gray-500 text-center"
      >
        No results found.
      </p>
    </div>
  </ModalsBase>
</template>
