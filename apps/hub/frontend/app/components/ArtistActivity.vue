<script setup lang="ts">
const $props = defineProps<{ activity: ArtistActivity }>();

const typeStyle = computed(() => {
  switch ($props.activity.type) {
    case "release":
      return "bg-orange-400/20 text-orange-400";
    case "update":
      return "bg-blue-400/20 text-blue-400";
    case "delete":
      return "bg-red-400/20 text-red-400";
  }
});

const typeLabel = computed(() => {
  switch ($props.activity.type) {
    case "release":
      return "Release";
    case "update":
      return "Update";
    case "delete":
      return "Removed";
  }
});
</script>

<template>
  <div
    class="relative rounded-lg overflow-hidden ring-1 ring-white/10 hover:ring-white/25 hover:scale-105 transition-all cursor-pointer"
  >
    <!-- Blurred background image -->
    <div class="absolute inset-0">
      <NuxtImg
        :src="activity.image"
        class="w-full h-full object-cover scale-110 blur-2xl opacity-50"
      />
      <div class="absolute inset-0 bg-black/30" />
    </div>

    <!-- Content -->
    <div class="relative flex gap-4 p-3">
      <NuxtImg class="w-14 h-14 rounded-md object-cover shrink-0" :src="activity.image" />
      <div class="flex-1 min-w-0 flex flex-col justify-center">
        <div class="flex items-center justify-between gap-2">
          <div class="flex items-center gap-2 min-w-0">
            <NuxtLink
              class="font-semibold text-sm text-white hover:text-orange-400 transition-colors truncate"
            >
              {{ activity.name }}
            </NuxtLink>
            <span
              class="text-[10px] font-medium px-1.5 py-0.5 rounded-full shrink-0"
              :class="typeStyle"
            >
              {{ typeLabel }}
            </span>
          </div>
          <span class="text-xs text-gray-500 shrink-0">{{ activity.ts }}</span>
        </div>
        <p class="text-sm text-gray-400 truncate mt-0.5">{{ activity.content }}</p>
      </div>
    </div>
  </div>
</template>
