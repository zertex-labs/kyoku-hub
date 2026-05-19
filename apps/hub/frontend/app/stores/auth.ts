export const useAuthStore = defineStore("auth", () => {
  const config = useRuntimeConfig();
  const apiBase = config.public.apiBaseUrl as string;

  const user = ref<{ username: string } | null>(null);
  const authenticated = ref(false);
  const loading = ref(true);

  async function fetchSession() {
    loading.value = true;
    try {
      const data = await $fetch<{ authenticated: boolean; user: { username: string } | null }>(
        `${apiBase}/api/auth/session`,
        { credentials: "include" },
      );
      authenticated.value = data.authenticated;
      user.value = data.user;
    } catch {
      authenticated.value = false;
      user.value = null;
    } finally {
      loading.value = false;
    }
  }

  async function logout() {
    await $fetch(`${apiBase}/api/auth/logout`, { method: "POST", credentials: "include" });
    authenticated.value = false;
    user.value = null;
    navigateTo("/login");
  }

  return { user, authenticated, loading, fetchSession, logout };
});
