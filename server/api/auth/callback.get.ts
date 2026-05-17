// GET /api/auth/callback - MusicBrainz OAuth callback
export default defineEventHandler(async (event) => {
  const config = useRuntimeConfig();
  const query = getQuery(event);

  if (config.public.authMode !== "oauth") {
    throw createError({ statusCode: 404, statusMessage: "OAuth is disabled" });
  }

  const storedState = getCookie(event, "kyoku_oauth_state");
  if (!storedState || storedState !== query.state) {
    throw createError({ statusCode: 400, statusMessage: "Invalid OAuth state" });
  }
  deleteCookie(event, "kyoku_oauth_state");

  if (query.error) {
    throw createError({ statusCode: 400, statusMessage: `OAuth error: ${query.error}` });
  }

  const code = query.code as string;
  if (!code) {
    throw createError({ statusCode: 400, statusMessage: "Missing authorization code" });
  }

  const redirectUri = `${config.public.baseUrl}/api/auth/callback`;

  let tokenRes: {
    access_token: string;
    refresh_token?: string;
    expires_in: number;
    token_type: string;
  };
  try {
    tokenRes = await $fetch("https://musicbrainz.org/oauth2/token", {
      method: "POST",
      headers: { "Content-Type": "application/x-www-form-urlencoded" },
      body: new URLSearchParams({
        grant_type: "authorization_code",
        code,
        client_id: config.musicbrainzClientId,
        client_secret: config.musicbrainzClientSecret,
        redirect_uri: redirectUri,
      }).toString(),
    });
  } catch {
    return sendRedirect(event, "/login?error=token_exchange_failed");
  }

  let userInfo: { sub: string; metabrainz_user_id: number; profile?: string };
  try {
    userInfo = await $fetch("https://musicbrainz.org/oauth2/userinfo", {
      headers: { Authorization: `Bearer ${tokenRes.access_token}` },
    });
  } catch {
    return sendRedirect(event, "/login?error=userinfo_failed");
  }

  const sessionId = generateSessionId();
  setAppSession(sessionId, {
    username: userInfo.sub,
    accessToken: tokenRes.access_token,
    refreshToken: tokenRes.refresh_token,
  });

  setCookie(event, "kyoku_session", sessionId, {
    httpOnly: true,
    maxAge: 60 * 60 * 24 * 30,
    sameSite: "lax",
    path: "/",
  });

  return sendRedirect(event, "/");
});
