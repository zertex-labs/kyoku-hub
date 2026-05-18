import { eq } from "drizzle-orm";
import { artists } from "~~/server/database/schema";

// GET /api/artist/list - Get user's tracked artists
export default defineEventHandler((event) => {
  const sessionId = getSessionIdFromEvent(event);
  if (!sessionId) {
    throw createError({ statusCode: 401, statusMessage: "Unauthorized" });
  }

  const session = getSessionWithUser(sessionId);
  if (!session) {
    throw createError({ statusCode: 401, statusMessage: "Unauthorized" });
  }

  return db
    .select()
    .from(artists)
    .where(eq(artists.userId, session.userId))
    .all();
});
