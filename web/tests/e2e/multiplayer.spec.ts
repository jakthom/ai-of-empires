import { type BrowserContext, type Page } from '@playwright/test';
import { test, expect } from './fixtures';
import { spawn, type ChildProcess } from 'node:child_process';
import { mkdtemp, rm } from 'node:fs/promises';
import { tmpdir } from 'node:os';
import { join, resolve } from 'node:path';
import { createServer } from 'node:net';
import type { GameInfo, MemberSession, Snapshot } from '../../src/api.generated';

async function gameRead<T>(context:BrowserContext,session:MemberSession,suffix=''){
 const response=await context.request.get(`/api/v1/games/${session.match_id}${suffix}`,{headers:{Authorization:`Bearer ${session.token}`}});expect(response.ok(),await response.text()).toBeTruthy();return response.json() as Promise<T>;
}
async function create(page:Page,name:string,friends=1){
 await page.goto('/');await page.getByLabel('Game name',{exact:true}).fill(name);await page.getByLabel('Your kingdom name',{exact:true}).fill('Alice');
 await page.getByRole('combobox',{name:'Friend seats',exact:true}).selectOption(String(friends));await page.getByRole('combobox',{name:'Difficulty',exact:true}).selectOption('peaceful');
 const created=page.waitForResponse(r=>new URL(r.url()).pathname==='/api/v1/games'&&r.request().method()==='POST');await page.getByRole('button',{name:'Begin your reign'}).click();const response=await created;expect(response.status()).toBe(201);await expect(page.locator('#room-panel')).toBeVisible();return response.json() as Promise<MemberSession>;
}
async function remove(context:BrowserContext,session:MemberSession){const response=await context.request.get(`/api/v1/games/${session.match_id}`,{headers:{Authorization:`Bearer ${session.token}`}});if(response.status()===404)return;expect(response.ok(),await response.text()).toBeTruthy();const game=await response.json() as GameInfo;const deleted=await context.request.delete(`/api/v1/games/${session.match_id}`,{headers:{Authorization:`Bearer ${session.token}`},data:{id:crypto.randomUUID(),revision:game.revision,confirm:true}});expect(deleted.ok(),await deleted.text()).toBeTruthy()}

