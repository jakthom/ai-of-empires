import type { Command, MemberSession, Snapshot } from '../../src/api.generated';
import { test, expect, battlefieldKey, type Game } from './fixtures';
import { opening, build, select } from './economy-helpers';

test('posts funded offers, preserves drafts, cancels stock, and displays finite merchant quotes',async({page,game},info)=>{
  test.setTimeout(90_000);
  await opening(page,game);
  await build(page,game,'mill','Mill');await build(page,game,'lumber_camp','Lumber Camp');
  await battlefieldKey(page,'h');await game.command('age',()=>page.locator('#actions').getByRole('button',{name:/Feudal Age/}).click());
  await expect.poll(async()=>(await game.snapshot()).player.age).toBe(1);
  await build(page,game,'market','Market');
  await page.getByRole('button',{name:'Open marketplace',exact:true}).click();
  const dialog=page.getByRole('dialog',{name:'Marketplace',exact:true});await expect(dialog).toBeVisible();
  const before=(await game.snapshot()).player.resources;
  await dialog.getByLabel('Offered amount per lot').fill('100');
  await dialog.getByLabel('Requested amount per lot').fill('80');
  await dialog.getByLabel('Lots',{exact:true}).fill('3');
  await game.command('market_post',()=>dialog.getByRole('button',{name:'Post offer',exact:true}).click());
  await expect(dialog.locator('#market-offers')).toContainText('3 lots available');
  await expect.poll(async()=>(await game.snapshot()).player.resources.wood).toBe(before.wood-300);
  await dialog.getByLabel('Offered amount per lot').fill('137');
  // A live update must not overwrite a partially edited order.
  await expect.poll(async()=>(await game.snapshot()).tick).toBeGreaterThan((await game.snapshot()).tick);
  await expect(dialog.getByLabel('Offered amount per lot')).toHaveValue('137');
  await page.screenshot({path:info.outputPath('marketplace-offers-desktop.png')});
  await page.setViewportSize({width:320,height:740});
  await expect(dialog.getByRole('button',{name:'Cancel offer'})).toBeVisible();
  const bounds=await dialog.boundingBox();expect(bounds!.x).toBeGreaterThanOrEqual(0);expect(bounds!.x+bounds!.width).toBeLessThanOrEqual(320);
  expect(await dialog.evaluate(el=>el.scrollWidth<=el.clientWidth)).toBe(true);
  await page.screenshot({path:info.outputPath('marketplace-offers-mobile.png')});
  await game.command('market_cancel',()=>dialog.getByRole('button',{name:'Cancel offer'}).click());
  await expect(dialog.locator('#market-offers')).toContainText('No standing offers');
  await expect.poll(async()=>(await game.snapshot()).player.resources.wood).toBe(before.wood);
  await page.setViewportSize({width:1440,height:960});
  await dialog.getByRole('button',{name:'Merchants',exact:true}).click();
  await expect(dialog.getByRole('button',{name:/Buy wood/})).toContainText('130 gold → 100 wood');
  await game.command('market_buy',()=>dialog.getByRole('button',{name:/Buy wood/}).click());
  await expect(dialog.getByRole('button',{name:/Buy wood/})).toContainText('141 gold → 100 wood');
  await expect(dialog.locator('.merchant-resource').filter({has:page.getByRole('heading',{name:'wood',exact:true})})).toContainText('900 in stock');
  await page.screenshot({path:info.outputPath('marketplace-merchants.png')});
  await dialog.getByRole('button',{name:'Resources',exact:true}).click();
  await expect(dialog.getByRole('row').filter({hasText:'Desert'})).toContainText('220%');
  await page.screenshot({path:info.outputPath('marketplace-regions.png')});
});

