export default defineNuxtConfig({
  compatibilityDate: "2025-07-15",
  devtools: { enabled: true },
  ssr: false,
  spaLoadingTemplate: true,

  runtimeConfig: {
    public: {
      apiBaseUrl: "http://localhost:8080",
    },
  },

  imports: {
    dirs: ["types"],
  },

  modules: [
    "@nuxt/eslint",
    "@nuxt/fonts",
    "@nuxt/hints",
    "@nuxt/icon",
    "@nuxt/image",
    "@compodium/nuxt",
    "@nuxtjs/color-mode",
    "@nuxtjs/tailwindcss",
    "@regle/nuxt",
    "@pinia/nuxt",
  ],
  tailwindcss: {
    configPath: "tailwind.config.ts",
    cssPath: "~/assets/css/main.css",
  },
  vite: {
    optimizeDeps: {
      include: [
        "@regle/rules",
        "@regle/core",
        "@vue/devtools-core",
        "@vue/devtools-kit",
      ],
    },
  },
  image: {
    domains: ["archive.org"],
  },
});
