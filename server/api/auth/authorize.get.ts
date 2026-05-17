// GET /api/auth/authorize - Redirects user to MusicBrainz OAuth
export default defineEventHandler((event) => {
  const config = useRuntimeConfig();

  if (config.public.authMode !== "oauth") {
    throw createError({ statusCode: 404, statusMessage: "OAuth is disabled" });
  }

  const redirectUri = `${config.public.baseUrl}/api/auth/callback`;
  const state = crypto.randomUUID();

  // Store state in a short-lived cookie for CSRF protection
  setCookie(event, "kyoku_oauth_state", state, {
    httpOnly: true,
    maxAge: 300,
    sameSite: "lax",
    path: "/",
  });

  const params = new URLSearchParams({
    response_type: "code",
    client_id: config.musicbrainzClientId,
    redirect_uri: redirectUri,
    scope: "profile",
    state,
    access_type: "offline",
  });

  return sendRedirect(event, `https://musicbrainz.org/oauth2/authorize?${params.toString()}`);
});
