/*
THESIS: A private council before a shared campaign, with a readable seat roster.
OWN-WORLD: Existing olive panels, brass actions, Palatino game titles, native forms.
STORY: Name your kingdom, invite friends, ready up, then return to the same seats.
FIRST VIEWPORT: Game name and world summary above kingdom rows; Ready and Start
remain beside their refusal reasons. Recovery and portability follow the roster.
FORM: Extend the established campaign dialog and its compact operating controls.
*/
import { GameAPI } from './api';
import type { AuditPage, Catalog, Config, GameInfo, Invitation, SeatView, WorldOptions } from './api.generated';
import { worldDescription } from './world-setup';

const escape = (value: string) => value.replace(/[&<>"']/g, c => ({'&':'&amp;','<':'&lt;','>':'&gt;','"':'&quot;',"'":'&#39;'}[c]!));
const node = <T extends HTMLElement = HTMLElement>(id: string) => document.getElementById(id) as T;
const statusName = (g: GameInfo) => g.runtime === 'frozen' ? 'Moving · source frozen' : g.runtime === 'released' ? 'Moved to another host' : ({lobby:'Private lobby',open:g.match_status === 'paused' ? 'Paused' : g.match_status === 'finished' ? 'Finished' : 'Playing',closing:'Closing · save pending',closed:'Closed and saved'}[g.status] ?? g.status);

export class MultiplayerUI {
  private catalog!: Catalog;
  private poll = 0;
  private busy = false;
  private signature = '';
  private inviteLinks = new Map<string,{id:string;url:string}>();
  private enterOnStart = false;
  private currentID = '';
  private root: HTMLElement;
  private inviteSecret = '';
  private completionReceipts = new Map<string,string>();
  constructor(private api: GameAPI, private enter: () => Promise<void>, private leave: () => Promise<void>, private notify: (s:string)=>void) {
    this.root=document.createElement('section');this.root.id='room-panel';this.root.hidden=true;node('start-dialog').append(this.root);
    const join=document.createElement('section');join.id='join-panel';join.hidden=true;node('start-dialog').append(join);
    const recovery=document.createElement('section');recovery.className='membership-recovery';recovery.innerHTML=`<details><summary>Rejoin from another browser or host</summary><form id="rejoin-form"><label>Private rejoin code<input id="rejoin-code" required autocomplete="off" spellcheck="false" maxlength="64" placeholder="Your personal 64-character code"></label><p>Use your own code to recover your original kingdom. Game names and IDs only search this browser's private library.</p><button class="secondary">Rejoin my kingdom</button></form></details><details><summary>Import a game archive</summary><form id="import-form"><label>Game archive<input name="archive" type="file" accept=".aoegame" required></label><label>Archive passphrase<input name="passphrase" type="password" autocomplete="off" maxlength="256" placeholder="If the archive is protected"></label><label>Owner's private rejoin code<input name="rejoin_code" required maxlength="64" autocomplete="off" spellcheck="false"></label><label class="check-label"><input name="copy" type="checkbox">Copy as a new game</label><label>New copy name<input name="name" maxlength="80" placeholder="Optional"></label><p>A move preserves kingdoms and opens paused. A copy creates a separate game with fresh player access.</p><button class="secondary">Import game</button></form></details><p id="membership-error" class="form-error" role="alert"></p></section>`;node('saved-games-panel').append(recovery);
    node<HTMLFormElement>('rejoin-form').onsubmit=e=>{e.preventDefault();void this.recover(async()=>{await this.api.rejoin(node<HTMLInputElement>('rejoin-code').value.trim());await this.continueSession()})};
    node<HTMLFormElement>('import-form').onsubmit=e=>{e.preventDefault();void this.recover(async()=>{const form=new FormData(node<HTMLFormElement>('import-form'));form.set('copy',String((node<HTMLFormElement>('import-form').elements.namedItem('copy') as HTMLInputElement).checked));const result=await this.api.importGame(form);if(result.completion_receipt)this.completionReceipts.set(result.session.match_id,result.completion_receipt);await this.show();})};
    const button=document.createElement('button');button.id='manage-game';button.className='secondary';button.textContent='Players & game management';node('save-game').after(button);button.onclick=()=>void this.show();
  }
  configure(catalog: Catalog) {this.catalog=catalog;}
  private async recover(action:()=>Promise<void>){node('membership-error').textContent='';node('saved-games-panel').querySelectorAll<HTMLButtonElement>('button').forEach(b=>b.disabled=true);try{await action()}catch(e){node('membership-error').textContent=e instanceof Error?e.message:'Could not recover this game.'}finally{node('saved-games-panel').querySelectorAll<HTMLButtonElement>('button').forEach(b=>b.disabled=false)}}
  private expose(which: 'room-panel'|'join-panel') {
    document.querySelectorAll<HTMLDialogElement>('dialog[open]').forEach(d=>{if(d.id!=='start-dialog')d.close()});
    node('start-form').hidden=true;node('saved-games-panel').hidden=true;node('room-panel').hidden=which!=='room-panel';node('join-panel').hidden=which!=='join-panel';node('start-dialog').querySelector<HTMLElement>('.lobby-tabs')!.hidden=true;
    if(!node<HTMLDialogElement>('start-dialog').open)node<HTMLDialogElement>('start-dialog').showModal();
  }
  hide(){clearInterval(this.poll);this.poll=0;this.root.hidden=true;this.root.replaceChildren();this.signature='';node('join-panel').hidden=true;node('start-dialog').querySelector<HTMLElement>('.lobby-tabs')!.hidden=false;}
  async continueSession(){
    if(!this.api.session?.membership_id){await this.enter();return}
    const g=await this.api.gameInfo();
    if(g.status==='open'&&g.runtime==='serving'){await this.api.ensureConnection();await this.enter();this.hide()}
    else await this.show();
  }
  async show(id?:string) {
    try {
      if(id&&id!==this.api.session?.match_id)await this.api.useGame(id);
      const g=await this.api.gameInfo();this.currentID=g.game_id;this.enterOnStart=g.status==='lobby';this.signature='';this.expose('room-panel');
      if(['lobby','open'].includes(g.status)&&g.runtime==='serving')await this.api.ensureConnection();
      this.render(g);clearInterval(this.poll);this.poll=window.setInterval(()=>void this.refresh(),1000);
    } catch(e){this.notify(e instanceof Error?e.message:'Game management is unavailable.')}
  }
  async refresh(force=false){
    if(this.busy||this.root.hidden||this.api.session?.match_id!==this.currentID)return;
    try {const g=await this.api.gameInfo();if(this.enterOnStart&&g.status==='open'&&g.runtime==='serving'){this.enterOnStart=false;this.render(g,force);await this.enter();this.hide();return}this.render(g,force)}
    catch(e){const error=node('room-error');if(error)error.textContent=e instanceof Error?e.message:'Connection interrupted. Retry or return to your library.'}
  }
  private async run(action:()=>Promise<unknown>){
    if(this.busy)return;this.busy=true;const error=node('room-error');if(error)error.textContent='';const buttons=[...this.root.querySelectorAll<HTMLButtonElement>('button')].map(button=>({button,disabled:button.disabled}));buttons.forEach(({button})=>button.disabled=true);
    try{await action();this.signature=''}catch(e){if(error)error.textContent=e instanceof Error?e.message:'This action could not be completed.'}
    finally{this.busy=false;buttons.forEach(({button,disabled})=>{if(button.isConnected)button.disabled=disabled});this.signature='';await this.refresh(true)}
  }
  private confirm(message:string,label:string,action:()=>Promise<unknown>){
    const revision=this.api.room?.revision;const box=node('room-confirm');box.hidden=false;box.replaceChildren();const p=document.createElement('p');p.textContent=message;const yes=document.createElement('button');yes.textContent=label;yes.className='danger';const cancel=document.createElement('button');cancel.textContent='Cancel';yes.onclick=()=>{box.hidden=true;void this.run(async()=>{if(this.api.room?.revision!==revision)throw new Error('The game changed. Review the latest details and confirm again.');return action()})};cancel.onclick=()=>{box.hidden=true};box.append(p,yes,cancel);yes.focus();
  }
  private feedback(message:string){const live=node('room-feedback');if(live&&!this.root.hidden)live.textContent=message;else this.notify(message)}
  private copy(input:HTMLInputElement|HTMLTextAreaElement){input.select();if(navigator.clipboard)void navigator.clipboard.writeText(input.value).then(()=>this.feedback('Copied.')).catch(()=>this.feedback('Text selected. Press ⌘C or Ctrl+C to copy.'));else this.feedback('Text selected. Press ⌘C or Ctrl+C to copy.')}
  private preserveRoom() {
    const key=(element:HTMLElement)=>element.id ? '#'+CSS.escape(element.id) : element.closest('form')?.hasAttribute('data-edit-seat') ? `form[data-edit-seat="${element.closest('form')!.getAttribute('data-edit-seat')}"] ${element.getAttribute('name') ? `[name="${CSS.escape(element.getAttribute('name')!)}"]` : element.tagName.toLowerCase()}` : element.closest('form')?.id && element.getAttribute('name') ? `#${CSS.escape(element.closest('form')!.id)} [name="${CSS.escape(element.getAttribute('name')!)}"]` : ['data-invite','data-copy-invite','data-revoke'].map(attr=>element.hasAttribute(attr)?`[${attr}="${CSS.escape(element.getAttribute(attr)!)}"]`:'').find(Boolean) ?? '';
    const detailKey=(d:HTMLDetailsElement)=>(d.closest<HTMLElement>('[data-seat]')?.dataset.seat??'')+':'+d.querySelector('summary')?.textContent;
    const opened=new Set([...this.root.querySelectorAll<HTMLDetailsElement>('details[open]')].map(detailKey));
    const drafts=[...this.root.querySelectorAll<HTMLInputElement|HTMLSelectElement>('input:not([readonly]),select')].filter(e=>e.closest('details')?.open).map(e=>({key:key(e),value:e.value}));
    const active=this.root.contains(document.activeElement)?document.activeElement as HTMLElement:null,focus=active?key(active):'';
    const summary=active?.tagName==='SUMMARY'?detailKey(active.parentElement as HTMLDetailsElement):'';
    const confirmation=node('room-confirm'),feedback=node('room-feedback')?.textContent??'';
    const pending=confirmation&&!confirmation.hidden?confirmation:null;
    return ()=>{
      this.root.querySelectorAll<HTMLDetailsElement>('details').forEach(d=>{if(opened.has(detailKey(d)))d.open=true;if(summary===detailKey(d))d.querySelector<HTMLElement>('summary')?.focus({preventScroll:true})});
      for(const draft of drafts){if(draft.key){const e=this.root.querySelector<HTMLInputElement|HTMLSelectElement>(draft.key);if(e)e.value=draft.value}}
      if(pending)node('room-confirm')?.replaceWith(pending);
      if(focus)this.root.querySelector<HTMLElement>(focus)?.focus({preventScroll:true});
      if(node('room-feedback'))node('room-feedback').textContent=feedback;
    };
  }
  private render(g:GameInfo,force=false){
    const signature=JSON.stringify([g.revision,g.status,g.runtime,g.save_error,g.seats.map(s=>[s.connected,s.invite_id,s.invite_expires_at]),this.completionReceipts.get(g.game_id)]);
    if(!force&&(signature===this.signature||this.root.contains(document.activeElement)&&['INPUT','SELECT','TEXTAREA'].includes(document.activeElement!.tagName)))return;
    const priorError=node('room-error')?.textContent??'';const state=this.preserveRoom();this.signature=signature;
    for(const seat of g.seats){if(this.inviteLinks.get(seat.id)?.id!==seat.invite_id)this.inviteLinks.delete(seat.id)}
    const lobby=g.status==='lobby', serving=g.runtime==='serving', own=g.seats.find(s=>s.yours), rejoin=this.api.session?.rejoin_code??'';
    this.root.innerHTML=`<div class="room-heading"><div><span class="room-status">${escape(statusName(g))}</span><h2>${escape(g.name)}</h2></div><button id="room-leave" class="text-button">Leave game</button></div><p class="room-world">${escape(worldDescription(this.catalog.worlds,g.config.world))} · ${escape(this.catalog.difficulties.find(d=>d.id===g.config.difficulty)?.name??g.config.difficulty)} · ${g.seats.length} kingdoms</p><p id="room-error" class="form-error" role="alert">${escape(priorError)}</p><p id="room-feedback" class="bonus" role="status" aria-live="polite"></p><div id="room-confirm" class="room-confirm" hidden></div><div class="seat-roster" aria-label="Kingdom seats">${g.seats.map(s=>this.seatRow(s,g)).join('')}</div><div id="room-primary" class="room-primary"></div><p id="room-start-reason" class="bonus" role="status">${escape(g.start_reason)}</p>${lobby&&g.owner&&g.seats.length<6?'<button id="room-add-seat" class="text-button">+ Add a friend seat</button>':''}<div id="room-world-edit"></div><details class="recovery-code"><summary>Your private rejoin code</summary><p>Keep this private. It restores your kingdom on another browser or server.</p><label>Rejoin code<input id="own-rejoin-code" readonly value="${escape(rejoin)}" placeholder="Use your previously saved code"></label>${rejoin?'<button id="copy-rejoin" class="secondary">Copy my rejoin code</button>':'<p>This browser does not have the original code. Use the code you saved when joining.</p>'}</details><details class="game-identification"><summary>Game address &amp; session ID</summary><label>Game address<input readonly id="room-address" value="${escape(location.origin+'/game#game='+g.game_id)}"></label><button class="secondary" id="copy-address">Copy game address</button><p>Existing members can use this address. New players need their own invitation.</p><label>Session ID<input readonly value="${escape(g.game_id)}"></label></details>${g.owner?this.ownerControls(g):''}<details><summary>Game activity</summary><ol class="room-audit" id="room-audit"></ol></details>`;
    node('room-leave').onclick=()=>void this.run(()=>this.leave());
    this.root.querySelectorAll<HTMLInputElement>('input[readonly]').forEach(i=>i.onclick=()=>i.select());
    if(rejoin)node('copy-rejoin').onclick=()=>this.copy(node('own-rejoin-code'));
    node('copy-address').onclick=()=>this.copy(node('room-address'));
    const primary=node('room-primary');
    const action=(id:string,label:string,fn:()=>Promise<unknown>,disabled=false)=>{const b=document.createElement('button');b.id=id;b.textContent=label;b.className='primary';b.disabled=disabled;b.onclick=()=>void this.run(fn);primary.append(b)};
    if(lobby&&own){action('room-ready',own.ready?'Not ready':'Ready',async()=>{await this.api.mutate(`/seats/${own.id}/ready`,'PUT',{ready:!own.ready})});if(g.owner)action('room-start','Start game',async()=>{await this.api.control('start');this.enterOnStart=false;await this.enter();this.hide()},!g.can_start)}
    if(g.status==='open'&&serving)action('room-return','Return to battlefield',async()=>{await this.enter();this.hide()});
    if(g.status==='closed'&&g.owner)action('room-reopen','Reopen game',async()=>{await this.api.control('reopen');await this.api.ensureConnection()});
    if(g.status==='closing'&&g.owner){action('room-retry-close','Retry close',()=>this.api.control('close'));action('room-cancel-close','Cancel close',()=>this.api.control('cancel-close'))}
    node('room-add-seat')?.addEventListener('click',()=>void this.run(()=>this.api.mutate('/seats','POST',{name:'Friend '+g.seats.length,civilization:'britons',controller:'human'})));
    for(const seat of g.seats)this.bindSeat(seat,g);
    if(lobby&&g.owner)this.worldEditor(g);
    if(g.owner)this.bindOwner(g);
    state();
    void this.api.request<AuditPage>(`${this.api.path()}/audit`).then(page=>{const list=node('room-audit');if(list&&!this.root.hidden&&this.api.session?.match_id===g.game_id)list.replaceChildren(...page.events.slice(-20).reverse().map(e=>{const li=document.createElement('li');li.textContent=`${new Date(e.at).toLocaleTimeString()} · ${e.actor_name}: ${e.message}`;return li}))}).catch(()=>{});
  }
  private seatRow(s:SeatView,g:GameInfo){
    const editable=g.status==='lobby'&&(g.owner||s.yours);
    const state=s.controller==='ai'?'AI kingdom':s.status==='claimed'?`${s.connected?'Connected':'Away'} · ${s.ready?'Ready':'Not ready'}`:s.invite_id?'Invitation pending':'Waiting for a player';
    return `<section class="seat-row" data-seat="${s.id}" aria-label="${escape(s.name)}"><span class="seat-banner" data-player="${s.player_id}">♜</span><div class="seat-identity"><strong>${escape(s.name)}${s.yours?' · You':''}${s.owner?' · Owner':''}</strong><span>${escape(this.catalog.civilizations.find(c=>c.id===s.civilization)?.name??s.civilization)} · ${state}</span>${editable?`<details><summary>Edit kingdom</summary><form data-edit-seat="${s.id}"><label>Kingdom name<input name="name" value="${escape(s.name)}" maxlength="40" required></label><label>Civilization<select name="civilization">${this.catalog.civilizations.map(c=>`<option value="${c.id}" ${c.id===s.civilization?'selected':''}>${escape(c.name)}</option>`).join('')}</select></label><label>Player<select name="controller" ${s.status==='claimed'?'disabled':''}><option value="human" ${s.controller==='human'?'selected':''}>Human</option><option value="ai" ${s.controller==='ai'?'selected':''}>AI</option></select></label><button class="secondary">Save kingdom</button></form></details>`:''}</div><div class="seat-actions">${g.owner&&s.controller==='human'&&!s.owner&&g.runtime==='serving'&&['lobby','open'].includes(g.status)?`<button data-invite="${s.id}">${s.status==='claimed'?'Replace access':s.invite_id?'New invitation':'Invite player'}</button>${s.invite_id?`<button class="text-button" data-revoke="${s.invite_id}">Revoke invite</button>`:''}`:''}</div>${this.inviteLinks.has(s.id)?`<div class="seat-invite"><label>Invitation for ${escape(s.name)}<input id="invite-${s.id}" readonly value="${escape(this.inviteLinks.get(s.id)!.url)}"></label><button data-copy-invite="${s.id}">Copy invite link</button><span>One use · expires after 24 hours</span></div>`:''}</section>`;
  }
  private bindSeat(s:SeatView,g:GameInfo){
    const form=this.root.querySelector<HTMLFormElement>(`[data-edit-seat="${s.id}"]`);
    if(form)form.onsubmit=e=>{e.preventDefault();const f=new FormData(form);void this.run(()=>this.api.mutate(`/seats/${s.id}`,'PATCH',{name:String(f.get('name')),civilization:String(f.get('civilization')),controller:s.status==='claimed'?s.controller:String(f.get('controller'))}))};
    const invite=this.root.querySelector<HTMLButtonElement>(`[data-invite="${s.id}"]`);
    if(invite)invite.onclick=()=>{const issue=async()=>{const i=await this.api.mutate<Invitation>(`/seats/${s.id}/invites`,'POST',{replace:s.status==='claimed'});this.inviteLinks.set(s.id,{id:i.id,url:`${location.origin}/join#invite=${i.secret}`})};if(s.status==='claimed')this.confirm(`Replace access for ${s.name}? Their current credentials will stop working. Their kingdom will stay intact and the game will pause.`,'Replace player access',issue);else void this.run(issue)};
    this.root.querySelector<HTMLButtonElement>(`[data-revoke="${s.invite_id}"]`)?.addEventListener('click',()=>void this.run(async()=>{await this.api.mutate(`/invites/${s.invite_id}`,'DELETE',{});this.inviteLinks.delete(s.id)}));
    this.root.querySelector<HTMLButtonElement>(`[data-copy-invite="${s.id}"]`)?.addEventListener('click',()=>this.copy(node(`invite-${s.id}`)));

  }
  private ownerControls(g:GameInfo){return `<details class="portable-game"><summary>Move or copy this game</summary><p>Download a complete SQLite save with every kingdom, event and recovery credential. Keep it private. Place it in the destination server’s game directory and restart there; players use their rejoin codes.</p><button id="download-database" class="secondary">Download SQLite save</button><p>Move freezes this host. Import the archive on another server, then share its address with your friends. Each player uses their own rejoin code.</p><label>Archive passphrase (optional)<input id="archive-passphrase" type="password" minlength="8" maxlength="256" autocomplete="new-password" placeholder="Protect this private game archive"></label><div class="room-actions">${g.runtime==='serving'&&['lobby','open'].includes(g.status)?'<button id="move-game">Move game</button><button id="copy-game">Copy as new game</button>':''}${g.transfer_id?'<button id="download-archive">Download archive</button>':''}</div>${g.runtime==='frozen'?'<p>The source is frozen. Keep it frozen after importing elsewhere.</p><label>Destination completion receipt<input id="transfer-receipt" autocomplete="off" spellcheck="false"></label><button id="complete-transfer" class="secondary">Confirm completed move</button><button id="cancel-transfer" class="text-button">Cancel move on this host</button>':''}${this.completionReceipts.get(g.game_id)?`<label>Completion receipt for the old host<input readonly id="import-receipt" value="${escape(this.completionReceipts.get(g.game_id)!)}"></label><button id="copy-receipt" class="secondary">Copy completion receipt</button>`:''}</details><div class="room-owner-actions">${['lobby','open'].includes(g.status)&&g.runtime==='serving'?'<button id="close-game">Close game for everyone</button>':''}<button id="delete-game" class="text-button danger">Delete game</button></div><p class="room-policy">Any player can pause. The owner resumes and changes speed. A player who joins and then disconnects pauses the game after 15 seconds; empty seats do not pause play, and when everyone leaves, the game saves and rests.</p>`}
  private bindOwner(g:GameInfo){
    node('close-game')?.addEventListener('click',()=>this.confirm(`Close “${g.name}” for everyone? Its kingdoms, saves and history will be kept. You can reopen it later.`,'Close and save',()=>this.api.control('close')));
    node('delete-game')?.addEventListener('click',()=>this.confirm(`Permanently delete “${g.name}”? This removes all local saves, kingdoms, invitations and history. Exported archives and external backups are not deleted.`,'Delete permanently',async()=>{await this.api.deleteGame();this.hide();await this.leave()}));
    for(const kind of ['move','copy'])node(`${kind}-game`)?.addEventListener('click',()=>{const run=async()=>{const pass=node<HTMLInputElement>('archive-passphrase').value;if(pass&&pass.length<8)throw new Error('Use at least 8 characters for the archive passphrase.');const transfer=await this.api.transfer(kind);await this.download(transfer.id,pass)};if(kind==='move')this.confirm('Freeze this game and prepare its archive for another host? Your friends will reconnect at the new address using their own rejoin codes.','Freeze and download',run);else void this.run(run)});
    node('download-database')?.addEventListener('click',()=>void this.run(async()=>{const blob=await this.api.downloadDatabase(),url=URL.createObjectURL(blob),link=document.createElement('a');link.href=url;link.download=`${g.game_id}.sqlite`;link.click();setTimeout(()=>URL.revokeObjectURL(url),1000);this.feedback('SQLite save downloaded. Keep it private with your rejoin code.')}));
    node('download-archive')?.addEventListener('click',()=>void this.run(()=>this.download(g.transfer_id!,node<HTMLInputElement>('archive-passphrase').value)));
    node('cancel-transfer')?.addEventListener('click',()=>this.confirm('Only cancel if this game is not running on a destination host. Offline servers cannot detect another active copy.','Cancel move; keep paused',()=>this.api.control('cancel-transfer',{confirm:true})));
    node('complete-transfer')?.addEventListener('click',()=>void this.run(()=>this.api.mutate('/transfers/complete','POST',{receipt:node<HTMLInputElement>('transfer-receipt').value.trim()})));
    node('copy-receipt')?.addEventListener('click',()=>this.copy(node('import-receipt')));
  }
  private async download(id:string,passphrase:string){const blob=await this.api.downloadArchive(id,passphrase),url=URL.createObjectURL(blob),link=document.createElement('a');link.href=url;link.download=`${this.api.session!.match_id}.aoegame`;link.click();setTimeout(()=>URL.revokeObjectURL(url),1000);this.feedback('Game archive downloaded. Keep it and your rejoin code private.')}
  private worldEditor(g:GameInfo){
    const c=this.catalog,w=g.config.world??c.worlds.defaults;
    const select=(name:string,label:string,values:{id:string;name:string}[],value:string)=>`<label>${label}<select name="${name}">${values.map(v=>`<option value="${v.id}" ${value===v.id?'selected':''}>${escape(v.name)}</option>`).join('')}</select></label>`;
    node('room-world-edit').innerHTML=`<details><summary>Edit world settings</summary><form id="lobby-world-form"><div class="advanced-grid">${select('difficulty','Difficulty',c.difficulties,g.config.difficulty)}${select('type','World type',c.worlds.types,w.type??'')}${select('biome','Biome',c.worlds.biomes,w.biome??'')}${select('size','World size',c.worlds.sizes,w.size??'')}${select('resources','Natural resources',c.worlds.resources,w.resources??'')}${select('separation','Starting separation',c.worlds.separations,w.separation??'')}${select('reveal','Map reveal',c.worlds.reveals,w.reveal??'')}${select('treaty_minutes','Initial peace period',c.worlds.treaty_minutes.map(n=>({id:String(n),name:`${n} minutes`})),String(w.treaty_minutes??0))}<label>Map seed<input name="seed" type="number" min="1" max="999999999" value="${g.config.seed}" required></label></div><p>Changing world settings clears everyone's readiness.</p><button class="secondary">Save world settings</button></form></details>`;
    node<HTMLFormElement>('lobby-world-form').onsubmit=e=>{e.preventDefault();const f=new FormData(node<HTMLFormElement>('lobby-world-form'));const world:WorldOptions={...w};for(const k of ['type','biome','size','resources','separation','reveal'] as const)world[k]=String(f.get(k));world.treaty_minutes=Number(f.get('treaty_minutes'));const config:Config={...g.config,world,seed:Number(f.get('seed')),difficulty:String(f.get('difficulty'))};void this.run(()=>this.api.mutate('/rules','PATCH',{config}))};
  }
  async handleLocation(){
    const fragment=new URLSearchParams(location.hash.slice(1));this.inviteSecret=fragment.get('invite')??(location.pathname==='/join'?sessionStorage.getItem('aoe.pending-invite')??'':'');const game=fragment.get('game');
    if(this.inviteSecret){
      sessionStorage.setItem('aoe.pending-invite',this.inviteSecret);history.replaceState(null,'','/join');this.expose('join-panel');node('join-panel').innerHTML='<h2>Joining a kingdom</h2><p role="status">Checking your invitation…</p>';
      try{const i=await this.api.inspectInvite(this.inviteSecret);node('join-panel').innerHTML=`<span class="room-status">Private invitation</span><h2>${escape(i.game_name)}</h2><p>Your invitation reserves ${escape(i.name)}. Opening this link does not claim the seat.</p><form id="claim-invite-form"><label>Your kingdom name<input id="claim-name" maxlength="40" value="${escape(i.name)}" required autocomplete="nickname"></label><button class="primary">Join this game</button></form><p id="join-error" class="form-error" role="alert"></p><button class="secondary" id="join-back">Back to games</button>`;
        node<HTMLFormElement>('claim-invite-form').onsubmit=e=>{e.preventDefault();const b=node('claim-invite-form').querySelector('button')!;b.disabled=true;void this.api.claim(this.inviteSecret,node<HTMLInputElement>('claim-name').value).then(()=>{this.inviteSecret='';sessionStorage.removeItem('aoe.pending-invite');return this.show()}).catch(e=>{node('join-error').textContent=e.message;b.disabled=false})};
      }catch(e){node('join-panel').innerHTML=`<h2>Invitation unavailable</h2><p>${escape(e instanceof Error?e.message:'Ask the owner for another invitation.')}</p><button class="secondary" id="join-back">Back to games</button>`}
      node('join-back')?.addEventListener('click',()=>{sessionStorage.removeItem('aoe.pending-invite');this.hide();void this.leave()});return true;
    }
    if(game){history.replaceState(null,'','/game');try{await this.api.useGame(game);await this.continueSession()}catch(e){this.notify(e instanceof Error?e.message:'Use your private rejoin code to open this game.');this.hide();await this.leave()}return true}
    return false;
  }
}
