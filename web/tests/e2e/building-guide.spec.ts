import { test,expect,battlefieldKey } from './fixtures';
import { opening,build,select } from './economy-helpers';

test('explains building choices before placement and exposes farm work',async({page,game},info)=>{
  await opening(page,game);
  await battlefieldKey(page,'1');await page.getByRole('button',{name:'Build',exact:true}).click();
  const market=page.locator('#actions').getByRole('button',{name:/^Market /});
  await market.focus();
  await expect(page.locator('#building-guide')).toContainText('Buy scarce resources');
  await expect(page.locator('#building-guide')).toContainText('Trains: Trade Cart');
  await expect(page.locator('#building-guide')).toContainText('Feudal Age is required');
  await page.screenshot({path:info.outputPath('building-capabilities-desktop.png')});
  await build(page,game,'farm','Farm');
  await select(page,game,'farm');
  await game.command('work_farm',()=>page.locator('#actions').getByRole('button',{name:/^Assign farmer/}).click());
  await expect.poll(async()=>(await game.snapshot()).entities.some(e=>e.type==='villager'&&e.state==='gathering')).toBe(true);
  await battlefieldKey(page,'1');
  await expect(page.locator('#actions').getByRole('button',{name:/^Repair /})).toBeVisible();
  await page.setViewportSize({width:390,height:844});
  await page.getByRole('button',{name:'Build',exact:true}).click();
  await page.locator('#actions').getByRole('button',{name:/^House /}).click();
  await expect(page.locator('#building-guide')).toContainText('Population capacity +5');
  await page.screenshot({path:info.outputPath('building-capabilities-mobile.png')});
});