test('two human kingdoms settle a repeating caravan through public marketplace controls',async({page,game,browser},info)=>{
  test.setTimeout(150_000);
  await page.goto('/');
  await page.getByRole('combobox',{name:'World type',exact:true}).selectOption('plains');
  await page.locator('.advanced-world summary').click();
  await page.getByRole('combobox',{name:'Map reveal',exact:true}).selectOption('all');
  await page.getByRole('combobox',{name:'Friend seats',exact:true}).selectOption('1');
  await game.start('sandbox','peaceful',2);
  await page.getByRole('button',{name:'Match menu',exact:true}).click();
  await page.getByRole('button',{name:'Players & game management',exact:true}).click();
  await page.getByRole('button',{name:'Invite player',exact:true}).click();
  const invite=await page.getByLabel('Invitation for Friend 1',{exact:true}).inputValue();
  const guest=await browser.newContext({baseURL:info.project.use.baseURL,viewport:{width:1440,height:960}});
  try {
    const friend=await guest.newPage();const errors:string[]=[];friend.on('pageerror',e=>errors.push(e.message));
    await friend.goto(invite);await friend.locator('#join-panel').getByLabel('Your kingdom name',{exact:true}).fill('Stonehaven');
    const claimed=friend.waitForResponse(r=>r.url().endsWith('/invites/claim'));
    await friend.getByRole('button',{name:'Join this game',exact:true}).click();
    const member=await (await claimed).json() as MemberSession;
    await friend.getByRole('button',{name:'Return to battlefield',exact:true}).click();
    await page.getByRole('button',{name:'Return to battlefield',exact:true}).click();
    const friendGame:Game={...game,
      snapshot:async()=>{const r=await guest.request.get(`/api/v1/games/${member.match_id}/snapshot`,{headers:{Authorization:`Bearer ${member.token}`}});expect(r.ok()).toBe(true);return r.json() as Promise<Snapshot>},
      command:async(kind,action)=>{const pending=friend.waitForResponse(r=>r.request().method()==='POST'&&r.url().endsWith('/commands')&&(r.request().postDataJSON() as Command).kind===kind);await action();const r=await pending;expect(r.status(),await r.text()).toBe(200);return r;},
    };
    for(const speed of ['3.4×','8×','16×','32×']){await game.command('speed',()=>page.locator('#speed').click());await expect(page.locator('#speed')).toHaveText(speed)}
    for(const [p,g] of [[page,game],[friend,friendGame]] as const){
      if(await p.getByRole('button',{name:'Understood'}).isVisible())await p.getByRole('button',{name:'Understood'}).click();
      await battlefieldKey(p,'.');await battlefieldKey(p,'Control+1');await battlefieldKey(p,'h');
      await build(p,g,'mill','Mill');await build(p,g,'lumber_camp','Lumber Camp');
      await battlefieldKey(p,'h');await g.command('age',()=>p.locator('#actions').getByRole('button',{name:/Feudal Age/}).click());
      await expect.poll(async()=>(await g.snapshot()).player.age).toBe(1);
      await build(p,g,'market','Market');await build(p,g,'house','House');
    }
    await select(page,game,'market');await game.command('train',()=>page.locator('#actions').getByRole('button',{name:/Trade Cart/}).click());
    await expect.poll(async()=>(await game.snapshot()).entities.some(e=>e.owner===1&&e.type==='trade_cart')).toBe(true);
    const sellerBefore=(await friendGame.snapshot()).player.resources,buyerBefore=(await game.snapshot()).player.resources;
    await friend.getByRole('button',{name:'Open marketplace',exact:true}).click();
    const seller=friend.getByRole('dialog',{name:'Marketplace',exact:true});
    await seller.locator('#offer-want-resource').selectOption('stone');await seller.getByLabel('Requested amount per lot').fill('50');await seller.getByLabel('Lots',{exact:true}).fill('2');
    await friendGame.command('market_post',()=>seller.getByRole('button',{name:'Post offer',exact:true}).click());
    await page.getByRole('button',{name:'Open marketplace',exact:true}).click();
    const buyer=page.getByRole('dialog',{name:'Marketplace',exact:true});
    await expect(buyer.locator('#market-offers')).toContainText('Stonehaven');
    await buyer.getByLabel('Repeat trips while the offer and your funds last').check();
    await game.command('market_accept',()=>buyer.getByRole('button',{name:'Send caravan',exact:true}).click());
    await buyer.getByRole('button',{name:'Caravans',exact:true}).click();
    await expect.poll(async()=>{const ships=(await game.snapshot()).marketplace.shipments;return ships.length===2&&ships.every(s=>s.state==='delivered')},{timeout:35_000}).toBe(true);
    const sellerAfter=(await friendGame.snapshot()).player.resources,buyerAfter=(await game.snapshot()).player.resources;
    expect(sellerAfter.wood).toBe(sellerBefore.wood-200);expect(sellerAfter.stone).toBe(sellerBefore.stone+100);
    expect(buyerAfter.wood).toBe(buyerBefore.wood+200);expect(buyerAfter.stone).toBe(buyerBefore.stone-100);
    await expect(buyer.locator('#market-caravans')).toContainText('Delivered');
    await page.screenshot({path:info.outputPath('marketplace-caravans-delivered.png')});
    expect(errors).toEqual([]);
  } finally {await guest.close()}
});
