import { defineStore } from "pinia";
import type { Job } from "~/types/download-job";

/**
 * One download job, driven by the server.
 *
 * The hub runs the walk; this store only starts it, listens for `download:job`
 * snapshots and answers decisions. Keeping orchestration server-side is what
 * makes closing the tab harmless — the job does not live in the browser.
 */
export const useDownloadStore = defineStore("download", () => {
  const config = useRuntimeConfig();
  const apiBase = config.public.apiBaseUrl as string;

  const job = ref<Job | null>(null);
  const open = ref(false);
  const starting = ref(false);
  const error = ref<string | null>(null);

  const { on } = useSSE();

  function onJob(data: Job) {
    // Only our job. The event is user-scoped, but a second tab may run another.
    if (!data?.id || data.id !== job.value?.id) return;
    job.value = data;
    if (data.phase === "results" || data.decision) open.value = true;
  }
  on("download:job", onJob);

  async function start(
    rgId: string,
    album: {
      title: string;
      artist: string;
      year: string;
      trackCount: number;
      cover?: string;
      releaseId?: string;
    },
    opts: {
      agentId?: string | null;
      provider?: string;
      albumId?: string;
      mode?: "order" | "best";
      kind?: "download" | "inspect" | "retag";
    } = {},
  ) {
    starting.value = true;
    error.value = null;

    // Open the modal immediately with a provisional job: the click must have a
    // visible effect even when the hub is slow, restarting, or missing the route.
    job.value = {
      album: album as Job["album"],
      kind: opts.kind ?? "download",
      phase: "walking",
      mode: opts.mode ?? "order",
      sources: [],
      log: [{ t: "--:--.-", level: "info", text: "contacting sources…" }],
      progress: null,
      candidates: [],
      decision: null,
    };
    open.value = true;

    try {
      const data = await $fetch<Job>(`${apiBase}/api/download/jobs`, {
        method: "POST",
        credentials: "include",
        body: {
          rg_id: rgId,
          kind: opts.kind ?? "download",
          album,
          agent_id: opts.agentId ?? undefined,
          provider: opts.provider,
          album_id: opts.albumId,
          mode: opts.mode ?? "order",
        },
      });
      job.value = data;
      open.value = true;
    } catch (e: any) {
      const msg =
        e?.data?.error ??
        e?.message ??
        (opts.kind === "retag"
          ? "could not re-tag this album"
          : opts.kind === "inspect"
            ? "could not check this album"
            : "could not start the download");
      error.value = msg;
      // Keep the modal open so the failure is where the user is looking.
      const bogus = job.value;
      job.value = {
        ...(bogus as Job),
        phase: "failed",
        result: msg,
        // The hint is only useful when the route itself is missing; a 409 from a
        // handler that answered is a real answer, and dressing it up as a stale
        // build sends the user looking in the wrong place.
        log: [
          ...(bogus?.log ?? []),
          { t: "--:--.-", level: "error", text: msg },
          ...(e?.status === 404
            ? [
                {
                  t: "--:--.-",
                  level: "warn" as const,
                  text: "the hub has no such endpoint — it may be running an older build",
                },
              ]
            : []),
        ],
      };
    } finally {
      starting.value = false;
    }
  }

  async function decide(choice: string) {
    const id = job.value?.id;
    if (!id) return;
    try {
      const data = await $fetch<Job>(`${apiBase}/api/download/jobs/${id}/decide`, {
        method: "POST",
        credentials: "include",
        body: { choice },
      });
      if (data?.id) job.value = data;
    } catch (e: any) {
      error.value = e?.data?.error ?? e?.message ?? "could not answer";
    }
  }

  function close() {
    // Closing never cancels: the job keeps running server-side and the modal can
    // be reopened from the page.
    open.value = false;
  }

  return { job, open, starting, error, start, decide, close };
});
