import type { H3Event } from "h3";
import type { CookieSerializeOptions } from "cookie-es";
import { eq } from "drizzle-orm";
import { sessions, users } from "../database/schema";

const SESSION_MAX_AGE = 60 * 60 * 24 * 30; // 30 days

export const SESSION_COOKIE_OPTIONS: CookieSerializeOptions = {
  httpOnly: true,
  secure: true,
  maxAge: SESSION_MAX_AGE,
  sameSite: "lax",
  path: "/",
};

export function generateSessionId(): string {
  return crypto.randomUUID();
}

export async function createSession(userId: number, expiresInDays = 30): Promise<string> {
  const sessionId = generateSessionId();
  const expiresAt = new Date(Date.now() + expiresInDays * 24 * 60 * 60 * 1000);

  db.insert(sessions).values({
    id: sessionId,
    userId,
    expiresAt,
  }).run();

  return sessionId;
}

export function getSessionWithUser(sessionId: string) {
  const result = db
    .select({
      sessionId: sessions.id,
      expiresAt: sessions.expiresAt,
      userId: users.id,
      username: users.username,
    })
    .from(sessions)
    .innerJoin(users, eq(sessions.userId, users.id))
    .where(eq(sessions.id, sessionId))
    .get();

  if (!result) return null;
  if (result.expiresAt < new Date()) {
    db.delete(sessions).where(eq(sessions.id, sessionId)).run();
    return null;
  }

  return result;
}

export function deleteSession(sessionId: string) {
  db.delete(sessions).where(eq(sessions.id, sessionId)).run();
}

export function getSessionIdFromEvent(event: H3Event): string | null {
  return getCookie(event, "kyoku_session") ?? null;
}

export function findOrCreateUser(username: string): number {
  const existing = db
    .select({ id: users.id })
    .from(users)
    .where(eq(users.username, username))
    .get();

  if (existing) return existing.id;

  const result = db.insert(users).values({ username }).returning({ id: users.id }).get();
  return result.id;
}

export function updateUserTokens(userId: number, accessToken: string, refreshToken?: string, expiresIn?: number) {
  const tokenExpiresAt = expiresIn ? new Date(Date.now() + expiresIn * 1000) : undefined;
  db.update(users)
    .set({
      musicbrainzAccessToken: encryptToken(accessToken),
      musicbrainzRefreshToken: refreshToken ? encryptToken(refreshToken) : undefined,
      tokenExpiresAt,
    })
    .where(eq(users.id, userId))
    .run();
}

export function getUserTokens(userId: number): { accessToken: string | null; refreshToken: string | null } {
  const user = db
    .select({
      accessToken: users.musicbrainzAccessToken,
      refreshToken: users.musicbrainzRefreshToken,
    })
    .from(users)
    .where(eq(users.id, userId))
    .get();

  if (!user) return { accessToken: null, refreshToken: null };
  return {
    accessToken: user.accessToken ? decryptToken(user.accessToken) : null,
    refreshToken: user.refreshToken ? decryptToken(user.refreshToken) : null,
  };
}

/**
 * Get a valid access token for a user, refreshing if expired.
 * Returns null if no tokens are available or refresh fails.
 */
export async function getValidAccessToken(userId: number): Promise<string | null> {
  const user = db
    .select({
      accessToken: users.musicbrainzAccessToken,
      refreshToken: users.musicbrainzRefreshToken,
      tokenExpiresAt: users.tokenExpiresAt,
    })
    .from(users)
    .where(eq(users.id, userId))
    .get();

  if (!user || !user.accessToken) return null;

  // If token is still valid (with 60s buffer), return it
  if (user.tokenExpiresAt && user.tokenExpiresAt > new Date(Date.now() + 60_000)) {
    return decryptToken(user.accessToken);
  }

  // Token expired — try to refresh
  if (!user.refreshToken) return null;

  const config = useRuntimeConfig();
  const refreshToken = decryptToken(user.refreshToken);

  try {
    const tokenRes: { access_token: string; refresh_token?: string; expires_in: number } = await $fetch(
      "https://musicbrainz.org/oauth2/token",
      {
        method: "POST",
        headers: { "Content-Type": "application/x-www-form-urlencoded" },
        body: new URLSearchParams({
          grant_type: "refresh_token",
          refresh_token: refreshToken,
          client_id: config.musicbrainzClientId,
          client_secret: config.musicbrainzClientSecret,
        }).toString(),
      },
    );

    updateUserTokens(userId, tokenRes.access_token, tokenRes.refresh_token ?? refreshToken, tokenRes.expires_in);
    return tokenRes.access_token;
  } catch {
    return null;
  }
}
