import { required, minLength } from "@regle/rules";
import { defineStore } from "pinia";
import { useRegle } from "@regle/core";
import { renderSlot } from "vue";
import { type IArtistMatch } from "musicbrainz-api";

const ARTIST_SEARCH_RESULTS_LIMIT = 10;

export const useMBAristSearchStore = defineStore("mb-artist-search", () => {
  const { $mbApi } = useNuxtApp();

  const loadedArtists = shallowRef<IArtistMatch[]>([]);

  const { r$ } = useRegle(
    { artistName: "" },
    {
      artistName: { required },
    },
  );

  async function doSearch(offset = 0) {
    if (offset == 0) loadedArtists.value = []; // if offset is 0, we assume it's the first search

    const artistName = r$.artistName.$value;
    r$.$reset();

    const res = await $mbApi.search("artist", {
      offset,
      limit: ARTIST_SEARCH_RESULTS_LIMIT,
      query: `artist: ${artistName} OR primary_alias: ${artistName}`,
    });

    loadedArtists.value = [...loadedArtists.value, ...res.artists];
  }

  return {
    r$,
    doSearch,
    loadedArtists,
  };
});
