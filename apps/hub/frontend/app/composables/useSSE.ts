type SSEEventHandler = (data: any) => void;

let eventSource: EventSource | null = null;
const handlers = new Map<string, Set<SSEEventHandler>>();

function connect(apiBase: string) {
  if (eventSource) return;

  eventSource = new EventSource(`${apiBase}/api/events`, {
    withCredentials: true,
  });

  const knownEvents = ["artist:status", "artist:updated", "artist:failed", "fs:synced"];
  for (const event of knownEvents) {
    eventSource.addEventListener(event, (e) => {
      dispatch(event, JSON.parse((e as MessageEvent).data));
    });
  }

  eventSource.onerror = () => {
    // EventSource auto-reconnects, nothing to do
  };
}

function disconnect() {
  if (eventSource) {
    eventSource.close();
    eventSource = null;
  }
}

function dispatch(event: string, data: any) {
  const set = handlers.get(event);
  if (set) {
    for (const handler of set) {
      handler(data);
    }
  }
}

export function useSSE() {
  const config = useRuntimeConfig();
  const apiBase = config.public.apiBaseUrl as string;

  if (import.meta.client && !eventSource) {
    connect(apiBase);
  }

  function on(event: string, handler: SSEEventHandler) {
    if (!handlers.has(event)) {
      handlers.set(event, new Set());
    }
    handlers.get(event)!.add(handler);
  }

  function off(event: string, handler: SSEEventHandler) {
    handlers.get(event)?.delete(handler);
  }

  return { on, off, disconnect };
}
