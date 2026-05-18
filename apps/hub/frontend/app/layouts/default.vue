<script setup lang="ts">
const authStore = useAuthStore();
const userMenuOpen = ref(false);

function toggleUserMenu() {
  userMenuOpen.value = !userMenuOpen.value;
}

function closeUserMenu() {
  userMenuOpen.value = false;
}

onMounted(() => {
  document.addEventListener("click", (e) => {
    const target = e.target as HTMLElement;
    if (!target.closest("[data-user-menu]")) {
      userMenuOpen.value = false;
    }
  });
});
</script>

<template>
  <div class="min-h-screen bg-[#0d0d0f] text-gray-100">
    <!-- Gradient overlay -->
    <div class="fixed inset-0 pointer-events-none">
      <div
        class="absolute bottom-0 left-0 right-0 h-[40%] bg-gradient-to-t from-purple-950/30 to-transparent"
      />
      <div
        class="absolute top-0 left-0 right-0 h-[30%] bg-gradient-to-b from-orange-950/10 to-transparent"
      />
    </div>

    <!-- Navigation -->
    <nav class="relative z-20 border-b border-white/5">
      <div class="max-w-[1400px] mx-auto px-4 sm:px-6 py-4 flex items-center justify-between">
        <NuxtLink to="/" class="text-xl font-bold">
          <span class="text-white">kyoku</span>
          <span class="text-orange-400">.</span>
        </NuxtLink>
        <div
          class="flex gap-4 sm:gap-6 text-sm *:text-gray-400 [&>*:hover]:text-white *:transition-colors"
        >
          <NuxtLink to="/search">Search</NuxtLink>
          <NuxtLink to="/artists">Artists</NuxtLink>
          <NuxtLink to="/chat">Chat</NuxtLink>
        </div>
        <div class="relative" data-user-menu>
          <button
            v-if="authStore.user"
            @click="toggleUserMenu"
            class="flex items-center gap-1.5 text-sm text-gray-400 hover:text-white transition-colors"
          >
            {{ authStore.user.username }}
            <Icon
              name="lucide:chevron-down"
              class="w-4 h-4 transition-transform"
              :class="{ 'rotate-180': userMenuOpen }"
            />
          </button>

          <!-- Dropdown -->
          <Transition
            enter-active-class="transition duration-100 ease-out"
            enter-from-class="opacity-0 scale-95"
            enter-to-class="opacity-100 scale-100"
            leave-active-class="transition duration-75 ease-in"
            leave-from-class="opacity-100 scale-100"
            leave-to-class="opacity-0 scale-95"
          >
            <div
              v-if="userMenuOpen"
              class="absolute right-0 mt-2 w-40 bg-[#1a1a1f] border border-white/10 rounded-lg shadow-xl overflow-hidden"
            >
              <NuxtLink
                to="/settings"
                @click="closeUserMenu"
                class="flex items-center gap-2 px-3 py-2.5 text-sm text-gray-400 hover:text-white hover:bg-white/5 transition-colors"
              >
                <Icon name="lucide:settings" class="w-4 h-4" />
                Settings
              </NuxtLink>
              <button
                @click="authStore.logout()"
                class="flex items-center gap-2 w-full px-3 py-2.5 text-sm text-gray-400 hover:text-red-400 hover:bg-white/5 transition-colors"
              >
                <Icon name="lucide:log-out" class="w-4 h-4" />
                Logout
              </button>
            </div>
          </Transition>
        </div>
      </div>
    </nav>

    <!-- Content -->
    <main class="relative z-10 max-w-[1400px] mx-auto px-4 sm:px-6 py-6 sm:py-8">
      <slot />
    </main>

    <!-- temp: footer from another project; -->
    <!-- <footer -->
    <!--   class="fixed bottom-0 left-0 right-0 z-10 border-t border-white/5 bg-[#0d0d0f]/80 backdrop-blur" -->
    <!-- > -->
    <!--   <div -->
    <!--     class="max-w-7xl mx-auto px-6 py-2 flex items-center justify-between text-xs text-gray-500" -->
    <!--   > -->
    <!--     <div class="flex items-center gap-4"> -->
    <!--       <span>⚡ <StatusDot /> {{ status?.servers_online || 0 }} servers</span> -->
    <!--       <span>↓ {{ status?.active_transfers || 0 }} transfers</span> -->
    <!--     </div> -->
    <!--     <div class="flex items-center gap-4"> -->
    <!--       <span>{{ status?.username || "—" }}</span> -->
    <!--     </div> -->
    <!--   </div> -->
    <!-- </footer> -->
  </div>
</template>
