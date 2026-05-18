import { z } from "zod";
import { useValidatedBody } from "h3-zod";

const loginSchema = z.object({
  username: z.string().trim().min(1, "Username is required"),
});

// POST /api/auth/login - Username-only login (when auth mode is "none")
export default defineEventHandler(async (event) => {
  const config = useRuntimeConfig();

  if (config.public.authMode !== "none") {
    throw createError({ statusCode: 404, statusMessage: "Username login is disabled when OAuth is enabled" });
  }

  const { username } = await useValidatedBody(event, loginSchema);
  const userId = findOrCreateUser(username);
  const sessionId = await createSession(userId);

  setCookie(event, "kyoku_session", sessionId, SESSION_COOKIE_OPTIONS);

  return { ok: true, user: { username } };
});
