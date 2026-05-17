import type { H3Event } from "h3";

// Simple in-memory session store (replace with DB/Redis in production)
const sessions = new Map<string, { username: string; accessToken?: string; refreshToken?: string }>();

export function generateSessionId(): string {
  return crypto.randomUUID();
}

export function getAppSession(sessionId: string) {
  return sessions.get(sessionId) ?? null;
}

export function setAppSession(
  sessionId: string,
  data: { username: string; accessToken?: string; refreshToken?: string },
) {
  sessions.set(sessionId, data);
}

export function deleteAppSession(sessionId: string) {
  sessions.delete(sessionId);
}

export function getSessionIdFromEvent(event: H3Event): string | null {
  return getCookie(event, "kyoku_session") ?? null;
}
