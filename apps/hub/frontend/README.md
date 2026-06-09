# hub/frontend

Vue SPA (Nuxt in SPA mode, no SSR). All data via hub/backend HTTP + WebSocket - no Nuxt server routes.

## Dev

```sh
bun install
bun run dev      # http://localhost:3000
```

## Build

```sh
bun run build    # outputs to .output/public — serve as static files
```

## Config

Point at the hub backend by setting the API base URL. Configure in `nuxt.config.ts` or via env:

```
NUXT_PUBLIC_API_BASE=http://localhost:8080
```
