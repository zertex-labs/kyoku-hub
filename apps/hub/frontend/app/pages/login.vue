<script setup lang="ts">
definePageMeta({ layout: false, middleware: [] });

const config = useRuntimeConfig();
const apiBase = config.public.apiBaseUrl as string;
</script>

<template>
  <div class="min-h-screen bg-[#0d0d0f] flex items-center justify-center px-4">
    <!-- Gradient overlay -->
    <div class="fixed inset-0 pointer-events-none">
      <div
        class="absolute bottom-0 left-0 right-0 h-[40%] bg-gradient-to-t from-purple-950/30 to-transparent"
      />
      <div
        class="absolute top-0 left-0 right-0 h-[30%] bg-gradient-to-b from-orange-950/10 to-transparent"
      />
    </div>

    <div class="relative z-10 w-full max-w-sm">
      <!-- Logo -->
      <div class="text-center mb-8">
        <h1 class="text-3xl font-bold">
          <span class="text-white">kyoku</span><span class="text-orange-400">.</span>
        </h1>
      </div>

      <!-- Login card -->
      <div class="bg-white/5 border border-white/10 rounded-xl p-6 backdrop-blur-sm">
        <a
          :href="`${apiBase}/api/auth/authorize`"
          class="flex items-center justify-center gap-3 w-full py-3 px-4 bg-orange-500 hover:bg-orange-400 text-white font-medium rounded-lg transition-colors"
        >
          <Icon name="simple-icons:musicbrainz" class="text-xl" />
          Login with MusicBrainz
        </a>
      </div>
    </div>
  </div>
</template>
