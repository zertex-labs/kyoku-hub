<script setup lang="ts">
const route = useRoute();
const router = useRouter();
const config = useRuntimeConfig();
const apiBase = config.public.apiBaseUrl as string;
const id = route.params.id as string;

type PluginField = {
  key: string;
  type: "string" | "secret" | "bool" | "int" | "select";
  label: string;
  help?: string;
  default?: unknown;
  options?: string[];
  min?: number | null;
  max?: number | null;
  required?: boolean;
};

type PluginView = {
  id: string;
  version: string;
  author: string;
  description: string;
  hooks: string[];
  required: boolean;
  permissions: { exec?: string[] | null; net?: string[] | null };
  config: PluginField[];
  values: Record<string, unknown>;
  enabled: boolean;
  updated_at: string;
  manifest: string;
};

const plugin = ref<PluginView | null>(null);
const draft = ref<Record<string, unknown>>({});
const loading = ref(true);
const saving = ref(false);
const savedAt = ref(false);
const toggling = ref(false);
const error = ref<string | null>(null);

async function load() {
  loading.value = true;
  error.value = null;
  try {
    const data = await $fetch<PluginView>(`${apiBase}/api/plugins/${id}`, { credentials: "include" });
    plugin.value = data;
    draft.value = { ...data.values };
  } catch (e: any) {
    error.value = e?.data?.error ?? "could not load plugin";
  } finally {
    loading.value = false;
  }
}

async function toggle() {
  if (!plugin.value) return;
  toggling.value = true;
  error.value = null;
  try {
    const next = !plugin.value.enabled;
    await $fetch(`${apiBase}/api/plugins/${id}`, {
      method: "PUT",
      credentials: "include",
      body: { enabled: next },
    });
    plugin.value.enabled = next;
  } catch (e: any) {
    error.value = e?.data?.error ?? "could not change state";
  } finally {
    toggling.value = false;
  }
}

async function save() {
  saving.value = true;
  error.value = null;
  try {
    const body = { ...draft.value };
    // A blank secret means "leave what is stored": the API never sends it back.
    for (const f of plugin.value?.config ?? []) {
      if (f.type === "secret" && !body[f.key]) delete body[f.key];
    }
    const res = await $fetch<{ values: Record<string, unknown> }>(
      `${apiBase}/api/plugins/${id}/config`,
      { method: "PUT", credentials: "include", body },
    );
    draft.value = { ...res.values };
    savedAt.value = true;
    setTimeout(() => (savedAt.value = false), 2000);
  } catch (e: any) {
    error.value = e?.data?.error ?? "could not save configuration";
  } finally {
    saving.value = false;
  }
}

await load();
</script>

