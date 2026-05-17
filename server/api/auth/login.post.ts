// POST /api/auth/login - Username-only login (when auth mode is "none")
export default defineEventHandler(async (event) => {
  const config = useRuntimeConfig();

  if (config.public.authMode !== "none") {
    throw createError({ statusCode: 404, statusMessage: "Username login is disabled when OAuth is enabled" });
  }

  const body = await readBody<{ username: string }>(event);
  if (!body?.username || body.username.trim().length === 0) {
    throw createError({ statusCode: 400, statusMessage: "Username is required" });
  }

  const username = body.username.trim();
  const userId = findOrCreateUser(username);
  const sessionId = await createSession(userId);

  setCookie(event, "kyoku_session", sessionId, SESSION_COOKIE_OPTIONS);

  return { ok: true, user: { username } };
});
