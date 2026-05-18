<script setup lang="ts">
const mBAristSearchStore = useMBAristSearchStore();
const { r$, loadedArtists } = storeToRefs(mBAristSearchStore);
</script>

<template>
  <NuxtLayout>
    <NuxtPage />
  </NuxtLayout>
</template>
