# Plugins

A plugin is a Go binary that runs after a download. It gets one JSON payload per
invocation, may change files in the album directory, and answers with a message
and an exit code.

Plugins are installed **by hand**: copy a directory into `$PLUGINS_DIR`
(`~/.config/kyoku/plugins` by default) and restart the hub. There is no upload
endpoint, deliberately — installing a plugin is granting code access to the
server, so it should require access to the server.

A freshly copied plugin is **disabled**. It appears in Settings → Plugins with
its declared permissions, and runs only once it is switched on there. That toggle
is the consent step: what it may touch is visible on the same screen, before it
ever runs.

    ~/.config/kyoku/plugins/replaygain/
      plugin.yaml     # what it is, which hooks it serves, what it may touch
      replaygain      # a compiled Go binary

## Manifest

```yaml
apiVersion: 1                  # the wire contract; a mismatch is refused
name: replaygain               # must equal the directory name
version: 2.0.0
author: kyoku
description: ReplayGain 2.0 album and track gain via rsgain

executable: ./replaygain       # relative to this directory
hooks: [album.beforePublish]
timeout: 600s                  # default 60s; the process group is killed
required: false                # true = a failure blocks the publish
extensions: [".flac", ".mp3"]  # optional filter, omit for every format

permissions:
  exec: [/usr/bin/rsgain]      # binaries this plugin may run (absolute paths)

config:
  - key: multithread
    type: string
    label: Threads
    help: MAX scans with every core, or give a number.
    default: MAX
  - key: skipExisting
    type: bool
    label: Skip files that already carry ReplayGain
    default: true
```

Unknown keys are a load error, not a warning: a manifest cannot carry a knob that
looks meaningful and is not. A default that violates its own bounds is also a
load error.

Field types: `string`, `secret`, `bool`, `int`, `select` (`secret` is write-only
in the API; values are stored encrypted with a key derived from the session
secret).

## Hooks

| Hook | When | Payload |
|---|---|---|
| `track.downloaded` | file on disk, before kyoku's own tagging | `track` |
| `track.tagged` | after tags and artwork | `track` |
| `album.beforePublish` | every file staged, before the move into the library | `files[]` |
| `album.published` | the album is in the library | `files[]` with final paths |

A plugin runs only for the hooks it declares, and only for the extensions it
lists. Files it creates in `album.dir` during `album.beforePublish` are published
with the album; it must not move, rename or delete the audio itself.

## Writing one

```go
package main

import (
	"context"

	kyoku "github.com/zertex-labs/kyoku-hub/packages/plugin-sdk"
)

func main() {
	kyoku.Serve(kyoku.Plugin{
		Name:    "replaygain",
		Version: "2.0.0",
		Registry: kyoku.Registry{
			AlbumBeforePublish: replaygain,
		},
	})
}

type config struct {
	Multithread  string `json:"multithread"`
	SkipExisting bool   `json:"skipExisting"`
}

func replaygain(ctx context.Context, ev *kyoku.Event) error {
	var cfg config
	if err := ev.Config(&cfg); err != nil {
		return err // the manifest and the struct disagree
	}

	args := []string{"easy", "-m", cfg.Multithread}
	if cfg.SkipExisting {
		args = append(args, "-S")
	}
	args = append(args, ev.Album.Dir)

	out, err := ev.Run(ctx, "rsgain", args...)
	if err != nil {
		return err
	}
	ev.Message("scanned %d track(s)", ev.Album.TrackCount)
	return nil
}
```

`Serve` owns the wire contract: `--describe`, reading stdin, writing the
response, and the exit code. Return `nil` for success, `kyoku.Skip("reason")`
when there is nothing to do, and any error for a failure (its text lands in the
job log). `ev.Added(paths...)` names files the plugin created.

Build it static for the machine that runs the hub:

    CGO_ENABLED=0 go build -trimpath -ldflags="-s -w" -o replaygain .

`kyoku.Dispatch(ctx, plugin, payload)` runs a handler without a process, which
is what a plugin's own `go test` should call.

## How a plugin is confined

Before a plugin is enabled, the hub runs its `--describe` and refuses it unless
the binary agrees with the manifest on `apiVersion`, `name` and hooks. Before
every invocation the hub re-execs itself as a launcher, applies a Landlock
ruleset, and becomes the plugin, so the rules are inherited by everything the
plugin starts and cannot be widened:

- **read**: `/usr`, `/lib`, `/lib64`, `/bin`, `/sbin`, a few files every process
  needs, and the plugin's own directory
- **write**: the album directory and one private temp directory
- **network**: denied — `RestrictNet` with no rules blocks all TCP
- **limits**: `RLIMIT_CPU`, `RLIMIT_AS`, `RLIMIT_NOFILE`

The hub's own config, the user's home and the database are not visible. If the
kernel cannot enforce the ruleset the plugin does not run; it never runs
unconfined. A plugin that declares `permissions.net` is refused until the hub can
proxy those requests through an allowlist.

What this does *not* do: it does not restrict *which* programs run — Landlock
scopes file and network access, so a plugin can execute anything it can see, but
only inside those rules. Treat a plugin as code you chose to install.

