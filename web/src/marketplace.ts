/*
THESIS: A live trading ledger with explicit quantities, partners and delivery.
OWN-WORLD: Existing olive console, brass headings, compact rectangular controls.
STORY: Read offers, reserve a surplus, send a caravan, follow its delivery.
FIRST VIEWPORT: Published offers beside an offer form; delivery and merchant
quotes live in adjacent tabs. The map remains visible around the ledger.
FORM: An extension of the established RTS console, with protected form focus.
*/
import type { Catalog, Command, Snapshot, TradeOfferView, MerchantView, TradeShipmentView, Vec, Resources } from './api.generated';
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
      <section data-market-page="offers" class="market-offers-layout"><div class="market-book"><div class="market-section-heading"><h3>Standing offers</h3><label>Show<select id="market-filter"><option value="all">All resources</option>${resources.map(r=>`<option value="${r}">${r}</option>`).join('')}</select></label></div><p class="market-note">One lot per trip. Scout the offering trading post, then send an idle Trade Cart from your Market or Trade Ship from your Dock.</p><label class="market-repeat"><input type="checkbox" id="market-repeat"> Repeat trips while the offer and your funds last</label><div id="market-offers"></div><p id="market-reserved" class="market-note"></p></div>
      <form id="market-offer-form"><h3>Post an offer</h3><p class="market-note">Offer goods for gold, buy with gold, or barter directly. Unclaimed lots stay listed until cancelled.</p><fieldset id="market-offer-fields"><legend class="sr-only">Offer terms</legend><label>Trading Market or Dock<select id="offer-market" required></select></label><div class="market-form-pair"><label>You offer<select id="offer-give-resource">${resources.map(r=>`<option value="${r}" ${r==='wood'?'selected':''}>${r}</option>`).join('')}</select></label><label>Per lot<input id="offer-give-amount" aria-label="Offered amount per lot" type="number" min="1" value="100" required></label></div><div class="market-form-pair"><label>You request<select id="offer-want-resource">${resources.map(r=>`<option value="${r}" ${r==='gold'?'selected':''}>${r}</option>`).join('')}</select></label><label>Per lot<input id="offer-want-amount" aria-label="Requested amount per lot" type="number" min="1" value="80" required></label></div><div class="market-form-pair"><label>Lots<input id="offer-lots" type="number" min="1" value="3" required></label><label>Audience<select id="offer-audience"><option value="0">All kingdoms</option></select></label></div><p class="market-note">Posting reserves your offered goods. Cancelling returns unclaimed lots; accepted caravans continue.</p><button class="primary" id="post-offer" type="submit">Post offer</button></fieldset><p id="market-requirement" class="market-note"></p></form></section>
      <section data-market-page="caravans" hidden><h3>Your trade deliveries</h3><p class="market-note">Cargo is exchanged at the destination Market or Dock. Goods or sale proceeds reach you when the carrier returns home. Stop or move a carrier to interrupt it, then resume here. Recall brings back the outbound cargo and is available before the exchange. Attackers capture cargo aboard destroyed carriers.</p><div id="market-caravans"></div></section>
      <section data-market-page="merchants" hidden><h3>Local markets & trade routes</h3><p class="market-note">Compare regional shortages and surpluses. Your home merchants exchange immediately. Other Markets and Docks require a carrier with goods or payment.</p><div class="merchant-selector"><label for="merchant-market">Merchant market</label><select id="merchant-market"><option value="home">Your home merchants</option></select></div><div id="merchant-route-controls" hidden><label class="market-repeat"><input type="checkbox" id="merchant-repeat"> Repeat trips at future local prices</label><div class="merchant-limit"><label for="merchant-limit">Price limit · gold per 100 goods</label><input aria-describedby="merchant-limit-help" id="merchant-limit" type="number" min="1" max="500" placeholder="Any price"><span id="merchant-limit-help" class="market-note">Minimum gold received when selling; maximum paid when buying. Leave blank to accept future quotes. Each accepted trip keeps its agreed price.</span></div></div><div id="market-merchants"></div></section>
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
    this.get('merchant-market').onchange=()=>this.render();
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
      else if(action==='resume' || action==='recall') void this.issue({kind:action==='resume'?'market_resume':'market_recall',shipment_id:id},action==='resume'?'Caravan resumed.':'Recall accepted. The carrier brings its outbound cargo home.');
      else if(action==='merchant') {
        const quote=v.merchants.actions[id];
        if(quote?.enabled && v.merchants.market_id) void this.issue({kind:quote.kind,product:quote.product,entity_ids:[v.merchants.market_id],market_revision:v.merchants.revision},quote.description);
      } else if(action==='merchant-route') {
        const market=v.markets.find(m=>m.merchant.region_id===Number(button.dataset.region)),quote=market?.routes[id];
        const field=this.get<HTMLInputElement>('merchant-limit');
        if(!field.reportValidity()) return;
        if(market && quote?.can_start && quote.cart_id) void this.issue({kind:'trade',entity_ids:[quote.cart_id],target_id:market.merchant.market_id,product:quote.product,trade_mode:quote.mode,repeat:this.get<HTMLInputElement>('merchant-repeat').checked,market_revision:market.merchant.revision,...(field.value?{trade_limit:Number(field.value)}:{})},'Merchant trade accepted. Cargo and payment reserved.');
      } else if(action==='locate-market') {
        const market=this.snapshot?.entities.find(e=>e.id===id && (e.type==='market'||e.type==='dock'));
        if(market) {this.dialog.close();this.locate(market.position,market.id);}
      } else if(action==='locate-supply') {
        const market=id===v.merchants.region_id?v.merchants:v.markets.find(m=>m.merchant.region_id===id)?.merchant;
        if(market?.supply_position) {this.dialog.close();this.locate(market.supply_position,market.supply_cart_id);}
      } else if(action==='locate-offer' || action==='locate-cart') {
        const item=action==='locate-offer'?v.offers.find(o=>o.id===id):v.shipments.find(s=>s.id===id);
        if(item?.position) { this.dialog.close();this.locate(item.position,action==='locate-cart'?('cart_id' in item?item.cart_id:undefined):('market_id' in item?item.market_id:undefined)); }
      }
    });
  }
  private get<T extends HTMLElement=HTMLElement>(id:string) { return this.dialog.querySelector<T>(`#${id}`)!; }
  private value(id:string) { return this.get<HTMLInputElement|HTMLSelectElement>(id).value; }
  open(tab?:string) {
    if(!this.snapshot) return;
    if(tab) this.dialog.querySelector<HTMLButtonElement>(`[data-market-tab="${tab}"]`)?.click();
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
  private shipmentStatus(t:TradeShipmentView) {
    if(t.merchant_id && t.trade_mode==='sell') {
      if(t.status==='outbound') return 'Exports outbound';
      if(t.status==='returning') return 'Sale proceeds returning';
      if(t.status==='returning_payment') return 'Exports returning';
    }
    return states[t.status]??t.status;
  }
  private routeTerms(t:TradeShipmentView) {
    if(!t.merchant_id) return '';
    const selling=t.trade_mode==='sell';
    return `<p class="market-note">${selling?'Export':'Import'} · ${selling?'Minimum received':'Maximum paid'}: ${t.price_limit} gold per 100 goods</p>`;
  }
  private supply(m:MerchantView) {
    const timing=m.supply_state==='travelling'?'Supply caravan en route':m.next_supply_in>0?`Next supply departure in ${Math.ceil(m.next_supply_in)} game seconds`:'Preparing the next supply caravan';
    return `<div class="merchant-supply"><p><strong>${escape(m.biome)} merchants</strong> · ${escape(timing)}</p><p class="market-note">Regional supplies: ${escape(quantity(m.production))} per caravan. Local buyers seek up to ${escape(quantity(m.demand))}. Departures are at least ${m.supply_interval} game seconds apart; travel and blockades add time.</p>${m.supply_position?`<button type="button" data-market-action="locate-supply" data-id="${m.region_id}">Locate supply caravan</button>`:''}</div>`;
  }
  private merchants() {
    const s=this.snapshot!,v=s.marketplace;
    const known=s.entities.filter(e=>e.owner===0 && (e.type==='market'||e.type==='dock'));
    this.options('merchant-market',[new Option('Your home merchants','home'),...known.map(e=>new Option(`${e.name} #${e.id}${e.visible?'':' · outside sight'}`,String(e.id)))]);
    const chosen=this.value('merchant-market');
    this.get('merchant-route-controls').hidden=chosen==='home';
    if(chosen==='home') {
      this.html('market-merchants',`${this.supply(v.merchants)}<p class="market-note">Your home merchants share inventory across all your Markets and Docks. Treasury: ${Math.floor(v.merchants.stock.gold).toLocaleString()} gold.</p>${['food','wood','stone'].map(resource=>`<section class="merchant-resource"><div><h4>${resource}</h4><p>${Math.floor(v.merchants.stock[resource as keyof Resources]).toLocaleString()} in stock</p></div><div class="merchant-quotes">${v.merchants.actions.map((a,i)=>a.product===resource?`<div><button type="button" data-market-action="merchant" data-id="${i}" ${!a.enabled || this.disabled()?'disabled':''}><strong>${escape(a.label)}</strong><span>${escape(quantity(a.cost))} → ${escape(quantity(a.gain!))}</span></button>${!a.enabled?`<p class="market-refusal">${escape(a.reason??'Unavailable')}</p>`:''}</div>`:'').join('')}</div></section>`).join('')}`);
      return;
    }
    const market=v.markets.find(m=>m.merchant.region_id===Number(chosen));
    if(!market) {
      this.html('market-merchants',`<p class="market-empty">Send a scout to refresh local prices. This trading post is outside your sight.</p><button type="button" data-market-action="locate-market" data-id="${Number(chosen)}">Locate trading post</button>`);
      return;
    }
    const m=market.merchant;
    this.html('market-merchants',`${this.supply(m)}<div class="market-row-actions"><button type="button" data-market-action="locate-market" data-id="${m.market_id}">Locate trading post</button></div><p class="market-note">Start with an idle Trade Cart at your Market, or Trade Ship at your Dock. Land and sea routes need matching trading posts. The margin compares buying at one trading post and selling at the other, using current quotes. It excludes travel time and risk; the later sale price can change.</p>${['food','wood','stone'].map(resource=>`<section class="merchant-resource"><div><h4>${resource}</h4><p>${Math.floor(m.stock[resource as keyof Resources]).toLocaleString()} in stock</p></div><div class="merchant-quotes">${market.routes.map((q,i)=>q.product===resource?`<div><button type="button" data-market-action="merchant-route" data-region="${m.region_id}" data-id="${i}" ${!q.can_start || this.disabled()?'disabled':''}><strong>${q.mode==='sell'?'Export':'Import'} ${resource}</strong><span>${escape(quantity(q.cost))} → ${escape(quantity(q.gain))}</span></button><p class="merchant-margin">${q.home_margin>0?'+':''}${q.home_margin} gold margin per 100 goods</p>${!q.can_start?`<p class="market-refusal">${escape(q.reason??'Unavailable')}</p>`:''}</div>`:'').join('')}</div></section>`).join('')}`);
  }
  private offer(o:TradeOfferView) {
    const own=o.owner===this.snapshot!.player.id, t=o.terms, closed=o.state!=='open';
    const button=own?`<button type="button" data-market-action="cancel" data-id="${o.id}" ${closed || this.disabled()?'disabled':''}>Cancel offer</button>`:`<button type="button" data-market-action="accept" data-id="${o.id}" ${!o.can_accept || this.disabled()?'disabled':''}>Send caravan</button>`;
    return `<article class="market-offer" aria-label="Offer ${o.id}"><div class="market-offer-title"><strong>${escape(this.kingdom(o.owner))}</strong><span>#${o.id}${t.target_player?' · Private':''}</span></div><p class="market-exchange"><span>${t.give_amount} ${escape(t.give_resource)}</span><span aria-label="in exchange for"> ↔ </span><span>${t.want_amount} ${escape(t.want_resource)}</span></p><p class="market-note">${closed?escape(o.state):`${o.remaining} lot${o.remaining===1?'':'s'} available`}${!own?' · You receive '+t.give_amount+' '+escape(t.give_resource):''}</p><div class="market-row-actions">${button}${o.position?`<button type="button" data-market-action="locate-offer" data-id="${o.id}">Locate trading post</button>`:''}</div>${!own && !o.can_accept?`<p class="market-refusal">${escape(o.reason || 'Unavailable')}</p>`:''}</article>`;
  }
  private render() {
    const s=this.snapshot,c=this.catalog;
    if(!s || !c) return;
    const v=s.marketplace;
    const markets=s.entities.filter(e=>e.owner===s.player.id && (e.type==='market'||e.type==='dock') && e.progress===1);
    this.options('offer-market',markets.length?markets.map(m=>new Option(`${m.name} #${m.id}`,String(m.id))):[new Option('Build a Market or Dock first','')]);
    this.options('offer-audience',[new Option('All kingdoms','0'),...s.opponents.filter(o=>!o.defeated).map(o=>new Option(o.name,String(o.id)))]);
    this.get<HTMLFieldSetElement>('market-offer-fields').disabled=this.disabled() || !markets.length;
    this.get<HTMLInputElement>('offer-give-amount').max=this.get<HTMLInputElement>('offer-want-amount').max=String(v.capacity);
    this.get<HTMLInputElement>('offer-lots').max=String(v.max_lots);
    this.get('market-requirement').textContent=!this.connected?'Waiting for your game connection.':s.status!=='running'?'Resume the game to trade.':!markets.length?'Use a Market or Feudal-age Dock to post offers and exchange resources.':`Up to ${v.max_offers} open offers; ${v.capacity} units of each resource per carrier.`;
    this.get('post-offer').textContent=this.busy?'Sending…':'Post offer';
    if(this.tab==='offers') {
      const filter=this.value('market-filter');
      const offers=v.offers.filter(o=>(filter==='all' || o.terms.give_resource===filter || o.terms.want_resource===filter) && o.state==='open');
      this.html('market-offers',offers.length?offers.map(o=>this.offer(o)).join(''):`<p class="market-empty">${filter==='all'?'No standing offers yet. Post a surplus and let another kingdom bring what you need.':'No open offers for this resource.'}</p>`);
      this.get('market-reserved').textContent=`Your goods reserved for trade: ${quantity(v.reserved)}.`;
    } else if(this.tab==='caravans') {
      this.html('market-caravans',v.shipments.length?[...v.shipments].reverse().map(t=>`<article class="market-offer" aria-label="Caravan ${t.id}"><div class="market-offer-title"><strong>Delivery #${t.id}</strong><span>${t.merchant_id?`Trading post #${t.merchant_id}`:escape(this.kingdom(t.seller))} → ${escape(this.kingdom(t.buyer))}</span></div><p class="market-exchange">${t.terms.give_amount} ${escape(t.terms.give_resource)} ↔ ${t.terms.want_amount} ${escape(t.terms.want_resource)}</p><p class="market-note">${escape(this.shipmentStatus(t))}${t.repeat && !['delivered','lost','recalled'].includes(t.state)?' · Repeating while available':''}</p>${this.routeTerms(t)}<div class="market-row-actions">${t.can_resume?`<button type="button" data-market-action="resume" data-id="${t.id}" ${this.disabled()?'disabled':''}>Resume caravan</button>`:''}${t.can_recall?`<button type="button" data-market-action="recall" data-id="${t.id}" ${this.disabled()?'disabled':''}>Recall caravan</button>`:''}${t.position?`<button type="button" data-market-action="locate-cart" data-id="${t.id}">Locate carrier</button>`:''}</div></article>`).join(''):'<p class="market-empty">No deliveries yet. Accept an offer or arrange an import or export from the Merchants page.</p>');
    } else if(this.tab==='merchants') {
      this.merchants();
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
      this.get('market-error').scrollIntoView({block:'nearest'});
    } finally {this.busy=false;this.render();}
  }
}
