export const useFsStore = defineStore("fs", () => {
  const config = useRuntimeConfig();
  const apiBase = config.public.apiBaseUrl as string;

  const untagged = ref<UntaggedTrack[]>([]);
  const untaggedLoaded = ref(false);

  /** Fetch per-track tag issues for a release group on demand. */
  async function fetchIssues(rgId: string): Promise<TrackIssue[]> {
    const res = await $fetch<TrackIssue[]>(`${apiBase}/api/fs/issues`, {
      params: { rg: rgId },
      credentials: "include",
    });
    return res ?? [];
  }

  /** Fetch all scanned files lacking MusicBrainz IDs. */
  async function fetchUntagged() {
    const res = await $fetch<UntaggedTrack[]>(`${apiBase}/api/fs/untagged`, {
      credentials: "include",
    });
    untagged.value = res ?? [];
    untaggedLoaded.value = true;
  }

  return { untagged, untaggedLoaded, fetchIssues, fetchUntagged };
});
