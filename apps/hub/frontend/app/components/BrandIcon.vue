<template>
  <IconMusicBrainz v-if="name === 'musicbrainz'" v-bind="$attrs" />
  <IconLastfm v-else-if="name === 'lastfm'" v-bind="$attrs" />
</template>

<script setup lang="ts">
defineProps<{ name: 'musicbrainz' | 'lastfm' }>();
</script>