<script setup lang="ts">
const authStore = useAuthStore();
const agentStore = useAgentStore();
const { agents } = storeToRefs(agentStore);

const config = useRuntimeConfig();
const apiBase = config.public.apiBaseUrl as string;

const MAX_AGENTS = 3;

type SectionId = "account" | "agents" | "connections" | "plugins"

const sections: { id: SectionId; label: string; icon: string }[] = [
  { id: "account", label: "Account", icon: "lucide:user" },
  { id: "agents", label: "File System Agents", icon: "lucide:hard-drive" },
  { id: "connections", label: "Connections", icon: "lucide:plug" },
  { id: "plugins", label: "Plugins", icon: "lucide:puzzle" },
];

const route = useRoute();
const router = useRouter();

// Active section is driven by the URL hash so links like /settings#agents work.
const active = ref<SectionId>(
  (sections.find((s) => s.id === route.hash.slice(1))?.id as SectionId) ?? "account",
);

function selectSection(id: SectionId) {
  active.value = id;
  router.replace({ hash: `#${id}` });
}

// --- Agents ---
const loadingAgents = ref(false);

onMounted(async () => {
  loadingAgents.value = true;
  try {
    await agentStore.fetchAgents();
  } catch (e) {
    console.error("Failed to load agents:", e);
  } finally {
    loadingAgents.value = false;
  }
});

// Token generation flow
const generatedToken = ref<string | null>(null);
const generatingToken = ref(false);
const copied = ref<"token" | "snippet" | null>(null);

async function generateToken() {
  if (generatingToken.value || agents.value.length >= MAX_AGENTS) return;
  generatingToken.value = true;
  copied.value = null;
  try {
    generatedToken.value = await agentStore.createToken();
  } catch (e) {
    console.error("Failed to create token:", e);
  } finally {
    generatingToken.value = false;
  }
}

async function copy(text: string, which: "token" | "snippet") {
  try {
    await navigator.clipboard.writeText(text);
    copied.value = which;
    setTimeout(() => (copied.value = null), 2000);
  } catch (e) {
    console.error("Clipboard write failed:", e);
  }
}

function dismissToken() {
  generatedToken.value = null;
  // Refresh in case the agent registered while the token was shown.
  agentStore.fetchAgents().catch(() => {});
}

// Install method shown in the registration card.
type Method = "docker" | "compose" | "manual";
const methods: { id: Method; label: string }[] = [
  { id: "docker", label: "Docker" },
  { id: "compose", label: "Docker Compose" },
  { id: "manual", label: "Manual" },
];
const method = ref<Method>("docker");

const removing = ref<string | null>(null);

async function removeAgent(id: string) {
  if (removing.value) return;
  removing.value = id;
  try {
    await agentStore.deleteAgent(id);
  } catch (e) {
    console.error("Failed to delete agent:", e);
  } finally {
    removing.value = null;
  }
}

// WS/API URLs the fs agent should connect to (derived from the API base).
const hubWsUrl = computed(() => apiBase.replace(/^http/, "ws") + "/ws/agent");
const hubApiUrl = computed(() => apiBase + "/api");

// Image is built from apps/fs/Dockerfile; replace with your published tag if you have one.
const IMAGE = "ghcr.io/zertex-labs/kyoku-fs:latest";

