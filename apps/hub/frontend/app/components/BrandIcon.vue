<template>
  <LazyIconMusicBrainz v-if="name === 'musicbrainz'" v-bind="$attrs" />
  <LazyIconLastfm v-else-if="name === 'lastfm'" v-bind="$attrs" />
</template>

<script setup lang="ts">
defineProps<{ name: "musicbrainz" | "lastfm" }>();
</script>

