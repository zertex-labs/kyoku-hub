// GET /api/auth/authorize - Redirects user to MusicBrainz OAuth
export default defineEventHandler(async (event) => {
  const config = useRuntimeConfig();

  if (config.public.authMode !== "oauth") {
    throw createError({ statusCode: 404, statusMessage: "OAuth is disabled" });
  }

  const redirectUri = `${config.public.baseUrl}/api/auth/callback`;
  const state = crypto.randomUUID();

  // PKCE: generate code_verifier and code_challenge
  const codeVerifier = generateCodeVerifier();
  const codeChallenge = await generateCodeChallenge(codeVerifier);

  // Store state and code_verifier in short-lived cookies
  const oauthCookieOptions = { httpOnly: true, secure: true, maxAge: 300, sameSite: "lax" as const, path: "/" };
  setCookie(event, "kyoku_oauth_state", state, oauthCookieOptions);
  setCookie(event, "kyoku_oauth_verifier", codeVerifier, oauthCookieOptions);

  const params = new URLSearchParams({
    response_type: "code",
    client_id: config.musicbrainzClientId,
    redirect_uri: redirectUri,
    scope: "profile",
    state,
    access_type: "offline",
    code_challenge: codeChallenge,
    code_challenge_method: "S256",
  });

  return sendRedirect(event, `https://musicbrainz.org/oauth2/authorize?${params.toString()}`);
});

function generateCodeVerifier(): string {
  const array = new Uint8Array(32);
  crypto.getRandomValues(array);
  return base64UrlEncode(array);
}

async function generateCodeChallenge(verifier: string): Promise<string> {
  const encoder = new TextEncoder();
  const data = encoder.encode(verifier);
  const digest = await crypto.subtle.digest("SHA-256", data);
  return base64UrlEncode(new Uint8Array(digest));
}

function base64UrlEncode(bytes: Uint8Array): string {
  let binary = "";
  for (const byte of bytes) {
    binary += String.fromCharCode(byte);
  }
  return btoa(binary).replace(/\+/g, "-").replace(/\//g, "_").replace(/=+$/, "");
}
