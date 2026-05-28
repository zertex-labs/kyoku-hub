import { required } from "@regle/rules";
import { defineStore } from "pinia";
import { useRegle } from "@regle/core";

interface ArtistMatch {
  id: string;
  name: string;
  disambiguation?: string;
  score?: number;
}

const ARTIST_SEARCH_RESULTS_LIMIT = 10;

export const useMBAristSearchStore = defineStore("mb-artist-search", () => {
  const config = useRuntimeConfig();
  const apiBase = config.public.apiBaseUrl as string;

  const loadedArtists = shallowRef<ArtistMatch[]>([]);
  const hasMore = ref(false);
  const totalResults = ref(0);
  const prefetchedNextPage = shallowRef<ArtistMatch[]>([]);

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

    const res = await $fetch<{ artists: ArtistMatch[]; nextPage: ArtistMatch[]; total: number; hasMore: boolean }>(`${apiBase}/api/artist/search`, {
      query: {
        q: artistName,
        offset,
        limit: ARTIST_SEARCH_RESULTS_LIMIT,
      },
      credentials: "include",
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
        $fetch<{ artists: ArtistMatch[]; nextPage: ArtistMatch[]; total: number; hasMore: boolean }>(`${apiBase}/api/artist/search`, {
          query: {
            q: artistName,
            offset: newOffset,
            limit: ARTIST_SEARCH_RESULTS_LIMIT,
          },
          credentials: "include",
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
