<script setup lang="ts">
const activities = ref<ArtistActivity[]>([]);
const incompleteArtists = ref<IncompleteArtist[]>([]);
</script>

<template>
  <div
    class="flex flex-col w-full justify-center items-center min-h-[60vh] gap-4"
    v-if="activities.length == 0 && incompleteArtists.length == 0"
  >
    <Icon name="lucide:music" class="w-12 h-12 text-gray-600" />
    <h2 class="text-xl font-semibold text-gray-300">Looks like it's your first time here</h2>
    <p class="text-gray-500 text-sm">Start by adding artists you want to track.</p>
    <NuxtLink
      to="/artists"
      class="mt-2 px-5 py-2.5 bg-orange-500 hover:bg-orange-400 text-white font-medium rounded-lg transition-colors"
    >
      Add your first artists
    </NuxtLink>
  </div>
  <div v-else class="flex flex-col-reverse lg:flex-row gap-6">
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
        <span v-else class="text-gray-400 text-sm">Nothing to show</span>
      </div>
    </div>

    <!-- Incomplete Artists -->
    <aside class="w-full lg:w-[45%] shrink-0">
      <h2 class="text-sm font-semibold text-gray-400 uppercase tracking-wider mb-4">
        Incomplete Artists
      </h2>
      <div class="grid grid-cols-3 sm:grid-cols-4 lg:grid-cols-5 gap-2">
        <NuxtLink
          v-if="incompleteArtists.length > 0"
          v-for="artist in incompleteArtists"
          :key="artist.name"
          class="group relative rounded-lg overflow-hidden aspect-[3/4] cursor-pointer"
        >
          <NuxtImg
            :src="artist.image"
            :alt="artist.name"
            class="w-full h-full object-cover transition-transform group-hover:scale-105"
          />
          <div
            class="absolute inset-0 bg-gradient-to-t from-black/90 via-black/40 to-transparent"
          />
          <div class="absolute bottom-0 left-0 right-0 p-1.5">
            <p class="text-[11px] font-medium text-white truncate">{{ artist.name }}</p>
            <p class="text-[11px] text-orange-400 font-bold">
              {{ artist.have }}/{{ artist.total }}
            </p>
          </div>
        </NuxtLink>
        <span v-else class="text-gray-400 text-sm">Nothing to show</span>
      </div>
    </aside>
  </div>
</template>
