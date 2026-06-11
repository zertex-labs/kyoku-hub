<script setup lang="ts">
const props = defineProps<{
  /** Release group to show issues for; null hides the modal. */
  rgId: string | null;
  rgTitle?: string;
}>();

const emit = defineEmits<{ close: [] }>();

const fsStore = useFsStore();

const open = computed(() => !!props.rgId);
const issues = ref<TrackIssue[]>([]);
const loading = ref(false);

// Fetch on demand whenever a release group is selected.
watch(
  () => props.rgId,
  async (rgId) => {
    issues.value = [];
    if (!rgId) return;
    loading.value = true;
    try {
      issues.value = await fsStore.fetchIssues(rgId);
    } catch (e) {
      console.error("Failed to fetch tag issues:", e);
    } finally {
      loading.value = false;
    }
  },
);
</script>

<template>
  <ModalsBase
    :open="open"
    title="Tag Issues"
    :subtitle="rgTitle"
    max-width="max-w-xl"
    @close="emit('close')"
  >
    <div v-if="loading" class="p-5 flex items-center gap-2 text-sm text-gray-500">
      <Icon name="lucide:loader-2" class="w-4 h-4 animate-spin" />
      Loading issues…
    </div>
    <div v-else-if="issues.length === 0" class="p-5 text-sm text-gray-500">
      No issue details available. The agent may be offline — try running a sync.
    </div>
    <div v-else class="divide-y divide-white/5">
      <div v-for="issue in issues" :key="issue.rpath" class="px-5 py-3">
        <p class="text-xs text-gray-300 truncate font-mono mb-1.5">{{ issue.rpath }}</p>
        <div class="flex flex-wrap gap-1">
          <span
            v-for="tag in issue.missing"
            :key="tag"
            class="text-[11px] px-1.5 py-0.5 rounded bg-red-500/15 text-red-400 font-medium"
          >
            {{ tag }}
          </span>
        </div>
      </div>
    </div>
  </ModalsBase>
</template>
