<script setup lang="ts">
const mBAristSearchStore = useMBAristSearchStore();
const { r$, loadedArtists } = storeToRefs(mBAristSearchStore);

const activities = ref<ArtistActivity[]>([
  {
    name: "Ado",
    image:
      "https://archive.org/download/mbid-e270332c-c637-45ec-bc9e-a670377c101a/mbid-e270332c-c637-45ec-bc9e-a670377c101a-39285887433_thumb250.jpg",
    type: "release",
    ts: "3 hours ago",
    content: "Released new Album Zanmu",
  },
  {
    name: "Ado",
    image:
      "https://archive.org/download/mbid-aaef585f-a419-4b54-9e33-0c12e72ae504/mbid-aaef585f-a419-4b54-9e33-0c12e72ae504-38111318058_thumb250.jpg",
    type: "update",
    ts: "5 hours ago",
    content: "Updated Single VALUE metadata",
  },
  {
    name: "YOASOBI",
    image:
      "https://archive.org/download/mbid-e270332c-c637-45ec-bc9e-a670377c101a/mbid-e270332c-c637-45ec-bc9e-a670377c101a-39285887433_thumb250.jpg",
    type: "delete",
    ts: "1 day ago",
    content: "Removed duplicate Single Idol",
  },
]);

const incompleteArtists = ref<IncompleteArtist[]>([
  {
    name: "Ado",
    image:
      "https://archive.org/download/mbid-e270332c-c637-45ec-bc9e-a670377c101a/mbid-e270332c-c637-45ec-bc9e-a670377c101a-39285887433_thumb250.jpg",
    have: 10,
    total: 25,
  },
  {
    name: "YOASOBI",
    image:
      "https://archive.org/download/mbid-aaef585f-a419-4b54-9e33-0c12e72ae504/mbid-aaef585f-a419-4b54-9e33-0c12e72ae504-38111318058_thumb250.jpg",
    have: 5,
    total: 12,
  },
  {
    name: "Kenshi Yonezu",
    image:
      "https://archive.org/download/mbid-e270332c-c637-45ec-bc9e-a670377c101a/mbid-e270332c-c637-45ec-bc9e-a670377c101a-39285887433_thumb250.jpg",
    have: 18,
    total: 34,
  },
  {
    name: "LiSA",
    image:
      "https://archive.org/download/mbid-aaef585f-a419-4b54-9e33-0c12e72ae504/mbid-aaef585f-a419-4b54-9e33-0c12e72ae504-38111318058_thumb250.jpg",
    have: 7,
    total: 20,
  },
  {
    name: "Eve",
    image:
      "https://archive.org/download/mbid-e270332c-c637-45ec-bc9e-a670377c101a/mbid-e270332c-c637-45ec-bc9e-a670377c101a-39285887433_thumb250.jpg",
    have: 3,
    total: 15,
  },
  {
    name: "Aimer",
    image:
      "https://archive.org/download/mbid-aaef585f-a419-4b54-9e33-0c12e72ae504/mbid-aaef585f-a419-4b54-9e33-0c12e72ae504-38111318058_thumb250.jpg",
    have: 22,
    total: 40,
  },
]);
</script>

<template>
  <div class="flex flex-col-reverse lg:flex-row gap-6">
    <!-- Activity Feed -->
    <div class="flex-1 min-w-0">
      <h2 class="text-sm font-semibold text-gray-400 uppercase tracking-wider mb-4">Activity</h2>
      <div class="flex flex-col gap-3">
        <ArtistActivity v-for="activity in activities" :key="activity.content" :activity />
      </div>
    </div>

    <!-- Incomplete Artists -->
    <aside class="w-full lg:w-[45%] shrink-0">
      <h2 class="text-sm font-semibold text-gray-400 uppercase tracking-wider mb-4">
        Incomplete Artists
      </h2>
      <div class="grid grid-cols-3 sm:grid-cols-4 lg:grid-cols-5 gap-2">
        <NuxtLink
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
      </div>
    </aside>
  </div>
</template>