const snippet = computed(() => {
  const token = generatedToken.value ?? "";
  if (method.value === "docker") {
    // --network host lets the agent reach a hub on the same Linux host via localhost.
    return [
      "docker run -d \\",
      "  --name kyoku-fs \\",
      "  --network host \\",
      `  -e AGENT_TOKEN=${token} \\`,
      '  -e AGENT_LABEL="My NAS" \\',
      `  -e HUB_WS_URL=${hubWsUrl.value} \\`,
      `  -e HUB_API_URL=${hubApiUrl.value} \\`,
      "  -e SCAN_DIRS=/music \\",
      "  -v /path/to/music:/music:ro \\",
      "  -v kyoku-fs-data:/data \\",
      `  ${IMAGE}`,
    ].join("\n");
  }
  if (method.value === "compose") {
    return [
      "# from the repo root (apps/fs/docker-compose.yml)",
      `AGENT_TOKEN=${token} \\`,
      'AGENT_LABEL="My NAS" \\',
      "MUSIC_DIR=/path/to/music \\",
      "  docker compose -f apps/fs/docker-compose.yml up -d",
    ].join("\n");
  }
  return [
    `AGENT_TOKEN=${token}`,
    'AGENT_LABEL="My NAS"',
    `HUB_WS_URL=${hubWsUrl.value}`,
    `HUB_API_URL=${hubApiUrl.value}`,
    "SCAN_DIRS=/path/to/music",
  ].join("\n");
});

// What actually lands on the clipboard — strip comment lines so users can paste
// straight into a terminal without deleting anything.
const copyableSnippet = computed(() =>
  snippet.value
    .split("\n")
    .filter((line) => !line.trim().startsWith("#"))
    .join("\n"),
);

const methodHint = computed(() => {
  if (method.value === "docker") return "Replace /path/to/music with the folder holding your library.";
  if (method.value === "compose") return "Uses the bundled compose file; MUSIC_DIR is mounted read-only at /music.";
  return "Set these in the agent's environment (or .env), then run the fs binary.";
});

function formatDate(date: string): string {
  return new Date(date).toLocaleDateString(undefined, {
    year: "numeric",
    month: "short",
    day: "numeric",
  });
}

// --- Connections ---
interface ConnectionStatus {
  provider: string;
  connected: boolean;
}

const connections = ref<ConnectionStatus[]>([]);
const connectionsLoading = ref(false);
const deezerARL = ref("");
const connectingProvider = ref<string | null>(null);
const disconnectingProvider = ref<string | null>(null);

async function fetchConnections() {
  connectionsLoading.value = true;
  try {
    const data = await $fetch<ConnectionStatus[]>(`${apiBase}/api/connections`, {
      credentials: "include",
    });
    connections.value = data ?? [];
  } catch (e) {
    console.error("Failed to load connections:", e);
  } finally {
    connectionsLoading.value = false;
  }
}

function isConnected(provider: string) {
  return connections.value.find((c) => c.provider === provider)?.connected ?? false;
}

async function connectDeezer() {
  if (!deezerARL.value.trim() || connectingProvider.value) return;
  connectingProvider.value = "deezer";
  try {
    await $fetch(`${apiBase}/api/connections/deezer`, {
      method: "PUT",
      credentials: "include",
      body: { arl: deezerARL.value.trim() },
    });
    deezerARL.value = "";
    await fetchConnections();
  } catch (e: any) {
    alert("Failed to connect Deezer: " + (e?.data?.error ?? e?.message ?? "unknown error"));
  } finally {
    connectingProvider.value = null;
  }
}

async function disconnectProvider(provider: string) {
  if (disconnectingProvider.value) return;
  disconnectingProvider.value = provider;
  try {
    await $fetch(`${apiBase}/api/connections/${provider}`, {
      method: "DELETE",
      credentials: "include",
    });
    await fetchConnections();
  } catch (e) {
    console.error("Failed to disconnect:", e);
  } finally {
    disconnectingProvider.value = null;
  }
}

watch(
  active,
  (v) => {
    if (v === "connections") fetchConnections();
  },
  { immediate: true },
);

// ── plugins ──────────────────────────────────────────────────────────────────
// A plugin is installed by dropping a directory on the server; this page only
// configures what is already there, and never uploads anything.
type PluginField = {
  key: string
  type: "string" | "secret" | "bool" | "int" | "select"
  label: string
  help?: string
  default?: unknown
  options?: string[]
  min?: number | null
  max?: number | null
  required?: boolean
}
type PluginView = {
  id: string
  version: string
  author: string
  description: string
  hooks: string[]
  required: boolean
  permissions: { exec?: string[] | null; net?: string[] | null }
  config: PluginField[]
  values: Record<string, unknown>
}

