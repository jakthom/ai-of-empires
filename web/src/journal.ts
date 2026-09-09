import { GameAPI, type LogQuery } from './api';
import type { Event, HistoryEntity, LogFilter, Snapshot } from './api.generated';

function stamp(seconds: number) {
  const parts = [Math.floor(seconds / 60), Math.floor(seconds % 60)];
  if (seconds >= 3600) parts.splice(0, 1, Math.floor(seconds / 3600), Math.floor(seconds / 60) % 60);
  return parts.map(n => String(n).padStart(2, '0')).join(':');
}

// Presentation state only: Go authors the events, activity, visibility and
// cursors. Paging and resizing never alter a match or an event record.
export class EventLog {
  private snapshot?: Snapshot;
  private entityID?: number;
  private entity?: HistoryEntity;
  private entries: Event[] = [];
  private hasOlder = false;
  private hasNewer = false;
  private following = true;
  private busy = false;
  private failed = false;
  private generation = 0;
  private observedCursor = -1;
  private latestCursor = 0;
  private metadataCursor = -1;
  private metadataBusy = false;
  private height = 35;
  private expandedHeight = 260;
  private adjustingScroll = false;
  private active = true;
  private search = '';
  private category = '';
  private searchTimer = 0;

  constructor(private api: GameAPI, private root: HTMLElement, private layout: 'tray' | 'panel' = 'tray', private locate?: (event: Event) => void) {
    root.innerHTML = `
      <div id="log-resize" role="separator" tabindex="0" aria-label="Resize event log" aria-orientation="horizontal" aria-controls="event-log-body" title="Drag to resize. Arrow keys resize; Home collapses; End expands."><span></span></div>
      <div class="log-summary"><button id="log-toggle" aria-expanded="false" aria-controls="event-log-body" title="Expand event log">Event log <span aria-hidden="true">▴</span></button><span id="activity">A new age awaits.</span><button id="idle" title="Select idle villagers (.)">Idle villagers <strong id="idle-count">0</strong></button></div>
      <div id="event-log-body" hidden>
        <div class="log-filters" role="search" aria-label="Search game events"><input id="log-search" type="search" aria-label="Search event log" placeholder="Search events or Villager 3…" maxlength="200" autocomplete="off"><select id="log-category" aria-label="Event category"><option value="">All categories</option></select><button id="log-clear" aria-label="Clear log filters" disabled>Clear</button></div>
        <div class="log-toolbar"><button id="log-global" aria-pressed="true">All events</button><div class="log-context"><strong id="log-title">Your kingdom</strong><span id="log-status">Your private events</span></div><button id="log-follow" aria-pressed="true">Live</button><button id="log-older">Older</button><button id="log-newer" disabled>Newer</button></div>
        <div id="log-feedback" role="status" hidden><span></span><button id="log-retry" hidden>Retry</button></div>
        <ol id="event-entries" role="log" aria-label="Event history" aria-live="off" tabindex="0"></ol>
      </div>`;
    if (layout === 'panel') {
      this.active = false;
      root.querySelector('#log-resize')!.remove();
      root.querySelector('.log-summary')!.remove();
      root.querySelector('#log-global')!.remove();
      root.querySelector('.log-filters')!.remove();
      root.querySelector<HTMLElement>('#event-log-body')!.hidden = false;
      root.querySelectorAll<HTMLElement>('[id]').forEach(element => { element.id = `entity-${element.id}`; });
      this.node('event-entries').setAttribute('aria-label', 'Selected entity history');
    } else {
      this.button('log-toggle').onclick = () => this.setHeight(this.height === 35 ? this.expandedHeight : 35);
      this.button('log-global').onclick = () => this.inspect();
      const search = this.node('log-search') as HTMLInputElement;
      search.oninput = () => {
        clearTimeout(this.searchTimer);
        this.searchTimer = window.setTimeout(() => this.applyFilters(), 220);
      };
      search.onkeydown = event => { if (event.key === 'Enter') { event.preventDefault(); this.applyFilters(); } };
      this.node('log-category').onchange = () => this.applyFilters();
      this.button('log-clear').onclick = () => { this.inspect(); search.focus(); };
    }
    this.button('log-follow').onclick = () => { this.following = true; void this.fetchPage('latest'); };
    this.button('log-older').onclick = () => { this.following = false; void this.fetchPage('older'); };
    this.button('log-newer').onclick = () => { this.following = false; void this.fetchPage('newer'); };
    this.button('log-retry').onclick = () => { this.failed = false; void this.fetchPage('latest'); };
    this.node('event-entries').addEventListener('scroll', () => {
      if (this.adjustingScroll || !this.following) return;
      const list = this.node('event-entries');
      if (list.scrollHeight - list.clientHeight - list.scrollTop > 32) { this.following = false; this.controls(); }
    });
    this.node('event-entries').addEventListener('focusin', () => { this.following = false; this.controls(); });
    if (layout === 'tray') {
      const handle = this.node('log-resize');
      let drag: { id: number; y: number; height: number } | undefined;
      handle.onpointerdown = event => {
        if (event.button !== 0) return;
        event.preventDefault(); handle.focus();
        drag = { id: event.pointerId, y: event.clientY, height: this.height };
        handle.setPointerCapture(event.pointerId);
      };
      handle.onpointermove = event => { if (drag?.id === event.pointerId) this.setHeight(drag.height + drag.y - event.clientY); };
      const finish = () => { drag = undefined; };
      handle.onpointerup = handle.onpointercancel = handle.onlostpointercapture = finish;
      handle.onkeydown = event => {
        const heights: Record<string, number> = { ArrowUp: this.height + 32, ArrowDown: this.height - 32, Home: 35, End: this.maxHeight() };
        if (event.key in heights) { event.preventDefault(); event.stopPropagation(); this.setHeight(heights[event.key]); }
      };
      window.addEventListener('resize', () => this.setHeight(this.height));
      this.setHeight(35);
    }
    this.controls();
  }

