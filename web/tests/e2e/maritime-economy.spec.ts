import type { Page } from '@playwright/test';
import type { Vec } from '../../src/api.generated';
import { test, expect, battlefieldKey, projectOpening, type Game } from './fixtures';

async function opening(page: Page, game: Game, world = 'plains') {
  await page.goto('/');
  await page.getByRole('combobox', { name: 'World type', exact: true }).selectOption(world);
  await page.getByRole('combobox', { name: 'Biome', exact: true }).selectOption('tropical');
  await page.locator('.advanced-world summary').click();
  await page.getByRole('combobox', { name: 'Map reveal', exact: true }).selectOption('all');
  await game.start('sandbox', 'peaceful', 1);
  for (const speed of ['3.4×','8×','16×','32×']) {
    await game.command('speed', () => page.locator('#speed').click());
    await expect(page.locator('#speed')).toHaveText(speed);
  }
  // Save the actual opening villagers through public control-group shortcuts.
  await battlefieldKey(page, '.');
  await battlefieldKey(page, 'Control+1');
  await battlefieldKey(page, 'h');
}

async function build(page: Page, game: Game, type: string, label: string, shore = false) {
  await battlefieldKey(page, '1');
  await page.getByRole('button', { name: 'Build', exact: true }).click();
  await page.locator('#actions').getByRole('button', { name: new RegExp(label) }).click();
  const snapshot = await game.snapshot(), home = snapshot.entities.find(e => e.type === 'town_center' && e.owner === 1)!.position;
  let candidates: Vec[] = [[-4,-3],[-5,3],[4,5],[-3,-6],[6,2],[-6,-3],[1,-6],[-5,6],[7,4],[-2,5]].map(([x,y]) => ({x:home.x+x,y:home.y+y}));
  if (shore) candidates = snapshot.map.tiles.flatMap((tile,i) => {
    const x=i%snapshot.map.width,y=Math.floor(i/snapshot.map.width), distance=Math.hypot(x+.5-home.x,y+.5-home.y);
    return tile.terrain === 'grass' && distance > 12 && distance < 16 && [-2,2,-2*snapshot.map.width,2*snapshot.map.width].some(offset => snapshot.map.tiles[i+offset]?.terrain === 'water') ? [{x:x+.5,y:y+.5}] : [];
  });
  let placed = false;
  for (const candidate of candidates) {
    const point = await projectOpening(page,snapshot,candidate);
    if (point.x<70 || point.x>1370 || point.y<205 || point.y>650) continue;
    const response = page.waitForResponse(r=>r.url().endsWith('/placement'));
    await page.mouse.move(point.x,point.y);
    if (!(await (await response).json()).valid) continue;
    await game.command('build',()=>page.mouse.click(point.x,point.y));
    placed = true; break;
  }
  expect(placed, `find an accepted ${label} site through placement previews`).toBe(true);
  await expect.poll(async () => (await game.snapshot()).entities.some(e=>e.type===type && e.owner===1 && e.progress===1),{timeout:15_000}).toBe(true);
  await game.command('stop',()=>battlefieldKey(page,'s'));
  await page.getByRole('button',{name:'Orders',exact:true}).click();
}

async function select(page: Page, game: Game, type: string) {
  const snapshot=await game.snapshot(), entity=snapshot.entities.find(e=>e.owner===1 && e.type===type)!;
  const point=await projectOpening(page,snapshot,entity.position,type==='fishing_ship'?.25:.45);
  await page.mouse.click(point.x,point.y);
  await expect(page.locator('#selected-name')).toHaveText(entity.name);
}

