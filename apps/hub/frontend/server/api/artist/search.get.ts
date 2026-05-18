import { z } from "zod";
import { useValidatedQuery } from "h3-zod";

const searchSchema = z.object({
  q: z.string().min(1),
  offset: z.coerce.number().min(0).optional(),
  limit: z.coerce.number().min(1).max(50).optional(),
});

// GET /api/artist/search - Search MusicBrainz for artists
export default defineEventHandler(async (event) => {
  const { q, offset = 0, limit = 10 } = await useValidatedQuery(event, searchSchema);

  // Fetch current page + next page for prefetch
  const prefetchLimit = limit + limit;

  const res = await mbApi.search("artist", {
    offset,
    limit: prefetchLimit,
    query: `artist: ${q} OR primary_alias: ${q}`,
  });

  const currentPage = res.artists.slice(0, limit);
  const nextPage = res.artists.slice(limit);

  return {
    artists: currentPage,
    nextPage,
    total: res.count,
    hasMore: offset + limit < res.count,
  };
});