  private node(id: string) { return this.root.querySelector<HTMLElement>(`#${this.layout === 'panel' ? 'entity-' : ''}${id}`)!; }
  private button(id: string) { return this.node(id) as HTMLButtonElement; }
  private get ready() { return this.layout === 'tray' || this.entityID !== undefined; }
  private get filtered() { return !!(this.search || this.category); }
  private filterQuery(): LogQuery { return { ...(this.search ? { q: this.search } : {}), ...(this.category ? { category: this.category } : {}) }; }

  setFilters(filters: LogFilter[]) {
    if (this.layout === 'tray') (this.node('log-category') as HTMLSelectElement).replaceChildren(...filters.map(filter => new Option(filter.name, filter.id)));
  }

  private applyFilters() {
    clearTimeout(this.searchTimer);
    const search = (this.node('log-search') as HTMLInputElement).value.trim();
    const category = (this.node('log-category') as HTMLSelectElement).value;
    if (search === this.search && category === this.category) return;
    this.search = search; this.category = category;
    this.inspect(this.entityID, true);
  }

  private clearFilters() {
    clearTimeout(this.searchTimer);
    this.search = this.category = '';
    if (this.layout === 'tray') {
      (this.node('log-search') as HTMLInputElement).value = '';
      (this.node('log-category') as HTMLSelectElement).value = '';
    }
  }

  setActive(active: boolean) { this.active = active; if (active) this.catchUp(); }
  selectEntity(entityID?: number) { if (entityID !== this.entityID) this.inspect(entityID); }
  private maxHeight() {
    const world = document.getElementById('world')!, deck = document.querySelector<HTMLElement>('.command-deck')!;
    return Math.max(35, Math.floor(window.innerHeight - world.offsetTop - deck.offsetHeight - 120));
  }
  private setHeight(value: number) {
    // Compact layouts make room for usable log rows before calculating the cap.
    const toggle = this.button('log-toggle');
    toggle.setAttribute('aria-expanded', String(value > 35));
    this.height = Math.round(Math.max(35, Math.min(this.maxHeight(), value)));
    if (this.height > 100) this.expandedHeight = this.height;
    document.documentElement.style.setProperty('--log-height', `${this.height}px`);
    const collapsed = this.height === 35;
    this.node('event-log-body').hidden = collapsed;
    toggle.setAttribute('aria-expanded', String(!collapsed));
    toggle.setAttribute('aria-label', collapsed ? 'Expand event log' : 'Collapse event log');
    toggle.title = collapsed ? 'Expand event log' : 'Collapse event log';
    toggle.querySelector('span')!.textContent = collapsed ? '▴' : '▾';
    const handle = this.node('log-resize');
    handle.setAttribute('aria-valuemin', '35');
    handle.setAttribute('aria-valuemax', String(this.maxHeight()));
    handle.setAttribute('aria-valuenow', String(this.height));
    handle.setAttribute('aria-valuetext', collapsed ? 'One line' : `${this.height} pixels`);
    if (this.following) this.scrollToEnd();
  }

