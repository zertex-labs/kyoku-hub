// POST /api/auth/logout - Destroys session
export default defineEventHandler((event) => {
  const sessionId = getSessionIdFromEvent(event);
  if (sessionId) {
    deleteSession(sessionId);
    deleteCookie(event, "kyoku_session");
  }
  return { ok: true };
});
