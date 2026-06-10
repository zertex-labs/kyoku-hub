<script setup lang="ts">
const mBAristSearchStore = useMBAristSearchStore();
const { r$, loadedArtists } = storeToRefs(mBAristSearchStore);

const fsStore = useFsStore();
const { on, off } = useSSE();

const handleFsSynced = (data: any) => fsStore.applySync(data);
on("fs:synced", handleFsSynced);
onUnmounted(() => off("fs:synced", handleFsSynced));
</script>

<template>
  <NuxtLayout>
    <NuxtPage />
  </NuxtLayout>
</template>
