import { createResolver, defineNuxtModule } from "@nuxt/kit";

const resolver = createResolver(import.meta.url);

export default defineNuxtConfig({
  compatibilityDate: "2025-07-15",
  devtools: { enabled: true },

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
  vite: {
    optimizeDeps: {
      include: ["@vue/devtools-core", "@vue/devtools-kit"],
    },
  },
});
