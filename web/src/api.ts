import type { Catalog, Command, Config, EventPage, PlacementResult, Receipt, SavedGame, SavedGames, Session, Snapshot, Vec } from './api.generated';

export type LogQuery = { after?: number; before?: number; limit?: number; q?: string; category?: string };

export class APIError extends Error {
  constructor(public code: string, message: string, public status: number) { super(message); }
}

// The client sends intent and consumes read models. It never mutates an
// authoritative snapshot, applies costs, predicts combat, or simulates movement.
export class GameAPI {
  session: Session | null = null;
  private stream?: AbortController;

  async request<T>(path: string, options: RequestInit = {}): Promise<T> {
    const headers = new Headers(options.headers);
    if (options.body) headers.set('Content-Type', 'application/json');
    if (this.session) headers.set('Authorization', `Bearer ${this.session.token}`);
    const response = await fetch(`/api/v1${path}`, { ...options, headers });
    if (!response.ok) {
      const body = await response.json().catch(() => ({ error: { code: 'network_error', message: 'The server could not complete this request.' } }));
      throw new APIError(body.error.code, body.error.message, response.status);
    }
    if (response.status === 204) return undefined as T;
    return response.json() as Promise<T>;
  }

  catalog() { return this.request<Catalog>('/catalog'); }
  async create(config: Config) {
    const session = await this.request<Session>('/matches', { method: 'POST', body: JSON.stringify(config) });
    this.remember(session);
    return session;
  }
  restore(): boolean {
    try {
      const s = JSON.parse(localStorage.getItem('aoe.session.v1') || sessionStorage.getItem('crowns.session.v1') || 'null');
      if (!s || typeof s.match_id !== 'string' || typeof s.token !== 'string') return false;
      this.session = s; return true;
    } catch { return false; }
  }
  private remember(session: Session) { this.session = session; localStorage.setItem('aoe.session.v1', JSON.stringify(session)); sessionStorage.removeItem('crowns.session.v1'); }
  sessions(query = '') { return this.request<SavedGames>(`/sessions?q=${encodeURIComponent(query)}`); }
  async resume(identifier: string) {
    const session = await this.request<Session>('/sessions/resume', { method: 'POST', body: JSON.stringify({ identifier }) });
    this.remember(session); return session;
  }
  info() { return this.request<SavedGame>(`${this.path()}/session`); }
  save() { return this.request<SavedGame>(`${this.path()}/save`, { method: 'POST' }); }
  private path() { if (!this.session) throw new Error('Start a match first.'); return `/matches/${this.session.match_id}`; }
  snapshot() { return this.request<Snapshot>(this.path()); }
  log(query: LogQuery = {}, entityID?: number) {
    const params = new URLSearchParams(Object.entries(query).map(([key, value]) => [key, String(value)]));
    const route = entityID === undefined ? '/log' : `/entities/${entityID}/history`;
    return this.request<EventPage>(`${this.path()}${route}?${params}`);
  }
  command(command: Omit<Command, 'id'>, id = crypto.randomUUID()) {
    return this.request<Receipt>(`${this.path()}/commands`, { method: 'POST', body: JSON.stringify({ ...command, id }) });
  }
  placement(product: string, position: Vec) {
    return this.request<PlacementResult>(`${this.path()}/placement`, { method: 'POST', body: JSON.stringify({ product, position }) });
  }
  async close() {
    this.stream?.abort();
    if (this.session) await this.request<SavedGame>(`${this.path()}/leave`, { method: 'POST' });
    this.forget();
  }
  forget() { this.stream?.abort(); this.session = null; localStorage.removeItem('aoe.session.v1'); sessionStorage.removeItem('crowns.session.v1'); }
  suspendOnClose() {
    this.stream?.abort();
    if (this.session) void fetch(`/api/v1${this.path()}/leave`, { method: 'POST', headers: { Authorization: `Bearer ${this.session.token}` }, keepalive: true }).catch(() => {});
  }
  subscribe(onSnapshot: (snapshot: Snapshot) => void, onConnection: (state: string) => void) {
    this.stream?.abort();
    const controller = new AbortController(); this.stream = controller;
    const connect = async () => {
      let attempts = 0;
      while (!controller.signal.aborted) {
        try {
          const response = await fetch(`/api/v1${this.path()}/events`, {
            headers: { Authorization: `Bearer ${this.session!.token}` }, signal: controller.signal,
          });
          if (response.status === 401 || response.status === 404) { onConnection('expired'); return; }
          if (!response.ok || !response.body) throw new Error('Stream unavailable');
          onConnection('connected'); attempts = 0;
          const reader = response.body.getReader(), decoder = new TextDecoder();
          let pending = '';
          try {
            while (!controller.signal.aborted) {
              const { value, done } = await reader.read(); if (done) break;
              pending += decoder.decode(value, { stream: true });
              let boundary: number;
              while ((boundary = pending.indexOf('\n\n')) >= 0) {
                const event = pending.slice(0, boundary); pending = pending.slice(boundary + 2);
                const data = event.split('\n').filter(line => line.startsWith('data: ')).map(line => line.slice(6)).join('\n');
                if (data) onSnapshot(JSON.parse(data) as Snapshot);
              }
            }
          } finally { await reader.cancel().catch(() => {}); }
        } catch { if (controller.signal.aborted) return; }
        if (controller.signal.aborted) return;
        onConnection('reconnecting');
        await new Promise<void>(resolve => {
          const complete = () => { clearTimeout(timer); controller.signal.removeEventListener('abort', complete); resolve(); };
          const timer = window.setTimeout(complete, Math.min(5000, 500 * 2 ** attempts++));
          controller.signal.addEventListener('abort', complete, { once: true });
        });
      }
    };
    void connect();
  }
}
