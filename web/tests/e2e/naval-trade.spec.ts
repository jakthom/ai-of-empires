import { test, expect, battlefieldKey, projectOpening } from './fixtures';
import { opening, build, select } from './economy-helpers';

test('escorts a funded Trade Ship between Docks through the public marketplace',async({page,game},info)=>{
  test.setTimeout(100_000);
  await opening(page,game,'islands');
  await build(page,game,'dock','Dock',true);
  await build(page,game,'mill','Mill');
  await battlefieldKey(page,'h');
  await game.command('age',()=>page.locator('#actions').getByRole('button',{name:/Feudal Age/}).click());
  await expect.poll(async()=>(await game.snapshot()).player.age).toBe(1);
  await build(page,game,'house','House');
  await select(page,game,'dock');
  await game.command('train',()=>page.locator('#actions').getByRole('button',{name:/Trade Ship/}).click());
  await game.command('train',()=>page.locator('#actions').getByRole('button',{name:/Galley/}).click());
  await expect.poll(async()=>(await game.snapshot()).entities.some(e=>e.owner===1&&e.type==='galley')).toBe(true);
  await game.command('speed',()=>page.locator('#speed').click());
  await expect(page.locator('#speed')).toHaveText('1×');
  const ready=await game.snapshot(),galley=ready.entities.find(e=>e.owner===1&&e.type==='galley')!,ship=ready.entities.find(e=>e.owner===1&&e.type==='trade_ship')!;
  await page.getByRole('button',{name:'Open marketplace',exact:true}).click();
  const dialog=page.getByRole('dialog',{name:'Marketplace',exact:true});
  await dialog.getByRole('button',{name:'Merchants',exact:true}).click();
  const before=await game.snapshot(),remote=before.marketplace.markets.find(m=>m.routes.some(q=>q.mode==='sell'&&q.product==='wood'&&q.can_start&&q.cart_id===ship.id))!;
  expect(remote,'a connected neutral Dock publishes a funded sea route').toBeTruthy();
  expect(before.entities.find(e=>e.id===remote.merchant.market_id)?.type).toBe('dock');
  await dialog.getByLabel('Merchant market',{exact:true}).selectOption(String(remote.merchant.region_id));
  const trade=await game.command('trade',()=>dialog.getByRole('button',{name:/Export wood/}).click());
  expect(trade.request().postDataJSON()).toMatchObject({entity_ids:[ship.id],target_id:remote.merchant.market_id});
  const shipment=(await game.snapshot()).marketplace.shipments.at(-1)!;
  await dialog.getByRole('button',{name:'Close marketplace',exact:true}).click();
  // Move loaded cargo into clear water using a normal order. The shoreline
  // can hide boats beside their Dock from this camera; resume preserves cargo.
  const chooseFromLog=async(name:string,id:number,group:string)=>{
    if(await page.getByRole('button',{name:'Expand event log',exact:true}).isVisible())await page.getByRole('button',{name:'Expand event log',exact:true}).click();
    await page.getByRole('searchbox',{name:'Search event log'}).fill(`${name} #${id}`);
    await page.getByRole('button',{name:`Locate ${name} #${id}`,exact:true}).first().click();
    await battlefieldKey(page,`Control+${group}`);
    await page.getByRole('button',{name:'Collapse event log',exact:true}).click();
    await battlefieldKey(page,'h');await battlefieldKey(page,group);
    await page.getByRole('button',{name:'Orders',exact:true}).click();
  };
  await chooseFromLog('Trade Ship',ship.id,'2');
  const coastal=await game.snapshot(),home=coastal.entities.find(e=>e.type==='town_center'&&e.owner===1)!.position;
  let rally:{x:number;y:number}|undefined,rallyPoint:{x:number;y:number}|undefined;
  for(const x of [4,8,12,16])for(const y of [-18,-16,-12,3,5,7]){
    if(rally)continue;
    const p={x:home.x+x,y:home.y+y};
    if(coastal.map.tiles[Math.floor(p.y)*coastal.map.width+Math.floor(p.x)].terrain!=='water'||coastal.entities.some(e=>Math.hypot(e.position.x-p.x,e.position.y-p.y)<1))continue;
    const point=await projectOpening(page,coastal,p);
    if(point.x>1050&&point.x<1370&&point.y>220&&point.y<630){rally=p;rallyPoint=point}
  }
  expect(rally,'an open-water rendezvous visible from home').toBeTruthy();
  await page.locator('#actions').getByRole('button',{name:/^Move /}).click();
  await game.command('move',()=>page.mouse.click(rallyPoint!.x,rallyPoint!.y));
  for(const speed of ['1.7×','3.4×','8×','16×','32×']){await game.command('speed',()=>page.locator('#speed').click());await expect(page.locator('#speed')).toHaveText(speed)}
  await expect.poll(async()=>(await game.snapshot()).entities.find(e=>e.id===ship.id)?.state).toBe('idle');
  await game.command('speed',()=>page.locator('#speed').click());await expect(page.locator('#speed')).toHaveText('1×');
  await chooseFromLog('Galley',galley.id,'3');
  await page.locator('#actions').getByRole('button',{name:/^Guard /}).click();
  const waiting=await game.snapshot(),target=await projectOpening(page,waiting,waiting.entities.find(e=>e.id===ship.id)!.position,.25);
  const escort=await game.command('guard',()=>page.mouse.click(target.x,target.y));
  expect(escort.request().postDataJSON()).toMatchObject({target_id:ship.id});
  await expect(page.locator('#selected-status')).toHaveText('Guarding');
  await page.screenshot({path:info.outputPath('guard-trade-ship.png')});
  await page.getByRole('button',{name:'Open marketplace',exact:true}).click();
  await dialog.getByRole('button',{name:'Caravans',exact:true}).click();
  await game.command('market_resume',()=>dialog.getByRole('button',{name:'Resume caravan',exact:true}).click());
  await dialog.getByRole('button',{name:'Close marketplace',exact:true}).click();
  for(const speed of ['1.7×','3.4×','8×','16×','32×']){await game.command('speed',()=>page.locator('#speed').click());await expect(page.locator('#speed')).toHaveText(speed)}
  await expect.poll(async()=>(await game.snapshot()).marketplace.shipments.find(s=>s.id===shipment.id)?.state,{timeout:25_000}).toBe('delivered');
  const delivered=await game.snapshot();
  expect(delivered.player.resources.wood).toBe(before.player.resources.wood-100);
  expect(delivered.player.resources.gold).toBe(before.player.resources.gold+shipment.terms.give_amount);
  expect(delivered.entities.find(e=>e.id===galley.id)?.guard_target).toBe(ship.id);
  await page.getByRole('button',{name:'Open marketplace',exact:true}).click();
  await dialog.getByRole('button',{name:'Caravans',exact:true}).click();
  await expect(dialog.locator('#market-caravans')).toContainText('Delivered');
  await page.screenshot({path:info.outputPath('naval-trade-delivered.png')});
});
