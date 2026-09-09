/*
THESIS: A living medieval miniature, with a familiar RTS command console.
OWN-WORLD: Limestone settlements, olive meadows, forest shadows, brass controls.
STORY: Choose a civilization, direct settlers, develop a kingdom and wage war.
FIRST VIEWPORT: The battlefield fills the screen; resources above, command deck
below, and a small expedition briefing at the upper left.
FORM: The user's pinned Age of Empires reference governs the composition.
*/
import './style.css';
import { APIError, GameAPI } from './api';
import { MultiplayerUI } from './multiplayer';
import { buildingIcon } from './building-icons';
import { WorldRenderer } from './world';
import { EventLog } from './journal';
import { ownerColor } from './models';
import { initializeWorldSetup, readWorldOptions, worldDescription } from './world-setup';
import { MinimapTerrain } from './minimap';
import { renderProduction } from './resource-rates';
import type { Action, Catalog, Command, EntityView, Resources, Snapshot, Vec } from './api.generated';

const crown = `<svg viewBox="0 0 40 40" fill="none" aria-hidden="true"><path d="M8 28 5 12l10 8 5-14 5 14 10-8-3 16H8Z" stroke="currentColor" stroke-width="2"/><path d="M9 33h22" stroke="currentColor" stroke-width="2"/></svg>`;
const symbols: Record<string, string> = { food: '◒', wood: '♠', gold: '◆', stone: '⬟' };
document.querySelector<HTMLDivElement>('#app')!.innerHTML = `
  <div id="world"></div>
  <header class="topbar">
    <a class="brand" href="#" aria-label="AI of Empires, show briefing">${crown}<span>AI <i>of</i><br>Empires</span></a>
    <div class="resources" aria-label="Your resources">${Object.entries(symbols).map(([name, icon]) => `<div class="resource ${name}" title="${name}"><span class="resource-icon">${icon}</span><div><strong id="res-${name}">—</strong><span>${name}</span></div><div class="production" id="production-${name}" role="img" aria-label="${name} production: awaiting first sample"><svg viewBox="0 0 120 20" preserveAspectRatio="none" aria-hidden="true"><path class="production-baseline" d="M0 19H120"/><path class="production-line"/></svg><small>+0/min</small></div></div>`).join('')}</div>
    <div class="population" title="Population / housing capacity"><span class="people-icon">♟</span><strong id="population">— / —</strong><span>Population</span></div>
    <div class="age"><span id="age-symbol">I</span><div><strong id="age">Dark Age</strong><span id="clock">00:00</span><span id="treaty-clock" hidden></span></div></div>
    <div class="match-difficulty"><span>Difficulty</span><strong id="match-difficulty">—</strong></div>
    <div class="match-controls"><button id="speed" title="Change game speed">1.7×</button><button id="pause" title="Pause or resume (Space)" aria-label="Pause match">Ⅱ</button><button id="help" title="Show controls" aria-label="Show controls">?</button><button id="menu" title="Match menu" aria-label="Match menu">☰</button></div>
  </header>
  <aside class="briefing" id="briefing"><div class="eyebrow"><span class="tiny-rule"></span> THE BORDERLANDS</div><h1>A kingdom begins<br>with its people.</h1><p>Gather. Build. Advance. Conquer.</p><div class="objective"><span>⚑</span><div><strong>Establish your settlement</strong><span>Select a villager, choose Give order, then click a resource.</span></div></div><button id="dismiss-briefing" class="text-button">Understood <span>↗</span></button></aside>
  <details class="rival" id="relationships"><summary aria-label="Kingdom relationships"><span class="shield" aria-hidden="true">♜</span><div><strong id="rival-name">Other kingdoms</strong><span id="rival-status">At peace</span></div><span class="rival-dot" aria-hidden="true"></span></summary><div class="relationship-body"><ul id="kingdom-relations"></ul><p>Builders favor growth. Defensive kingdoms may counterattack. Expansionists may start conflicts. Peace returns after five game minutes without attacks.</p></div></details>
  <div id="connection" role="status" hidden></div>
  <div id="notice" role="status" aria-live="polite" hidden></div>
  <section id="startup-error" hidden role="alert"><h2>The kingdom is out of reach</h2><p id="startup-error-copy"></p><button id="startup-retry" class="primary">Try again</button></section>
  <div id="selection-box" hidden></div>
  <div id="paused" hidden><span>Ⅱ</span><h2>The realm rests</h2><p>The match is paused.</p><button id="resume" class="primary">Resume battle</button></div>
  <footer class="command-deck">
    <section class="map-panel"><div class="panel-heading"><span>THE BORDERLANDS</span><button id="home" title="Return to Town Center (H)">⌂</button></div><canvas id="minimap" width="220" height="160" aria-label="Minimap. Click to move the camera." tabindex="0"></canvas><div class="map-caption"><span class="legend-dot"></span><span id="civilization-label">Your kingdom</span><span id="unit-count">0 units</span></div></section>
    <section class="selection-panel"><div class="selection-top"><div id="portrait" class="portrait">${crown}</div><div><span class="eyebrow" id="selected-category">YOUR KINGDOM</span><h2 id="selected-name">The first chapter</h2><div class="health-track"><span id="health-fill"></span></div><span id="selected-health" class="muted">Select a unit or building to give orders.</span></div></div><div class="selection-activity"><div id="selected-status" class="selection-status">Your story is waiting to be written.</div></div><div id="queue" class="queue"></div></section>
    <section class="actions-panel"><div class="panel-heading"><div class="action-tabs" role="group" aria-label="Entity panels"><button class="active" data-tab="orders" aria-pressed="true">Orders</button><button data-tab="build" aria-pressed="false">Build</button><button id="trade-tab" data-tab="trade" aria-pressed="false" hidden>Trade</button><button data-tab="research" aria-pressed="false">Research</button><button id="history-tab" data-tab="history" aria-pressed="false">History</button></div><span id="selection-count">No selection</span></div><div id="mode-hint" hidden><span id="mode-copy" role="status"></span><button id="confirm-delete" hidden>Confirm removal</button><button id="cancel-mode">Cancel</button></div><p id="action-help" class="action-help" aria-live="polite"></p><div id="actions" class="actions"><p class="empty-actions">Select your Town Center to train villagers,<br>or select settlers to begin building.</p></div><div id="entity-history-panel" role="region" aria-label="Entity history" hidden></div></section>
  </footer>
  <section id="event-tray" aria-label="Game event log"></section>
  <dialog id="start-dialog"><nav class="lobby-tabs" aria-label="Campaign setup"><button type="button" id="new-game-tab" aria-pressed="true">New game</button><button type="button" id="saved-games-tab" aria-pressed="false">Saved games</button></nav><form id="start-form">
    <div class="creation-heading"><div class="dialog-brand">${crown}<span>AI of Empires</span></div><h2>Make your mark on history.</h2><p>Choose a kingdom and the world it will call home.</p></div>
    <div class="creation-columns">
      <fieldset><legend>Your kingdom</legend><label class="game-name-label">Game name<input id="game-name" maxlength="80" placeholder="Name this campaign (optional)" autocomplete="off"></label><label>Your kingdom name<input id="player-name" maxlength="40" placeholder="Your name or kingdom (optional)" autocomplete="nickname"></label><label>Your civilization<select id="civilization" name="civilization"></select></label><p id="civilization-bonus" class="bonus"></p><div class="form-row"><label>Difficulty<select id="difficulty" aria-describedby="difficulty-description"></select></label><label>Opening<select id="game-mode"><option value="skirmish">Standard economy</option><option value="sandbox">Abundant resources</option></select></label></div><p id="difficulty-description" class="bonus" aria-live="polite"></p><label>Settlements<select id="settlements" aria-describedby="settlements-hint"></select></label><p id="settlements-hint" class="bonus">Includes your kingdom. Each starts with 3 villagers and a scout.</p><label>Friend seats<select id="friend-seats"></select></label><p class="bonus">Reserve human seats for friends. The remaining kingdoms use AI.</p></fieldset>
      <fieldset class="world-settings"><legend>Your world</legend><label>World type<select id="world-type" aria-describedby="world-description"></select></label><p id="world-description" class="bonus" aria-live="polite"></p><label>Biome<select id="world-biome" aria-describedby="biome-description"></select></label><p id="biome-description" class="bonus" aria-live="polite"></p><label>World size<select id="world-size" aria-describedby="size-description"></select></label><p id="size-description" class="bonus" aria-live="polite"></p><p class="world-note">Every settlement receives the same starting supplies. Biomes change the scenery; terrain shapes the journey.</p></fieldset>
    </div>
    <details class="advanced-world"><summary>Advanced world options <span>Resources, distance, visibility, peace &amp; seed</span></summary><div class="advanced-grid"><div><label>Natural resources<select id="world-resources" aria-describedby="resources-description"></select></label><span id="resources-description" class="field-hint"></span></div><div><label>Starting separation<select id="world-separation" aria-describedby="separation-description"></select></label><span id="separation-description" class="field-hint"></span></div><div><label>Map reveal<select id="world-reveal" aria-describedby="reveal-description"></select></label><span id="reveal-description" class="field-hint"></span></div><div><label>Initial peace period<select id="world-treaty" aria-describedby="treaty-description"></select></label><span id="treaty-description" class="field-hint">Game minutes. Blocks attacks and conversions for every kingdom.</span></div><div><label>Map seed<input id="seed" type="number" value="4817" min="1" max="999999999" required></label><span class="field-hint">The same seed and settings recreate the same world.</span></div></div></details>
    <p class="form-error" id="start-error" role="alert"></p><button id="start" class="primary" type="submit">Begin your reign <span>→</span></button><small>Mouse &amp; trackpad controls · Autosaves every 10 seconds</small>
  </form><section id="saved-games-panel" hidden><h2>Return to your kingdom.</h2><p>Your private game library. Find a game by name or ID; use your rejoin code when changing browsers.</p><form id="resume-form"><label>Game name or session ID<input id="session-search" type="search" maxlength="200" autocomplete="off" placeholder="Find a saved game"></label><button id="resume-game" class="secondary" type="submit">Resume by name or ID</button></form><p id="sessions-error" class="form-error" role="alert"></p><p id="sessions-status" role="status"></p><div id="saved-games-list"></div></section></dialog>
  <dialog id="help-dialog"><button class="dialog-close" data-close="help-dialog" aria-label="Close controls">×</button><div class="eyebrow">FIELD MANUAL</div><h2>Command your kingdom</h2><dl><dt>Click / Shift-drag</dt><dd>Click to select a unit. Shift-click adds or removes a unit; Shift-drag selects a group, replacing your previous selection.</dd><dt>Pan the world</dt><dd>Click to select. Hold the left mouse button briefly, then move to pan while keeping your selection and pending order. A click still gives the order. Wall drawing uses dragging; Shift-drag selects a group.</dd><dt>Rotate the perspective</dt><dd>Hold both the left and right mouse buttons. Move left or right to rotate, and up or down to tilt. Release either button to stop. On a trackpad, use Shift + arrow keys to rotate and tilt. Reset view (R) restores the starting angle and zoom.</dd><dt>Give order / Q</dt><dd>Select your units, choose Give order (or press Q), then click a resource, enemy, building, or destination. With a building selected, click the ground to set its rally point.</dd><dt>Quick orders</dt><dd>Option/Alt-click, Mac Control-click, or right-click gives the same order directly. Two-finger secondary click works too, if enabled on your trackpad.</dd><dt>Build</dt><dd>Select villagers, open Build, choose a building, then click its site. For Stone Walls and Palisades, click and drag a line, then release to place all segments. Gates can replace one of your wall segments.</dd><dt>Fishing</dt><dd>Build a Dock on an explored shore and train a Fishing Ship. Select the ship, choose Fish, then click a fish shoal. Catches become Food when delivered to a Dock. Shoals are finite and disappear when depleted.</dd><dt>Market exchanges</dt><dd>In Feudal Age, build a Market and open its Trade tab to buy or sell food, wood, or stone for gold. Each button shows the price and what you receive. Train a Trade Cart and choose Trade route to trade with an explored neutral Market on connected land.</dd><dt>Reseed farm</dt><dd>Select a depleted Farm and choose Reseed farm. For 60 wood, an assigned farmer or the nearest idle villager rebuilds it and resumes farming. Assigned farmers also reseed automatically while wood is available.</dd><dt>Shift + order</dt><dd>Queue the order after the current task. Keep Shift held to place several orders.</dd><dt>Cmd/Ctrl + 1–9</dt><dd>Save a control group. Press its number to recall it.</dd><dt>H / . / A / S</dt><dd>Find your Town Center, select idle villagers, attack move, or stop.</dd><dt>Pan view / P</dt><dd>Press P, then click and drag the battlefield. Press P again or Escape to return to normal selection and camera gestures. Arrow keys, middle-drag, and clicking the minimap also move the camera.</dd><dt>Zoom / pause</dt><dd>Scroll or pinch to zoom around the ground under your cursor. The + / − keys zoom around the center of the view. Space pauses the match.</dd><dt>Cancel / Escape</dt><dd>Leave any targeting mode. Right-click also cancels building placement.</dd><dt>Military stances</dt><dd>Return fire is the default: units respond to actual attacks on themselves or nearby friends, with limited pursuit. Hold position returns fire without pursuit. Hold fire disables automatic attacks. Aggressive and Attack move can start conflicts with passing kingdoms.</dd><dt>Kingdom relationships</dt><dd>Open the kingdom panel above the world to see who is at peace with you and whether they favor building, defense, or expansion.</dd><dt>Remove units</dt><dd>Choose Delete, then Confirm removal. Delete or the Mac Delete/Backspace key also confirms.</dd></dl><p>Villagers carry resources to a drop-off. Houses raise population capacity. Two distinct buildings from your current age unlock the next age at your Town Center.</p><button class="primary" data-close="help-dialog">Return to the realm</button></dialog>
  <dialog id="menu-dialog"><button class="dialog-close" data-close="menu-dialog" aria-label="Close menu">×</button><div class="eyebrow">YOUR CAMPAIGN</div><h2>A moment to plan</h2><p>Autosaved every 10 seconds. Leaving reserves your kingdom; the shared clock follows the game’s pause policy.</p><p id="session-name" class="session-name"></p><p id="session-world" class="bonus"></p><label>Session ID<input id="session-id" readonly></label><p id="save-status" class="bonus" role="status"></p><p id="save-error" class="form-error" role="alert"></p><button id="save-game" class="secondary">Save now</button><label>Game speed<select id="game-speed"></select></label><button id="menu-pause" class="primary">Pause / resume</button><button id="new-match" class="secondary">Start a new match</button><button id="saved-matches" class="secondary">Leave game</button><button id="resign" class="text-button danger">Resign this battle</button></dialog>
  <dialog id="result-dialog"><div class="dialog-brand">${crown}</div><div class="eyebrow">THE CHRONICLE IS WRITTEN</div><h2 id="result-title">Victory</h2><p id="result-copy"></p><button id="play-again" class="primary">Begin another chapter →</button></dialog>
`;

