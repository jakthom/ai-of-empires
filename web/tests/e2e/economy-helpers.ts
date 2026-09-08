import type { Page } from '@playwright/test';
import type { Vec } from '../../src/api.generated';
import { expect, battlefieldKey, projectOpening, type Game } from './fixtures';

export async function opening(page: Page, game: Game, world = 'plains') {
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

export async function build(page: Page, game: Game, type: string, label: string, shore = false) {
  await battlefieldKey(page, '1');
  await page.getByRole('button', { name: 'Build', exact: true }).click();
  await page.locator('#actions').getByRole('button', { name: new RegExp(label) }).click();
  const snapshot = await game.snapshot(), home = snapshot.entities.find(e => e.type === 'town_center' && e.owner === 1)!.position;
  let candidates: Vec[] = [[-4,-3],[-5,3],[4,5],[-3,-6],[6,2],[-6,-3],[1,-6],[-5,6],[7,4],[-2,5],[-8,1],[3,-8],[8,-2],[-7,-6],[6,-6],[0,8]].map(([x,y]) => ({x:home.x+x,y:home.y+y}));
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

export async function select(page: Page, game: Game, type: string) {
  const snapshot=await game.snapshot(), entity=snapshot.entities.find(e=>e.owner===1 && e.type===type)!;
  const point=await projectOpening(page,snapshot,entity.position,type==='fishing_ship'?.25:.45);
  await page.mouse.click(point.x,point.y);
  await expect(page.locator('#selected-name')).toHaveText(entity.name);
}
