<template>
  <span
    class="inline-block w-1.5 h-1.5 rounded-full"
    :class="online ? 'bg-green-400' : 'bg-red-400'"
  />
</template>

<script setup lang="ts">
defineProps<{ online?: boolean }>();
</script>
