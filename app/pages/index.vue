<script setup lang="ts">
const mBAristSearchStore = useMBAristSearchStore();
const { r$, loadedArtists } = storeToRefs(mBAristSearchStore);

type ArtistActivity = {
  name: string;
  image: string;
  type: "release";
  content: string;
  ts: string;
};

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
    type: "release",
    ts: "5 hours ago",
    content: "Released new Single VALUE",
  },
]);
</script>

<template>
  <!-- <div class="text-white"> -->
  <!--   <input -->
  <!--     class="text-black border-4" -->
  <!--     v-model="r$.$value.artistName" -->
  <!--     :class="{ 'border-lime-400': r$.artistName.$correct, 'border-red-400': r$.artistName.$error }" -->
  <!--   /> -->
  <!--   <button @click.prevent="mBAristSearchStore.doSearch(0)">Search</button> -->
  <!---->
  <!--   <div> -->
  <!--     <p v-for="artist in loadedArtists">{{ artist.name }} ({{ artist.disambiguation ?? "" }})</p> -->
  <!--   </div> -->
  <!--   <button @click.prevent="mBAristSearchStore.doSearch(loadedArtists.length)">More</button> -->
  <!-- </div> -->
  <div class="w-full gap-4 flex flex-row">
    <div class="flex flex-grow flex-col">
      <h2 class="pb-4">Activity</h2>
      <div class="flex flex-col gap-y-4">
        <ArtistActivity v-for="activity in activities" :activity />
      </div>
    </div>
    <div class="w-[40%] h-full">
      <h2 class="pb-4">Incomplete Artists</h2>
    </div>
  </div>
</template>
