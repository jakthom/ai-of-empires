import type { Catalog, Command, Config, MemberSession, GameInfo, GameLibrary, ConnectionInfo, Invitation, ImportResult, TransferInfo, GameControl, EventPage, PlacementResult, Receipt, SavedGame, Session, Snapshot, SnapshotFrame, Vec } from './api.generated';

import { SnapshotAssembler } from './snapshot-stream';

export type LogQuery = { after?: number; before?: number; limit?: number; q?: string; category?: string };

// getRandomValues is available on HTTP LAN origins as well as HTTPS. Unlike
// randomUUID, it does not require a secure browser context.
const requestID = () => Array.from(crypto.getRandomValues(new Uint8Array(16)), byte => byte.toString(16).padStart(2, '0')).join('');

export class APIError extends Error {
  constructor(public code: string, message: string, public status: number) { super(message); }
}

// The client sends intent and consumes read models. It never mutates an
// authoritative snapshot, applies costs, predicts combat, or simulates movement.
export class GameAPI {
  session: (Session & Partial<MemberSession>) | null = null;
  room: GameInfo | null = null;
  connectionID = '';
  private heartbeatTimer = 0;
  private connecting?: Promise<void>;
  private stream?: AbortController;

  async request<T>(path: string, options: RequestInit = {}): Promise<T> {
    const headers = new Headers(options.headers);
    if (options.body && !(options.body instanceof FormData)) headers.set('Content-Type', 'application/json');
    if (this.session?.token) headers.set('Authorization', `Bearer ${this.session.token}`);
    const response = await fetch(`/api/v1${path}`, { ...options, headers });
    if (!response.ok) {
      const body = await response.json().catch(() => ({ error: { code: 'network_error', message: 'The server could not complete this request.' } }));
      throw new APIError(body.error.code, body.error.message, response.status);
    }
    if (response.status === 204) return undefined as T;
    return response.json() as Promise<T>;
  }

