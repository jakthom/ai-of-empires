import type { Page } from '@playwright/test';
import type { Command, MemberSession, Snapshot, Vec } from '../../src/api.generated';
import { test, expect, battlefieldKey, projectOpening, type Game } from './fixtures';
import { build, select } from './economy-helpers';

async function mapCenter(page:Page,snapshot:Snapshot,center:Vec) {
  const box=(await page.locator('#minimap').boundingBox())!;
  await page.mouse.click(box.x+center.x/snapshot.map.width*box.width,box.y+center.y/snapshot.map.height*box.height);
}

test('guards a worker and shows attacked buildings burn, recover and crumble',async({page,game,browser},info)=>{
  test.setTimeout(200_000);
  const renderErrors:string[]=[];
  page.on('console',message=>{if(message.type()==='error'&&message.text().startsWith('THREE.'))renderErrors.push(message.text())});
  await page.emulateMedia({reducedMotion:'no-preference'});
  await page.goto('/');
  await page.getByRole('combobox',{name:'World type',exact:true}).selectOption('plains');
  await page.locator('.advanced-world summary').click();
  await page.getByRole('combobox',{name:'Map reveal',exact:true}).selectOption('all');
  await page.getByRole('combobox',{name:'Friend seats',exact:true}).selectOption('1');
  await game.start('sandbox','peaceful',2);
  for(const speed of ['3.4×','8×','16×','32×']){await game.command('speed',()=>page.locator('#speed').click());await expect(page.locator('#speed')).toHaveText(speed)}
  await battlefieldKey(page,'.');await battlefieldKey(page,'Control+1');await battlefieldKey(page,'h');
  await build(page,game,'house','House');
  const initial=await game.snapshot(),worker=initial.entities.find(e=>e.owner===1&&e.type==='villager')!,house=initial.entities.find(e=>e.owner===1&&e.type==='house')!,home=initial.entities.find(e=>e.owner===1&&e.type==='town_center')!.position;
  await select(page,game,'scout');
  await battlefieldKey(page,'Control+2');
  await page.locator('#actions').getByRole('button',{name:/^Guard /}).click();
  const workerPoint=await projectOpening(page,await game.snapshot(),worker.position,.45);
  const guard=await game.command('guard',()=>page.mouse.click(workerPoint.x,workerPoint.y));
  expect(guard.request().postDataJSON()).toMatchObject({target_id:worker.id});
  await expect(page.locator('#selected-status')).toHaveText('Guarding');
  await page.screenshot({path:info.outputPath('guard-worker.png')});
  await game.command('stop',()=>battlefieldKey(page,'s'));
  await game.command('stance',()=>page.locator('#actions').getByRole('button',{name:/^Hold fire/}).click());
  await page.getByRole('button',{name:'Match menu',exact:true}).click();
  await page.getByRole('button',{name:'Players & game management',exact:true}).click();
  await page.getByRole('button',{name:'Invite player',exact:true}).click();
  const invite=await page.getByLabel('Invitation for Friend 1',{exact:true}).inputValue();
  const guest=await browser.newContext({baseURL:info.project.use.baseURL,viewport:{width:1440,height:960}});
  try {
    const friend=await guest.newPage();const errors:string[]=[];friend.on('pageerror',e=>errors.push(e.message));
    await friend.goto(invite);await friend.locator('#join-panel').getByLabel('Your kingdom name',{exact:true}).fill('Raiders');
    const claim=friend.waitForResponse(r=>r.url().endsWith('/invites/claim'));
    await friend.getByRole('button',{name:'Join this game',exact:true}).click();const member=await(await claim).json() as MemberSession;
    await friend.getByRole('button',{name:'Return to battlefield',exact:true}).click();
    await page.getByRole('button',{name:'Return to battlefield',exact:true}).click();
    if(await friend.getByRole('button',{name:'Understood'}).isVisible())await friend.getByRole('button',{name:'Understood'}).click();
    const other:Game={...game,
      snapshot:async()=>{const r=await guest.request.get(`/api/v1/games/${member.match_id}/snapshot`,{headers:{Authorization:`Bearer ${member.token}`}});expect(r.ok()).toBe(true);return r.json() as Promise<Snapshot>},
      command:async(kind,action)=>{const pending=friend.waitForResponse(r=>r.url().endsWith('/commands')&&r.request().method()==='POST'&&(r.request().postDataJSON() as Command).kind===kind);await action();const r=await pending;expect(r.status(),await r.text()).toBe(200);return r},
    };
    await select(friend,other,'scout');await other.command('stance',()=>friend.locator('#actions').getByRole('button',{name:/^Hold fire/}).click());
    await mapCenter(friend,await other.snapshot(),home);
    const target=await projectOpening(friend,await other.snapshot(),house.position,.65,home);
    const attack=()=>other.command('interact',()=>friend.mouse.click(target.x,target.y,{button:'right'}));
    await attack();
    await battlefieldKey(page,'h');await select(page,game,'house');
    await expect.poll(async()=>(await game.snapshot()).entities.find(e=>e.id===house.id)?.damage_stage??0,{timeout:45_000}).toBeGreaterThanOrEqual(2);
    await other.command('stop',()=>battlefieldKey(friend,'s'));
    await game.command('speed',()=>page.locator('#speed').click());await expect(page.locator('#speed')).toHaveText('1×');
    await expect(page.locator('#selected-health')).toContainText('Badly damaged');
    await page.screenshot({path:info.outputPath('burning-house-desktop.png')});
    await page.setViewportSize({width:390,height:740});
    await expect(page.locator('#selected-health')).toContainText('Badly damaged');
    await page.screenshot({path:info.outputPath('burning-house-mobile.png')});
    await page.setViewportSize({width:1440,height:960});
    // The attacker can be wounded too. Both damage and movement remain real
    // game orders; a visible foreign unit exposes condition, not private plans.
    await battlefieldKey(page,'2');
    const enemy=(await game.snapshot()).entities.find(e=>e.owner===member.player_id&&e.type==='scout')!;
    const enemyPoint=await projectOpening(page,await game.snapshot(),enemy.position,.85);
    await game.command('interact',()=>page.mouse.click(enemyPoint.x,enemyPoint.y,{button:'right'}));
    await expect.poll(async()=>(await game.snapshot()).entities.find(e=>e.id===enemy.id)?.damage_stage??0,{timeout:25_000,intervals:[100]}).toBeGreaterThanOrEqual(2);
    await game.command('stop',()=>battlefieldKey(page,'s'));
    const wounded=(await other.snapshot()).entities.find(e=>e.id===enemy.id)!;
    await friend.locator('#actions').getByRole('button',{name:/^Move /}).click();
    const escapePoint=await projectOpening(friend,await other.snapshot(),{x:wounded.position.x+4,y:wounded.position.y+1},0,home);
    await other.command('move',()=>friend.mouse.click(escapePoint.x,escapePoint.y));
    await expect.poll(async()=>{const e=(await game.snapshot()).entities.find(e=>e.id===enemy.id)!;return Math.hypot(e.position.x-wounded.position.x,e.position.y-wounded.position.y)}).toBeGreaterThan(.5);
    await battlefieldKey(page,'+');
    await page.screenshot({path:info.outputPath('wounded-raider.png')});
    await battlefieldKey(page,'r');
    await other.command('stop',()=>battlefieldKey(friend,'s'));
    // Repairs reverse the same observed condition; the browser does not heal it.
    await battlefieldKey(page,'1');await battlefieldKey(page,'h');await battlefieldKey(page,'1');
    await game.command('stop',()=>battlefieldKey(page,'s'));
    await select(page,game,'house');
    await game.command('repair_building',()=>page.locator('#actions').getByRole('button',{name:/^Repair building/}).click());
    for(const speed of ['1.7×','3.4×','8×','16×','32×']){await game.command('speed',()=>page.locator('#speed').click());await expect(page.locator('#speed')).toHaveText(speed)}
    await expect.poll(async()=>(await game.snapshot()).entities.find(e=>e.id===house.id)?.damage_stage??0).toBe(0);
    await battlefieldKey(page,'1');await game.command('stop',()=>battlefieldKey(page,'s'));
    await select(page,game,'house');await expect(page.locator('#selected-health')).not.toContainText('damaged');
    await page.screenshot({path:info.outputPath('repaired-house.png')});
    await attack();
    await expect.poll(async()=>(await game.snapshot()).entities.find(e=>e.id===house.id)?.hp??0,{timeout:40_000,intervals:[100]}).toBeLessThan(30);
    await game.command('speed',()=>page.locator('#speed').click());await expect(page.locator('#speed')).toHaveText('1×');
    await expect.poll(async()=>(await game.snapshot()).effects.some(e=>e.type==='house'&&e.owner===1),{timeout:25_000,intervals:[100]}).toBe(true);
    await page.screenshot({path:info.outputPath('house-collapse.png')});
    await expect.poll(async()=>{const v=await game.snapshot(),remains=v.effects.find(e=>e.type==='house'&&e.owner===1);return !!remains&&v.time-remains.started_at>1}).toBe(true);
    await page.screenshot({path:info.outputPath('house-rubble.png')});
    expect(errors).toEqual([]);
    expect(renderErrors).toEqual([]);
  } finally {await guest.close()}
});
