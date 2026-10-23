export interface RavenConfig {
  baseURL: string;
  gameID: string;
  apiKey?: string;
  accessToken?: string;
  refreshToken?: string;
  onTokenRefresh?: (tokens: { accessToken: string; refreshToken: string }) => void;
}

export class RavenClient {
  private cfg: RavenConfig;
  private queue: Array<() => Promise<any>> = [];

  constructor(cfg: RavenConfig) {
    this.cfg = cfg;
  }

  private async request(method: string, path: string, body?: any, retries = 3): Promise<any> {
    const url = this.cfg.baseURL.replace(/\/$/, "") + path;
    const headers: Record<string, string> = { "Content-Type": "application/json" };
    if (this.cfg.accessToken) headers["Authorization"] = `Bearer ${this.cfg.accessToken}`;
    if (this.cfg.apiKey) headers["X-Api-Key"] = this.cfg.apiKey;

    for (let attempt = 0; attempt <= retries; attempt++) {
      try {
        const res = await fetch(url, {
          method,
          headers,
          body: body ? JSON.stringify(body) : undefined,
        });
        if (res.status === 401 && this.cfg.refreshToken && attempt === 0) {
          await this.refresh();
          continue;
        }
        if (!res.ok) {
          const text = await res.text();
          throw new Error(`HTTP ${res.status}: ${text}`);
        }
        if (res.status === 204) return null;
        return await res.json();
      } catch (e) {
        if (attempt === retries) throw e;
        await new Promise((r) => setTimeout(r, 200 * Math.pow(2, attempt)));
      }
    }
  }

  private async refresh() {
    const res = await fetch(this.cfg.baseURL.replace(/\/$/, "") + "/v1/auth/refresh", {
      method: "POST",
      headers: { "Content-Type": "application/json" },
      body: JSON.stringify({ refresh_token: this.cfg.refreshToken }),
    });
    if (!res.ok) throw new Error("refresh failed");
    const data = await res.json();
    this.cfg.accessToken = data.access_token;
    this.cfg.refreshToken = data.refresh_token;
    this.cfg.onTokenRefresh?.({ accessToken: data.access_token, refreshToken: data.refresh_token });
  }

  async guest(deviceId: string) {
    return this.request("POST", "/v1/auth/guest", { game_id: this.cfg.gameID, device_id: deviceId });
  }

  async getSave(slot: string) {
    return this.request("GET", `/v1/saves/${encodeURIComponent(slot)}`);
  }

  async putSave(slot: string, data: any, ifMatch?: number) {
    const headers: any = {};
    // ifMatch handled by caller via body? Server expects If-Match header; pass via extra
    return this.request("PUT", `/v1/saves/${encodeURIComponent(slot)}`, { data, if_match: ifMatch });
  }

  async listSaves() {
    return this.request("GET", "/v1/saves");
  }

  async submitScore(boardId: string, score: number, meta?: any) {
    return this.request("POST", `/v1/leaderboards/${boardId}/scores`, { score, meta });
  }

  async topScores(boardId: string, limit = 10) {
    return this.request("GET", `/v1/leaderboards/${boardId}?limit=${limit}`);
  }
}
