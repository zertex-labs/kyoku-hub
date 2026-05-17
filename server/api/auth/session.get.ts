// GET /api/auth/session - Returns current session info
export default defineEventHandler((event) => {
  const sessionId = getSessionIdFromEvent(event);
  if (!sessionId) {
    return { authenticated: false, user: null };
  }

  const session = getAppSession(sessionId);
  if (!session) {
    return { authenticated: false, user: null };
  }

  return {
    authenticated: true,
    user: { username: session.username },
  };
});
