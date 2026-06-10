export type TrackIssue = {
  rpath: string;
  missing: string[];
};

export const useFsStore = defineStore("fs", () => {
  // rgId -> list of tracks with issues
  const issues = ref<Record<string, TrackIssue[]>>({});

  function applySync(data: { states?: Record<string, string>; issues?: Record<string, TrackIssue[]> }) {
    if (data.issues) {
      Object.assign(issues.value, data.issues);
    }
  }

  function getIssues(rgId: string): TrackIssue[] {
    return issues.value[rgId] ?? [];
  }

  return { issues, applySync, getIssues };
});