const plugins = ref<PluginView[]>([])
const pluginsLoading = ref(false)
const pluginDraft = ref<Record<string, Record<string, unknown>>>({})
const pluginSaving = ref<string | null>(null)
const pluginSaved = ref<Record<string, boolean>>({})
const pluginError = ref<string | null>(null)

async function loadPlugins() {
  pluginsLoading.value = true
  pluginError.value = null
  try {
    const data = await $fetch<PluginView[]>(`${apiBase}/api/plugins`, { credentials: "include" })
    plugins.value = data ?? []
    const drafts: Record<string, Record<string, unknown>> = {}
    for (const p of plugins.value) drafts[p.id] = { ...p.values }
    pluginDraft.value = drafts
  } catch (e: any) {
    pluginError.value = e?.data?.error ?? "could not load plugins"
  } finally {
    pluginsLoading.value = false
  }
}

async function savePlugin(id: string) {
  pluginSaving.value = id
  pluginError.value = null
  try {
    const body = { ...pluginDraft.value[id] }
    // A blank secret means "leave what is stored": the API never sent it back.
    for (const f of plugins.value.find((p) => p.id === id)?.config ?? []) {
      if (f.type === "secret" && !body[f.key]) delete body[f.key]
    }
    const res = await $fetch<{ values: Record<string, unknown> }>(
      `${apiBase}/api/plugins/${id}/config`,
      { method: "PUT", credentials: "include", body },
    )
    pluginDraft.value[id] = { ...res.values }
    pluginSaved.value[id] = true
    setTimeout(() => (pluginSaved.value[id] = false), 2000)
  } catch (e: any) {
    pluginError.value = e?.data?.error ?? "could not save plugin config"
  } finally {
    pluginSaving.value = null
  }
}

if (import.meta.client) loadPlugins()
</script>

