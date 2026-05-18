import { z } from "zod";
import { useValidatedBody } from "h3-zod";
import { eq, and } from "drizzle-orm";
import { artists } from "~~/server/database/schema";

const addArtistSchema = z.object({
  musicbrainzId: z.uuid({ error: "Invalid MusicBrainz ID" }),
});

// POST /api/artist/add - Add an artist to the user's tracked list
export default defineEventHandler(async (event) => {
  const sessionId = getSessionIdFromEvent(event);
  if (!sessionId) {
    throw createError({ statusCode: 401, statusMessage: "Unauthorized" });
  }

  const session = getSessionWithUser(sessionId);
  if (!session) {
    throw createError({ statusCode: 401, statusMessage: "Unauthorized" });
  }

  const { musicbrainzId } = await useValidatedBody(event, addArtistSchema);

  // Check if already tracking this artist
  const existing = db
    .select({ id: artists.id })
    .from(artists)
    .where(and(eq(artists.userId, session.userId), eq(artists.musicbrainzId, musicbrainzId)))
    .get();

  if (existing) {
    throw createError({ statusCode: 409, statusMessage: "Artist already tracked" });
  }

  // Fetch artist data from MusicBrainz to verify it exists
  let mbArtist;
  try {
    mbArtist = await mbApi.lookup("artist", musicbrainzId);
  } catch (e) {
    console.error("MusicBrainz fetch failed:", e);
    throw createError({ statusCode: 502, statusMessage: "Failed to fetch artist from MusicBrainz" });
  }

  const result = db
    .insert(artists)
    .values({
      userId: session.userId,
      musicbrainzId,
      name: mbArtist.name,
      disambiguation: mbArtist.disambiguation || null,
    })
    .returning()
    .get();

  return result;
});
