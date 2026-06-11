<script setup lang="ts">
/**
 * Generic modal shell. Handles teleport, fade transition, backdrop
 * click-to-close, Escape-to-close, body scroll locking and a scrollable
 * content area that doesn't chain scroll to the page behind.
 *
 * Content goes in the default slot (bring your own padding). The header can
 * be replaced entirely via the #header slot.
 */
const props = withDefaults(
  defineProps<{
    open: boolean;
    title?: string;
    subtitle?: string;
    /** Tailwind max-width class for the panel. */
    maxWidth?: string;
  }>(),
  {
    title: undefined,
    subtitle: undefined,
    maxWidth: "max-w-md",
  },
);

const emit = defineEmits<{ close: [] }>();

function close() {
  emit("close");
}

useBodyScrollLock(computed(() => props.open));

function onKeydown(e: KeyboardEvent) {
  if (e.key === "Escape" && props.open) close();
}

onMounted(() => window.addEventListener("keydown", onKeydown));
onUnmounted(() => window.removeEventListener("keydown", onKeydown));
</script>

<template>
  <Teleport to="body">
    <Transition
      enter-active-class="transition duration-150 ease-out"
      enter-from-class="opacity-0"
      enter-to-class="opacity-100"
      leave-active-class="transition duration-100 ease-in"
      leave-from-class="opacity-100"
      leave-to-class="opacity-0"
    >
      <div
        v-if="open"
        class="fixed inset-0 z-50 flex items-center justify-center px-4"
        role="dialog"
        aria-modal="true"
      >
        <!-- Backdrop: click outside closes -->
        <div class="absolute inset-0 bg-black/60 backdrop-blur-sm" @click="close" />

        <!-- Panel -->
        <div
          class="relative w-full max-h-[80vh] bg-[#1a1a1f] border border-white/10 rounded-xl shadow-2xl flex flex-col"
          :class="maxWidth"
        >
          <!-- Header -->
          <div class="flex items-center justify-between px-5 py-4 border-b border-white/5 shrink-0">
            <slot name="header">
              <div>
                <h2 class="text-sm font-semibold text-white">{{ title }}</h2>
                <p v-if="subtitle" class="text-xs text-gray-500 mt-0.5">{{ subtitle }}</p>
              </div>
            </slot>
            <button class="text-gray-500 hover:text-white transition-colors" @click="close">
              <Icon name="lucide:x" class="w-5 h-5" />
            </button>
          </div>

          <!-- Scrollable content; overscroll-contain stops scroll chaining
               to the page when the area hits its top/bottom. -->
          <div class="overflow-y-auto overscroll-contain flex-1 min-h-0">
            <slot />
          </div>
        </div>
      </div>
    </Transition>
  </Teleport>
</template>