<template>
  <div>
    <h1 class="text-2xl font-bold text-white mb-6">Settings</h1>

    <div class="flex flex-col md:flex-row gap-8">
      <!-- Left: section nav -->
      <nav class="md:w-56 shrink-0">
        <ul class="flex md:flex-col gap-1 overflow-x-auto md:overflow-visible">
          <li v-for="s in sections" :key="s.id">
            <button
              class="flex items-center gap-2.5 w-full px-3 py-2 rounded-lg text-sm font-medium whitespace-nowrap transition-colors"
              :class="
                active === s.id
                  ? 'bg-white/[0.06] text-white'
                  : 'text-gray-400 hover:text-white hover:bg-white/[0.03]'
              "
              @click="selectSection(s.id)"
            >
              <Icon :name="s.icon" class="w-4 h-4 shrink-0" />
              {{ s.label }}
            </button>
          </li>
        </ul>
      </nav>

      <!-- Right: content -->
      <div class="flex-1 min-w-0 max-w-2xl">
        <!-- Account -->
        <section v-if="active === 'account'">
          <h2 class="text-lg font-semibold text-white mb-1">Account</h2>
          <p class="text-sm text-gray-500 mb-6">Your profile and sign-in details.</p>

          <div class="rounded-xl border border-white/10 bg-white/[0.02] divide-y divide-white/5">
            <div class="flex items-center justify-between px-5 py-4">
              <div>
                <p class="text-sm font-medium text-white">Username</p>
                <p class="text-xs text-gray-500 mt-0.5">From your MusicBrainz account</p>
              </div>
              <span class="text-sm text-gray-300">{{ authStore.user?.username ?? "—" }}</span>
            </div>
            <div class="flex items-center justify-between px-5 py-4">
              <div>
                <p class="text-sm font-medium text-white">Sign in</p>
                <p class="text-xs text-gray-500 mt-0.5">Authenticated via MusicBrainz OAuth</p>
              </div>
              <BrandIcon name="musicbrainz" class="w-5 h-5 text-gray-400" />
            </div>
          </div>

          <div class="mt-6">
            <button
              class="inline-flex items-center gap-2 px-4 py-2 text-sm font-medium text-red-400 border border-red-400/20 rounded-lg hover:bg-red-400/10 transition-colors"
              @click="authStore.logout()"
            >
              <Icon name="lucide:log-out" class="w-4 h-4" />
              Log out
            </button>
          </div>
        </section>

        <!-- File System Agents -->
        <section v-else-if="active === 'agents'">
          <div class="flex items-start justify-between gap-4 mb-6">
            <div>
              <h2 class="text-lg font-semibold text-white mb-1">File System Agents</h2>
              <p class="text-sm text-gray-500">
                Agents run on your machine or NAS and expose your local music library to kyoku.
                <span class="text-gray-600">({{ agents.length }}/{{ MAX_AGENTS }})</span>
              </p>
            </div>
            <button
              :disabled="generatingToken || agents.length >= MAX_AGENTS || !!generatedToken"
              class="shrink-0 inline-flex items-center gap-2 px-3.5 py-2 bg-orange-500 hover:bg-orange-400 disabled:opacity-40 disabled:cursor-not-allowed text-white text-sm font-medium rounded-lg transition-colors"
              @click="generateToken"
            >
              <Icon
                :name="generatingToken ? 'lucide:loader-2' : 'lucide:plus'"
                class="w-4 h-4"
                :class="{ 'animate-spin': generatingToken }"
              />
              Add agent
            </button>
          </div>

          <!-- Registration token card -->
          <div
            v-if="generatedToken"
            class="mb-6 rounded-xl border border-orange-400/30 bg-orange-400/[0.04] p-5"
          >
            <div class="flex items-start justify-between gap-3 mb-3">
              <div class="flex items-center gap-2">
                <Icon name="lucide:key-round" class="w-4 h-4 text-orange-400" />
                <h3 class="text-sm font-semibold text-white">Register a new agent</h3>
              </div>
              <button
                class="text-gray-500 hover:text-white transition-colors"
                title="Dismiss"
                @click="dismissToken"
              >
                <Icon name="lucide:x" class="w-4 h-4" />
              </button>
            </div>

            <p class="text-xs text-gray-400 mb-3">
              This one-time token expires in 10 minutes. Start the fs agent using one of the methods
              below, then it will register itself and appear here.
            </p>

            <!-- Token -->
            <div class="flex items-center gap-2 mb-4">
              <code
                class="flex-1 min-w-0 truncate text-xs text-orange-300 bg-black/30 border border-white/10 rounded-md px-3 py-2 font-mono"
              >
                {{ generatedToken }}
              </code>
              <button
                class="shrink-0 inline-flex items-center gap-1.5 px-3 py-2 text-xs font-medium text-gray-300 border border-white/10 rounded-md hover:bg-white/5 transition-colors"
                @click="copy(generatedToken!, 'token')"
              >
                <Icon :name="copied === 'token' ? 'lucide:check' : 'lucide:copy'" class="w-3.5 h-3.5" />
                {{ copied === "token" ? "Copied" : "Copy" }}
              </button>
            </div>

            <!-- Method tabs -->
            <div class="flex items-center gap-1 mb-2">
              <button
                v-for="m in methods"
                :key="m.id"
                class="inline-flex items-center gap-1.5 px-2.5 py-1 rounded-md text-xs font-medium transition-colors"
                :class="
                  method === m.id
                    ? 'bg-white/10 text-white'
                    : 'text-gray-400 hover:text-white hover:bg-white/5'
                "
                @click="method = m.id"
              >
                {{ m.label }}
                <span
                  v-if="m.id === 'docker'"
                  class="text-[10px] font-normal text-orange-400/80"
                >recommended</span>
              </button>
            </div>

            <!-- Command snippet -->
            <div class="relative">
              <button
                class="absolute top-2 right-2 inline-flex items-center gap-1.5 px-2 py-1 text-[11px] font-medium text-gray-300 bg-white/5 border border-white/10 rounded-md hover:bg-white/10 transition-colors"
                @click="copy(copyableSnippet, 'snippet')"
              >
                <Icon :name="copied === 'snippet' ? 'lucide:check' : 'lucide:copy'" class="w-3 h-3" />
                {{ copied === "snippet" ? "Copied" : "Copy" }}
              </button>
              <pre
                class="text-[11px] leading-relaxed text-gray-300 bg-black/30 border border-white/10 rounded-md px-3 py-2.5 pr-20 overflow-x-auto font-mono"
              >{{ snippet }}</pre>
            </div>
            <p class="text-[11px] text-gray-500 mt-2">{{ methodHint }}</p>
          </div>

          <!-- Agent list -->
          <div v-if="loadingAgents" class="text-sm text-gray-500">Loading agents…</div>

          <div
            v-else-if="agents.length"
            class="rounded-xl border border-white/10 overflow-hidden divide-y divide-white/5"
          >
            <div
              v-for="agent in agents"
              :key="agent.id"
              class="group flex items-center gap-3 px-5 py-4 bg-white/[0.02]"
            >
              <div class="relative w-9 h-9 shrink-0 rounded-lg bg-white/5 flex items-center justify-center">
                <Icon name="lucide:hard-drive" class="w-4 h-4 text-gray-400" />
                <span
                  class="absolute -top-0.5 -right-0.5 w-2.5 h-2.5 rounded-full border-2 border-[#111114]"
                  :class="agent.connected ? 'bg-green-500' : 'bg-gray-600'"
                  :title="agent.connected ? 'Online' : 'Offline'"
                />
              </div>
              <div class="flex-1 min-w-0">
                <p class="text-sm font-medium text-white truncate">
                  {{ agent.label || "Unnamed agent" }}
                  <span
                    class="ml-1.5 text-[10px] font-semibold uppercase tracking-wide align-middle"
                    :class="agent.connected ? 'text-green-400' : 'text-gray-600'"
                  >
                    {{ agent.connected ? "online" : "offline" }}
                  </span>
                </p>
                <p class="text-xs text-gray-500 mt-0.5">
                  <template v-if="!agent.connected">
                    {{ agent.last_seen_at ? `Last seen ${formatDate(agent.last_seen_at)} · ` : "Never connected · " }}
                  </template>
                  Added {{ formatDate(agent.created_at) }} · <span class="font-mono">{{ agent.id }}</span>
                </p>
              </div>
              <button
                :disabled="removing === agent.id"
                class="shrink-0 p-1.5 text-gray-600 opacity-0 group-hover:opacity-100 hover:text-red-400 disabled:opacity-100 transition-all"
                title="Remove agent"
                @click="removeAgent(agent.id)"
              >
                <Icon
                  :name="removing === agent.id ? 'lucide:loader-2' : 'lucide:trash-2'"
                  class="w-4 h-4"
                  :class="{ 'animate-spin': removing === agent.id }"
                />
              </button>
            </div>
          </div>

          <!-- Empty state -->
          <div
            v-else-if="!generatedToken"
            class="rounded-xl border border-dashed border-white/10 px-5 py-10 text-center"
          >
            <Icon name="lucide:hard-drive" class="w-8 h-8 text-gray-700 mx-auto mb-3" />
            <p class="text-sm text-gray-400">No agents registered yet.</p>
            <p class="text-xs text-gray-600 mt-1">
              Add one to connect your local music library.
            </p>
          </div>
        </section>

        <!-- Connections -->
        <section v-else-if="active === 'connections'">
          <h2 class="text-lg font-semibold text-white mb-1">Connections</h2>
          <p class="text-sm text-gray-500 mb-6">
            Connect your music service accounts so kyoku can search and download releases.
          </p>

          <div v-if="connectionsLoading" class="text-sm text-gray-500">Loading…</div>

          <div v-else class="space-y-4">
            <!-- Deezer -->
            <div class="rounded-xl border border-white/10 bg-white/[0.02] p-5">
              <div class="flex items-center justify-between gap-4 mb-4">
                <div class="flex items-center gap-3">
                  <div class="w-9 h-9 rounded-lg bg-white/5 flex items-center justify-center">
                    <Icon name="lucide:music-2" class="w-4 h-4 text-orange-400" />
                  </div>
                  <div>
                    <p class="text-sm font-medium text-white">Deezer</p>
                    <p class="text-xs" :class="isConnected('deezer') ? 'text-green-400' : 'text-gray-500'">
                      {{ isConnected("deezer") ? "Connected" : "Not connected" }}
                    </p>
                  </div>
                </div>
                <button
                  v-if="isConnected('deezer')"
                  :disabled="disconnectingProvider === 'deezer'"
                  class="inline-flex items-center gap-1.5 px-3 py-1.5 text-xs font-medium text-red-400 border border-red-400/20 rounded-lg hover:bg-red-400/10 disabled:opacity-40 transition-colors"
                  @click="disconnectProvider('deezer')"
                >
                  <Icon
                    :name="disconnectingProvider === 'deezer' ? 'lucide:loader-2' : 'lucide:unplug'"
                    class="w-3.5 h-3.5"
                    :class="{ 'animate-spin': disconnectingProvider === 'deezer' }"
                  />
                  Disconnect
                </button>
              </div>

              <div v-if="!isConnected('deezer')" class="space-y-3">
                <p class="text-xs text-gray-400">
                  Paste your Deezer ARL cookie to connect. Find it in your browser's cookies for
                  <code class="text-orange-300">deezer.com</code> after logging in.
                </p>
                <div class="flex gap-2">
                  <input
                    v-model="deezerARL"
                    type="password"
                    placeholder="ARL cookie value…"
                    class="flex-1 min-w-0 text-xs bg-black/30 border border-white/10 rounded-lg px-3 py-2 text-white placeholder-gray-600 focus:outline-none focus:border-orange-500/50 font-mono"
                    @keydown.enter="connectDeezer"
                  />
                  <button
                    :disabled="!deezerARL.trim() || connectingProvider === 'deezer'"
                    class="shrink-0 inline-flex items-center gap-1.5 px-3.5 py-2 bg-orange-500 hover:bg-orange-400 disabled:opacity-40 disabled:cursor-not-allowed text-white text-xs font-medium rounded-lg transition-colors"
                    @click="connectDeezer"
                  >
                    <Icon
                      :name="connectingProvider === 'deezer' ? 'lucide:loader-2' : 'lucide:plug'"
                      class="w-3.5 h-3.5"
                      :class="{ 'animate-spin': connectingProvider === 'deezer' }"
                    />
                    Connect
                  </button>
                </div>
              </div>
            </div>
          </div>
        </section>

        <section v-else-if="active === 'plugins'">
          <h2 class="text-lg font-semibold text-white mb-1">Plugins</h2>
          <p class="text-sm text-gray-500 mb-6">
            Post-download steps installed on this server. Drop a plugin directory into
            <code class="text-orange-300">~/.config/kyoku/plugins</code> and restart the hub; each
            runs as a separate process, confined to the album it is working on.
          </p>

          <div v-if="pluginsLoading" class="text-sm text-gray-500">Loading…</div>

          <div
            v-else-if="!plugins.length"
            class="rounded-xl border border-white/10 bg-white/[0.02] p-5 text-sm text-gray-500"
          >
            No plugins installed.
          </div>

          <div v-else class="space-y-4">
            <div
              v-for="plugin in plugins"
              :key="plugin.id"
              class="rounded-xl border border-white/10 bg-white/[0.02] p-5"
            >
              <div class="flex items-start justify-between gap-4">
                <div class="min-w-0">
                  <p class="text-sm font-medium text-white">
                    {{ plugin.id }}
                    <span class="text-gray-600 font-normal text-xs">v{{ plugin.version }}</span>
                    <span
                      v-if="plugin.required"
                      class="ml-2 text-[10px] px-1.5 py-0.5 rounded border border-amber-400/25 bg-amber-400/10 text-amber-300"
                      >required</span
                    >
                  </p>
                  <p class="text-xs text-gray-500 mt-1">{{ plugin.description }}</p>
                  <div class="flex flex-wrap gap-1.5 mt-2">
                    <span
                      v-for="hook in plugin.hooks"
                      :key="hook"
                      class="text-[10px] px-1.5 py-0.5 rounded bg-white/5 text-gray-400 font-mono"
                      >{{ hook }}</span
                    >
                    <span
                      v-for="bin in plugin.permissions.exec ?? []"
                      :key="bin"
                      class="text-[10px] px-1.5 py-0.5 rounded bg-white/5 text-gray-500 font-mono"
                      title="May execute"
                      >exec: {{ bin }}</span
                    >
                    <span
                      v-for="host in plugin.permissions.net ?? []"
                      :key="host"
                      class="text-[10px] px-1.5 py-0.5 rounded border border-amber-400/25 bg-amber-400/10 text-amber-300 font-mono"
                      title="May reach"
                      >net: {{ host }}</span
                    >
                  </div>
                </div>
              </div>

              <div v-if="plugin.config.length" class="mt-4 space-y-3">
                <div v-for="field in plugin.config" :key="field.key">
                  <label class="block text-xs text-gray-400 mb-1">
                    {{ field.label || field.key }}
                    <span v-if="field.required" class="text-orange-400">*</span>
                  </label>

                  <select
                    v-if="field.type === 'select'"
                    v-model="pluginDraft[plugin.id][field.key]"
                    class="w-full text-xs bg-black/30 border border-white/10 rounded-lg px-3 py-2 text-white focus:outline-none focus:border-orange-500/50"
                  >
                    <option v-for="opt in field.options" :key="opt" :value="opt">{{ opt }}</option>
                  </select>

                  <label
                    v-else-if="field.type === 'bool'"
                    class="inline-flex items-center gap-2 text-xs text-gray-300"
                  >
                    <input
                      v-model="pluginDraft[plugin.id][field.key]"
                      type="checkbox"
                      class="accent-orange-500"
                    />
                    enabled
                  </label>

                  <input
                    v-else-if="field.type === 'int'"
                    v-model.number="pluginDraft[plugin.id][field.key]"
                    type="number"
                    :min="field.min ?? undefined"
                    :max="field.max ?? undefined"
                    class="w-full text-xs bg-black/30 border border-white/10 rounded-lg px-3 py-2 text-white focus:outline-none focus:border-orange-500/50"
                  />

                  <input
                    v-else
                    v-model="pluginDraft[plugin.id][field.key]"
                    :type="field.type === 'secret' ? 'password' : 'text'"
                    :placeholder="field.type === 'secret' ? 'unchanged' : ''"
                    class="w-full text-xs bg-black/30 border border-white/10 rounded-lg px-3 py-2 text-white placeholder-gray-600 focus:outline-none focus:border-orange-500/50"
                  />

                  <p v-if="field.help" class="text-[11px] text-gray-600 mt-1">{{ field.help }}</p>
                </div>

                <div class="flex items-center gap-3 pt-1">
                  <button
                    :disabled="pluginSaving === plugin.id"
                    class="inline-flex items-center gap-1.5 px-3.5 py-2 bg-orange-500 hover:bg-orange-400 disabled:opacity-40 text-white text-xs font-medium rounded-lg transition-colors"
                    @click="savePlugin(plugin.id)"
                  >
                    <Icon
                      :name="pluginSaving === plugin.id ? 'lucide:loader-2' : 'lucide:check'"
                      class="w-3.5 h-3.5"
                      :class="{ 'animate-spin': pluginSaving === plugin.id }"
                    />
                    Save
                  </button>
                  <span v-if="pluginSaved[plugin.id]" class="text-xs text-green-400">Saved</span>
                </div>
              </div>
            </div>

            <p v-if="pluginError" class="text-xs text-red-400">{{ pluginError }}</p>
          </div>
        </section>
      </div>
    </div>
  </div>
</template>
