import { required } from "@regle/rules";
import { defineStore } from "pinia";
import { useRegle } from "@regle/core";
import { type IArtistMatch } from "musicbrainz-api";

const ARTIST_SEARCH_RESULTS_LIMIT = 10;

export const useMBAristSearchStore = defineStore("mb-artist-search", () => {
  const loadedArtists = shallowRef<IArtistMatch[]>([]);
  const hasMore = ref(false);
  const totalResults = ref(0);
  const prefetchedNextPage = shallowRef<IArtistMatch[]>([]);

  const { r$ } = useRegle(
    { artistName: "" },
    {
      artistName: { required },
    },
  );

  async function doSearch(offset = 0) {
    if (offset === 0) {
      loadedArtists.value = [];
      prefetchedNextPage.value = [];
    }

    const artistName = r$.artistName.$value;
    r$.$reset();

    const res = await $fetch("/api/artist/search", {
      query: {
        q: artistName,
        offset,
        limit: ARTIST_SEARCH_RESULTS_LIMIT,
      },
    });

    loadedArtists.value = [...loadedArtists.value, ...res.artists];
    prefetchedNextPage.value = res.nextPage;
    totalResults.value = res.total;
    hasMore.value = res.hasMore;
  }

  async function loadMore() {
    // If we have a prefetched page, use it immediately
    if (prefetchedNextPage.value.length > 0) {
      const nextPage = prefetchedNextPage.value;
      loadedArtists.value = [...loadedArtists.value, ...nextPage];
      prefetchedNextPage.value = [];

      const newOffset = loadedArtists.value.length;
      hasMore.value = newOffset < totalResults.value;

      // Prefetch the next page in the background
      if (hasMore.value) {
        const artistName = r$.artistName.$value;
        $fetch("/api/artist/search", {
          query: {
            q: artistName,
            offset: newOffset,
            limit: ARTIST_SEARCH_RESULTS_LIMIT,
          },
        }).then((res) => {
          prefetchedNextPage.value = res.nextPage;
          hasMore.value = res.hasMore;
        });
      }
    } else {
      return doSearch(loadedArtists.value.length);
    }
  }

  return {
    r$,
    doSearch,
    loadMore,
    loadedArtists,
    hasMore,
    totalResults,
  };
});