  catalog() { return this.request<Catalog>('/catalog'); }
  async create(config: Config, friends = 0, playerName = '') {
    const session = await this.request<MemberSession>('/games', { method: 'POST', body: JSON.stringify({ config, friends, player_name: playerName }) });
    this.remember(session);
    return session;
  }
  restore(): boolean {
    try {
      const s = JSON.parse(sessionStorage.getItem('aoe.tab-session.v1') || localStorage.getItem('aoe.session.v1') || sessionStorage.getItem('crowns.session.v1') || 'null');
      if (!s || typeof s.match_id !== 'string' || typeof s.token !== 'string') return false;
      this.session = s; return true;
    } catch { return false; }
  }
  remember(session: Session & Partial<MemberSession>) {
    this.stopPresence(); this.stream?.abort();
    const codes = JSON.parse(localStorage.getItem('aoe.rejoin.v1') || '{}') as Record<string,string>;
    if (session.membership_id) {
      if (session.rejoin_code) codes[session.membership_id] = session.rejoin_code;
      else session.rejoin_code = codes[session.membership_id];
    }
    this.session = session; this.room = null;
    localStorage.setItem('aoe.rejoin.v1', JSON.stringify(codes));
    localStorage.setItem('aoe.session.v1', JSON.stringify(session)); sessionStorage.setItem('aoe.tab-session.v1',JSON.stringify(session)); sessionStorage.removeItem('crowns.session.v1');
  }
  async games(query = '') { return this.request<GameLibrary>(`/games?q=${encodeURIComponent(query)}`); }
  async sessions(query = '') {
    const result = await this.games(query);
    return { games: result.games.map(g => ({ match_id:g.game_id, name:g.name, world:g.config.world ?? {}, saved_at:g.saved_at, time:g.time, difficulty:g.config.difficulty, settlements:g.config.settlements ?? 2, status:g.status, active:g.status === 'open', autosave_seconds:10, owner:g.owner, runtime:g.runtime })) };
  }
  async resume(identifier: string) {
    const result = await this.games(identifier);
    const game = result.games.find(g => g.game_id === identifier || g.name.toLocaleLowerCase() === identifier.toLocaleLowerCase());
    if (!game) throw new APIError('game_not_found', 'No saved game in your private library has that name or ID. Use a rejoin code on a new browser.', 404);
    return this.useGame(game.game_id);
  }
  async useGame(id: string) {
    await this.depart(); this.session = null;
    const session = await this.request<MemberSession>(`/games/${id}/session`);
    this.remember(session); await this.gameInfo(); return session;
  }
  async adoptLegacy() { if(this.session && !this.session.membership_id){const v=await this.request<MemberSession>(`${this.path()}/adopt`,{method:'POST'});this.remember(v);} }
  async gameInfo() { const info = await this.request<GameInfo>(this.path()); if(this.room && this.room.game_id===info.game_id && this.room.epoch===info.epoch && this.room.revision>info.revision)return this.room; this.room = info; return info; }
  async info(): Promise<SavedGame> {
    if (!this.session?.membership_id) return this.request<SavedGame>(`${this.path()}/session`);
    const g = await this.gameInfo();
    return { match_id:g.game_id, name:g.name, world:g.config.world ?? {}, saved_at:g.saved_at, time:g.time, difficulty:g.config.difficulty, settlements:g.config.settlements ?? 2, status:g.status, active:g.status === 'open', autosave_seconds:10, save_error:g.save_error };
  }
  async save() { if (!this.session?.membership_id) return this.request<SavedGame>(`${this.path()}/save`, {method:'POST'}); this.room = await this.request<GameInfo>(`${this.path()}/save`, {method:'POST'}); return this.info(); }
  path() { if (!this.session) throw new Error('Start a game first.'); return `/${this.session.membership_id ? 'games' : 'matches'}/${this.session.match_id}`; }
  async control(action: string, values: Partial<GameControl> = {}) {
    if (!this.room) await this.gameInfo();
    const info = await this.request<GameInfo>(`${this.path()}/${action}`, {method:'POST', body:JSON.stringify({ id:requestID(), revision:this.room!.revision, ...values })});
    this.room = info; if(action==='close'||action==='reopen'||action==='cancel-close')this.stopPresence();return info;
  }
  async mutate<T>(suffix: string, method: string, values: Record<string,unknown>) {
    if (!this.room) await this.gameInfo();
    return this.request<T>(`${this.path()}${suffix}`, {method,body:JSON.stringify({id:requestID(),revision:this.room!.revision,...values})});
  }
  inspectInvite(secret: string) { return this.request<Invitation>('/invites/inspect',{method:'POST',body:JSON.stringify({secret})}); }
  async claim(secret: string,name: string) { const v=await this.request<MemberSession>('/invites/claim',{method:'POST',body:JSON.stringify({id:requestID(),secret,name})});this.remember(v);return v; }
  async rejoin(code: string) { const v=await this.request<MemberSession>('/memberships/rejoin',{method:'POST',body:JSON.stringify({code})});this.remember(v);return v; }
  async importGame(form: FormData) { const v=await this.request<ImportResult>('/game-imports',{method:'POST',body:form});this.remember(v.session);return v; }
  async transfer(kind: string) {return this.mutate<TransferInfo>('/transfers','POST',{kind});}
  async downloadArchive(id: string,passphrase: string) {
    const headers=new Headers({'Content-Type':'application/json'});if(this.session?.token)headers.set('Authorization',`Bearer ${this.session.token}`);
    const response=await fetch(`/api/v1${this.path()}/transfers/${id}/archive`,{method:'POST',headers,body:JSON.stringify({passphrase})});
    if(!response.ok){const body=await response.json();throw new APIError(body.error.code,body.error.message,response.status)}
    return response.blob();
  }
  async downloadDatabase() {
    const headers = new Headers();
    if (this.session?.token) headers.set('Authorization', `Bearer ${this.session.token}`);
    const response = await fetch(`/api/v1${this.path()}/database`, { method: 'POST', headers });
    if (!response.ok) { const body = await response.json(); throw new APIError(body.error.code, body.error.message, response.status); }
    return response.blob();
  }
  async deleteGame() { await this.mutate<void>('','DELETE',{confirm:true});this.forget(); }
  async ensureConnection() {
    if (!this.session?.membership_id || this.connectionID) return;
    if (this.connecting) return this.connecting;
    const id = this.session.match_id;
    this.connecting = (async () => {
      const c = await this.request<ConnectionInfo>(`${this.path()}/connections`,{method:'POST'});
      if(this.session?.match_id !== id) return;
      this.connectionID = c.id;
      this.heartbeatTimer=window.setInterval(()=>{void this.request(`${this.path()}/connections/${c.id}/heartbeat`,{method:'POST'}).catch(()=>{});},5000);
    })();
    try { await this.connecting; } finally { this.connecting=undefined; }
  }
  private stopPresence() { clearInterval(this.heartbeatTimer); this.heartbeatTimer=0;this.connectionID=''; }
  async depart() {
    if(this.session?.membership_id && this.connectionID) await this.request(`${this.path()}/connections/${this.connectionID}/leave`,{method:'POST'});
    this.stream?.abort();this.stopPresence();
  }
  snapshot() { return this.request<Snapshot>(`${this.path()}${this.session?.membership_id ? '/snapshot' : ''}`); }
  log(query: LogQuery = {}, entityID?: number) {
    const params = new URLSearchParams(Object.entries(query).map(([key, value]) => [key, String(value)]));
    const route = entityID === undefined ? '/log' : `/entities/${entityID}/history`;
    return this.request<EventPage>(`${this.path()}${route}?${params}`);
  }
  command(command: Omit<Command, 'id'>, id = requestID()) {
    return this.request<Receipt>(`${this.path()}/commands`, { method: 'POST', body: JSON.stringify({ ...command, id }) });
  }
  placement(product: string, position: Vec, end_position?: Vec) {
    return this.request<PlacementResult>(`${this.path()}/placement`, { method: 'POST', body: JSON.stringify({ product, position, end_position }) });
  }
  async close() {
    if (this.session?.membership_id) { await this.save(); await this.depart(); }
    else if (this.session) await this.request<SavedGame>(`${this.path()}/leave`, {method:'POST'});
    this.forget();
  }
  forget() { this.stream?.abort(); this.stopPresence(); this.room=null;this.session=null; localStorage.removeItem('aoe.session.v1'); sessionStorage.removeItem('aoe.tab-session.v1'); sessionStorage.removeItem('crowns.session.v1'); }
  suspendOnClose() {
    if(this.session){
      const suffix=this.session.membership_id ? (this.connectionID ? `/connections/${this.connectionID}/leave` : '') : '/leave';
      if(suffix){const headers=new Headers();if(this.session.token)headers.set('Authorization',`Bearer ${this.session.token}`);void fetch(`/api/v1${this.path()}${suffix}`,{method:'POST',headers,keepalive:true}).catch(()=>{});}
    }
    this.stream?.abort();this.stopPresence();
  }
  subscribe(onSnapshot: (snapshot: Snapshot) => void, onConnection: (state: string) => void) {
    this.stream?.abort();
    const controller = new AbortController(); this.stream = controller;
    const connect = async () => {
      let attempts = 0;
      while (!controller.signal.aborted) {
        try {
          await this.ensureConnection();
          if(controller.signal.aborted)return;
          const query=this.session?.membership_id ? `?format=delta-v1&connection=${encodeURIComponent(this.connectionID)}` : '?format=delta-v1';
          const response = await fetch(`/api/v1${this.path()}/events${query}`, {
            headers: this.session?.token ? { Authorization: `Bearer ${this.session.token}` } : {}, signal: controller.signal,
          });
          if ([401,404,422].includes(response.status)) { this.stopPresence(); onConnection('expired'); return; }
          if (!response.ok || !response.body) throw new Error('Stream unavailable');
          onConnection('connected'); attempts = 0;
          const reader = response.body.getReader(), decoder = new TextDecoder();
          let pending = '';
          const assembler = new SnapshotAssembler();
          try {
            while (!controller.signal.aborted) {
              const { value, done } = await reader.read(); if (done || controller.signal.aborted) break;
              pending += decoder.decode(value, { stream: true });
              let boundary: number, latest: Snapshot | undefined;
              while ((boundary = pending.indexOf('\n\n')) >= 0) {
                const event = pending.slice(0, boundary); pending = pending.slice(boundary + 2);
                const data = event.split('\n').filter(line => line.startsWith('data: ')).map(line => line.slice(6)).join('\n');
                if (data) latest = assembler.apply(JSON.parse(data) as SnapshotFrame);
              }
              // Coalesce frames already delivered together after a busy tab.
              if (latest) onSnapshot(latest);
            }
          } finally { await reader.cancel().catch(() => {}); }
        } catch (error) {
          if (controller.signal.aborted) return;
          if (error instanceof APIError && [401,404,422].includes(error.status)) { this.stopPresence(); onConnection('expired'); return; }
        }
        if (controller.signal.aborted) return;
        // Connection IDs belong to one runtime. A restarted host restores the
        // membership and checkpoint, then gives this tab a fresh connection.
        this.stopPresence();
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
