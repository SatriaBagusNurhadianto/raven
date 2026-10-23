# Quickstart — Raven JS/TS SDK

```bash
npm install @raven/sdk
```

```ts
import { RavenClient } from "@raven/sdk";

const client = new RavenClient({
  baseURL: "http://localhost:8080",
  gameID: "your-game-uuid",
});

// Guest login
const auth = await client.guest("device-123");
client.accessToken = auth.access_token;
client.refreshToken = auth.refresh_token;

// Cloud save
await client.putSave("main", { level: 5 });
const save = await client.getSave("main");

// Leaderboard
await client.submitScore("kills-board", 42);
const top = await client.topScores("kills-board", 10);
```

Lihat `sdk/js/src/index.ts` untuk API lengkap.