test('two browsers claim distinct kingdoms, pause together, recover and close a private game',async({browser,page,context},info)=>{
 test.setTimeout(90_000);
 // HTTP LAN origins expose getRandomValues but do not provide randomUUID.
 await context.addInitScript(()=>Object.defineProperty(crypto,'randomUUID',{value:undefined,configurable:true}));
 const owner=await create(page,`Shared kingdom ${Date.now()}`);
 const friendContext=await browser.newContext({baseURL:info.project.use.baseURL,viewport:{width:1100,height:850}});await friendContext.addInitScript(()=>Object.defineProperty(crypto,'randomUUID',{value:undefined,configurable:true}));const friendPage=await friendContext.newPage();
 const errors:string[]=[];friendPage.on('pageerror',e=>errors.push(e.message));
 try{
  await expect(page.getByRole('button',{name:'Start game',exact:true})).toBeEnabled();
  await page.getByRole('button',{name:'Invite player',exact:true}).click();
  const link=page.getByLabel('Invitation for Friend 1',{exact:true});await expect(link).toHaveValue(/\/join#invite=/);const invite=await link.inputValue();
  await page.screenshot({path:info.outputPath('private-lobby.png')});await page.screenshot({path:'.browser-artifacts/multiplayer-desktop.png'});
  await friendPage.goto(invite);await expect(friendPage.getByRole('heading',{name:owner.name})).toBeVisible();expect(friendPage.url()).not.toContain('invite=');
  // A preview is not a seat claim, including when a link-preview client visits.
  const before=await gameRead<GameInfo>(context,owner);expect(before.seats[1].status).toBe('reserved');
  await friendPage.locator('#join-panel').getByLabel('Your kingdom name',{exact:true}).fill('Bob');
  const claimed=friendPage.waitForResponse(r=>r.url().endsWith('/invites/claim'));await friendPage.getByRole('button',{name:'Join this game',exact:true}).click();const friend=await (await claimed).json() as MemberSession;
  expect(friend.player_id).toBe(2);expect(friend.token).not.toBe(owner.token);
  await expect(friendPage.locator('#room-panel')).toContainText('Bob · You');
  await expect(page.locator('#room-panel')).toContainText('Bob');
  await page.getByRole('button',{name:'Ready',exact:true}).click();await friendPage.getByRole('button',{name:'Ready',exact:true}).click();
  await expect(page.getByRole('button',{name:'Start game',exact:true})).toBeEnabled();await page.getByRole('button',{name:'Start game',exact:true}).click();
  await expect(page.locator('#start-dialog')).toBeHidden();await expect(friendPage.locator('#start-dialog')).toBeHidden();
  await expect(friendPage.locator('#selected-name')).toHaveText('Town Center');
  const [a,b]=await Promise.all([gameRead<Snapshot>(context,owner,'/snapshot'),gameRead<Snapshot>(friendContext,friend,'/snapshot')]);expect(a.player.id).toBe(1);expect(b.player.id).toBe(2);
  const ownerTC=a.entities.find(e=>e.owner===1&&e.type==='town_center')!;
  expect(b.entities.some(e=>e.id===ownerTC.id)).toBe(false);
  await friendPage.getByRole('button',{name:'Pause match',exact:true}).click();await expect(page.locator('#paused')).toBeVisible();await expect(friendPage.getByRole('button',{name:'Waiting for the owner to resume'})).toBeDisabled();
  await expect(friendPage.locator('#speed')).toBeDisabled();await page.getByRole('button',{name:'Resume battle',exact:true}).click();await expect(friendPage.locator('#paused')).toBeHidden();
  const fresh=await browser.newContext({baseURL:info.project.use.baseURL});const recovered=await fresh.newPage();
  await recovered.goto('/');await recovered.getByRole('button',{name:'Saved games',exact:true}).click();await expect(recovered.locator('#saved-games-list .saved-game')).toHaveCount(0);
  await recovered.getByText('Rejoin from another browser or host',{exact:true}).click();await recovered.getByLabel('Private rejoin code',{exact:true}).fill(friend.rejoin_code!);await recovered.getByRole('button',{name:'Rejoin my kingdom',exact:true}).click();await expect(recovered.locator('#selected-name')).toHaveText('Town Center');
  await recovered.close();await fresh.close();
  await page.getByRole('button',{name:'Match menu',exact:true}).click();await page.getByRole('button',{name:'Players & game management',exact:true}).click();
  await page.getByRole('button',{name:'Close game for everyone',exact:true}).click();await expect(page.locator('#room-confirm')).toContainText(owner.name);await page.getByRole('button',{name:'Close and save',exact:true}).click();
  await expect(page.locator('.room-status')).toHaveText('Closed and saved');
  await page.getByRole('button',{name:'Reopen game',exact:true}).click();await expect(page.locator('.room-status')).toHaveText('Paused');
  await page.getByRole('button',{name:'Delete game',exact:true}).click();await expect(page.locator('#room-confirm')).toContainText('all local saves');await page.getByRole('button',{name:'Delete permanently',exact:true}).click();
  await expect(page.locator('#saved-games-panel')).toBeVisible();await expect(page.locator('#saved-games-list .saved-game')).toHaveCount(0);
  expect(errors).toEqual([]);
 }finally{await friendContext.close();await remove(context,owner)}
});

test('a compact lobby exposes invite, readiness and private recovery without overflow',async({page,context},info)=>{
 await page.setViewportSize({width:390,height:700});const owner=await create(page,`Compact council ${Date.now()}`);
 try{await expect(page.getByRole('button',{name:'Ready',exact:true})).toBeVisible();await page.getByRole('button',{name:'Invite player',exact:true}).click();await expect(page.getByRole('button',{name:'Copy invite link',exact:true})).toBeVisible();await page.screenshot({path:info.outputPath('compact-lobby.png')});await page.screenshot({path:'.browser-artifacts/multiplayer-compact.png'});expect(await page.locator('#start-dialog').evaluate(e=>e.scrollWidth>e.clientWidth)).toBe(false)}finally{await remove(context,owner)}
});


test('moves an encrypted running game to a second Go process and survives its restart',async({page,game,browser},info)=>{
 test.setTimeout(90_000);
 const directory=await mkdtemp(join(tmpdir(),'aoe-move-'));
 const probe=createServer();await new Promise<void>(resolve=>probe.listen(0,'127.0.0.1',resolve));const port=(probe.address() as {port:number}).port;await new Promise<void>(resolve=>probe.close(()=>resolve()));
 const origin=`http://127.0.0.1:${port}`;let process:ChildProcess|undefined;let output='';
 const launch=async()=>{process=spawn(resolve('../bin/ai-of-empires'),['-addr',`127.0.0.1:${port}`,'-db',join(directory,'games.sqlite')],{stdio:['ignore','pipe','pipe']});process.stderr?.on('data',chunk=>output+=String(chunk));await expect.poll(async()=>{try{return (await fetch(origin+'/api/v1/health')).status}catch{return 0}},{timeout:15000,message:output}).toBe(200)};
 const stop=async()=>{if(!process||process.exitCode!==null)return;const finished=new Promise<void>(resolve=>process!.once('exit',()=>resolve()));process.kill('SIGTERM');await finished;process=undefined};
 const target=await browser.newContext({baseURL:origin,viewport:{width:1280,height:900}});
 try{
  await launch();const owner=await game.start('sandbox','peaceful',2,`Portable realm ${Date.now()}`) as MemberSession;
  await game.command('train',()=>page.locator('#actions').getByRole('button',{name:/Villager/}).click());
  await game.command('pause',()=>page.getByRole('button',{name:'Pause match',exact:true}).click());const before=await game.snapshot();
  await page.getByRole('button',{name:'Match menu',exact:true}).click();await page.getByRole('button',{name:'Players & game management',exact:true}).click();
  await page.getByText('Move or copy this game',{exact:true}).click();await page.getByLabel('Archive passphrase (optional)',{exact:true}).fill('a portable kingdom');
  await page.getByRole('button',{name:'Move game',exact:true}).click();const download=page.waitForEvent('download');await page.getByRole('button',{name:'Freeze and download',exact:true}).click();const file=await download;const archive=join(directory,'move.aoegame');await file.saveAs(archive);
  await expect(page.locator('.room-status')).toHaveText('Moving · source frozen');
  const destination=await target.newPage();await destination.goto('/');await destination.getByRole('button',{name:'Saved games',exact:true}).click();await destination.getByText('Import a game archive',{exact:true}).click();
  const form=destination.locator('#import-form');await form.getByLabel('Game archive',{exact:true}).setInputFiles(archive);await form.getByLabel('Archive passphrase',{exact:true}).fill('a portable kingdom');await form.getByLabel("Owner's private rejoin code",{exact:true}).fill(owner.rejoin_code!);
  const imported=destination.waitForResponse(r=>r.url().endsWith('/game-imports'));await form.getByRole('button',{name:'Import game',exact:true}).click();const result=await (await imported).json();expect(result.session.match_id).toBe(owner.match_id);expect(result.session.membership_id).toBe(owner.membership_id);expect(result.session.epoch).not.toBe(owner.epoch);
  await expect(destination.locator('.room-status')).toHaveText('Paused');await destination.getByRole('button',{name:'Return to battlefield',exact:true}).click();await expect(destination.locator('#paused')).toBeVisible();
  let view=await gameRead<Snapshot>(target,result.session,'/snapshot');expect(view.tick).toBe(before.tick);expect(view.entities.find(e=>e.type==='town_center'&&e.owner===1)?.tasks).toEqual(before.entities.find(e=>e.type==='town_center'&&e.owner===1)?.tasks);
  const reconnected=destination.waitForResponse(r=>r.url().endsWith('/connections')&&r.request().method()==='POST'&&r.status()===201);
  await stop();await launch();await reconnected;await expect(destination.locator('#start-dialog')).toBeHidden();
  await destination.reload();await expect(destination.locator('#paused')).toBeVisible();view=await gameRead<Snapshot>(target,result.session,'/snapshot');expect(view.tick).toBe(before.tick);
  await destination.getByRole('button',{name:'Resume battle',exact:true}).click();await expect.poll(async()=>(await gameRead<Snapshot>(target,result.session,'/snapshot')).tick).toBeGreaterThan(before.tick);
  await destination.screenshot({path:info.outputPath('moved-game.png')});
 }finally{await target.close();await stop();await rm(directory,{recursive:true,force:true})}
});

test('keeps lobby recovery available when an action, refresh and leave save fail',async({page,context})=>{
 const owner=await create(page,`Recoverable lobby ${Date.now()}`);
 try{
  await page.route(`**/api/v1/games/${owner.match_id}/seats/*/ready`,route=>route.fulfill({status:503,contentType:'application/json',body:JSON.stringify({error:{code:'save_failed',message:'Save failed. Try again.'}})}),{times:1});
  await page.route(`**/api/v1/games/${owner.match_id}`,route=>route.request().method()==='GET'?route.fulfill({status:503,contentType:'application/json',body:JSON.stringify({error:{code:'offline',message:'Connection interrupted. Try again.'}})}):route.continue());
  await page.getByRole('button',{name:'Ready',exact:true}).click();await expect(page.locator('#room-error')).toContainText('Connection interrupted');await expect(page.locator('#room-leave')).toBeEnabled();
  await page.unroute(`**/api/v1/games/${owner.match_id}`);
  await page.route('**/save',route=>route.fulfill({status:503,contentType:'application/json',body:JSON.stringify({error:{code:'save_failed',message:'Save failed. Try again.'}})}),{times:1});
  await page.locator('#room-leave').click();await expect(page.locator('#room-panel')).toBeVisible();await expect(page.locator('#room-error')).toContainText('Save failed');await expect(page.locator('#room-leave')).toBeEnabled();
  await page.locator('#room-leave').click();await expect(page.locator('#saved-games-panel')).toBeVisible();
 }finally{await remove(context,owner)}
});

test('a peer joining preserves lobby drafts and focus while retiring the consumed invite',async({page,browser,context},info)=>{
 const owner=await create(page,`Live council ${Date.now()}`),guest=await browser.newContext({baseURL:info.project.use.baseURL});
 try{
  await page.getByRole('button',{name:'Invite player',exact:true}).click();const invite=await page.getByLabel('Invitation for Friend 1',{exact:true}).inputValue();
  const own=page.locator('[data-seat]').filter({hasText:'Alice · You'});await own.getByText('Edit kingdom',{exact:true}).click();await own.getByLabel('Kingdom name',{exact:true}).fill('Unfinished draft');await own.getByRole('button',{name:'Save kingdom',exact:true}).focus();
  const friend=await guest.newPage();await friend.goto(invite);await friend.locator('#join-panel').getByLabel('Your kingdom name',{exact:true}).fill('Bob');await friend.getByRole('button',{name:'Join this game',exact:true}).click();
  await expect(page.locator('#room-panel')).toContainText('Bob');await expect(own.getByLabel('Kingdom name',{exact:true})).toHaveValue('Unfinished draft');await expect(own.getByRole('button',{name:'Save kingdom',exact:true})).toBeFocused();await expect(page.getByRole('button',{name:'Copy invite link',exact:true})).toHaveCount(0);
  await own.getByRole('button',{name:'Save kingdom',exact:true}).click();await expect(page.locator('#room-panel')).toContainText('Unfinished draft · You');
 }finally{await guest.close();await remove(context,owner)}
});

test('a failed battlefield load keeps the started lobby available for retry',async({page,context})=>{
 const owner=await create(page,`Battlefield retry ${Date.now()}`,0);
 try{
  await page.getByRole('button',{name:'Ready',exact:true}).click();await expect(page.getByRole('button',{name:'Start game',exact:true})).toBeEnabled();
  await page.route('**/snapshot',route=>route.fulfill({status:503,contentType:'application/json',body:JSON.stringify({error:{code:'offline',message:'The battlefield could not load. Try again.'}})}),{times:1});
  await page.getByRole('button',{name:'Start game',exact:true}).click();
  await expect(page.locator('#room-panel')).toBeVisible();await expect(page.locator('#room-error')).toContainText('could not load');
  await page.getByRole('button',{name:'Return to battlefield',exact:true}).click();await expect(page.locator('#start-dialog')).toBeHidden();await expect(page.locator('#selected-name')).toHaveText('Town Center');
 }finally{await remove(context,owner)}
});

test('starts with an empty friend seat and lets its player join the running world',async({page,context,browser},info)=>{
 const owner=await create(page,`Join later ${Date.now()}`),guest=await browser.newContext({baseURL:info.project.use.baseURL});
 try{
  await expect(page.locator('#room-start-reason')).toContainText('Ready is optional');
  await page.getByRole('button',{name:'Start game',exact:true}).click();await expect(page.locator('#start-dialog')).toBeHidden();
  await expect(page.locator('#paused')).toBeHidden();await expect.poll(async()=>(await gameRead<Snapshot>(context,owner,'/snapshot')).tick).toBeGreaterThan(0);
  await page.getByRole('button',{name:'Match menu',exact:true}).click();await page.getByRole('button',{name:'Players & game management',exact:true}).click();
  await page.getByRole('button',{name:'Invite player',exact:true}).click();const invite=await page.getByLabel('Invitation for Friend 1',{exact:true}).inputValue();
  const before=await gameRead<Snapshot>(context,owner,'/snapshot');expect(before.paused).toBe(false);
  const friend=await guest.newPage();await friend.goto(invite);await friend.locator('#join-panel').getByLabel('Your kingdom name',{exact:true}).fill('Bob');
  const joined=friend.waitForResponse(r=>r.url().endsWith('/invites/claim'));await friend.getByRole('button',{name:'Join this game',exact:true}).click();const member=await (await joined).json() as MemberSession;
  await friend.getByRole('button',{name:'Return to battlefield',exact:true}).click();await expect(friend.locator('#selected-name')).toHaveText('Town Center');await expect(friend.locator('#paused')).toBeHidden();
  const view=await gameRead<Snapshot>(guest,member,'/snapshot');expect(view.player.id).toBe(2);expect(view.player.name).toBe('Bob');expect(view.tick).toBeGreaterThanOrEqual(before.tick);expect(view.entities.filter(e=>e.owner===2&&e.type==='villager')).toHaveLength(3);
 }finally{await guest.close();await remove(context,owner)}
});
