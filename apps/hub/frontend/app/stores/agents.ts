import { defineStore } from "pinia";

export const useAgentStore = defineStore("agents", () => {
  const config = useRuntimeConfig();
  const apiBase = config.public.apiBaseUrl as string;

  const agents = ref<FsAgent[]>([]);
  const loaded = ref(false);

  async function fetchAgents() {
    const res = await $fetch<FsAgent[]>(`${apiBase}/api/agent/list`, {
      credentials: "include",
    });
    agents.value = res ?? [];
    loaded.value = true;
  }

  async function createToken(): Promise<string> {
    const res = await $fetch<{ token: string }>(`${apiBase}/api/agent/token`, {
      method: "POST",
      credentials: "include",
    });
    return res.token;
  }

  async function deleteAgent(id: string) {
    await $fetch(`${apiBase}/api/agent/${id}`, {
      method: "DELETE",
      credentials: "include",
    });
    await fetchAgents();
  }

  return {
    agents,
    loaded,
    fetchAgents,
    createToken,
    deleteAgent,
  };
});
