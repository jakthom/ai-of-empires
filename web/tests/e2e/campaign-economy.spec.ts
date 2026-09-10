import { readFile } from 'node:fs/promises';
import { test, expect, battlefieldKey, projectOpening } from './fixtures';
import type { StatisticsReport } from '../../src/api.generated';
import { opening } from './economy-helpers';

test('shows resource consumption, private and world statistics, and a rich final report',async({page,game},info)=>{
  test.setTimeout(90_000);
  await page.goto('/');await page.locator('.advanced-world summary').click();
  await page.getByRole('combobox',{name:'Map reveal',exact:true}).selectOption('hidden');
  await game.start('sandbox','peaceful',2);
  const initial=await game.snapshot();
  const worker=await game.point('villager'),tree=await game.point('tree');
  await page.mouse.click(worker.x,worker.y);
  await game.command('interact',()=>page.mouse.click(tree.x,tree.y,{button:'right'}));
  for(let i=0;i<4;i++)await game.command('speed',()=>page.locator('#speed').click());
  await expect.poll(async()=>(await game.snapshot()).player.production.rates.wood,{timeout:12_000}).toBeGreaterThan(0);
  const next=await game.snapshot();
  expect(next.player.resources.food).toBeLessThan(initial.player.resources.food);
  expect(next.player.food_supply!.demand_per_minute).toBeGreaterThan(0);
  await expect(page.locator('#production-food .consumption-line')).toHaveAttribute('d',/M/);
  await page.locator('#open-statistics').click();
  const ledger=page.locator('#statistics-dialog');
  await expect(ledger.locator('figure[aria-label$="resource chart"]')).toHaveCount(4);
  await expect(ledger).toContainText('Population fed');
  const reportResponse=page.waitForResponse(r=>r.url().endsWith('/statistics?scope=world'));
  await ledger.getByLabel('Report scope').selectOption('world');
  const report=await(await reportResponse).json() as StatisticsReport;
  expect(report.kingdoms).toHaveLength(2);expect(report.awards).toHaveLength(7);
  await expect(ledger.getByLabel('Kingdom',{exact:true}).locator('option')).toHaveCount(2);
  await page.screenshot({path:info.outputPath('resource-ledger-desktop.png')});
  const download=page.waitForEvent('download');await ledger.getByRole('button',{name:'Download report',exact:true}).click();
  const contents=JSON.parse(await readFile((await(await download).path())!,'utf8')) as StatisticsReport;
  expect(contents.scope).toBe('world');expect(contents.kingdoms[0].consumed.food).toBeGreaterThan(0);
  await page.setViewportSize({width:390,height:844});
  await expect(ledger).toBeVisible();
  expect(await page.evaluate(()=>document.documentElement.scrollWidth>innerWidth)).toBe(false);
  await page.screenshot({path:info.outputPath('resource-ledger-mobile.png')});
  await ledger.getByRole('button',{name:'Close statistics'}).click();await page.setViewportSize({width:1440,height:960});
  await page.getByRole('button',{name:'Match menu',exact:true}).click();
  const reveal=page.waitForResponse(r=>r.url().endsWith('/observer'));
  await page.getByRole('button',{name:'Enable god mode',exact:true}).click();
  const god=await(await reveal).json();expect(god.god_mode).toBe(true);expect(god.map.fog.every((v:number)=>v===2)).toBe(true);
  await expect(page.locator('#god-banner')).toBeVisible();
  const ordinary=await game.snapshot();expect(ordinary.god_mode).toBeFalsy();expect(ordinary.map.fog.some(f=>f!==2)).toBe(true);
  await page.screenshot({path:info.outputPath('god-mode.png')});
  await page.getByRole('button',{name:'Return to kingdom view',exact:true}).click();await expect(page.locator('#god-banner')).toBeHidden();
  await page.getByRole('button',{name:'Match menu',exact:true}).click();
  await game.command('resign',()=>page.getByRole('button',{name:'Resign this battle'}).click());
  await expect(page.locator('#result-summary')).toContainText('Economic leader');
  await page.getByRole('button',{name:'Read the full campaign report'}).click();
  await expect(ledger).toContainText('Final report');await expect(ledger).toContainText('won the match');
  await expect(ledger.getByRole('rowheader',{name:/Economic leader/})).toBeVisible();
  await page.screenshot({path:info.outputPath('final-campaign-report.png')});
});

