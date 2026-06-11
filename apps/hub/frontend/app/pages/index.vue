<script setup lang="ts">
const artistStore = useArtistStore();
const { artists: trackedArtists } = storeToRefs(artistStore);
await artistStore.fetchArtists();

const activities = ref<ArtistActivity[]>([]);

const showAddModal = ref(false);
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

// Untagged files (scanned by fs agents but lacking MusicBrainz IDs)
const fsStore = useFsStore();
const { untagged } = storeToRefs(fsStore);
const showUntagged = ref(false);

if (import.meta.client) {
  fsStore.fetchUntagged().catch((e) => console.error("Failed to fetch untagged files:", e));
}

// SSE: real-time artist updates
const { on, off } = useSSE();

function handleFsSynced() {
  fsStore.fetchUntagged().catch(() => {});
}

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
on("fs:synced", handleFsSynced);

onUnmounted(() => {
  off("artist:status", handleArtistStatus);
  off("artist:updated", handleArtistUpdated);
  off("artist:failed", handleArtistFailed);
  off("fs:synced", handleFsSynced);
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
          @click="showAddModal = true"
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
          :data-is-queued="artist.status === 'queued'"
          class="data-[is-queued=true]:pointer-events-none data-[is-queued=true]:opacity-50 group flex items-center gap-3 bg-white/[0.03] px-3 py-2 hover:bg-white/[0.06] transition-colors"
        >
          <!-- Name + disambiguation + links -->
          <div class="flex items-center gap-2 truncate flex-1">
            <p class="text-sm font-medium text-white truncate">
              {{ artist.name }}
              <span
                v-if="artist.disambiguation && artist.disambiguation !== artist.name"
                class="text-gray-500 font-normal text-xs"
                >({{ artist.disambiguation }})</span
              >
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
            <span class="text-[11px] text-orange-400 font-semibold">
              {{ Math.round(artist.release_group_count * 0.75) }}/{{ artist.release_group_count }}
            </span>
            <div class="w-16 h-1 rounded-full bg-white/10 overflow-hidden">
              <div class="h-full rounded-full bg-orange-500" style="width: 75%" />
            </div>
          </template>
          <span v-else class="text-[11px] text-gray-500 italic">queued</span>

          <!-- Last synced -->
          <span class="text-[11px] text-blue-400 w-14 text-right" title="Last synced">
            {{ artist.synced_at ? timeAgo(artist.synced_at) : "—" }}
          </span>

          <!-- Date added -->
          <span class="text-[11px] text-gray-500 w-14 text-right" title="Date added">
            {{ timeAgo(artist.created_at) }}
          </span>

          <!-- Request button -->
          <button
            class="p-0.5 text-gray-600 hover:text-orange-400 transition-colors"
            title="Request sync"
            @click.stop="console.log"
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
            <Icon v-else name="lucide:loader-2" class="w-3.5 h-3.5 text-blue-400 animate-spin" />
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
        {{
          nameFilter.trim()
            ? "No matches. Try a different filter or add a new artist."
            : "No artists yet. Add one to get started."
        }}
      </p>

      <!-- Untagged files -->
      <div
        v-if="untagged.length"
        class="mt-4 rounded-lg border border-yellow-500/20 bg-yellow-500/[0.03] overflow-hidden"
      >
        <button
          class="w-full flex items-center gap-2 px-3 py-2.5 text-left hover:bg-yellow-500/[0.04] transition-colors"
          @click="showUntagged = !showUntagged"
        >
          <Icon name="lucide:file-question" class="w-3.5 h-3.5 text-yellow-400 shrink-0" />
          <span class="text-xs font-medium text-yellow-400">
            {{ untagged.length }} untagged file{{ untagged.length === 1 ? "" : "s" }}
          </span>
          <span class="text-[11px] text-gray-500 truncate">missing MusicBrainz tags</span>
          <div class="flex-1" />
          <Icon
            :name="showUntagged ? 'lucide:chevron-up' : 'lucide:chevron-down'"
            class="w-3.5 h-3.5 text-gray-500 shrink-0"
          />
        </button>
        <div
          v-if="showUntagged"
          class="max-h-56 overflow-y-auto divide-y divide-white/5 border-t border-white/5"
        >
          <div v-for="t in untagged" :key="`${t.agent_id}:${t.rpath}`" class="px-3 py-2">
            <p class="text-[11px] text-gray-300 font-mono truncate" :title="t.rpath">
              {{ t.rpath }}
            </p>
            <p v-if="t.artist || t.album" class="text-[11px] text-gray-500 truncate mt-0.5">
              {{ [t.artist, t.album].filter(Boolean).join(" — ") }}
            </p>
          </div>
          <p class="px-3 py-2 text-[11px] text-gray-600">
            Tag these files with MusicBrainz Picard so kyoku can match them to release groups.
          </p>
        </div>
      </div>
    </aside>

    <!-- Add Artist Modal -->
    <ModalsAddArtist :open="showAddModal" @close="showAddModal = false" />
  </div>
</template>
