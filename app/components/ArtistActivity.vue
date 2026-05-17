<script setup lang="ts">
type ArtistActivity = {
  name: string;
  image: string;
  type: "release";
  content: string;
  ts: string;
};

const $props = defineProps<{ activity: ArtistActivity }>();
</script>

<template>
  <div class="border-r-4 border-orange-400 flex flex-row gap-4 w-full rounded-sm bg-orange-950/30">
    <NuxtImg
      class="rounded-l-xl w-[10%] min-w-[100px] object-cover aspect-square"
      :src="activity.image"
    />
    <div class="py-2 pr-4 w-full">
      <div class="flex flex-row justify-between">
        <NuxtLink class="font-bold">Ado</NuxtLink>
        <p class="text-gray-400 text-xs">{{ activity.ts }}</p>
      </div>
      <p class="text-gray-400 text-sm">{{ activity.content }}</p>
    </div>
  </div>
</template>
