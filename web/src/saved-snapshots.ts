import { GameAPI, requestID } from './api';
import type { SavedSnapshot, SnapshotLibrary, ImportResult } from './api.generated';

const escape=(v:string)=>v.replace(/[&<>"']/g,c=>({'&':'&amp;','<':'&lt;','>':'&gt;','"':'&quot;',"'":'&#39;'}[c]!));
const clock=(s:number)=>`${Math.floor(s/60)}:${String(Math.floor(s%60)).padStart(2,'0')}`;

export class SavedSnapshotsUI {
  private dialog=document.createElement('dialog');
  private points:SavedSnapshot[]=[];
  private selected?:SavedSnapshot;
  private busy=false;
  private saveID=requestID();
  private forkID=requestID();
  private returnTo?:()=>void;
  constructor(private api:GameAPI,private enter:()=>Promise<void>){
    this.dialog.id='snapshots-dialog';this.dialog.setAttribute('aria-labelledby','snapshots-title');
    this.dialog.innerHTML=`<button class="dialog-close" id="snapshots-close" aria-label="Close snapshots">×</button><h2 id="snapshots-title">Snapshots & exports</h2><p id="snapshots-game"></p><p>Save a turning point. Start a separate, paused game from any snapshot, with fresh invitations for other players.</p><form id="snapshot-save-form"><label>Snapshot name<input id="snapshot-name" required maxlength="80" placeholder="Before the siege"></label><button class="primary">Save snapshot</button></form><p id="snapshots-status" role="status"></p><p id="snapshots-error" class="form-error" role="alert"></p><div id="snapshot-list" aria-label="Saved snapshots"></div><form id="snapshot-fork-form" hidden><h3 id="snapshot-choice"></h3><label>New game name<input id="snapshot-copy-name" required maxlength="80"></label><p>Your current game stays in the library. The new game starts at the saved time, paused.</p><button class="primary">Start from snapshot</button><button type="button" id="snapshot-fork-cancel" class="text-button">Cancel</button></form><div class="snapshot-export"><h3>Download this game</h3><p>The SQLite file includes the current game, named snapshots, all kingdoms, history and recovery credentials. Keep this complete backup private.</p><button id="snapshot-export" class="secondary">Download SQLite file</button></div>`;
    document.body.append(this.dialog);
    this.dialog.oncancel=e=>{if(this.busy)e.preventDefault();};
    this.dialog.onclose=()=>{const back=this.returnTo;this.returnTo=undefined;back?.();};
    this.get<HTMLButtonElement>('snapshots-close').onclick=()=>this.dialog.close();
    this.get<HTMLButtonElement>('snapshot-fork-cancel').onclick=()=>{this.selected=undefined;this.get('snapshot-fork-form').hidden=true;};
    this.get<HTMLFormElement>('snapshot-save-form').onsubmit=e=>{e.preventDefault();void this.run(async()=>{
      await this.api.request<SavedSnapshot>(`${this.api.path()}/snapshots`,{method:'POST',body:JSON.stringify({id:this.saveID,name:this.get<HTMLInputElement>('snapshot-name').value})});
      this.saveID=requestID();this.get<HTMLInputElement>('snapshot-name').value='';await this.load();this.status('Snapshot saved.');
    });};
    this.get<HTMLInputElement>('snapshot-name').oninput=()=>{this.saveID=requestID();};
    this.get<HTMLInputElement>('snapshot-copy-name').oninput=()=>{this.forkID=requestID();};
    this.get<HTMLFormElement>('snapshot-fork-form').onsubmit=e=>{e.preventDefault();if(!this.selected)return;void this.run(async()=>{
      const result=await this.api.request<ImportResult>(`${this.api.path()}/snapshots/${encodeURIComponent(this.selected!.id)}/fork`,{method:'POST',body:JSON.stringify({id:this.forkID,name:this.get<HTMLInputElement>('snapshot-copy-name').value})});
      await this.api.depart();this.api.remember(result.session);this.returnTo=undefined;this.dialog.close();await this.enter();
    });};
    this.get<HTMLButtonElement>('snapshot-export').onclick=()=>void this.run(()=>this.download());
  }
  private get<T extends HTMLElement=HTMLElement>(id:string){return this.dialog.querySelector<T>(`#${id}`)!;}
  private status(message:string){this.get('snapshots-status').textContent=message;}
  private async run(action:()=>Promise<void>){
    if(this.busy)return;this.busy=true;this.get('snapshots-error').textContent='';
    const controls=[...this.dialog.querySelectorAll<HTMLButtonElement|HTMLInputElement>('button,input')];controls.forEach(c=>c.disabled=true);
    try{await action();}catch(e){this.get('snapshots-error').textContent=e instanceof Error?e.message:'The save could not be completed. Try again.';}
    finally{this.busy=false;controls.forEach(c=>c.disabled=false);}
  }
  async open(returnTo?:()=>void){
    document.querySelectorAll<HTMLDialogElement>('dialog[open]').forEach(d=>d.close());
    this.returnTo=returnTo;
    this.get('snapshots-game').textContent=this.api.session?.name??'';
    this.selected=undefined;this.get('snapshot-fork-form').hidden=true;this.status('Loading snapshots…');this.dialog.showModal();
    await this.run(async()=>{
      const game=await this.api.gameInfo();
      const canSave=game.status==='open'&&game.runtime==='serving'&&game.match_status!=='finished';
      this.get('snapshot-save-form').hidden=!canSave;
      await this.load();this.status(canSave?'':'Start or reopen an unfinished game to save another snapshot. Your existing snapshots remain available.');
    });
  }
  private async load(){
    const library=await this.api.request<SnapshotLibrary>(`${this.api.path()}/snapshots`);this.points=library.snapshots;
    this.get('snapshot-list').innerHTML=this.points.length?this.points.map((p,i)=>`<div class="snapshot-row"><div><strong>${escape(p.name)}</strong><span>${clock(p.time)} in game · ${escape(new Date(p.created_at).toLocaleString())}</span></div><button data-start="${i}" class="secondary">Choose snapshot</button><button data-remove="${i}" class="text-button" aria-label="Delete snapshot ${escape(p.name)}">Delete</button><div class="snapshot-remove" hidden><span>Delete this saved point?</span><button data-confirm="${i}" class="danger">Delete snapshot</button><button data-cancel="${i}">Keep snapshot</button></div></div>`).join(''):'<p class="snapshot-empty">No snapshots yet. Save one above before trying a new strategy.</p>';
    this.dialog.querySelectorAll<HTMLButtonElement>('[data-start]').forEach(b=>b.onclick=()=>{
      this.selected=this.points[Number(b.dataset.start)];this.forkID=requestID();this.get('snapshot-choice').textContent=`Start at “${this.selected.name}” · ${clock(this.selected.time)}`;
      this.get<HTMLInputElement>('snapshot-copy-name').value=`${this.selected.name} — new chapter`.slice(0,80);this.get('snapshot-fork-form').hidden=false;this.get<HTMLInputElement>('snapshot-copy-name').focus();
    });
    this.dialog.querySelectorAll<HTMLButtonElement>('[data-remove]').forEach(b=>b.onclick=()=>{b.closest('.snapshot-row')!.querySelector<HTMLElement>('.snapshot-remove')!.hidden=false;});
    this.dialog.querySelectorAll<HTMLButtonElement>('[data-cancel]').forEach(b=>b.onclick=()=>{b.closest<HTMLElement>('.snapshot-remove')!.hidden=true;});
    this.dialog.querySelectorAll<HTMLButtonElement>('[data-confirm]').forEach(b=>b.onclick=()=>void this.run(async()=>{
      const p=this.points[Number(b.dataset.confirm)];await this.api.request<void>(`${this.api.path()}/snapshots/${encodeURIComponent(p.id)}`,{method:'DELETE'});
      if(this.selected?.id===p.id){this.selected=undefined;this.get('snapshot-fork-form').hidden=true;}await this.load();this.status('Snapshot deleted.');
    }));
  }
  async download(){
    const blob=await this.api.downloadDatabase(),url=URL.createObjectURL(blob),a=document.createElement('a');
    a.href=url;a.download=`${this.api.session!.match_id}.sqlite`;a.click();window.setTimeout(()=>URL.revokeObjectURL(url),1000);this.status('SQLite download ready.');
  }
}