test('adjacent farms share grid edges and a replacement villager completes the whole batch',async({page,game},info)=>{
  test.setTimeout(60_000);await opening(page,game);
  await page.getByRole('button',{name:'Match menu',exact:true}).click();await game.command('speed',()=>page.locator('#game-speed').selectOption('1'));await page.getByRole('button',{name:'Close menu',exact:true}).click();
  const first=await game.point('villager',0),replacement=await game.point('villager',1);
  await page.mouse.click(replacement.x,replacement.y);await battlefieldKey(page,'Control+2');
  await page.mouse.click(first.x,first.y);await battlefieldKey(page,'Control+1');
  const before=await game.snapshot(),home=before.entities.find(e=>e.owner===1&&e.type==='town_center')!.position;
  const candidates=[];
  for(let y=-8;y<=8;y+=2)for(let x=-8;x<=4;x+=2){
    const sites=[0,2,4].map(dx=>({x:Math.round(home.x)+x+dx,y:Math.round(home.y)+y}));
    if(sites.some(p=>before.entities.some(e=>e.kind!=='unit'&&Math.abs(e.position.x-p.x)<e.radius+1.5&&Math.abs(e.position.y-p.y)<e.radius+1.5)))continue;
    if(sites.some(p=>[-1,0].some(dx=>[-1,0].some(dy=>before.map.tiles[(p.y+dy)*before.map.width+p.x+dx]?.terrain!=='grass'))))continue;
    const points=await Promise.all(sites.map(p=>projectOpening(page,before,p)));
    if(points.every(p=>p.x>80&&p.x<1360&&p.y>215&&p.y<650))candidates.push({sites,points});
  }
  expect(candidates.length).toBeGreaterThan(0);
  await page.getByRole('button',{name:'Build',exact:true}).click();await page.locator('#actions').getByRole('button',{name:/^Farm/}).click();await page.getByRole('button',{name:'Close building details'}).click();
  const choice=candidates[0];
  await page.keyboard.down('Shift');
  try {for(const point of choice.points){const preview=page.waitForResponse(r=>r.url().endsWith('/placement'));await page.mouse.move(point.x,point.y);expect((await(await preview).json()).valid).toBe(true);await game.command('build',()=>page.mouse.click(point.x,point.y));}}finally{await page.keyboard.up('Shift');}
  await battlefieldKey(page,'Escape');await game.command('stop',()=>battlefieldKey(page,'s'));
  const foundations=(await game.snapshot()).entities.filter(e=>e.type==='farm');expect(foundations).toHaveLength(3);
  expect(foundations.every(f=>Number.isInteger(f.position.x)&&Number.isInteger(f.position.y))).toBe(true);
  const ordered=foundations.sort((a,b)=>a.position.x-b.position.x);expect(ordered[1].position.x-ordered[0].position.x).toBe(2);expect(ordered[2].position.x-ordered[1].position.x).toBe(2);
  await battlefieldKey(page,'2');const target=await projectOpening(page,await game.snapshot(),ordered[0].position,.2);
  await game.command('interact',()=>page.mouse.click(target.x,target.y,{button:'right'}));
  await page.getByRole('button',{name:'Match menu',exact:true}).click();await game.command('speed',()=>page.locator('#game-speed').selectOption('32'));await page.getByRole('button',{name:'Close menu',exact:true}).click();
  await expect.poll(async()=>(await game.snapshot()).entities.filter(e=>e.type==='farm'&&e.progress===1).length,{timeout:15_000}).toBe(3);
  expect((await game.snapshot()).player.resources.wood).toBe(before.player.resources.wood-180);
  await page.screenshot({path:info.outputPath('adjacent-farms.png')});
});

test('an attack breaks peace immediately and a damage payment restores it',async({page,game},info)=>{
  test.setTimeout(60_000);
  await page.goto('/');await page.locator('.advanced-world summary').click();await page.getByRole('combobox',{name:'Map reveal',exact:true}).selectOption('all');
  await game.start('sandbox','peaceful',2);
  const initial=await game.snapshot(),scout=initial.entities.find(e=>e.owner===1&&e.type==='scout')!,target=initial.entities.find(e=>e.owner===2&&e.type==='town_center')!;
  const point=await projectOpening(page,initial,scout.position,.45);await page.mouse.click(point.x,point.y);await expect(page.locator('#selected-name')).toHaveText(scout.name);
  const map=(await page.locator('#minimap').boundingBox())!;
  await page.mouse.click(map.x+target.position.x/initial.map.width*map.width,map.y+target.position.y/initial.map.height*map.height);
  const enemy=await projectOpening(page,initial,target.position,1,target.position);
  await game.command('interact',()=>page.mouse.click(enemy.x,enemy.y,{button:'right'}));
  await expect.poll(async()=>(await game.snapshot()).opponents[0].relation).toBe('hostile');
  expect((await game.snapshot()).entities.find(e=>e.id===target.id)?.hp).toBe(target.hp);
  await page.locator('#relationships summary').click();
  await page.getByRole('button',{name:'Reparations & peace',exact:true}).click();
  await expect(page.locator('#diplomacy-dialog')).toContainText('AI acceptance threshold: 50 gold');
  await page.screenshot({path:info.outputPath('peace-payment.png')});
  await game.command('peace_offer',()=>page.getByRole('button',{name:'Offer reparations',exact:true}).click());
  await expect(page.locator('#peace-offers')).toContainText('accepted');
  await expect.poll(async()=>(await game.snapshot()).opponents[0].relation).toBe('peaceful');
  expect((await game.snapshot()).entities.find(e=>e.id===scout.id)?.state).toBe('idle');
});

