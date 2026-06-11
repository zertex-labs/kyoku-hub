<script setup lang="ts">
defineProps<{
  open: boolean;
  name: string;
  description: string;
}>();

const emit = defineEmits<{ close: [] }>();
</script>

<template>
  <ModalsBase :open="open" :title="`About ${name}`" max-width="max-w-lg" @close="emit('close')">
    <div class="p-4 mt-1 mb-2">
      <p class="text-sm text-gray-300 leading-relaxed whitespace-pre-line">
        {{ description }}
      </p>
    </div>
  </ModalsBase>
</template>
