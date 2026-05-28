import { defineStore } from "pinia";

export const useArtistStore = defineStore("artist", () => {
  const config = useRuntimeConfig();
  const apiBase = config.public.apiBaseUrl as string;

  const artists = ref<TrackedArtist[]>([]);
  const loaded = ref(false);

  async function fetchArtists() {
    const res = await $fetch<TrackedArtist[]>(`${apiBase}/api/artist/list`, {
      credentials: "include",
    });
    artists.value = res ?? [];
    loaded.value = true;
  }

  async function addArtist(mbid: string, name: string, disambiguation: string) {
    await $fetch(`${apiBase}/api/artist/add`, {
      method: "POST",
      body: { mbid, name, disambiguation },
      credentials: "include",
    });
    await fetchArtists();
  }

  async function removeArtist(mbArtistId: string) {
    await $fetch(`${apiBase}/api/artist/remove`, {
      method: "POST",
      body: { mbid: mbArtistId },
      credentials: "include",
    });
    await fetchArtists();
  }

  return {
    artists,
    loaded,
    fetchArtists,
    addArtist,
    removeArtist,
  };
});