test('trades at quoted prices and explicitly reseeds a depleted farm', async ({page,game},info)=>{
  test.setTimeout(90_000);
  await opening(page,game);
  await build(page,game,'mill','Mill');
  await build(page,game,'lumber_camp','Lumber Camp');
  await battlefieldKey(page,'h');
  await game.command('age',()=>page.locator('#actions').getByRole('button',{name:/Feudal Age/}).click());
  await expect.poll(async()=>(await game.snapshot()).player.age).toBe(1);
  await build(page,game,'market','Market');
  await build(page,game,'house','House');
  await build(page,game,'farm','Farm');
  await select(page,game,'farm');
  await expect(page.locator('#actions').getByRole('button',{name:/Reseed farm/})).toHaveAttribute('aria-disabled','true');
  await select(page,game,'market');
  await page.getByRole('button',{name:'Trade',exact:true}).click();
  const sell = page.locator('#actions').getByRole('button',{name:/Sell wood/}), buy = page.locator('#actions').getByRole('button',{name:/Buy wood/});
  await expect(sell).toContainText('100W → 70G');
  await expect(buy).toContainText('130G → 100W');
  const before = (await game.snapshot()).player.resources;
  for(let i=0;i<Math.floor(before.wood/100);i++) await game.command('market_sell',()=>sell.click());
  await expect(page.locator('#res-wood')).toHaveText('40');
  await expect(sell).toHaveAttribute('aria-disabled','true');
  await game.command('market_buy',()=>buy.click());
  await expect(page.locator('#res-wood')).toHaveText('140');
  await game.command('market_sell',()=>sell.click());
  await expect(page.locator('#res-wood')).toHaveText('40');
  await page.setViewportSize({width:320,height:740});
  const tabs=page.locator('.action-tabs');
  for(const button of await tabs.getByRole('button').all()) {
    const box=await button.boundingBox(),bounds=await tabs.boundingBox();
    expect(box!.x+box!.width).toBeLessThanOrEqual(bounds!.x+bounds!.width+1);
  }
  await page.screenshot({path:info.outputPath('market-trade-mobile.png')});
  await page.setViewportSize({width:1440,height:960});
  await page.screenshot({path:info.outputPath('market-trade.png')});
  await battlefieldKey(page,'1');
  await battlefieldKey(page,'q');
  const farmPoint=await game.point('farm');
  await game.command('interact',()=>page.mouse.click(farmPoint.x,farmPoint.y));
  await expect.poll(async()=>((await game.snapshot()).entities.find(e=>e.type==='farm')?.amount ?? 0),{timeout:30_000}).toBe(0);
  await game.command('stop',()=>battlefieldKey(page,'s'));
  await select(page,game,'farm');
  const reseed=page.locator('#actions').getByRole('button',{name:/Reseed farm/});
  await expect(reseed).toHaveAttribute('aria-disabled','true');
  await reseed.focus();
  await expect(page.locator('#action-help')).toContainText('requires 60 wood');
  await page.screenshot({path:info.outputPath('depleted-farm.png')});
  await select(page,game,'market');
  await page.getByRole('button',{name:'Trade',exact:true}).click();
  await game.command('market_buy',()=>buy.click());
  await expect(page.locator('#res-wood')).toHaveText('140');
  await select(page,game,'farm');
  await expect(reseed).toHaveAttribute('aria-disabled','false');
  await game.command('reseed_farm',()=>reseed.click());
  await expect(page.locator('#res-wood')).toHaveText('80');
  await expect.poll(async()=>{const farm=(await game.snapshot()).entities.find(e=>e.type==='farm')!;return farm.progress===1 && farm.amount!>0 && farm.amount!<175;}).toBe(true);
  await page.screenshot({path:info.outputPath('reseeded-farm.png')});
});

test('builds a dock, trains a fishing ship and delivers a visible shoal as food',async({page,game},info)=>{
  test.setTimeout(60_000);
  await opening(page,game,'islands');
  await build(page,game,'dock','Dock',true);
  await select(page,game,'dock');
  // Send the new boat into open water so the raised bank and Dock do not
  // occlude it from the opening camera. This is a normal building rally order.
  const coastal=await game.snapshot(),home=coastal.entities.find(e=>e.type==='town_center' && e.owner===1)!.position;
  let rally: Vec|undefined, rallyPoint: Vec|undefined;
  for(const x of [4,8,12,16]) for(const y of [-18,-16,-12,3,5,7]) {
    if(rally) continue;
    const p={x:home.x+x,y:home.y+y};
    if(coastal.map.tiles[Math.floor(p.y)*coastal.map.width+Math.floor(p.x)].terrain!=='water' || coastal.entities.some(e=>Math.hypot(e.position.x-p.x,e.position.y-p.y)<1)) continue;
    const screen=await projectOpening(page,coastal,p);
    if(screen.x>1050 && screen.x<1370 && screen.y>220 && screen.y<630) {rally=p;rallyPoint=screen;}
  }
  if(!rally || !rallyPoint) throw new Error('No visible water rally site');
  await battlefieldKey(page,'q');
  await game.command('rally',()=>page.mouse.click(rallyPoint!.x,rallyPoint!.y));
  await game.command('train',()=>page.locator('#actions').getByRole('button',{name:/Fishing Ship/}).click());
  await expect.poll(async()=>{const ship=(await game.snapshot()).entities.find(e=>e.type==='fishing_ship');return !!ship && ship.state==='idle' && Math.hypot(ship.position.x-rally!.x,ship.position.y-rally!.y)<1;},{timeout:15_000}).toBe(true);
  await select(page,game,'fishing_ship');
  await expect(page.locator('#selected-name')).toHaveText('Fishing Ship');
  const before=await game.snapshot(),ship=before.entities.find(e=>e.type==='fishing_ship')!;
  const fish=before.entities.filter(e=>e.type==='fish').sort((a,b)=>Math.hypot(a.position.x-ship.position.x,a.position.y-ship.position.y)-Math.hypot(b.position.x-ship.position.x,b.position.y-ship.position.y));
  let target=fish[0],point=await projectOpening(page,before,target.position,.065);
  for(const candidate of fish) {
    const p=await projectOpening(page,before,candidate.position,.065);
    if(p.x>80 && p.x<1360 && p.y>205 && p.y<640) {target=candidate;point=p;break;}
  }
  await page.locator('#actions').getByRole('button',{name:/^.*Fish Command$/}).click();
  const response=await game.command('gather',()=>page.mouse.click(point.x,point.y));
  expect(response.request().postDataJSON().target_id).toBe(target.id);
  await expect(page.locator('#selected-status')).toContainText('Fishing');
  await expect.poll(async()=>(await game.snapshot()).entities.find(e=>e.id===target.id)?.amount).toBeLessThan(target.amount!);
  await expect.poll(async()=>(await game.snapshot()).player.resources.food,{timeout:15_000}).toBeGreaterThan(before.player.resources.food);
  await page.screenshot({path:info.outputPath('fishing-food-delivered.png')});
});