  reset() {
    this.clearFilters();
    this.generation++;
    this.snapshot = undefined; this.entityID = undefined; this.entity = undefined;
    this.entries = []; this.observedCursor = -1; this.busy = false; this.failed = false;
    this.latestCursor = 0; this.metadataCursor = -1; this.metadataBusy = false;
    this.following = true; this.hasOlder = this.hasNewer = false;
    if (this.layout === 'tray') this.node('activity').textContent = 'A new age awaits.';
    this.render();
  }

  update(snapshot: Snapshot) {
    this.snapshot = snapshot;
    const latest = snapshot.events.at(-1);
    if (this.layout === 'tray') {
      const event = this.filtered ? this.entries.at(-1) : latest;
      const prefix = this.filtered ? 'Filtered · ' : '';
      const preview = event ? `${prefix}${stamp(event.time)} · ${event.entity_name ? `${event.entity_name} #${event.entity_id} · ` : ''}${event.message}` : `${prefix}No matching events`;
      this.node('activity').textContent = preview;
      this.node('activity').title = preview;
    }
    if (this.entityID) {
      const current = snapshot.entities.find(e => e.id === this.entityID && e.owner === snapshot.player.id);
      if (current) this.entity = { id: current.id, name: current.name, type: current.type, activity: current.activity, present: true };
    }
    this.controls();
    this.catchUp();
    if (this.active && (this.entityID || this.filtered) && !this.following) void this.refreshMetadata();
  }

  inspect(entityID?: number, preserveFilters = false) {
    if (!preserveFilters) this.clearFilters();
    this.generation++;
    this.entityID = entityID; this.entity = undefined; this.entries = [];
    this.following = true; this.busy = false; this.failed = false; this.observedCursor = -1;
    this.latestCursor = 0; this.metadataCursor = -1; this.metadataBusy = false;
    this.hasOlder = this.hasNewer = false;
    if (this.layout === 'tray' && this.height === 35) this.setHeight(this.expandedHeight);
    this.render();
    void this.fetchPage('latest');
  }

  private catchUp() {
    if (this.active && this.ready && this.snapshot && this.following && !this.busy && !this.failed && this.observedCursor !== this.snapshot.event_cursor) {
      void this.fetchPage(this.entries.length ? 'append' : 'latest');
    }
  }

  private async refreshMetadata() {
    if (!this.snapshot || this.metadataBusy || this.metadataCursor === this.snapshot.event_cursor || this.failed) return;
    const generation = this.generation, cursor = this.snapshot.event_cursor;
    this.metadataBusy = true;
    try {
      // Current status and the entity's newest cursor stay live even when
      // the reader pauses its event rows on an older page.
      const page = await this.api.log({ ...this.filterQuery(), limit: 1 }, this.entityID);
      if (generation !== this.generation) return;
      this.entity = page.entity; this.latestCursor = page.latest_cursor; this.metadataCursor = cursor;
    } catch {
      if (generation === this.generation) {
        this.metadataCursor = cursor;
        if (this.entity) this.entity = { ...this.entity, activity: 'Current status unavailable' };
      }
    } finally {
      if (generation === this.generation) { this.metadataBusy = false; this.controls(); }
    }
  }

  private async fetchPage(direction: 'latest' | 'append' | 'older' | 'newer') {
    if (!this.active || !this.ready || this.busy || !this.api.session) return;
    const generation = this.generation, cursor = this.snapshot?.event_cursor ?? -1;
    const query: LogQuery = { ...this.filterQuery(), limit: 100 };
    if (direction === 'older') query.before = this.entries[0]?.id;
    if (direction === 'append' || direction === 'newer') query.after = this.entries.at(-1)?.id ?? 0;
    this.busy = true; this.failed = false; this.controls();
    try {
      const page = await this.api.log(query, this.entityID);
      if (generation !== this.generation) return;
      this.entity = page.entity;
      this.latestCursor = page.latest_cursor;
      this.metadataCursor = cursor;
      if (direction === 'append' && !this.following) {
        // An in-flight read cannot move the reader or replace their page.
        this.hasNewer = page.latest_cursor > (this.entries.at(-1)?.id ?? 0);
        this.observedCursor = cursor;
        return;
      }
      if (direction === 'newer' && !page.events.length) {
        this.hasNewer = false; this.observedCursor = cursor;
        return;
      }
      if (direction === 'append') {
        const previousCount = this.entries.length;
        const lastID = this.entries.at(-1)?.id ?? 0;
        this.entries = [...this.entries, ...page.events.filter(e => e.id > lastID)].slice(-200);
        // The append page's has_older refers to its own first new entry,
        // not the first entry already retained by this tray.
        this.hasOlder ||= previousCount + page.events.length > 200;
      } else {
        this.entries = page.events; this.hasOlder = page.has_older;
      }
      this.hasNewer = page.has_newer;
      this.observedCursor = page.has_newer && this.following ? -1 : cursor;
      this.render();
      if (this.following) this.scrollToEnd();
      else this.node('event-entries').scrollTop = 0;
    } catch (error) {
      if (generation !== this.generation) return;
      this.failed = true;
      this.node('log-feedback').querySelector('span')!.textContent = `History unavailable. ${error instanceof Error ? error.message : 'Check the connection and retry.'}`;
    } finally {
      if (generation === this.generation) { this.busy = false; this.controls(); this.catchUp(); }
    }
  }

