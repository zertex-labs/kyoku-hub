<script setup lang="ts">
const collapsed = ref(false);
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
    <nav class="relative z-10 border-b border-white/5">
      <div class="max-w-7xl mx-auto px-1 py-4 flex items-center justify-between">
        <NuxtLink to="/" class="text-xl font-bold">
          <span class="text-white">kyoku</span>
          <span class="text-orange-400">.</span>
        </NuxtLink>
        <div class="flex gap-6 text-sm *:text-gray-400 [&>*:hover]:text-white *:transition-colors">
          <NuxtLink to="/search" active-class="text-white">Search</NuxtLink>
          <NuxtLink to="/transfers" active-class="text-white">Transfers</NuxtLink>
          <NuxtLink to="/chat" active-class="text-white">Chat</NuxtLink>
          <NuxtLink to="/settings" active-class="text-white">Settings</NuxtLink>
        </div>
      </div>
    </nav>

    <!-- Content -->
    <main class="relative z-10 max-w-7xl mx-auto px-1 py-8">
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
