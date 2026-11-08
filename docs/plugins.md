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

Everything about a plugin lives on the machine that runs it: the binary, its
manifest, whether it is enabled, and its settings.

That is a reversal of the first draft, which kept settings at the hub so the form
could validate them. The scale argument was the excuse, but the real ones are
better:

- **A shared hub has no business holding a user's plugin secrets.** An API key for
  a metadata service is the user's; with settings on the agent it never leaves
  their host, and the hub needs no encryption key for plugin data at all.
- **Config describes a machine, not a person.** `/usr/bin/rsgain`, a cache
  directory, a GPU toggle: these are per host. Keeping them next to the host is
  the only place they are not a lie.
- **One owner, no sync.** With settings at both ends there are two writers and a
  merge question ("the agent changed, the hub changed, who wins"). With settings
  on the agent there is nothing to merge.

So the hub stores **nothing** about plugins. It asks, displays, and forwards.

| data | where |
| --- | --- |
| binary, `plugin.yaml` | `PLUGINS_DIR` on the agent |
| enabled flag, config values (incl. secrets) | `PLUGINS_DIR/<id>/config.json`, mode 0600 |
| plugin's own data (caches, databases) | `PLUGINS_DIR/<id>/data/` |
| what happened on a run | job log at the hub, produced by events from the agent |

### How the hub shows it

The Plugins page is a proxy, not a store:

```
GET  /api/agents/{agentId}/plugins          → what that agent has
PUT  /api/agents/{agentId}/plugins/{id}     → enable/disable + config
```

The agent answers `plugin.list` with id, version, hooks, permissions, the config
schema from `--describe`, the enabled flag and the config with **secret values
masked** (`"apiKey": "••••"`). The form is generated from the schema as before;
saving sends only the fields the user touched, so a masked secret is never
overwritten with the mask.

An offline agent means an empty page with a reason, not a stale list pretending to
be current.

### What the protocol needs

Today the hub→agent channel is fire-and-forget: `SyncRequest`, `WriteAlbum`, and
nothing comes back tied to a request. Listing plugins and saving a config need an
answer, so the envelope gains a correlation id:

```proto
message Envelope {
  string request_id = 100;   // empty for one-way messages
  oneof payload { … }
}
```

The agent echoes `request_id` on the reply, the hub waits with a deadline, and a
disconnected agent fails the call instead of hanging.

### What moves in the code

`internal/plugins` (registry, dispatch, sandbox, exec) becomes
`packages/plugin-host`, owned by the agent. The hub keeps the two proxy
endpoints, the page, and the schema-driven form — no table, no encryption.

Plugin directory and identity become agent config:

```bash
PLUGINS_DIR=~/.config/kyoku/plugins
```