test('the global market exposes demand and prepares a public buy or sell offer',async({page,game},info)=>{
  await game.start('sandbox','peaceful',1);
  await page.getByRole('button',{name:'Open marketplace'}).click();
  await page.getByRole('button',{name:'Global market',exact:true}).click();
  const market=page.locator('#market-global');await expect(market).toContainText('Demand');await expect(market).toContainText('Best bid');
  await page.screenshot({path:info.outputPath('global-market.png')});
  await page.getByRole('button',{name:'buy wood globally',exact:true}).click();
  await expect(page.locator('#offer-give-resource')).toHaveValue('gold');await expect(page.locator('#offer-want-resource')).toHaveValue('wood');await expect(page.locator('#offer-audience')).toHaveValue('0');
  await page.getByRole('button',{name:'Global market',exact:true}).click();await page.getByRole('button',{name:'sell stone globally',exact:true}).click();
  await expect(page.locator('#offer-give-resource')).toHaveValue('stone');await expect(page.locator('#offer-want-resource')).toHaveValue('gold');
});

test('draws a working bridge over an observed river through the build controls',async({page,game},info)=>{
  test.setTimeout(90_000);await opening(page,game,'rivers');
  const before=await game.snapshot(),map=before.map,home=before.entities.find(e=>e.owner===1&&e.type==='town_center')!.position;
  const candidates:{a:{x:number;y:number};b:{x:number;y:number};distance:number}[]=[];
  for(let y=3;y<map.height-3;y++)for(let x=2;x<map.width-12;x++) {
    if(map.tiles[y*map.width+x].terrain!=='grass'||map.tiles[y*map.width+x+1].terrain!=='water')continue;
    let end=x+1;while(end<map.width&&['water','shallows'].includes(map.tiles[y*map.width+end].terrain))end++;
    if(end-x<3||end-x>10||map.tiles[y*map.width+end]?.terrain!=='grass')continue;
    let a={x:x+.5,y:y+.5},b={x:end+.5,y:y+.5};if(Math.hypot(home.x-a.x,home.y-a.y)>Math.hypot(home.x-b.x,home.y-b.y))[a,b]=[b,a];
    candidates.push({a,b,distance:Math.hypot(home.x-a.x,home.y-a.y)});
  }
  candidates.sort((a,b)=>a.distance-b.distance);expect(candidates.length).toBeGreaterThan(0);
  await battlefieldKey(page,'1');await page.getByRole('button',{name:'Build',exact:true}).click();await page.locator('#actions').getByRole('button',{name:/^Bridge/}).click();
  await page.getByRole('button',{name:'Close building details'}).click();
  let placed=false;
  for(const candidate of candidates.slice(0,10)) {
    const center={x:(candidate.a.x+candidate.b.x)/2,y:candidate.a.y},mini=(await page.locator('#minimap').boundingBox())!;
    await page.mouse.click(mini.x+center.x/map.width*mini.width,mini.y+center.y/map.height*mini.height);
    const a=await projectOpening(page,before,candidate.a,0,center),b=await projectOpening(page,before,candidate.b,0,center);
    const response=page.waitForResponse(r=>r.url().endsWith('/commands')&&r.request().postDataJSON().product==='bridge');
    await page.mouse.move(a.x,a.y);await page.mouse.down();await page.mouse.move(b.x,b.y,{steps:12});await page.mouse.up();
    if((await response).ok()){placed=true;break;}
  }
  expect(placed,'accepted bank-to-bank bridge').toBe(true);
  await expect.poll(async()=>{const v=await game.snapshot();const bridge=v.entities.filter(e=>e.type==='bridge');return bridge.length>0&&bridge.every(e=>e.progress===1);},{timeout:30_000}).toBe(true);
  const after=await game.snapshot();expect(after.map.tiles.some(t=>t.bridge)).toBe(true);
  await expect(page.locator('#notice')).toBeHidden();
  await page.screenshot({path:info.outputPath('completed-river-bridge.png')});
});
