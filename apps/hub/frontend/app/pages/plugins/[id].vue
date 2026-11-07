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
// Which field has focus, so its label can float into the border.
const focused = reactive<Record<string, boolean>>({});

function hasValue(key: string): boolean {
  const v = draft.value[key];
  return v !== undefined && v !== null && v !== "";
}

// A label sits inside the field until it is focused or filled, then it shrinks
// onto the border - the Material outline pattern, without a placeholder hack.
function labelFloats(field: { key: string; type: string }): boolean {
  return field.type === "select" || focused[field.key] || hasValue(field.key);
}

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

        <div v-else class="mt-5 space-y-6">
          <div v-for="field in plugin.config" :key="field.key" class="space-y-1">
            <!-- boolean: a checkbox with its label beside it -->
            <label
              v-if="field.type === 'bool'"
              class="flex items-center gap-2.5 cursor-pointer select-none py-1"
            >
              <input v-model="draft[field.key]" type="checkbox" class="sr-only" />
              <span
                class="w-4 h-4 rounded-[3px] border flex items-center justify-center transition-colors shrink-0"
                :class="
                  draft[field.key]
                    ? 'bg-orange-500 border-orange-500'
                    : 'bg-transparent border-white/25'
                "
              >
                <Icon
                  v-if="draft[field.key]"
                  name="lucide:check"
                  class="w-3 h-3 text-white"
                  :stroke-width="3"
                />
              </span>
              <span class="text-sm text-gray-200">{{ field.label || field.key }}</span>
            </label>

            <!-- everything else: the label starts centred in the box and pops up to
                 its top corner, so the border and the spacing never move -->
            <div
              v-else
              class="relative h-11 rounded-lg border transition-colors"
              :class="focused[field.key] ? 'border-orange-500/60' : 'border-white/10'"
            >
              <div v-if="field.type === 'select'" class="relative h-full">
                <select
                  v-model="draft[field.key]"
                  class="h-full w-full bg-transparent pl-3 pr-8 pt-3 text-sm text-white outline-none appearance-none"
                  @focus="focused[field.key] = true"
                  @blur="focused[field.key] = false"
                >
                  <option v-for="opt in field.options" :key="opt" :value="opt">{{ opt }}</option>
                </select>
                <Icon
                  name="lucide:chevron-down"
                  class="w-3.5 h-3.5 text-gray-500 absolute right-2.5 top-1/2 -translate-y-1/2 pointer-events-none"
                />
              </div>

              <input
                v-else
                v-model="draft[field.key]"
                :type="field.type === 'secret' ? 'password' : 'text'"
                class="h-full w-full bg-transparent px-3 pt-3 text-sm text-white outline-none"
                @focus="focused[field.key] = true"
                @blur="focused[field.key] = false"
              />

              <label
                class="absolute left-3 -translate-y-1/2 leading-none pointer-events-none select-none whitespace-nowrap transition-all duration-150 ease-out"
                :class="labelFloats(field) ? 'top-[9px] text-[11px]' : 'top-1/2 text-sm'"
                :style="{ color: focused[field.key] ? '#fb923c' : labelFloats(field) ? '#9ca3af' : '#6b7280' }"
              >
                {{ field.label || field.key }}
                <span v-if="field.required" class="text-orange-400">*</span>
              </label>
            </div>

            <p v-if="field.help" class="text-[11px] text-gray-600 pl-0.5">{{ field.help }}</p>
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
