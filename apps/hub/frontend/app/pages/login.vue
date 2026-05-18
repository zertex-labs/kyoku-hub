<script setup lang="ts">
definePageMeta({ layout: false, middleware: [] });

const config = useRuntimeConfig();
const authMode = config.public.authMode as string;

const username = ref("");
const authStore = useAuthStore();

async function handleUsernameLogin() {
  if (!username.value.trim()) return;
  await authStore.loginWithUsername(username.value.trim());
}
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
        <!-- OAuth mode -->
        <template v-if="authMode === 'oauth'">
          <a
            href="/api/auth/authorize"
            class="flex items-center justify-center gap-3 w-full py-3 px-4 bg-orange-500 hover:bg-orange-400 text-white font-medium rounded-lg transition-colors"
          >
            <Icon name="simple-icons:musicbrainz" class="text-xl" />
            Login with MusicBrainz
          </a>
        </template>

        <!-- No-auth mode (username only) -->
        <template v-else>
          <form @submit.prevent="handleUsernameLogin" class="flex flex-col gap-4">
            <div>
              <label for="username" class="block text-sm text-gray-400 mb-1.5">Username</label>
              <input
                id="username"
                v-model="username"
                type="text"
                placeholder="Enter a username"
                class="w-full bg-white/5 border border-white/10 rounded-lg px-3 py-2.5 text-white placeholder-gray-600 focus:outline-none focus:border-orange-400/50 transition-colors"
                autocomplete="username"
              />
            </div>
            <button
              type="submit"
              :disabled="!username.trim()"
              class="w-full py-3 px-4 bg-orange-500 hover:bg-orange-400 disabled:opacity-40 disabled:cursor-not-allowed text-white font-medium rounded-lg transition-colors"
            >
              Enter
            </button>
          </form>
        </template>
      </div>
    </div>
  </div>
</template>
