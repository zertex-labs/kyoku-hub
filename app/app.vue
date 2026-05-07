<script setup lang="ts">
const mBAristSearchStore = useMBAristSearchStore();
const { r$, loadedArtists } = storeToRefs(mBAristSearchStore);
</script>

<template>
  <div class="text-white">
    <input
      class="text-black border-4"
      v-model="r$.$value.artistName"
      :class="{ 'border-lime-400': r$.artistName.$correct, 'border-red-400': r$.artistName.$error }"
    />
    <button @click.prevent="mBAristSearchStore.doSearch(0)">Search</button>

    <div>
      <p v-for="artist in loadedArtists">{{ artist.name }} ({{ artist.disambiguation ?? "" }})</p>
    </div>
    <button @click.prevent="mBAristSearchStore.doSearch(loadedArtists.length)">More</button>
  </div>
</template>