<template>
  <div class="max-w-3xl">
    <button
      class="inline-flex items-center gap-1.5 text-sm text-gray-400 hover:text-white transition-colors mb-6"
      @click="router.push('/settings#plugins')"
    >
      <Icon name="lucide:arrow-left" class="w-4 h-4" />
      Plugins
    </button>

    <div v-if="loading" class="text-sm text-gray-500">Loading…</div>
    <div v-else-if="!plugin" class="text-sm text-red-400">{{ error ?? "Plugin not found." }}</div>

    <div v-else class="space-y-4">
      <!-- Status -->
      <div class="rounded-xl border border-white/10 bg-white/[0.02] p-5">
        <p class="text-sm font-semibold text-white mb-3">Status</p>
        <label class="inline-flex items-center gap-3 cursor-pointer select-none">
          <button
            :disabled="toggling"
            class="relative w-10 h-5 rounded-full transition-colors disabled:opacity-50"
            :class="plugin.enabled ? 'bg-orange-500' : 'bg-white/15'"
            role="switch"
            :aria-checked="plugin.enabled"
            @click="toggle"
          >
            <span
              class="absolute top-0.5 w-4 h-4 rounded-full bg-white transition-all"
              :class="plugin.enabled ? 'left-[1.375rem]' : 'left-0.5'"
            />
          </button>
          <span class="text-sm" :class="plugin.enabled ? 'text-white' : 'text-gray-400'">
            {{ plugin.enabled ? "Enabled" : "Disabled" }}
          </span>
        </label>
        <p class="text-xs text-gray-500 mt-2">
          A plugin runs only while it is enabled. Its own process, confined to the album directory.
        </p>
      </div>

      <!-- Information -->
      <div class="rounded-xl border border-white/10 bg-white/[0.02] p-5">
        <p class="text-sm font-semibold text-white mb-4">Plugin information</p>
        <dl class="grid grid-cols-1 sm:grid-cols-[9rem_1fr] gap-x-4 gap-y-3 text-sm">
          <dt class="text-gray-500">ID</dt>
          <dd class="text-gray-200 font-mono text-xs pt-0.5">{{ plugin.id }}</dd>

          <dt class="text-gray-500">Version</dt>
          <dd class="text-gray-200">{{ plugin.version }}</dd>

          <dt class="text-gray-500">Author</dt>
          <dd class="text-gray-200">{{ plugin.author || "—" }}</dd>

          <dt class="text-gray-500">Description</dt>
          <dd class="text-gray-200">{{ plugin.description }}</dd>

          <dt class="text-gray-500">Hooks</dt>
          <dd class="flex flex-wrap gap-1.5">
            <span
              v-for="hook in plugin.hooks"
              :key="hook"
              class="text-[10px] px-1.5 py-0.5 rounded bg-white/5 text-gray-400 font-mono"
              >{{ hook }}</span
            >
          </dd>

          <dt class="text-gray-500">Permissions</dt>
          <dd class="space-y-1">
            <div class="flex flex-wrap gap-1.5">
              <span
                v-for="bin in plugin.permissions.exec ?? []"
                :key="bin"
                class="text-[10px] px-1.5 py-0.5 rounded bg-white/5 text-gray-300 font-mono"
                >exec: {{ bin }}</span
              >
              <span
                v-for="host in plugin.permissions.net ?? []"
                :key="host"
                class="text-[10px] px-1.5 py-0.5 rounded border border-amber-400/25 bg-amber-400/10 text-amber-300 font-mono"
                >net: {{ host }}</span
              >
              <span v-if="!plugin.permissions.exec?.length && !plugin.permissions.net?.length" class="text-xs text-gray-500">
                album directory only
              </span>
            </div>
            <p class="text-[11px] text-gray-600">
              No network, no access to the hub's configuration or your home directory.
            </p>
          </dd>

          <dt class="text-gray-500">Updated</dt>
          <dd class="text-gray-200 text-xs">
            {{ plugin.updated_at ? new Date(plugin.updated_at).toLocaleString() : "never" }}
          </dd>

          <dt class="text-gray-500">Required</dt>
          <dd class="text-gray-200 text-xs">{{ plugin.required ? "a failure blocks the publish" : "no" }}</dd>
        </dl>
      </div>

      <!-- Manifest -->
      <details class="rounded-xl border border-white/10 bg-white/[0.02]">
        <summary class="px-5 py-3 text-sm font-semibold text-white cursor-pointer select-none">
          Manifest
        </summary>
        <pre class="px-5 pb-5 overflow-auto text-[11px] leading-relaxed text-gray-400 font-mono">{{
          plugin.manifest || "manifest unavailable"
        }}</pre>
      </details>

      <!-- Configuration -->
      <div class="rounded-xl border border-white/10 bg-white/[0.02] p-5">
        <p class="text-sm font-semibold text-white mb-1">Configuration</p>
        <p v-if="!plugin.config.length" class="text-xs text-gray-500 mt-2">
          This plugin takes no configuration.
        </p>

        <div v-else class="mt-4 space-y-4">
          <div v-for="field in plugin.config" :key="field.key">
            <label class="block text-xs text-gray-400 mb-1">
              {{ field.label || field.key }}
              <span v-if="field.required" class="text-orange-400">*</span>
            </label>

            <select
              v-if="field.type === 'select'"
              v-model="draft[field.key]"
              class="w-full text-xs bg-black/30 border border-white/10 rounded-lg px-3 py-2 text-white focus:outline-none focus:border-orange-500/50"
            >
              <option v-for="opt in field.options" :key="opt" :value="opt">{{ opt }}</option>
            </select>

            <label v-else-if="field.type === 'bool'" class="inline-flex items-center gap-2 text-xs text-gray-300">
              <input v-model="draft[field.key]" type="checkbox" class="accent-orange-500" />
              enabled
            </label>

            <input
              v-else-if="field.type === 'int'"
              v-model.number="draft[field.key]"
              type="number"
              :min="field.min ?? undefined"
              :max="field.max ?? undefined"
              class="w-full text-xs bg-black/30 border border-white/10 rounded-lg px-3 py-2 text-white focus:outline-none focus:border-orange-500/50"
            />

            <input
              v-else
              v-model="draft[field.key]"
              :type="field.type === 'secret' ? 'password' : 'text'"
              :placeholder="field.type === 'secret' ? 'unchanged' : ''"
              class="w-full text-xs bg-black/30 border border-white/10 rounded-lg px-3 py-2 text-white placeholder-gray-600 focus:outline-none focus:border-orange-500/50"
            />

            <p v-if="field.help" class="text-[11px] text-gray-600 mt-1">{{ field.help }}</p>
          </div>

          <div class="flex items-center gap-3">
            <button
              :disabled="saving"
              class="inline-flex items-center gap-1.5 px-3.5 py-2 bg-orange-500 hover:bg-orange-400 disabled:opacity-40 text-white text-xs font-medium rounded-lg transition-colors"
              @click="save"
            >
              <Icon
                :name="saving ? 'lucide:loader-2' : 'lucide:save'"
                class="w-3.5 h-3.5"
                :class="{ 'animate-spin': saving }"
              />
              Save
            </button>
            <span v-if="savedAt" class="text-xs text-green-400">Saved</span>
          </div>
        </div>
      </div>

      <p v-if="error" class="text-xs text-red-400">{{ error }}</p>
    </div>
  </div>
</template>
