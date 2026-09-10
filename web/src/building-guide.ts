import type { Action, Catalog } from './api.generated';
import { buildingIcon } from './building-icons';

const escape = (value:string) => value.replace(/[&<>"']/g,c=>({'&':'&amp;','<':'&lt;','>':'&gt;','"':'&quot;',"'":'&#39;'}[c]!));

// A field guide beside the battlefield, available by hover, focus or touch.
// All facts and availability come from Go's catalog and current actions.
export class BuildingGuide {
  private root = document.createElement('aside');
  private signature = '';
  constructor() {
    this.root.id='building-guide';this.root.hidden=true;
    this.root.setAttribute('aria-label','Building capabilities');document.body.append(this.root);
  }
  hide() {this.root.hidden=true;this.signature='';}
  show(action:Action,catalog:Catalog) {
    const d=catalog.definitions.find(d=>d.id===action.product);if(!d||action.kind!=='build')return;
    const signature=JSON.stringify([d.id,action.enabled,action.reason]);if(signature===this.signature&&!this.root.hidden)return;this.signature=signature;
    const cost=Object.entries(action.cost).filter(([,v])=>v>0).map(([k,v])=>`${Math.round(v)} ${k}`).join(' · ');
    this.root.innerHTML=`<button class="guide-close" aria-label="Close building details">×</button><div class="guide-title"><span aria-hidden="true">${buildingIcon(d.id)}</span><div><h2>${escape(d.name)}</h2><p>${escape(catalog.ages[d.age])} · ${action.duration}s to build</p></div></div><p class="guide-purpose">${escape(d.description)}</p><p>${escape(d.importance??'')}</p>${d.capabilities?.length?`<ul>${d.capabilities.map(c=>`<li>${escape(c)}</li>`).join('')}</ul>`:''}<p class="guide-cost">${escape(cost)}</p>${action.reason?`<p class="guide-reason">${escape(action.reason)}</p>`:''}`;
    this.root.querySelector('button')!.onclick=()=>this.hide();this.root.hidden=false;
  }
}
