import type { EconomicSample, KingdomStatistics, Resources, Snapshot, StatisticsReport } from './api.generated';
import type { GameAPI } from './api';

const resources = ['food', 'wood', 'gold', 'stone'] as (keyof Resources)[];
const escape = (s: string) => s.replace(/[&<>"']/g, c => ({'&':'&amp;','<':'&lt;','>':'&gt;','"':'&quot;',"'":'&#39;'}[c]!));
const number = (n: number) => n.toLocaleString(undefined, {maximumFractionDigits: 1});
const clock = (t: number) => `${Math.floor(t/60)}:${String(Math.floor(t%60)).padStart(2,'0')}`;
const label = (s: string) => escape(s.replaceAll('_', ' ').replace(/^\w/, c => c.toUpperCase()));

// Charts project sampled Go observations. Stockpile differences are never
// treated as production, and wall time never advances the plotted game clock.
export class StatisticsUI {
  private dialog = document.createElement('dialog');
  private snapshot?: Snapshot;
  private report?: StatisticsReport;
  private loading = false;
  private version = 0;
  private session = '';
  constructor(private api: GameAPI) {
    this.dialog.id = 'statistics-dialog';
    this.dialog.setAttribute('aria-labelledby', 'statistics-title');
    this.dialog.innerHTML = `<header class="market-heading"><div><div class="eyebrow">THE KINGDOM LEDGER</div><h2 id="statistics-title">Campaign statistics</h2><p id="statistics-clock"></p></div><button id="statistics-close" aria-label="Close statistics">×</button></header>
      <div class="statistics-controls"><label>Report scope<select id="statistics-scope"><option value="kingdom">Your kingdom</option><option value="world">All kingdoms</option></select></label><label>Kingdom<select id="statistics-kingdom" aria-label="Kingdom"></select></label><button id="statistics-refresh">Refresh</button><button id="statistics-download">Download report</button><button id="statistics-csv">Download resource CSV</button></div>
      <p id="statistics-error" class="form-error" role="alert"></p><p id="statistics-status" role="status"></p>
      <section id="statistics-outcome"></section><div id="statistics-content"></div><details class="statistics-method"><summary>Accounting & definitions</summary><p id="statistics-accounting"></p></details>`;
    document.getElementById('app')!.append(this.dialog);
    this.get('statistics-close').onclick = () => this.dialog.close();
    this.get('statistics-refresh').onclick = () => void this.refresh();
    this.get('statistics-scope').onchange = () => { this.version++; this.report=undefined;this.get('statistics-content').replaceChildren();void this.refresh(); };
    this.get('statistics-kingdom').onchange = () => this.render();
    this.get('statistics-download').onclick = () => { if(this.report) this.download(JSON.stringify(this.report,null,2),'application/json','campaign-report.json'); };
    this.get('statistics-csv').onclick = () => {
      if(!this.report) return;
      const rows = ['kingdom_id,game_seconds,resource,produced_per_game_minute,consumed_per_game_minute'];
      for(const k of this.report.kingdoms) for(const s of k.production.history) for(const r of resources) rows.push([k.id,s.time,r,s.rates[r],s.consumption?.[r]??0].join(','));
      this.download(rows.join('\n'),'text/csv','resource-rates.csv');
    };
    this.dialog.addEventListener('pointerover', e => this.inspect(e.target));
    this.dialog.addEventListener('focusin', e => this.inspect(e.target));
    // Large campaign accounting is fetched only while the ledger is open.
    window.setInterval(() => { if(this.dialog.open) void this.refresh(); }, 2500);
  }
  private get<T extends HTMLElement=HTMLElement>(id:string) { return this.dialog.querySelector<T>(`#${id}`)!; }
  update(snapshot: Snapshot) {
    const session=this.api.session?.match_id??'';
    if(session!==this.session) { this.session=session;this.version++;this.report=undefined;this.get('statistics-content').replaceChildren();this.get('statistics-outcome').replaceChildren();this.get('statistics-error').textContent='';this.get<HTMLSelectElement>('statistics-scope').value='kingdom'; }
    this.snapshot=snapshot;
    const all=!!this.api.session?.owner || snapshot.status==='finished';
    const option=this.get<HTMLSelectElement>('statistics-scope').options[1];option.disabled=!all;option.hidden=!all;
    if(!all) this.get<HTMLSelectElement>('statistics-scope').value='kingdom';
  }
  open(world=false) {
    if(!this.snapshot) return;
    if(world && (this.api.session?.owner || this.snapshot.status==='finished')) this.get<HTMLSelectElement>('statistics-scope').value='world';
    if(!this.dialog.open) this.dialog.showModal();
    void this.refresh();
  }
  private async refresh() {
    if(this.loading || !this.api.session || !this.dialog.open) return;
    this.loading=true;
    const version=this.version, session=this.api.session.match_id, scope=this.get<HTMLSelectElement>('statistics-scope').value;
    this.get('statistics-status').textContent=this.report?'':'Loading campaign accounting…';
    this.get<HTMLButtonElement>('statistics-refresh').disabled=true;
    try {
      const r=await this.api.request<StatisticsReport>(`${this.api.path()}/statistics?scope=${scope}`);
      if(version!==this.version || session!==this.api.session?.match_id) return;
      this.report=r;this.get('statistics-error').textContent='';this.get('statistics-status').textContent='';
      const select=this.get<HTMLSelectElement>('statistics-kingdom'),old=Number(select.value)||this.snapshot!.player.id;
      const signature=r.kingdoms.map(k=>`${k.id}:${k.name}`).join('|');
      if(select.dataset.signature!==signature) {select.replaceChildren(...r.kingdoms.map(k=>new Option(k.name,String(k.id))));select.dataset.signature=signature;select.value=String(r.kingdoms.some(k=>k.id===old)?old:r.kingdoms[0]?.id);}
      // Do not replace a chart point, disclosure or table while it has focus.
      if(!this.get('statistics-content').contains(document.activeElement) && !this.get('statistics-outcome').contains(document.activeElement)) this.render();
    } catch(e) { if(version===this.version) {this.get('statistics-status').textContent='';this.get('statistics-error').textContent=e instanceof Error?e.message:'Statistics could not be loaded. Try Refresh.';} }
    finally {this.loading=false;this.get<HTMLButtonElement>('statistics-refresh').disabled=false;}
  }
  private inspect(target: EventTarget | null) {
    if(!(target instanceof Element)) return;
    const point=target.closest<SVGElement>('[data-sample]');
    const output=point?.closest('figure')?.querySelector('output');
    if(point && output) output.textContent=point.dataset.sample!;
  }
  private series(samples: {time:number; incoming:number; outgoing?:number}[], title: string, unit: string, windowSeconds?: number) {
    if(!samples.length) return '<p class="market-empty">Waiting for the first game-time sample.</p>';
    const last=samples.at(-1)!.time, first=windowSeconds?Math.max(0,last-windowSeconds):samples[0].time;
    const scale=Math.max(1,...samples.flatMap(s=>[s.incoming,s.outgoing??0]));
    const x=(t:number)=>42+(t-first)/Math.max(1,last-first)*548, y=(n:number)=>120-n/scale*102;
    const path=(out=false)=>samples.map((s,i)=>`${i?'L':'M'}${x(s.time).toFixed(1)},${y(out?s.outgoing??0:s.incoming).toFixed(1)}`).join('');
    const dual=samples[0].outgoing!==undefined;
    return `<svg class="statistics-chart" role="img" aria-label="${escape(title)} over game time, ${escape(unit)}" viewBox="0 0 610 148"><path class="chart-grid" d="M42 18H590M42 69H590M42 120H590"/><text x="36" y="22" text-anchor="end">${number(scale)}</text><text x="36" y="124" text-anchor="end">0</text><text x="42" y="142">${clock(first)}</text><text x="590" y="142" text-anchor="end">${clock(last)}</text><path class="production-line" d="${path()}"/>${dual?`<path class="consumption-line" d="${path(true)}"/>`:''}${samples.map(s=>{const detail=`${clock(s.time)} · ${number(s.incoming)} ${dual?'produced':' '+unit}${dual?` · ${number(s.outgoing??0)} consumed per game minute`:''}`;return `<circle tabindex="0" cx="${x(s.time).toFixed(1)}" cy="${y(s.incoming).toFixed(1)}" r="4" data-sample="${escape(detail)}" aria-label="${escape(detail)}"><title>${escape(detail)}</title></circle>`;}).join('')}</svg><output>Hover or focus a sample to inspect it.</output>`;
  }
  private resourceCharts(k: KingdomStatistics) {
    return `<section><h3>Production & consumption</h3><p class="chart-legend"><span class="production-key">Solid: produced</span><span class="consumption-key">Dashed: consumed</span> · Resources per game minute · Rolling 60 seconds</p><div class="statistics-charts">${resources.map(r=>`<figure aria-label="${r} resource chart"><figcaption><strong>${r}</strong><span>+${number(k.production.rates[r])} / −${number(k.production.consumption_rates?.[r]??0)} per minute</span></figcaption>${this.series(k.production.history.map(s=>({time:s.time,incoming:s.rates[r],outgoing:s.consumption?.[r]??0})),`${r} production and consumption`,'resources per game minute',300)}</figure>`).join('')}</div><details><summary>Resource samples as a table</summary><div class="market-table-scroll"><table><caption>Rolling rates per game minute. Each pair is produced / consumed.</caption><thead><tr><th>Game time</th>${resources.map(r=>`<th>${r}</th>`).join('')}</tr></thead><tbody>${k.production.history.map(s=>`<tr><th scope="row">${clock(s.time)}</th>${resources.map(r=>`<td>${number(s.rates[r])} / ${number(s.consumption?.[r]??0)}</td>`).join('')}</tr>`).join('')}</tbody></table></div></details></section>`;
  }
  private pairs(rows: [string,string|number][]) { return `<dl class="statistics-facts">${rows.map(([k,v])=>`<div><dt>${k}</dt><dd>${typeof v==='number'?number(v):escape(v)}</dd></div>`).join('')}</dl>`; }
  private breakdown(title: string, items: Record<string,number>) { return `<section><h4>${title}</h4>${Object.keys(items).length?this.pairs(Object.entries(items).map(([k,v])=>[label(k),v])):'<p class="market-note">None recorded.</p>'}</section>`; }
  private render() {
    const r=this.report;
    if(!r) return;
    const k=r.kingdoms.find(k=>k.id===Number(this.get<HTMLSelectElement>('statistics-kingdom').value))??r.kingdoms[0];
    if(!k) return;
    this.get('statistics-clock').textContent=`${clock(r.time)} game time · ${r.status==='finished'?'Final report':'Live accounting'} · ${r.scope==='world'?'All kingdoms':'Your kingdom'}`;
    this.get('statistics-accounting').textContent=r.accounting;
    const names=(ids:number[])=>ids.map(id=>r.kingdoms.find(k=>k.id===id)?.name??`Kingdom ${id}`).join(', ');
    const outcome=this.get('statistics-outcome');
    const outcomeDisclosures=outcome.dataset.status===r.status?[...outcome.querySelectorAll('details')].map(d=>d.open):[];
    outcome.dataset.status=r.status;
    outcome.innerHTML=`${r.status==='finished'?`<h3>${r.official_winner?`${escape(names([r.official_winner]))} won the match`:'The match has ended'}</h3><p>${label(r.victory_reason||'No surviving kingdom')}</p>`:''}${r.awards.length?`<details ${r.status==='finished'?'open':''}><summary>Leaders by seven measures</summary><div class="market-table-scroll"><table><caption>These awards recognize different achievements. The match winner follows the game’s victory rules.</caption><thead><tr><th>Achievement</th><th>Leading kingdom${r.status==='finished'?'':' so far'}</th><th>Score</th></tr></thead><tbody>${r.awards.map(a=>`<tr><th scope="row">${escape(a.name)}<small>${escape(a.rule)}</small></th><td>${escape(names(a.winners))}${a.winners.length>1?' · tied':''}</td><td>${number(a.value)}</td></tr>`).join('')}</tbody></table></div></details>`:''}<details ${r.status==='finished'?'open':''}><summary>Campaign summary</summary>${r.summary.map(s=>`<p>${escape(s)}</p>`).join('')}</details>`;
    outcome.querySelectorAll('details').forEach((d,i)=>{if(outcomeDisclosures[i]!==undefined)d.open=outcomeDisclosures[i];});
    const old=this.get('statistics-content');const disclosures=[...old.querySelectorAll('details')].map(d=>d.open);
    old.innerHTML=`<h3>${escape(k.name)} · ${escape(k.age)}</h3><p class="market-note">${label(k.civilization)} · ${k.defeated?'Defeated':'Standing'} · Accounting from ${clock(k.accounting_since)} game time</p>
      <div class="statistics-highlights">${this.pairs([['GDP produced',k.gdp],['GDP / game minute',k.gdp_per_minute],['GDP / current person',k.gdp_per_capita],['Stock value',k.stock_value],['Completed investment',k.construction_value],['Buildings / game minute',k.development_per_minute]])}</div>
      ${this.resourceCharts(k)}
      <section><h3>Population & food</h3><p class="food-condition" data-food="${k.food.state}">${k.food.state==='fed'?'Population fed':k.food.state==='famine'?'Famine · gathering, construction and production slowed':'Food shortage'} · ${number(k.food.demand_per_minute)} food required per game minute · ${number(k.food.per_person_minute)} per person</p>${this.pairs([['Population',k.population],['Peak population',k.peak_population],['Workers',k.workers],['Idle workers',k.idle_workers],['Military',k.military],['Food eaten',k.food.consumed],['Unmet food demand',k.food.unmet],['Current shortage (seconds)',k.food.shortage_seconds],['Work speed',`${number(k.food.work_multiplier*100)}%`]])}</section>
      <section><h3>Resource accounts</h3><div class="market-table-scroll"><table><caption>Actual resource quantities since accounting began</caption><thead><tr><th>Account</th>${resources.map(r=>`<th>${r}</th>`).join('')}</tr></thead><tbody>${([['In stock',k.stock],['Produced',k.produced],['Consumed / spent',k.consumed],['Refunded',k.refunded],['Traded out',k.trade_sold],['Traded in',k.trade_bought],...Object.entries(k.consumption_by_purpose).map(([purpose,amount])=>[`Spent: ${purpose.replaceAll('_',' ')}`,amount])] as [string,Resources][]).map(([name,amount])=>`<tr><th scope="row">${escape(name)}</th>${resources.map(r=>`<td>${number(amount[r])}</td>`).join('')}</tr>`).join('')}</tbody></table></div></section>
      <section><h3>Development, trade & military</h3>${this.pairs([['Buildings completed',k.buildings_completed],['Foundations underway',k.foundations],['Units trained',k.units_trained],['Trade value',k.trade_volume],['Trade exchanges / deliveries',k.trade_deliveries],['Enemy entities destroyed',k.kills],['Units lost',k.units_lost],['Buildings lost',k.buildings_lost],['Damage dealt (HP)',k.damage_dealt],['Damage taken (HP)',k.damage_taken],['Map explored',`${number(k.explored_percent)}%`]])}</section>
      <section><h3>Campaign development</h3><p class="market-note">One observation per game minute. Up to 4,320 observations retained.</p><div class="statistics-charts">${([['gdp','GDP','production value'],['population','Population','people'],['buildings','Active buildings','buildings']] as [keyof EconomicSample,string,string][]).map(([key,title,unit])=>`<figure><figcaption><strong>${title}</strong></figcaption>${this.series(k.history.map(s=>({time:s.time,incoming:s[key] as number})),title,unit)}</figure>`).join('')}</div></section>
      <details><summary>Workforce, buildings, units & technology</summary><div class="statistics-breakdowns">${this.breakdown('Worker assignments',k.worker_tasks)}${this.breakdown('Standing buildings',k.buildings)}${this.breakdown('Current units',k.units)}<section><h4>Researched technologies</h4><p>${k.technologies.map(label).join(' · ')||'None researched yet.'}</p></section></div></details>`;
    old.querySelectorAll('details').forEach((d,i)=>{if(disclosures[i]!==undefined)d.open=disclosures[i];});
  }
  private download(content: string, type: string, name: string) {
    const url=URL.createObjectURL(new Blob([content],{type})),a=document.createElement('a');a.href=url;a.download=name;a.click();window.setTimeout(()=>URL.revokeObjectURL(url),1000);
  }
}
