export const useAuthStore = defineStore("auth", () => {
  const user = ref<{ username: string } | null>(null);
  const authenticated = ref(false);
  const loading = ref(true);

  async function fetchSession() {
    loading.value = true;
    try {
      const fetch = useRequestFetch();
      const data = await fetch<{ authenticated: boolean; user: { username: string } | null }>(
        "/api/auth/session",
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
    await $fetch("/api/auth/logout", { method: "POST" });
    authenticated.value = false;
    user.value = null;
    navigateTo("/login");
  }

  async function loginWithUsername(username: string) {
    const data = await $fetch<{ ok: boolean; user: { username: string } }>(
      "/api/auth/login",
      { method: "POST", body: { username } },
    );
    authenticated.value = true;
    user.value = data.user;
    navigateTo("/");
  }

  return { user, authenticated, loading, fetchSession, logout, loginWithUsername };
});
