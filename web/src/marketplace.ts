/*
THESIS: A live trading ledger with explicit quantities, partners and delivery.
OWN-WORLD: Existing olive console, brass headings, compact rectangular controls.
STORY: Read offers, reserve a surplus, send a caravan, follow its delivery.
FIRST VIEWPORT: Published offers beside an offer form; delivery and merchant
quotes live in adjacent tabs. The map remains visible around the ledger.
FORM: An extension of the established RTS console, with protected form focus.
*/
import type { Catalog, Command, Snapshot, TradeOfferView, Vec, Resources } from './api.generated';
import type { GameAPI } from './api';

const escape = (s: string) => s.replace(/[&<>"']/g, c => ({'&':'&amp;','<':'&lt;','>':'&gt;','"':'&quot;',"'":'&#39;'}[c]!));
const quantity = (r: Resources) => Object.entries(r).filter(([,n])=>n>0).map(([k,n])=>`${Math.round(n).toLocaleString()} ${k}`).join(' · ') || 'None';
const resources = ['food','wood','gold','stone'];
const states: Record<string,string> = {outbound:'Payment outbound',returning:'Goods returning',returning_payment:'Payment returning',delivered:'Delivered',recalled:'Recalled',lost:'Caravan lost'};

export class MarketplaceUI {
  private dialog = document.createElement('dialog');
  private snapshot?: Snapshot;
  private catalog?: Catalog;
  private connected = false;
  private busy = false;
  private tab = 'offers';
  private session = '';
  constructor(private api: GameAPI, private locate: (position: Vec, id?: number) => void) {
    this.dialog.id = 'marketplace-dialog';
    this.dialog.setAttribute('aria-labelledby','marketplace-title');
    this.dialog.innerHTML = `<header class="market-heading"><div><h2 id="marketplace-title">Marketplace</h2><p>Trade local surpluses. Keep your caravans safe.</p></div><button type="button" id="market-close" aria-label="Close marketplace">×</button></header>
      <nav class="market-tabs" aria-label="Marketplace pages">${[['offers','Offers'],['caravans','Caravans'],['merchants','Merchants'],['regions','Resources']].map(([id,label])=>`<button type="button" data-market-tab="${id}" aria-pressed="${id==='offers'}">${label}</button>`).join('')}</nav>
      <p id="market-status" role="status" aria-live="polite"></p><p id="market-error" role="alert"></p>
      <section data-market-page="offers" class="market-offers-layout"><div class="market-book"><div class="market-section-heading"><h3>Standing offers</h3><label>Show<select id="market-filter"><option value="all">All resources</option>${resources.map(r=>`<option value="${r}">${r}</option>`).join('')}</select></label></div><p class="market-note">One lot per trip. Scout the offering Market, then send an idle Trade Cart from your own Market.</p><label class="market-repeat"><input type="checkbox" id="market-repeat"> Repeat trips while the offer and your funds last</label><div id="market-offers"></div><p id="market-reserved" class="market-note"></p></div>
      <form id="market-offer-form"><h3>Post an offer</h3><p class="market-note">Offer goods for gold, buy with gold, or barter directly. Unclaimed lots stay listed until cancelled.</p><fieldset id="market-offer-fields"><legend class="sr-only">Offer terms</legend><label>Trading Market<select id="offer-market" required></select></label><div class="market-form-pair"><label>You offer<select id="offer-give-resource">${resources.map(r=>`<option value="${r}" ${r==='wood'?'selected':''}>${r}</option>`).join('')}</select></label><label>Per lot<input id="offer-give-amount" aria-label="Offered amount per lot" type="number" min="1" value="100" required></label></div><div class="market-form-pair"><label>You request<select id="offer-want-resource">${resources.map(r=>`<option value="${r}" ${r==='gold'?'selected':''}>${r}</option>`).join('')}</select></label><label>Per lot<input id="offer-want-amount" aria-label="Requested amount per lot" type="number" min="1" value="80" required></label></div><div class="market-form-pair"><label>Lots<input id="offer-lots" type="number" min="1" value="3" required></label><label>Audience<select id="offer-audience"><option value="0">All kingdoms</option></select></label></div><p class="market-note">Posting reserves your offered goods. Cancelling returns unclaimed lots; accepted caravans continue.</p><button class="primary" id="post-offer" type="submit">Post offer</button></fieldset><p id="market-requirement" class="market-note"></p></form></section>
      <section data-market-page="caravans" hidden><h3>Your trade deliveries</h3><p class="market-note">Payment reaches the seller at pickup. Goods reach you when the cart returns. Stop or move a cart to interrupt it, then resume here. Recall is available before pickup. Lost carts lose their cargo.</p><div id="market-caravans"></div></section>
      <section data-market-page="merchants" hidden><h3>Merchant exchange</h3><p class="market-note">Shared, finite stock. Buying raises prices; selling lowers them. Players replenish supplies by trading. Each button shows the current cost and return.</p><div id="market-merchants"></div></section>
      <section data-market-page="regions" hidden><h3>Regional resources</h3><p class="market-note">New worlds give countryside deposits the strengths below. Mixed Regions combines these landscapes. Every kingdom keeps viable starting patches and the same opening stockpile. Fishing and farms follow their own rules; existing saved deposits keep their amounts.</p><div id="market-regions"></div></section>`;
    document.getElementById('app')!.append(this.dialog);
    this.get('market-close').onclick = () => this.dialog.close();
    this.dialog.querySelectorAll<HTMLButtonElement>('[data-market-tab]').forEach(button=>button.onclick=()=>{
      this.tab=button.dataset.marketTab!;
      this.dialog.querySelectorAll<HTMLElement>('[data-market-page]').forEach(page=>page.hidden=page.dataset.marketPage!==this.tab);
      this.dialog.querySelectorAll('[data-market-tab]').forEach(b=>b.setAttribute('aria-pressed',String((b as HTMLElement).dataset.marketTab===this.tab)));
      this.render();
    });
    this.get('market-filter').onchange=()=>this.render();
    this.get<HTMLFormElement>('market-offer-form').onsubmit=e=>{e.preventDefault();void this.issue({kind:'market_post',entity_ids:[Number(this.value('offer-market'))],offer:{give_resource:this.value('offer-give-resource'),give_amount:Number(this.value('offer-give-amount')),want_resource:this.value('offer-want-resource'),want_amount:Number(this.value('offer-want-amount')),lots:Number(this.value('offer-lots')),target_player:Number(this.value('offer-audience'))}},'Offer posted. Goods reserved.');};
    this.dialog.addEventListener('click',e=>{
      const button=(e.target as HTMLElement).closest<HTMLButtonElement>('[data-market-action]');
      if(!button || button.disabled) return;
      const id=Number(button.dataset.id), action=button.dataset.marketAction;
      const v=this.snapshot?.marketplace;
      if(!v) return;
      if(action==='accept') {
        const offer=v.offers.find(o=>o.id===id);
        if(offer?.can_accept && offer.cart_id) void this.issue({kind:'market_accept',offer_id:id,entity_ids:[offer.cart_id],repeat:this.get<HTMLInputElement>('market-repeat').checked},'Caravan dispatched.');
      } else if(action==='cancel') void this.issue({kind:'market_cancel',offer_id:id},'Offer cancelled. Unclaimed goods returned.');
      else if(action==='resume' || action==='recall') void this.issue({kind:action==='resume'?'market_resume':'market_recall',shipment_id:id},action==='resume'?'Caravan resumed.':'Caravan recalled. Payment returns with the cart.');
      else if(action==='merchant') {
        const quote=v.merchants.actions[id];
        if(quote?.enabled && v.merchants.market_id) void this.issue({kind:quote.kind,product:quote.product,entity_ids:[v.merchants.market_id],market_revision:v.merchants.revision},quote.description);
      } else if(action==='locate-offer' || action==='locate-cart') {
        const item=action==='locate-offer'?v.offers.find(o=>o.id===id):v.shipments.find(s=>s.id===id);
        if(item?.position) { this.dialog.close();this.locate(item.position,action==='locate-cart'?('cart_id' in item?item.cart_id:undefined):('market_id' in item?item.market_id:undefined)); }
      }
    });
  }
  private get<T extends HTMLElement=HTMLElement>(id:string) { return this.dialog.querySelector<T>(`#${id}`)!; }
  private value(id:string) { return this.get<HTMLInputElement|HTMLSelectElement>(id).value; }
  open() {
    if(!this.snapshot) return;
    this.render();
    if(!this.dialog.open) this.dialog.showModal();
  }
  update(snapshot:Snapshot,catalog:Catalog,connected:boolean) {
    const session=this.api.session?.match_id ?? '';
    if(session!==this.session) {this.session=session;this.get('market-error').textContent='';this.get('market-status').textContent='';}
    this.snapshot=snapshot;this.catalog=catalog;this.connected=connected;
    if(this.dialog.open) this.render();
  }
  private disabled() { return this.busy || !this.connected || this.snapshot?.status!=='running' || !!this.snapshot?.player.defeated; }
  private options(id:string,options:HTMLOptionElement[]) {
    const select=this.get<HTMLSelectElement>(id),value=select.value;
    const signature=JSON.stringify(options.map(o=>[o.value,o.text]));
    if(select.dataset.signature===signature) return;
    select.dataset.signature=signature;select.replaceChildren(...options);
    if(options.some(o=>o.value===value)) select.value=value;
  }
  private html(id:string,html:string) {
    const target=this.get(id);
    if(target.dataset.html===html) return;
    const focus=target.contains(document.activeElement)?document.activeElement as HTMLElement:null;
    const action=focus?.dataset.marketAction,key=focus?.dataset.id;
    target.innerHTML=html;target.dataset.html=html;
    if(action && key) target.querySelector<HTMLButtonElement>(`[data-market-action="${action}"][data-id="${key}"]`)?.focus({preventScroll:true});
  }
  private kingdom(id:number) { return id===this.snapshot?.player.id?'Your kingdom':this.snapshot?.opponents.find(o=>o.id===id)?.name ?? 'Kingdom'; }
  private offer(o:TradeOfferView) {
    const own=o.owner===this.snapshot!.player.id, t=o.terms, closed=o.state!=='open';
    const button=own?`<button type="button" data-market-action="cancel" data-id="${o.id}" ${closed || this.disabled()?'disabled':''}>Cancel offer</button>`:`<button type="button" data-market-action="accept" data-id="${o.id}" ${!o.can_accept || this.disabled()?'disabled':''}>Send caravan</button>`;
    return `<article class="market-offer" aria-label="Offer ${o.id}"><div class="market-offer-title"><strong>${escape(this.kingdom(o.owner))}</strong><span>#${o.id}${t.target_player?' · Private':''}</span></div><p class="market-exchange"><span>${t.give_amount} ${escape(t.give_resource)}</span><span aria-label="in exchange for"> ↔ </span><span>${t.want_amount} ${escape(t.want_resource)}</span></p><p class="market-note">${closed?escape(o.state):`${o.remaining} lot${o.remaining===1?'':'s'} available`}${!own?' · You receive '+t.give_amount+' '+escape(t.give_resource):''}</p><div class="market-row-actions">${button}${o.position?`<button type="button" data-market-action="locate-offer" data-id="${o.id}">Locate Market</button>`:''}</div>${!own && !o.can_accept?`<p class="market-refusal">${escape(o.reason || 'Unavailable')}</p>`:''}</article>`;
  }
  private render() {
    const s=this.snapshot,c=this.catalog;
    if(!s || !c) return;
    const v=s.marketplace;
    const markets=s.entities.filter(e=>e.owner===s.player.id && e.type==='market' && e.progress===1);
    this.options('offer-market',markets.length?markets.map(m=>new Option(`Market #${m.id}`,String(m.id))):[new Option('Build a Market first','')]);
    this.options('offer-audience',[new Option('All kingdoms','0'),...s.opponents.filter(o=>!o.defeated).map(o=>new Option(o.name,String(o.id)))]);
    this.get<HTMLFieldSetElement>('market-offer-fields').disabled=this.disabled() || !markets.length;
    this.get<HTMLInputElement>('offer-give-amount').max=this.get<HTMLInputElement>('offer-want-amount').max=String(v.capacity);
    this.get<HTMLInputElement>('offer-lots').max=String(v.max_lots);
    this.get('market-requirement').textContent=!this.connected?'Waiting for your game connection.':s.status!=='running'?'Resume the game to trade.':!markets.length?'Build a Market in the Feudal Age to post offers and exchange resources.':`Up to ${v.max_offers} open offers; ${v.capacity} units of each resource per cart.`;
    this.get('post-offer').textContent=this.busy?'Sending…':'Post offer';
    if(this.tab==='offers') {
      const filter=this.value('market-filter');
      const offers=v.offers.filter(o=>(filter==='all' || o.terms.give_resource===filter || o.terms.want_resource===filter) && o.state==='open');
      this.html('market-offers',offers.length?offers.map(o=>this.offer(o)).join(''):`<p class="market-empty">${filter==='all'?'No standing offers yet. Post a surplus and let another kingdom bring what you need.':'No open offers for this resource.'}</p>`);
      this.get('market-reserved').textContent=`Your goods reserved for trade: ${quantity(v.reserved)}.`;
    } else if(this.tab==='caravans') {
      this.html('market-caravans',v.shipments.length?[...v.shipments].reverse().map(t=>`<article class="market-offer" aria-label="Caravan ${t.id}"><div class="market-offer-title"><strong>Caravan #${t.id}</strong><span>${escape(this.kingdom(t.seller))} → ${escape(this.kingdom(t.buyer))}</span></div><p class="market-exchange">${t.terms.give_amount} ${escape(t.terms.give_resource)} ↔ ${t.terms.want_amount} ${escape(t.terms.want_resource)}</p><p class="market-note">${escape(states[t.status]??t.status)}${t.repeat && !['delivered','lost','recalled'].includes(t.state)?' · Repeating while available':''}</p><div class="market-row-actions">${t.can_resume?`<button type="button" data-market-action="resume" data-id="${t.id}" ${this.disabled()?'disabled':''}>Resume caravan</button>`:''}${t.can_recall?`<button type="button" data-market-action="recall" data-id="${t.id}" ${this.disabled()?'disabled':''}>Recall caravan</button>`:''}${t.position?`<button type="button" data-market-action="locate-cart" data-id="${t.id}">Locate cart</button>`:''}</div></article>`).join(''):'<p class="market-empty">No deliveries yet. Accept an offer to send a caravan, or post an offer for another kingdom to collect.</p>');
    } else if(this.tab==='merchants') {
      this.html('market-merchants',`<p class="market-note">Merchant treasury: ${Math.floor(v.merchants.stock.gold).toLocaleString()} gold</p>${['food','wood','stone'].map(resource=>`<section class="merchant-resource"><div><h4>${resource}</h4><p>${Math.floor(v.merchants.stock[resource as keyof Resources]).toLocaleString()} in stock</p></div><div class="merchant-quotes">${v.merchants.actions.map((a,i)=>a.product===resource?`<div><button type="button" data-market-action="merchant" data-id="${i}" ${!a.enabled || this.disabled()?'disabled':''}><strong>${escape(a.label)}</strong><span>${escape(quantity(a.cost))} → ${escape(quantity(a.gain!))}</span></button>${!a.enabled?`<p class="market-refusal">${escape(a.reason??'Unavailable')}</p>`:''}</div>`:'').join('')}</div></section>`).join('')}`);
    } else if(this.tab==='regions') {
      this.html('market-regions',`<div class="market-table-scroll"><table><caption>Countryside deposit amounts relative to standard</caption><thead><tr><th scope="col">Biome</th>${resources.map(r=>`<th scope="col">${r}</th>`).join('')}</tr></thead><tbody>${c.worlds.economies.map(b=>`<tr><th scope="row">${escape(c.worlds.biomes.find(x=>x.id===b.biome)?.name??b.biome)}</th>${resources.map(r=>`<td>${Math.round(b.deposits[r as keyof Resources]*100)}%</td>`).join('')}</tr>`).join('')}</tbody></table></div><p class="market-note">World abundance also scales natural deposits. Trade agreements grant caravans passage through their peaceful partner's gates; they do not share vision or military access.</p>`);
    }
  }
  private async issue(command:Omit<Command,'id'>,message:string) {
    if(this.disabled()) return;
    const session=this.session;
    this.busy=true;this.get('market-error').textContent='';this.get('market-status').textContent='Sending order…';this.render();
    try {
      await this.api.command(command);
      if(session!==this.session) return;
      this.get('market-status').textContent=message;
      const snapshot=await this.api.snapshot();
      if(session===this.session) this.snapshot=snapshot;
    } catch(error) {
      if(session!==this.session) return;
      this.get('market-status').textContent='';this.get('market-error').textContent=error instanceof Error?error.message:'The order could not be completed. Try again.';
    } finally {this.busy=false;this.render();}
  }
}