function el<T extends HTMLElement = HTMLElement>(id: string) { return document.getElementById(id) as T; }
function text(id: string, value: string) { el(id).textContent = value; }
function escape(value: string) { return value.replace(/[&<>"']/g, c => ({ '&': '&amp;', '<': '&lt;', '>': '&gt;', '"': '&quot;', "'": '&#39;' }[c]!)); }
function cost(resources: Resources) { return Object.entries(resources).filter(([, amount]) => amount > 0).map(([key, amount]) => `${Math.round(amount)}${key[0].toUpperCase()}`).join(' · '); }
function time(seconds: number) { return `${Math.floor(seconds / 60).toString().padStart(2, '0')}:${Math.floor(seconds % 60).toString().padStart(2, '0')}`; }
const api = new GameAPI();
const journal = new EventLog(api, el('event-tray'), 'tray', locateEvent);
const entityHistory = new EventLog(api, el('entity-history-panel'), 'panel');
let historyTarget: number | undefined;
let world: WorldRenderer;
let loadedGameID = '';
const multiplayer = new MultiplayerUI(api, enterWorld, () => resetMatch(true), showNotice);
// Input modes only describe the next gesture; Go owns every gameplay lifecycle.
type InputMode = { kind: 'order' | 'pan' } | { kind: 'target'; action: Action } | { kind: 'delete'; entityIds: number[] };
let catalog: Catalog, snapshot: Snapshot | null = null, selection: number[] = [], tab = 'orders', actionSignature = '', queueSignature = '', activeMode: InputMode | null = null;
let buildingPoint: Vec | null = null, previewVersion = 0, previewTimer = 0, noticeTimer = 0, connectionReady = false, resultShown = false;
const groups = new Map<string, number[]>();
function showNotice(message: string) { text('notice', message); el('notice').hidden = false; clearTimeout(noticeTimer); noticeTimer = window.setTimeout(() => { el('notice').hidden = true; }, 5000); }
function selectedViews() { return snapshot?.entities.filter(e => selection.includes(e.id)) ?? []; }
function select(ids: number[]) { cancelMode(); selection = ids; historyTarget = ids[0]; world.setSelection(ids); actionSignature = ''; queueSignature = '__reset__'; refreshSelection(); }
function setMode(mode: InputMode | null, message = '') {
  activeMode = mode; buildingPoint = null; previewVersion++; clearTimeout(previewTimer); previewTimer = 0; world.preview(null);
  text('mode-copy', message); el('mode-hint').hidden = !mode;
  el('confirm-delete').hidden = mode?.kind !== 'delete';
  world.canvas.style.cursor = mode?.kind === 'pan' ? 'grab' : mode ? 'crosshair' : 'default';
}
function cancelMode() { setMode(null); }
function canOrder() {
  if (!connectionReady) { showNotice('Waiting for the server connection.'); return false; }
  if (snapshot?.paused) { showNotice('Resume the match before issuing an order.'); return false; }
  if (!selectedViews().some(e => e.owner === snapshot?.player.id)) { showNotice('Select your units or a building first.'); return false; }
  return true;
}
async function send(command: Omit<Command, 'id'>) {
  if (!connectionReady) { showNotice('Waiting for the server connection.'); return false; }
  try { if (api.session?.membership_id && (command.kind === 'pause' || command.kind === 'speed')) { await api.control(command.kind === 'pause' ? snapshot?.paused ? 'resume' : 'pause' : 'speed', command.kind === 'speed' ? {value:command.value} : {}); } else await api.command(command); return true; } catch (error) { showNotice(error instanceof Error ? error.message : 'The order could not be completed.'); return false; }
}
function startTargeting(action: Action) {
  setMode({ kind: 'target', action }, action.kind === 'build' && ['wall','palisade'].includes(action.product ?? '') ? `${action.label}: click and drag a line, release to place · Esc to cancel` : `${action.label}: click ${action.kind === 'build' ? 'a building site' : 'a target'} · Esc to cancel`); world.canvas.focus();
}
function toggleOrder() {
  if (activeMode?.kind === 'order') cancelMode();
  else if (canOrder()) setMode({ kind: 'order' }, 'Give order: click a resource, target, or destination · Shift queues · Esc cancels');
  world.canvas.focus();
}
function togglePan() {
  if (activeMode?.kind === 'pan') cancelMode();
  else setMode({ kind: 'pan' }, 'Pan view: click and drag the battlefield · Esc returns to selection');
  world.canvas.focus();
}
function confirmRemoval() {
  if (activeMode?.kind !== 'delete') return;
  const ids = activeMode.entityIds; cancelMode(); world.canvas.focus();
  void send({ kind: 'delete', entity_ids: ids });
}
function contextualOrder(x: number, y: number, queue: boolean) {
  if (!canOrder()) return;
  const point = world.groundPoint(x, y), target = world.pick(x, y);
  if (!point) return;
  const own = selectedViews().filter(e => e.owner === snapshot!.player.id);
  // This is the same target/destination API boundary for every input device.
  // Go resolves interact into gathering, combat, healing, garrisoning, etc.
  const cmd: Omit<Command, 'id'> = { kind: target ? 'interact' : own.every(e => e.kind === 'building') ? 'rally' : 'move', entity_ids: own.map(e => e.id), queue };
  if (target) cmd.target_id = target; else cmd.position = point;
  const mode = activeMode;
  void send(cmd).then(accepted => {
    if (!accepted) return;
    world.orderMarker(point);
    if (!queue && activeMode === mode) cancelMode();
  });
}
function runAction(action: Action) {
  if (action.kind === 'interact' && action.enabled) { toggleOrder(); return; }
  if (!action.enabled) { showNotice(action.reason || 'This action is unavailable.'); return; }
  if (snapshot?.paused || !connectionReady) { showNotice(snapshot?.paused ? 'Resume the match before issuing an order.' : 'Waiting for the server connection.'); return; }
  if (['build', 'move', 'attack_move', 'convert', 'heal', 'gather', 'trade'].includes(action.kind)) { startTargeting(action); return; }
  if (action.kind === 'delete') {
    setMode({ kind: 'delete', entityIds: [...selection] }, 'Remove this selection?'); world.canvas.focus(); return;
  }
  const ids = ['train', 'research', 'age', 'deploy', 'unload', 'market_sell', 'market_buy', 'reseed_farm'].includes(action.kind) ? selection.slice(0, 1) : selection;
  void send({ kind: action.kind, product: action.product, entity_ids: ids });
}
function refreshSelection() {
  if (!snapshot) return;
  const selected = selectedViews(), first = selected[0], own = selected.filter(e => e.owner === snapshot!.player.id);
  const hasTrade = first?.actions.some(a => a.gain) ?? false;
  el('trade-tab').hidden = !hasTrade;
  document.querySelector<HTMLButtonElement>('[data-tab=build]')!.hidden = hasTrade;
  if ((tab === 'trade' && !hasTrade) || (tab === 'build' && hasTrade)) { setTab('orders'); return; }
  text('selection-count', selected.length ? `${selected.length} selected` : 'No selection');
  text('selected-name', first ? selected.length > 1 ? `${selected.length} ${own.length === selected.length ? 'units selected' : 'entities selected'}` : first.name : 'The first chapter');
  const other = snapshot.opponents.find(o => o.id === first?.owner);
  text('selected-category', first ? first.owner === snapshot.player.id ? 'YOUR KINGDOM' : other ? `${other.relation === 'hostile' ? 'IN CONFLICT' : 'AT PEACE'} · ${other.temperament.toUpperCase()}` : 'THE BORDERLANDS' : 'YOUR KINGDOM');
  text('selected-health', first ? first.resource ? `${Math.ceil(first.amount ?? 0)} ${first.resource} remaining` : `${Math.ceil(first.hp)} / ${Math.round(first.max_hp)} hit points` : 'Select a unit or building to give orders.');
  el('health-fill').style.transform = `scaleX(${first ? Math.max(0, Math.min(1, first.hp / first.max_hp)) : 0})`;
  const activity = first ? `${first.activity}${first.cargo ? ` · Carrying ${Math.floor(first.cargo)} ${first.cargo_type}` : ''}${first.relic ? ' · Carrying a relic' : ''}` : 'Your story is waiting to be written.';
  text('selected-status', activity); el('selected-status').title = activity;
  const historyOpen = tab === 'history';
  el('entity-history-panel').hidden = !historyOpen;
  el('actions').hidden = el('action-help').hidden = historyOpen;
  document.querySelector('.actions-panel')!.classList.toggle('show-history', historyOpen);
  entityHistory.selectEntity(historyTarget ?? first?.id);
  entityHistory.update(snapshot);
  entityHistory.setActive(historyOpen);
  const portrait = el('portrait'); portrait.dataset.kind = first?.kind || 'empty';
  const pSignature = first ? `${first.type}:${selected.length}` : 'none';
  if (portrait.dataset.signature !== pSignature) { portrait.dataset.signature = pSignature; portrait.innerHTML = first ? `<span class="portrait-glyph">${first.kind === 'building' ? buildingIcon(first.type) : first.kind === 'resource' ? symbols[first.resource || 'gold'] || '◆' : first.type === 'villager' ? '♟' : '♞'}</span>` : crown; }
  const actions = first?.actions ?? [];
  const filtered = actions.filter(a => tab === 'build' ? a.kind === 'build' : tab === 'research' ? a.kind === 'research' || a.kind === 'age' : tab === 'trade' ? !!a.gain : !a.gain && !['build', 'research'].includes(a.kind));
  const signature = JSON.stringify([selection, tab, filtered, own.map(e => e.stance), connectionReady, snapshot.paused]);
  if (signature !== actionSignature) {
    actionSignature = signature;
    const focusedAction = document.activeElement instanceof HTMLElement && el('actions').contains(document.activeElement) ? document.activeElement.dataset.action : undefined;
    el('actions').innerHTML = filtered.length ? filtered.map((a, i) => {
      const stance = a.kind === 'stance';
      const pressed = stance && own.every(e => e.stance === a.product) ? 'true' : stance && own.some(e => e.stance === a.product) ? 'mixed' : 'false';
      const detail = stance ? pressed === 'true' ? 'Selected' : pressed === 'mixed' ? 'Some selected' : 'Stance' : a.gain ? `${cost(a.cost)} → ${cost(a.gain)}` : cost(a.cost) || (a.duration ? `${a.duration}s` : a.kind === 'delete' ? 'Choose to confirm' : 'Command');
      return `<button class="action ${a.kind === 'age' ? 'advance' : ''}" data-action="${i}" ${stance ? `aria-pressed="${pressed}"` : ''} aria-disabled="${!a.enabled || !connectionReady || snapshot!.paused}" aria-describedby="action-help" title="${escape(a.reason || a.description)}${a.duration ? ` · ${Math.round(a.duration)} seconds` : ''}"><span class="action-icon" aria-hidden="true">${a.kind === 'build' ? buildingIcon(a.product ?? '') : a.kind === 'train' ? '♟' : a.kind === 'research' ? '✧' : a.kind === 'age' ? '⇧' : a.kind === 'delete' ? '×' : a.kind === 'stop' ? '■' : stance ? pressed === 'true' ? '✓' : '◇' : '↗'}</span><span class="action-copy"><strong>${escape(a.label)}</strong><small>${escape(detail)}</small></span></button>`;
    }).join('') : `<p class="empty-actions">${first ? tab === 'build' ? 'Select villagers to construct buildings.' : tab === 'research' ? 'Select a building to see its technologies.' : 'No orders available for this selection.' : 'Select your Town Center to train villagers,<br>or select settlers to begin building.'}</p>`;
    text('action-help', tab === 'trade' ? 'Market exchange · prices shown as cost → received' : '');
    el('actions').querySelectorAll<HTMLButtonElement>('[data-action]').forEach(button => { const action = filtered[Number(button.dataset.action)]; button.onclick = () => runAction(action); button.onfocus = button.onpointerenter = () => text('action-help', action.reason || action.description); });
    if (focusedAction !== undefined) el('actions').querySelector<HTMLButtonElement>(`[data-action="${focusedAction}"]`)?.focus({ preventScroll: true });
  }
  const tasks = first?.tasks ?? [], qSignature = tasks.map(t => t.product).join(':');
  if (qSignature !== queueSignature) {
    queueSignature = qSignature;
    el('queue').innerHTML = tasks.map((t, i) => `<button data-queue="${i}" title="Cancel ${escape(t.product.replaceAll('_', ' '))}"><span>${escape(t.product.replaceAll('_', ' '))}</span><i></i><b>×</b></button>`).join('');
    el('queue').querySelectorAll<HTMLButtonElement>('[data-queue]').forEach(button => button.onclick = () => void send({ kind: 'cancel', entity_ids: selection.slice(0, 1), value: Number(button.dataset.queue) }));
  }
  tasks.forEach((task, i) => { const bar = el('queue').querySelectorAll('i')[i]; if (bar) bar.style.width = `${(1 - task.remaining / task.duration) * 100}%`; });
}
const minimapTerrain = new MinimapTerrain();
function minimap() {
  if (!snapshot) return;
  const canvas = el<HTMLCanvasElement>('minimap'), ctx = canvas.getContext('2d')!, map = snapshot.map, sx = canvas.width / map.width, sy = canvas.height / map.height;
  minimapTerrain.draw(ctx, map, canvas.width, canvas.height);
  for (const e of snapshot.entities) { if (e.container) continue; ctx.fillStyle = e.owner > 0 ? ownerColor(e.owner) : e.type === 'tree' ? '#455c35' : e.resource === 'gold' ? '#e3c576' : '#cdc7a4'; const r = e.kind === 'building' ? 2.5 : 1.3; ctx.fillRect(e.position.x * sx - r / 2, e.position.y * sy - r / 2, r, r); }
  const rect = world.canvas.getBoundingClientRect();
  const corners = [[rect.left, rect.top], [rect.right, rect.top], [rect.right, rect.bottom], [rect.left, rect.bottom]].map(([x, y]) => world.groundPoint(x, y));
  ctx.strokeStyle = '#f0e6c6'; ctx.lineWidth = 1; ctx.beginPath();
  corners.forEach((point, index) => { if (point) { if (!index) ctx.moveTo(point.x * sx, point.y * sy); else ctx.lineTo(point.x * sx, point.y * sy); } });
  ctx.closePath(); ctx.stroke();
}
function receive(next: Snapshot) {
  const previous = snapshot;
  snapshot = next; world.update(next);
  if(api.room && next.control_revision!==undefined && next.control_revision>=api.room.revision){api.room.revision=next.control_revision;api.room.match_status=next.status;}
  const tooltip = document.getElementById('building-tooltip');
  if (tooltip && !tooltip.hidden) {
    const entity = next.entities.find(e => e.id === Number(tooltip.dataset.entity));
    if (!entity) { tooltip.hidden = true; world.setHovered(null); }
    else { const status = tooltip.querySelector('span'); if (status) status.textContent = entity.visible ? entity.activity : 'Last seen'; }
  }
  selection = selection.filter(id => next.entities.some(e => e.id === id));
  for (const resource of Object.keys(symbols)) text(`res-${resource}`, Math.floor(next.player.resources[resource as keyof Resources]).toLocaleString());
  if (previous?.player.production !== next.player.production) renderProduction(next.player.production);
  text('population', `${next.player.population} / ${next.player.capacity}`); el('population').classList.toggle('capped', next.player.population >= next.player.capacity);
  text('age', next.player.age_name); text('age-symbol', ['I', 'II', 'III', 'IV'][next.player.age]); text('clock', time(next.time)); text('speed', `${next.speed}×`);
  el('treaty-clock').hidden = next.treaty_remaining <= 0; text('treaty-clock', `Peace ${time(next.treaty_remaining)}`);
  text('match-difficulty', next.difficulty.name); el('match-difficulty').title = next.difficulty.description;
  el<HTMLSelectElement>('game-speed').value = String(next.speed);
  el('speed').title = `Game speed: ${next.speed}×. Click to cycle up to ${catalog.speeds.at(-1)}×; choose any speed in the match menu.`;
  text('idle-count', String(next.player.idle)); text('unit-count', `${next.player.workers} settlers · ${next.player.military} military`); text('civilization-label', catalog.civilizations.find(c => c.id === next.player.civilization)?.name || 'Your kingdom');
  el('paused').hidden = !next.paused; el('pause').setAttribute('aria-label', next.paused ? 'Resume match' : 'Pause match'); text('pause', next.paused ? '▶' : 'Ⅱ');
  const owner = !api.session?.membership_id || !!api.session.owner;
  el<HTMLButtonElement>('pause').disabled = next.paused && !owner; el<HTMLButtonElement>('resume').disabled = !owner;
  el<HTMLButtonElement>('speed').disabled = !owner; el<HTMLSelectElement>('game-speed').disabled = !owner;
  el<HTMLButtonElement>('menu-pause').disabled = next.paused && !owner;
  text('resume', owner ? 'Resume battle' : 'Waiting for the owner to resume');
  const standing = next.opponents.filter(o => !o.defeated), conflicts = standing.filter(o => o.relation === 'hostile').length;
  text('rival-name', next.opponents.length === 1 ? next.opponents[0].name : next.opponents.length ? `${next.opponents.length} other kingdoms` : 'Solo settlement');
  text('rival-status', next.opponents.length === 1 ? `${standing.length ? conflicts ? 'In conflict' : 'At peace' : 'Defeated'} · ${next.opponents[0].temperament}` : next.opponents.length ? `${standing.length - conflicts} at peace · ${conflicts} in conflict` : 'Build at your own pace');
  el('relationships').dataset.conflict = String(conflicts > 0);
  const relations = el('kingdom-relations'), signature = JSON.stringify(next.opponents);
  if (relations.dataset.signature !== signature) {
    relations.dataset.signature = signature;
    relations.innerHTML = next.opponents.length ? next.opponents.map(o => `<li><strong>${escape(o.name)}</strong><span>${o.defeated ? 'Defeated' : o.relation === 'hostile' ? 'In conflict' : 'At peace'} · ${escape(o.temperament)}</span></li>`).join('') : '<li>No other kingdoms in this game.</li>';
  }
  journal.update(next);
  refreshSelection(); minimap();
  if ((next.status === 'finished' || next.player.defeated) && !resultShown) { resultShown = true; text('result-title', next.winner === next.player.id ? 'A kingdom victorious.' : next.winner || next.player.defeated ? 'Your banner has fallen.' : 'An unfinished chronicle.'); text('result-copy', `${time(next.time)} in the field. ${next.player.kills} enemy units and buildings defeated.`); el<HTMLDialogElement>('result-dialog').showModal(); }
}
function connection(state: string) {
  connectionReady = state === 'connected'; el('connection').hidden = connectionReady;
  if (!connectionReady) text('connection', state === 'expired' ? 'This session is no longer connected. Open Saved games from the menu to resume.' : 'Reconnecting to your kingdom…');
  if(state==='expired'&&api.session?.membership_id)void multiplayer.show();
  refreshSelection();
}

function sitesLabel(count: number) { return `${count} wall segment${count === 1 ? '' : 's'}`; }
function attachControls() {
  type Gesture = { wallStart?: Vec; id: number; kind: 'pending' | 'orbit' | 'pan' | 'select' | 'action' | 'secondary'; click: 'select' | 'action' | null; x: number; y: number; lastX: number; lastY: number; shift: boolean; moved: boolean; handled: boolean };
  let pointer: Gesture | null = null, secondaryGesture: Gesture | null = null;
  const panHoldDelay = 180;
  let holdTimer = 0;
  function clearHold() { clearTimeout(holdTimer); holdTimer = 0; }
  const tooltip = document.createElement('div'); tooltip.className = 'building-tooltip'; tooltip.id = 'building-tooltip'; tooltip.setAttribute('role', 'tooltip'); tooltip.hidden = true; document.body.append(tooltip);
  let hoverFrame = 0;
  function clearHover() { cancelAnimationFrame(hoverFrame); hoverFrame = 0; tooltip.hidden = true; world.setHovered(null); }
  world.canvas.addEventListener('pointerleave', clearHover);
  el('world').addEventListener('viewchange', clearHover);
  world.canvas.addEventListener('pointermove', e => {
    if (pointer || activeMode || !snapshot) { clearHover(); return; }
    cancelAnimationFrame(hoverFrame);
    hoverFrame = requestAnimationFrame(() => {
      hoverFrame = 0;
      const id = world.pick(e.clientX, e.clientY), building = snapshot?.entities.find(entity => entity.id === id && entity.kind === 'building');
      if (!building) { clearHover(); return; }
      const name = document.createElement('strong'), status = document.createElement('span');
      name.textContent = `${building.name} #${building.id}`; status.textContent = building.visible ? building.activity : 'Last seen';
      tooltip.dataset.entity = String(building.id);
      tooltip.replaceChildren(name, status); tooltip.hidden = false; world.setHovered(building.id);
      const bounds = world.canvas.getBoundingClientRect();
      tooltip.style.left = `${Math.max(bounds.left + 4, Math.min(e.clientX + 16, bounds.right - tooltip.offsetWidth - 8))}px`;
      tooltip.style.top = `${Math.max(bounds.top + 4, Math.min(e.clientY + 16, bounds.bottom - tooltip.offsetHeight - 8))}px`;
    });
  });
  const restingCursor = () => activeMode?.kind === 'pan' ? 'grab' : activeMode ? 'crosshair' : 'default';
  function secondaryOrder(gesture: Gesture, x: number, y: number, queue: boolean) {
    if (gesture.handled) return;
    gesture.handled = true;
    if (activeMode && activeMode.kind !== 'order') { cancelMode(); return; }
    contextualOrder(x, y, queue);
  }
  world.canvas.addEventListener('contextmenu', e => {
    e.preventDefault();
    // Some systems open the menu on press, others on release. Wait until
    // release so adding the left button can turn a right hold into an orbit.
    if (!pointer && secondaryGesture) secondaryOrder(secondaryGesture, e.clientX, e.clientY, e.shiftKey);
  });
  function beginOrbit(e: PointerEvent) {
    if (!pointer || (e.buttons & 3) !== 3) return;
    clearHold(); secondaryGesture = null;
    pointer.kind = 'orbit'; pointer.handled = true;
    pointer.lastX = e.clientX; pointer.lastY = e.clientY;
    el('selection-box').hidden = true;
    world.canvas.style.cursor = 'grabbing';
  }
  world.canvas.addEventListener('pointerdown', e => {
    clearHover();
    if (!e.isPrimary || pointer || e.button > 2) return;
    world.canvas.focus();
    const secondary = e.button === 2 || (e.button === 0 && (e.ctrlKey || e.altKey));
    const click = !secondary && e.button === 0 && activeMode?.kind !== 'pan' ? activeMode ? 'action' : 'select' : null;
    const drawingWall = click === 'action' && activeMode?.kind === 'target' && activeMode.action.kind === 'build' && ['wall','palisade'].includes(activeMode.action.product ?? '');
    // Selection and armed orders share hold-to-pan. Only an explicit wall
    // drawing tool or Shift-selection owns a primary drag immediately.
    const kind = secondary ? 'secondary' : e.button === 1 || activeMode?.kind === 'pan' ? 'pan' : drawingWall ? 'action' : click === 'select' && e.shiftKey ? 'select' : 'pending';
    pointer = { id: e.pointerId, kind, click, x: e.clientX, y: e.clientY, lastX: e.clientX, lastY: e.clientY, shift: e.shiftKey, moved: false, handled: false };
    if (drawingWall) pointer.wallStart = world.groundPoint(e.clientX,e.clientY) ?? undefined;
    secondaryGesture = secondary ? pointer : null;
    world.canvas.setPointerCapture(e.pointerId);
    if (kind === 'select') world.canvas.style.cursor = 'crosshair';
    if (kind === 'pending') {
      const held = pointer;
      holdTimer = window.setTimeout(() => {
        holdTimer = 0;
        if (pointer !== held || held.kind !== 'pending') return;
        held.kind = 'pan'; world.canvas.style.cursor = 'grab';
      }, panHoldDelay);
    }
    beginOrbit(e);
  });
  type PreviewRequest = { product: string; point: Vec; start?: Vec; preview: EntityView; mode: InputMode | null; version: number };
  let pendingPreview: PreviewRequest | null = null, previewInFlight = false, lastPreviewAt = 0;
  function schedulePreview() {
    if (previewInFlight || previewTimer || !pendingPreview) return;
    previewTimer = window.setTimeout(() => { previewTimer = 0; void checkPreview(); }, Math.max(0, 120 - (performance.now() - lastPreviewAt)));
  }
  async function checkPreview() {
    const request = pendingPreview; pendingPreview = null;
    if (!request || request.mode !== activeMode || request.version !== previewVersion) return;
    previewInFlight = true; lastPreviewAt = performance.now();
    try {
      const { product, point, start, preview } = request;
      const result = await api.placement(product, start ?? point, start ? point : undefined);
      if (request.mode !== activeMode || request.version !== previewVersion) return;
      if (start && result.positions?.length) {
        const sites = result.positions;
        world.previewMany(sites.map(position => ({...preview, position, connections: [{x:0,y:-1},{x:1,y:0},{x:0,y:1},{x:-1,y:0}].filter(d => sites.some(other => other.x === position.x+d.x && other.y === position.y+d.y))})), result.valid);
      } else world.preview(preview, point, result.valid);
      text('mode-copy', result.valid ? start ? `${sitesLabel(result.positions.length)} · ${cost(result.cost)} · Release to place` : `${preview.name}: ${['wall','palisade'].includes(product) ? 'click and drag a line' : 'click to place'} · Esc to cancel` : result.reason || 'Choose another site.');
    } catch {
      if (request.mode === activeMode) text('mode-copy', 'Site check unavailable. Try again shortly.');
    } finally { previewInFlight = false; schedulePreview(); }
  }
  world.canvas.addEventListener('pointermove', e => {
    if (pointer && pointer.id !== e.pointerId) return;
    // Pointer Events report extra mouse buttons as pointermove, including
    // their release. Consume the entire chord, even after one button lifts.
    if (pointer && (e.buttons & 3) === 3 && (pointer.kind !== 'orbit' || e.button !== -1)) beginOrbit(e);
    if (pointer?.kind === 'orbit') {
      if ((e.buttons & 3) === 3) world.orbitBy(e.clientX - pointer.lastX, e.clientY - pointer.lastY);
      pointer.lastX = e.clientX; pointer.lastY = e.clientY; return;
    }
    if (pointer?.kind === 'pending') {
      // Discard motion before the hold delay so a click cannot nudge the
      // camera, and a subsequent pan starts without a jump. An early drag
      // in an armed order must not become a command when released.
      if (pointer.click === 'action' && Math.hypot(e.clientX - pointer.x, e.clientY - pointer.y) > 5) pointer.moved = true;
      pointer.lastX = e.clientX; pointer.lastY = e.clientY; return;
    }
    if (pointer && pointer.kind !== 'secondary') {
      if (Math.hypot(e.clientX - pointer.x, e.clientY - pointer.y) > 5) pointer.moved = true;
      if (pointer.moved && pointer.kind === 'pan') {
        world.canvas.style.cursor = 'grabbing';
        world.panBetween(pointer.lastX, pointer.lastY, e.clientX, e.clientY);
        pointer.lastX = e.clientX; pointer.lastY = e.clientY; return;
      }
      if (pointer.moved && pointer.kind === 'select') { const box = el('selection-box'); box.hidden = false; Object.assign(box.style, { left: `${Math.min(pointer.x, e.clientX)}px`, top: `${Math.min(pointer.y, e.clientY)}px`, width: `${Math.abs(e.clientX - pointer.x)}px`, height: `${Math.abs(e.clientY - pointer.y)}px` }); }
    }
    const action = activeMode?.kind === 'target' ? activeMode.action : null;
    if (action?.kind === 'build') {
      const point = world.groundPoint(e.clientX, e.clientY); if (!point) return; buildingPoint = point;
      const definition = catalog.definitions.find(d => d.id === action.product); if (!definition) return;
      const preview: EntityView = { ...definition, id: -1, type: definition.id, kind: definition.kind, name: definition.name, owner: 1, position: point, hp: definition.hp, max_hp: definition.hp, progress: 1, state: 'preview', activity: 'Placement preview', visible: true, actions: [], tasks: [], passengers: [], deployed: false, relic: false };
      if (!pointer?.wallStart) world.preview(preview, point);
      pendingPreview = { product: action.product!, point, start: pointer?.wallStart, preview, mode: activeMode, version: previewVersion };
      text('mode-copy', `${definition.name}: checking ${pointer?.wallStart ? 'wall line' : 'this site'}…`);
      schedulePreview();
    }
  });
  world.canvas.addEventListener('pointerup', e => {
    if (!pointer || pointer.id !== e.pointerId) return;
    clearHold();
    const down = pointer; pointer = null; el('selection-box').hidden = true;
    world.canvas.style.cursor = restingCursor();
    if (world.canvas.hasPointerCapture(e.pointerId)) world.canvas.releasePointerCapture(e.pointerId);
    if (!snapshot) return;
    if (down.kind === 'secondary') { secondaryOrder(down, e.clientX, e.clientY, e.shiftKey); return; }
    // A completed camera drag must never select or order whatever is under
    // the release point. Targeting also requires a click, not a drag.
    if (down.handled || down.kind === 'pan' && !down.click || (down.moved && down.kind !== 'select' && !down.wallStart)) return;
    if (down.click === 'action' && activeMode?.kind === 'order') { contextualOrder(e.clientX, e.clientY, e.shiftKey); return; }
    if (down.click === 'action' && activeMode?.kind === 'delete') { cancelMode(); return; }
    const point = world.groundPoint(e.clientX, e.clientY), target = world.pick(down.click === 'select' ? down.x : e.clientX, down.click === 'select' ? down.y : e.clientY);
    if (down.click === 'action' && activeMode?.kind === 'target' && point) {
      const action = activeMode.action;
      const cmd: Omit<Command, 'id'> = { kind: action.kind, entity_ids: selection, queue: e.shiftKey };
      if (action.kind === 'build') { cmd.position = down.wallStart ?? point; cmd.product = action.product; if (down.wallStart) cmd.end_position = point; }
      else if (['heal', 'convert', 'gather', 'trade'].includes(action.kind)) { if (!target) return; cmd.target_id = target; }
      else cmd.position = point;
      const mode = activeMode, keepMode = e.shiftKey;
      void send(cmd).then(accepted => { if (!accepted) return; world.orderMarker(point); if (!keepMode && activeMode === mode) cancelMode(); }); return;
    }
    if (down.click === 'select') {
      if (down.moved) {
        const ids = snapshot.entities.filter(entity => { if (entity.owner !== snapshot!.player.id || entity.kind !== 'unit' || entity.container) return false; const p = world.screenPoint(entity.id); return p && p.x >= Math.min(down.x, e.clientX) && p.x <= Math.max(down.x, e.clientX) && p.y >= Math.min(down.y, e.clientY) && p.y <= Math.max(down.y, e.clientY); }).map(e => e.id);
        select(ids);
      } else if (target) select(down.shift ? selection.includes(target) ? selection.filter(id => id !== target) : [...selection, target] : [target]);
      else if (!down.shift) select([]);
    }
  });
  window.addEventListener('keydown', e => {
    if (e.key === 'Escape' && !document.querySelector('dialog[open]')) { world.keys.clear(); clearGesture(); cancelMode(); return; }
    if ((e.target as HTMLElement).matches('input, select, textarea, button, a, [contenteditable=true]') || (e.target as HTMLElement).closest('#event-tray, #entity-history-panel') || document.querySelector('dialog[open]')) return;
    if (/^[0-9]$/.test(e.key) && (e.ctrlKey || e.metaKey) && !e.altKey) {
      e.preventDefault(); if (!e.repeat) groups.set(e.key, [...selection]); return;
    }
    // Leave browser/OS combinations (Cmd+S, Cmd+A, Cmd+H, etc.) alone.
    if (e.metaKey || e.ctrlKey || e.altKey) return;
    if (e.key === 'Shift') world.keys.add('Shift');
    if (e.key.startsWith('Arrow')) { e.preventDefault(); world.keys.add(e.key); if (e.shiftKey) world.keys.add('Shift'); }
    if (e.key === '-' || e.key === '_') { e.preventDefault(); world.zoomBy(4); }
    if (e.key === '+' || e.key === '=') { e.preventDefault(); world.zoomBy(-4); }
    if (e.repeat) return;
    if (e.key.toLowerCase() === 'q') { e.preventDefault(); toggleOrder(); }
    if (e.key.toLowerCase() === 'p') { e.preventDefault(); togglePan(); }
    if (e.key.toLowerCase() === 'r') { e.preventDefault(); resetView(); }
    if (e.code === 'Space') { e.preventDefault(); void send({ kind: 'pause' }); }
    if (e.key.toLowerCase() === 'h') home();
    if (e.key === '.') idle();
    if (e.key.toLowerCase() === 's' && selection.length) void send({ kind: 'stop', entity_ids: selection });
    if (e.key.toLowerCase() === 'a') { const action = selectedViews()[0]?.actions.find(a => a.kind === 'attack_move'); if (action) startTargeting(action); }
    if ((e.key === 'Delete' || e.key === 'Backspace') && activeMode?.kind === 'delete') { e.preventDefault(); confirmRemoval(); }
    if (/^[0-9]$/.test(e.key)) { e.preventDefault(); select((groups.get(e.key) ?? []).filter(id => snapshot?.entities.some(e => e.id === id))); }
  });
  function clearGesture() {
    clearHold();
    const previous = pointer;
    pointer = null; secondaryGesture = null; el('selection-box').hidden = true;
    world.canvas.style.cursor = restingCursor();
    if (previous && world.canvas.hasPointerCapture(previous.id)) world.canvas.releasePointerCapture(previous.id);
  }
  world.canvas.addEventListener('pointercancel', e => { if (pointer?.id === e.pointerId) clearGesture(); });
  world.canvas.addEventListener('lostpointercapture', e => { if (pointer?.id === e.pointerId) clearGesture(); });
  window.addEventListener('keyup', e => world.keys.delete(e.key)); window.addEventListener('blur', () => { world.keys.clear(); clearGesture(); });
  el('cancel-mode').onclick = () => { cancelMode(); world.canvas.focus(); };
  el('confirm-delete').onclick = confirmRemoval;
  function resetView() { clearGesture(); cancelMode(); world.resetView(); world.canvas.focus(); }
  el<HTMLCanvasElement>('minimap').onclick = e => { const rect = el('minimap').getBoundingClientRect(); world.focus({ x: (e.clientX - rect.left) / rect.width * (snapshot?.map.width || 72), y: (e.clientY - rect.top) / rect.height * (snapshot?.map.height || 72) }); minimap(); };
}
function locateEvent(event: import('./api.generated').Event) {
  if (!snapshot || !event.entity_id) return;
  const current = snapshot.entities.find(entity => entity.id === event.entity_id && (entity.visible || entity.owner === snapshot!.player.id));
  world.focus(current?.position ?? event.position);
  select(current ? [current.id] : []);
  historyTarget = event.entity_id;
  setTab('history');
  minimap();
  if (!current) showNotice(`${event.entity_name} #${event.entity_id} is no longer visible. Showing its recorded location at ${time(event.time)}.`);
}
function home() { const tc = snapshot?.entities.find(e => e.owner === snapshot!.player.id && e.type === 'town_center'); if (tc) { select([tc.id]); world.focus(tc.position); } }
function idle() { const units = snapshot?.entities.filter(e => e.owner === snapshot!.player.id && e.type === 'villager' && e.state === 'idle') ?? []; select(units.map(e => e.id)); if (units.length) world.focus(units[0].position); }
async function resetMatch(saved = false) {
  text('save-error', '');
  try { await api.close(); }
  catch (error) {
    if (error instanceof APIError && [401, 404].includes(error.status)) api.forget();
    else { if(snapshot)api.subscribe(receive, connection); const message = error instanceof Error ? error.message : 'Save failed. Try again before leaving.'; text('save-error', message); const roomError=document.getElementById('room-error');if(roomError)roomError.textContent=message;showNotice(message);return; }
  }
  multiplayer.hide();
  journal.reset(); entityHistory.reset(); historyTarget = undefined;
  document.querySelectorAll<HTMLDialogElement>('dialog[open]').forEach(d => d.close());
  selection = []; groups.clear(); world.resetWorld(); resultShown = false; connectionReady = false; snapshot = null; loadedGameID = ''; cancelMode(); setTab('orders');
  setLobbyTab(saved); el<HTMLDialogElement>('start-dialog').showModal();
}
document.querySelectorAll<HTMLButtonElement>('[data-close]').forEach(b => b.onclick = () => el<HTMLDialogElement>(b.dataset.close!).close());
function setTab(next: string) { tab = next; document.querySelectorAll<HTMLButtonElement>('[data-tab]').forEach(button => { const active = button.dataset.tab === tab; button.classList.toggle('active', active); button.setAttribute('aria-pressed', String(active)); }); refreshSelection(); }
document.querySelectorAll<HTMLButtonElement>('[data-tab]').forEach(b => b.onclick = () => setTab(b.dataset.tab!));
el('dismiss-briefing').onclick = () => { el('briefing').hidden = true; };
el('help').onclick = () => el<HTMLDialogElement>('help-dialog').showModal();
el('menu').onclick = () => { el<HTMLDialogElement>('menu-dialog').showModal(); void refreshSaveStatus(); };
el('pause').onclick = el('resume').onclick = () => void send({ kind: 'pause' });
el('menu-pause').onclick = () => { el<HTMLDialogElement>('menu-dialog').close(); void send({ kind: 'pause' }); };
el('speed').onclick = () => { if (catalog && snapshot) void send({ kind: 'speed', value: catalog.speeds[(catalog.speeds.indexOf(snapshot.speed) + 1) % catalog.speeds.length] }); };
el('game-speed').onchange = () => void send({ kind: 'speed', value: Number(el<HTMLSelectElement>('game-speed').value) });
el('home').onclick = home; el('idle').onclick = idle;
el('new-match').onclick = el('play-again').onclick = () => void resetMatch();
el('saved-matches').onclick = () => void resetMatch(true);
el('save-game').onclick = async () => { text('save-error', ''); el<HTMLButtonElement>('save-game').disabled = true; text('save-status', 'Saving…'); try { showSaveStatus(await api.save()); } catch (error) { text('save-error', error instanceof Error ? error.message : 'Save failed. Try again.'); void refreshSaveStatus(); } finally { el<HTMLButtonElement>('save-game').disabled = false; } };
el('resign').onclick = () => { el<HTMLDialogElement>('menu-dialog').close(); void send({ kind: 'resign' }); };
document.querySelector<HTMLAnchorElement>('.brand')!.onclick = e => { e.preventDefault(); el('briefing').hidden = !el('briefing').hidden; };
el('world').addEventListener('renderlost', () => showNotice('Graphics interrupted. Your match remains on the server.'));
el('world').addEventListener('renderrestored', () => showNotice('Battlefield graphics restored.'));
let minimapFrame = 0;
el('world').addEventListener('viewchange', () => {
  if (!minimapFrame) minimapFrame = requestAnimationFrame(() => { minimapFrame = 0; minimap(); });
});

async function boot() {
  try { world = new WorldRenderer(el('world')); } catch { el('world').innerHTML = '<div class="unsupported"><h1>A battlefield needs room to breathe.</h1><p>This device could not start WebGL 2. Try a desktop browser with hardware acceleration enabled.</p></div>'; return; }
  attachControls();
  await loadSession();
}
async function loadSession() {
  el<HTMLButtonElement>('startup-retry').disabled = true;
  try {
    catalog = await api.catalog();
    initializeWorldSetup(catalog.worlds); multiplayer.configure(catalog);
    el<HTMLSelectElement>('game-speed').replaceChildren(...catalog.speeds.map(speed => new Option(`${speed}×`, String(speed))));
    journal.setFilters(catalog.log_filters);
    el<HTMLSelectElement>('settlements').replaceChildren(...catalog.settlement_counts.map(n => new Option(n === 1 ? '1 · Solo' : `${n} · You + ${n - 1} other${n === 2 ? '' : 's'}`, String(n))));
    el<HTMLSelectElement>('settlements').value = '2';
    const friends = () => { const f=el<HTMLSelectElement>('friend-seats'),previous=f.value;f.replaceChildren(...Array.from({length:Number(el<HTMLSelectElement>('settlements').value)},(_,i)=>new Option(i===0?'None · play with AI':`${i} friend${i===1?'':'s'}`,String(i))));f.value=Number(previous)<f.options.length?previous:'0';if(!f.value)f.value='0'; };
    el('settlements').onchange=friends; friends();
    const difficulty = el<HTMLSelectElement>('difficulty');
    difficulty.replaceChildren(...catalog.difficulties.map(option => new Option(option.name, option.id)));
    difficulty.value = 'normal';
    const describeDifficulty = () => text('difficulty-description', catalog.difficulties.find(option => option.id === difficulty.value)?.description ?? '');
    difficulty.onchange = describeDifficulty; describeDifficulty();
    el('startup-error').hidden = true;
    el<HTMLSelectElement>('civilization').innerHTML = catalog.civilizations.map(c => `<option value="${escape(c.id)}">${escape(c.name)} · ${escape(c.description)}</option>`).join('');
    const updateBonus = () => text('civilization-bonus', catalog.civilizations.find(c => c.id === el<HTMLSelectElement>('civilization').value)?.bonus ?? '');
    el('civilization').onchange = updateBonus; updateBonus();
    if (await multiplayer.handleLocation()) return;
    if (api.restore()) {
      try { await api.adoptLegacy(); await multiplayer.continueSession(); return; }
      catch (error) { if (error instanceof APIError && [401, 404].includes(error.status)) api.forget(); else throw error; }
    }
    el<HTMLDialogElement>('start-dialog').showModal();
  } catch (error) { text('startup-error-copy', error instanceof Error ? error.message : 'Could not reach the game server.'); el('startup-error').hidden = false; }
  finally { el<HTMLButtonElement>('startup-retry').disabled = false; }
}
el('startup-retry').onclick = () => void loadSession();
el<HTMLInputElement>('seed').addEventListener('invalid', () => {
  el<HTMLDialogElement>('start-dialog').querySelector<HTMLDetailsElement>('.advanced-world')!.open = true;
  text('start-error', 'Enter a whole-number map seed between 1 and 999999999.');
  el<HTMLInputElement>('seed').focus();
});
el<HTMLInputElement>('seed').addEventListener('input', () => text('start-error', ''));
el<HTMLFormElement>('start-form').onsubmit = async e => {
  e.preventDefault(); el<HTMLButtonElement>('start').disabled = true; text('start-error', '');
  try {
    await api.create({ civilization: el<HTMLSelectElement>('civilization').value, difficulty: el<HTMLSelectElement>('difficulty').value, mode: el<HTMLSelectElement>('game-mode').value, seed: Number(el<HTMLInputElement>('seed').value) || 4817, name: el<HTMLInputElement>('game-name').value, settlements: Number(el<HTMLSelectElement>('settlements').value), world: readWorldOptions() }, Number(el<HTMLSelectElement>('friend-seats').value), el<HTMLInputElement>('player-name').value);
    await multiplayer.show();
  } catch (error) { text('start-error', error instanceof Error ? error.message : 'Could not start the match.'); }
  finally { el<HTMLButtonElement>('start').disabled = false; }
};
el<HTMLDialogElement>('start-dialog').addEventListener('cancel', e => { if (!snapshot) e.preventDefault(); });
let listVersion = 0, searchTimer = 0, resuming = false;
function showSaveStatus(info: import('./api.generated').SavedGame) {
  text('session-name', info.name); text('session-world', worldDescription(catalog.worlds, info.world)); el<HTMLInputElement>('session-id').value = info.match_id;
  text('save-status', info.save_error || `Saved ${new Date(info.saved_at).toLocaleTimeString()} · Autosave every ${info.autosave_seconds}s`);
}
async function refreshSaveStatus() {
  const id = api.session?.match_id; if (!id) return;
  try { const info = await api.info(); if (api.session?.match_id === id) showSaveStatus(info); }
  catch { if (api.session?.match_id === id) text('save-status', 'Save status unavailable. Reconnect, then try Save now.'); }
}
function setLobbyTab(saved: boolean) {
  multiplayer.hide();
  el('start-form').hidden = saved; el('saved-games-panel').hidden = !saved;
  el('new-game-tab').setAttribute('aria-pressed', String(!saved)); el('saved-games-tab').setAttribute('aria-pressed', String(saved));
  if (saved) { void loadSavedGames(); el<HTMLInputElement>('session-search').focus(); }
}
async function loadSavedGames() {
  const version = ++listVersion; text('sessions-error', ''); text('sessions-status', 'Loading saved games…');
  try {
    const result = await api.sessions(el<HTMLInputElement>('session-search').value);
    if (version !== listVersion) return;
    el('saved-games-list').replaceChildren(...result.games.map(game => {
      const row = document.createElement('div'); row.className = 'saved-game';
      const copy = document.createElement('div'), name = document.createElement('strong'), details = document.createElement('span');
      name.textContent = game.name; details.textContent = `${worldDescription(catalog.worlds, game.world)} · ${time(game.time)} played · ${game.settlements} settlements · ${catalog.difficulties.find(d => d.id === game.difficulty)?.name ?? game.difficulty} · Saved ${new Date(game.saved_at).toLocaleString()}`;
      copy.append(name, details); const button = document.createElement('button'); button.textContent = 'Resume'; button.setAttribute('aria-label', `Resume ${game.name}`); button.disabled = resuming;
      button.onclick = () => void resumeGame(game.match_id);
      const manage=document.createElement('button');manage.textContent='Manage';manage.setAttribute('aria-label',`Manage ${game.name}`);manage.onclick=()=>void multiplayer.show(game.match_id);
      const controls=document.createElement('div');controls.className='saved-game-actions';controls.append(button,manage);row.append(copy,controls);return row;
    }));
    text('sessions-status', result.games.length ? `${result.games.length} saved game${result.games.length === 1 ? '' : 's'}${result.games.length === 100 ? ' shown — narrow your search for more' : ''}` : el<HTMLInputElement>('session-search').value ? 'No games match this search. Try another name or session ID.' : 'No saved games yet. Begin a new campaign to create one.');
  } catch (error) { if (version === listVersion) { text('sessions-status', ''); text('sessions-error', error instanceof Error ? error.message : 'Could not load saved games. Try again.'); } }
}
async function resumeGame(identifier: string) {
  if (resuming) return; resuming = true; text('sessions-error', ''); text('sessions-status', 'Resuming your kingdom…');
  el<HTMLButtonElement>('resume-game').disabled = true; el('saved-games-list').querySelectorAll('button').forEach(b => b.disabled = true);
  try {
    await api.resume(identifier); await multiplayer.continueSession();
  } catch (error) { text('sessions-status', ''); text('sessions-error', error instanceof Error ? error.message : 'Could not resume. Try again.'); }
  finally { resuming = false; el<HTMLButtonElement>('resume-game').disabled = false; el('saved-games-list').querySelectorAll('button').forEach(b => b.disabled = false); }
}
el('new-game-tab').onclick = () => setLobbyTab(false);
el('saved-games-tab').onclick = () => setLobbyTab(true);
el<HTMLInputElement>('session-search').oninput = () => { ++listVersion; clearTimeout(searchTimer); searchTimer = window.setTimeout(() => void loadSavedGames(), 180); };
el<HTMLFormElement>('resume-form').onsubmit = e => { e.preventDefault(); void resumeGame(el<HTMLInputElement>('session-search').value); };
el<HTMLInputElement>('session-id').onclick = () => el<HTMLInputElement>('session-id').select();
window.setInterval(() => { if (api.session && snapshot) void refreshSaveStatus(); }, 2_000);
window.addEventListener('pagehide', () => api.suspendOnClose());
window.addEventListener('pageshow', event => {
  if (!event.persisted) return;
  document.querySelectorAll<HTMLDialogElement>('dialog[open]').forEach(d => d.close());
  journal.reset(); entityHistory.reset(); world.resetWorld(); selection = []; groups.clear(); snapshot = null; resultShown = false; connectionReady = false; historyTarget = undefined; cancelMode();
  void loadSession();
});
async function enterWorld() {
  const changed=loadedGameID!==api.session?.match_id;
  if(changed){world.resetWorld();journal.reset();entityHistory.reset();selection=[];groups.clear();resultShown=false;historyTarget=undefined;}
  await api.ensureConnection(); receive(await api.snapshot()); api.subscribe(receive,connection);
  if(changed)home(); loadedGameID=api.session!.match_id;
  void refreshSaveStatus();el<HTMLDialogElement>('start-dialog').close();
}
void boot();
