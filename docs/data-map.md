# Where data lives

One table per place, so "where does X live" has one answer.

## brainz — one instance, shared by every hub

The catalogue. Nothing user-specific, nothing private.

| data | notes |
| --- | --- |
| `artists`, `release_groups`, `releases` | what MusicBrainz knows about the artists people follow |
| `hub_instances` | registered hubs and their public keys |
| `hub_subscriptions` | which hub wants which artist |
| `artist_requests` | the worker's queue |
| MusicBrainz response cache | one rate budget for everyone; the reason this service exists |

## hub — one per deployment, usually ours

Everything that is *decided* rather than *observed*, plus a cache of what the
agents observe.

| data | written by | notes |
| --- | --- | --- |
| `users`, `sessions` | hub | sign-in |
| `user_artists`, `artists` | hub | the artists a user follows ("wanted") |
| `fs_agents`, `agent_tokens` | hub | agent registration, public keys |
| `provider_credentials` | hub | Deezer ARL and friends, **encrypted** |
| `download_jobs` | hub | the job, its log and its result — the authoritative state of a download |
| `user_settings` | hub | tagger options |
| `agent_plugins` | **agents** | inventory cache: id, version, hooks, permissions, config schema |
| `agent_plugin_settings` | **hub UI** | enabled + config values, per `(user, agent, plugin)`, secrets **encrypted** |
| `fs_release_group_states` | hub (from agent scans) | derived: owned / incomplete per release group |
| SSE subscribers | hub, memory | nothing durable |
| MusicBrainz cache | hub, memory | avoids re-asking brainz; safe to lose |

## agent — the user's machine, one or more per user

Everything that is *observed*, everything that touches a file, and every
credential that only makes sense on that host.

| data | notes |
| --- | --- |
| `agent.json` | keypair + agent id; the identity the hub authenticates |
| `SCAN_DIRS`, `PLUGINS_DIR`, `HUB_WS_URL`, `HUB_API_URL` | local config, env |
| scan cache | path + mtime → parsed tags, so a rescan is cheap |
| plugin binaries, `plugin.yaml`, plugin data | installed by hand; state reported up |
| plugin config replica | pushed by the hub; **not** authoritative |
| download staging and `.part` files | never seen by the hub |
| **the music files** | the whole point |
| Navidrome URL, user, password | for scan triggers; never leaves the host |
| the running write job | memory |

## provider — Deezer

| data | notes |
| --- | --- |
| the user's own account | we hold only the ARL, at the hub, encrypted |

## What is never stored

- **Audio bytes at the hub.** The agent fetches from the CDN; a shared hub in the
  byte path is a bandwidth bill.
- **Plugin binaries at the hub.** They are compiled for one machine.
- **Secret values in the browser.** Config forms show `••••`, write-only.

## The three rules behind the split

1. The hub decides, the agent observes. Anything the UI lets a user change is at
   the hub; anything the filesystem says is at the agent.
2. The agent never writes user settings; the hub never writes inventory. A
   rescan cannot reset someone's toggles, a reinstall cannot lose their key.
3. Credentials live where they can be used: Deezer at the hub (it resolves
   tickets), Navidrome at the agent (it only has to reach the LAN), plugin
   secrets at the hub (the form is there) and travel to the agent only for the
   job that needs them.