## Pages

- **Settings → Plugins** — every installed plugin: ID, description, version,
  enabled toggle, last change. Disabled plugins are listed but never dispatched.
- **/plugins/:id** — status toggle, plugin information (author, hooks, declared
  permissions), the manifest verbatim, and the configuration form generated from
  it.

## Configuration API

    GET  /api/plugins                installed plugins, fields, masked values
    GET  /api/plugins/:id            one plugin, including its manifest
    PUT  /api/plugins/:id            {"enabled": true|false}
    GET  /api/plugins/:id/config     fields + values
    PUT  /api/plugins/:id/config     validate, encrypt, store

Secret values are never returned; `PUT` omitting a secret leaves the stored value
alone. Values reach the plugin as a `config` object in the payload, already
merged with the manifest defaults and type-checked.

## Examples

- `examples/plugins/hello` — both hook shapes, typed config, writes one file.
- `examples/plugins/replaygain` — drives rsgain with typed config; runs at
  `album.beforePublish` so the album is scanned as a unit and gets album gain.

## Where a plugin lives

Three kinds of state, three homes — because the plugin runs on the user's machine
while the settings form lives in a hub that may be ours and may serve hundreds of
people.

| state | home | why |
| --- | --- | --- |
| binary and `plugin.yaml` | the agent's disk | code compiled for one machine and one OS; shipping binaries to a shared hub is absurd |
| what is installed (id, version, hooks, permissions, config schema) | reported by the agent, cached by the hub | the Plugins page must list them without poking every agent on every load |
| enabled / disabled | hub, per `(user, agent, plugin)` | it is consent, given in the UI, and it must survive the agent being rebuilt |
| config values | hub, per `(user, agent, plugin)`, secret fields encrypted | the form and the schema validation are in the hub |
| the plugin's own data (caches, databases) | the agent, under its plugin directory | nobody else can use it |
| what happened on a run | the job log | "why did replaygain fail" is a support question, and the answer belongs next to the job |

The invariant is what keeps this sane: **the agent never writes user settings, and
the hub never writes inventory.** Otherwise a rescan would reset somebody's
toggles, and a reinstall would lose their API key.

### Two tables

```sql
-- inventory: written only by the agent, one row per plugin per agent
CREATE TABLE agent_plugins (
  user_id     TEXT NOT NULL,
  agent_id    TEXT NOT NULL,
  plugin_id   TEXT NOT NULL,
  api_version INT  NOT NULL,
  version     TEXT NOT NULL,
  hooks       TEXT[] NOT NULL,
  permissions JSONB NOT NULL,
  config_schema JSONB NOT NULL,   -- straight from `--describe`
  description TEXT,
  author      TEXT,
  reported_at TIMESTAMPTZ NOT NULL,
  PRIMARY KEY (user_id, agent_id, plugin_id)
);

-- intent: written only by the hub UI
CREATE TABLE agent_plugin_settings (
  user_id   TEXT NOT NULL,
  agent_id  TEXT NOT NULL,
  plugin_id TEXT NOT NULL,
  enabled   BOOLEAN NOT NULL DEFAULT FALSE,
  config    JSONB NOT NULL DEFAULT '{}'::jsonb,  -- secret fields encrypted
  updated_at TIMESTAMPTZ NOT NULL,
  PRIMARY KEY (user_id, agent_id, plugin_id)
);
```

A plugin that disappears from the agent's inventory is marked *missing*, not
deleted: reinstalling it restores the same settings, and a half-broken setup is
visible instead of silently vanishing.

### The flow

1. The agent connects (or rescans) and sends `PluginInventory`: everything from
   its `--describe`, which the hub already uses to generate the config form.
2. The hub upserts inventory rows and marks what is gone.
3. The user toggles or configures on Settings → Plugins → `PUT
   /api/plugins/{agent}/{plugin}`.
4. The hub pushes the active config to the agent, which caches it, and includes
   it in every `WriteAlbum` so a job never runs with stale settings.
5. The agent runs the hooks inside the write job; each result comes back as a
   line in the job log. A `required: true` plugin that fails fails the job.

### Secrets

Encrypted at rest with the plugin secretbox that already exists (keyed per user,
agent and plugin), and never returned to the browser: the form shows `••••` and
writes only when the value changes. This matches how provider credentials are
already stored, and it is the one place where a self-hosted hub and our hub
behave differently — ours holds the key, theirs holds theirs.

### Per-machine settings

A field that names a binary (`/usr/bin/rsgain`) is machine-specific; an API key is
not. Storing config per agent handles both, at the cost of typing a key twice for
two agents. That is the right trade for now — a "copy settings from another
agent" button can come later, but config inheritance cannot be retrofitted
without confusion.

### What moves in the code

`internal/plugins` (registry, dispatch, sandbox, secretbox, exec) becomes
`packages/plugin-host`, used by the agent. The hub keeps the API, the UI, the two
tables and the encryption key. The plugin directory becomes agent config:

```bash
PLUGINS_DIR=~/.config/kyoku/plugins
```