  private controls() {
    if (this.layout === 'tray') {
      this.button('log-global').setAttribute('aria-pressed', String(this.entityID === undefined && !this.filtered));
      this.button('log-clear').disabled = !this.filtered && !this.entityID;
    }
    this.node('log-title').textContent = this.entityID ? this.entity ? `${this.entity.name} #${this.entity.id}` : `Entity #${this.entityID}` : this.ready ? 'Your kingdom' : 'Entity history';
    this.node('log-status').textContent = this.entityID ? this.entity?.activity ?? 'Loading current status…' : this.ready ? this.filtered ? 'Matching events · full history' : 'Your private events' : '';
    const follow = this.button('log-follow');
    follow.textContent = this.following ? 'Live' : 'Follow live';
    follow.setAttribute('aria-pressed', String(this.following));
    follow.disabled = this.busy || !this.snapshot || !this.ready;
    this.button('log-older').disabled = this.busy || !this.hasOlder;
    const newest = this.entityID || this.filtered ? this.latestCursor : this.snapshot?.event_cursor ?? 0;
    this.button('log-newer').disabled = this.busy || !(this.hasNewer || (!this.following && newest > (this.entries.at(-1)?.id ?? 0)));
    const feedback = this.node('log-feedback');
    feedback.hidden = !(this.failed || (this.busy && !this.entries.length) || (!this.busy && !this.entries.length));
    this.button('log-retry').hidden = !this.failed;
    if (!this.failed) feedback.querySelector('span')!.textContent = !this.ready ? 'Select an entity to read its history.' : this.busy ? 'Loading history…' : this.filtered ? 'No matching events. Try another search or clear the filters.' : 'No events yet. Orders and activities will appear here.';
    this.node('event-entries').setAttribute('aria-busy', String(this.busy));
  }

  private render() {
    const fragment = document.createDocumentFragment();
    for (const event of this.entries) {
      const row = document.createElement('li');
      row.className = 'log-entry'; row.dataset.eventId = String(event.id); row.dataset.kind = event.kind;
      const time = document.createElement('time'); time.textContent = stamp(event.time); time.title = `Game time · tick ${event.tick}`;
      const subject = document.createElement(event.entity_id ? 'button' : 'span');
      subject.className = 'log-entity'; subject.textContent = event.entity_name ? `${event.entity_name} #${event.entity_id}` : 'Kingdom';
      if (event.entity_id) {
        subject.setAttribute('aria-label', `View history of ${event.entity_name} #${event.entity_id}`);
        subject.onclick = () => this.inspect(event.entity_id);
      }
      const message = document.createElement('span'); message.className = 'log-message'; message.textContent = event.message;
      if (this.layout === 'panel') row.append(time, message);
      else {
        const locate = document.createElement(event.entity_id ? 'button' : 'span');
        locate.className = 'log-locate';
        if (event.entity_id) {
          locate.textContent = 'Locate';
          locate.setAttribute('aria-label', `Locate ${event.entity_name} #${event.entity_id}`);
          locate.title = 'Select and center on this entity, or visit its last recorded location';
          locate.onclick = () => this.locate?.(event);
        }
        row.append(time, subject, message, locate);
      }
      fragment.append(row);
    }
    this.adjustingScroll = true;
    this.node('event-entries').replaceChildren(fragment);
    requestAnimationFrame(() => { this.adjustingScroll = false; });
    this.controls();
  }

  private scrollToEnd() {
    this.adjustingScroll = true;
    requestAnimationFrame(() => {
      if (!this.following) { this.adjustingScroll = false; return; }
      const list = this.node('event-entries'); list.scrollTop = list.scrollHeight;
      requestAnimationFrame(() => { this.adjustingScroll = false; });
    });
  }
}
