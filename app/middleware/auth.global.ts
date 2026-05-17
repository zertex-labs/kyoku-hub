export default defineNuxtRouteMiddleware(async (to) => {
  const authStore = useAuthStore();

  // Fetch session if not loaded yet
  if (authStore.loading) {
    await authStore.fetchSession();
  }

  // Allow access to login page
  if (to.path === "/login") {
    if (authStore.authenticated) {
      return navigateTo("/");
    }
    return;
  }

  // Redirect to login if not authenticated
  if (!authStore.authenticated) {
    return navigateTo("/login");
  }
});
