import type { Command, Snapshot } from './api.generated';
import type { GameAPI } from './api';

const escape = (s:string) => s.replace(/[&<>"']/g,c=>({'&':'&amp;','<':'&lt;','>':'&gt;','"':'&quot;',"'":'&#39;'}[c]!));

export class DiplomacyUI {
  private dialog=document.createElement('dialog');
  private snapshot?:Snapshot;
  private connected=false;
  private busy=false;
  private session='';
  constructor(private api:GameAPI) {
    this.dialog.id='diplomacy-dialog';this.dialog.setAttribute('aria-labelledby','diplomacy-title');
    this.dialog.innerHTML=`<header class="market-heading"><div><div class="eyebrow">KINGDOM RELATIONSHIPS</div><h2 id="diplomacy-title">Reparations & peace</h2></div><button id="diplomacy-close" aria-label="Close diplomacy">×</button></header><p>Offer gold to repair the damage of war. Accepted reparations restore peace immediately and recall mutual attacks. AI kingdoms honor peace for five game minutes unless attacked again.</p><form id="peace-form"><fieldset id="peace-fields"><legend>Offer a damage payment</legend><label>Kingdom<select id="peace-kingdom" required></select></label><label>Gold payment<input id="peace-gold" type="number" min="1" max="10000000" step="1" required value="50"></label><p id="peace-price" class="market-note"></p><button class="primary" type="submit">Offer reparations</button></fieldset></form><p class="market-note">Gold is reserved when offered. Declined, withdrawn or expired offers return it. Offers expire after two game minutes. Human kingdoms choose whether to accept.</p><p id="peace-status" role="status"></p><p id="peace-error" class="form-error" role="alert"></p><h3>Your peace offers</h3><div id="peace-offers"></div>`;
    document.getElementById('app')!.append(this.dialog);
    this.get('diplomacy-close').onclick=()=>this.dialog.close();
    this.get<HTMLFormElement>('peace-form').onsubmit=e=>{e.preventDefault();void this.issue({kind:'peace_offer',target_player:Number(this.get<HTMLSelectElement>('peace-kingdom').value),value:Number(this.get<HTMLInputElement>('peace-gold').value)});};
    this.get('peace-kingdom').onchange=()=>{this.suggest();this.render();};
    this.dialog.addEventListener('click',e=>{const b=(e.target as HTMLElement).closest<HTMLButtonElement>('[data-peace-action]');if(b&&!b.disabled)void this.issue({kind:b.dataset.peaceAction!,offer_id:Number(b.dataset.id)});});
  }
  private get<T extends HTMLElement=HTMLElement>(id:string){return this.dialog.querySelector<T>(`#${id}`)!;}
  update(snapshot:Snapshot,connected:boolean){
    const session=this.api.session?.match_id??'';
    if(this.session!==session){this.session=session;this.get('peace-error').textContent='';this.get('peace-status').textContent='';this.get<HTMLSelectElement>('peace-kingdom').dataset.signature='';}
    this.snapshot=snapshot;this.connected=connected;if(this.dialog.open)this.render();
  }
  open(id?:number){this.render();if(id)this.get<HTMLSelectElement>('peace-kingdom').value=String(id);this.suggest();this.render();if(!this.dialog.open)this.dialog.showModal();}
  private suggest(){const o=this.snapshot?.opponents.find(o=>o.id===Number(this.get<HTMLSelectElement>('peace-kingdom').value));this.get<HTMLInputElement>('peace-gold').value=String(o?.peace_price??50);}
  private disabled(){return this.busy||!this.connected||this.snapshot?.status!=='running'||!!this.snapshot?.player.defeated;}
  private render(){
    const s=this.snapshot;if(!s)return;
    const opponents=s.opponents.filter(o=>!o.defeated&&o.relation==='hostile'), select=this.get<HTMLSelectElement>('peace-kingdom');
    const sig=JSON.stringify(opponents.map(o=>[o.id,o.name]));
    if(select.dataset.signature!==sig){const old=select.value;select.replaceChildren(...opponents.map(o=>new Option(o.name,String(o.id))));select.dataset.signature=sig;if(opponents.some(o=>String(o.id)===old))select.value=old;}
    this.get<HTMLFieldSetElement>('peace-fields').disabled=this.disabled()||!opponents.length;
    const o=opponents.find(o=>o.id===Number(select.value));
    this.get('peace-price').textContent=o?`AI acceptance threshold: ${o.peace_price??50} gold, based on damage inflicted. You have ${Math.floor(s.player.resources.gold)} gold available.`:'You are at peace with all standing kingdoms.';
    const name=(id:number)=>id===s.player.id?'You':s.opponents.find(o=>o.id===id)?.name??'Kingdom';
    const html=[...(s.peace_offers??[])].reverse().map(o=>`<article class="peace-offer" aria-label="Peace offer ${o.id}"><strong>${escape(name(o.from))} → ${escape(name(o.to))} · ${o.gold.toLocaleString()} gold</strong><p>${escape(o.state)}${o.state==='offered'?` · ${Math.ceil(o.expires_in)} game seconds remaining`:''}</p><div class="market-row-actions">${[[o.can_accept,'peace_accept','Accept peace'],[o.can_decline,'peace_decline','Decline'],[o.can_withdraw,'peace_withdraw','Withdraw']].map(([available,kind,title])=>available?`<button data-peace-action="${kind}" data-id="${o.id}" ${this.disabled()?'disabled':''}>${title}</button>`:'').join('')}</div></article>`).join('')||'<p class="market-empty">No peace offers yet.</p>';
    const list=this.get('peace-offers');if(list.dataset.html!==html){const focused=list.contains(document.activeElement)?document.activeElement as HTMLElement:null,action=focused?.dataset.peaceAction,id=focused?.dataset.id;list.innerHTML=html;list.dataset.html=html;if(action)list.querySelector<HTMLButtonElement>(`[data-peace-action="${action}"][data-id="${id}"]`)?.focus({preventScroll:true});}
  }
  private async issue(command:Omit<Command,'id'>){
    if(this.disabled())return;this.busy=true;const session=this.session;this.get('peace-error').textContent='';this.get('peace-status').textContent='Sending proposal…';this.render();
    try {await this.api.command(command);if(session!==this.session)return;this.snapshot=await this.api.snapshot();this.get('peace-status').textContent='Decision recorded.';}
    catch(e){if(session===this.session){this.get('peace-status').textContent='';this.get('peace-error').textContent=e instanceof Error?e.message:'Unable to submit this decision. Try again.';}}
    finally {this.busy=false;this.render();}
  }
}
