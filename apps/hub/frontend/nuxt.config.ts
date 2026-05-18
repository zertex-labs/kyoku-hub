export default defineNuxtConfig({
  compatibilityDate: "2025-07-15",
  devtools: { enabled: true },

  runtimeConfig: {
    musicbrainzClientId: "",
    musicbrainzClientSecret: "",
    tokenEncryptionKey: "", // 32-byte hex key for AES-256-GCM
    public: {
      baseUrl: "http://localhost:3000",
      authMode: "oauth",
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
