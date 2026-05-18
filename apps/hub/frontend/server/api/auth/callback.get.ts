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
  const codeVerifier = getCookie(event, "kyoku_oauth_verifier");
  if (!codeVerifier) {
    throw createError({ statusCode: 400, statusMessage: "Missing PKCE verifier" });
  }
  deleteCookie(event, "kyoku_oauth_state");
  deleteCookie(event, "kyoku_oauth_verifier");

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
        code_verifier: codeVerifier,
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

  // Find or create user, update tokens
  const userId = findOrCreateUser(userInfo.sub);
  updateUserTokens(userId, tokenRes.access_token, tokenRes.refresh_token, tokenRes.expires_in);

  // Create session
  const sessionId = await createSession(userId);

  setCookie(event, "kyoku_session", sessionId, SESSION_COOKIE_OPTIONS);

  return sendRedirect(event, "/");
});
