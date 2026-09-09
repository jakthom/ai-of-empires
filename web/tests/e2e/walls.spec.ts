import { test, expect, battlefieldKey, projectOpening } from './fixtures';
import { opening, build } from './economy-helpers';

async function drag(page: import('@playwright/test').Page, from: { x: number; y: number }, to: { x: number; y: number }) {
  await page.mouse.move(from.x, from.y);
  await page.mouse.down();
  await page.mouse.move(to.x, to.y, { steps: 8 });
  await page.mouse.up();
}

async function pacedDrag(page: import('@playwright/test').Page, from: { x: number; y: number }, to: { x: number; y: number }) {
  await page.mouse.move(from.x, from.y);
  await page.mouse.down();
  for (let i = 1; i <= 8; i++) {
    await page.mouse.move(from.x + (to.x - from.x) * i / 8, from.y + (to.y - from.y) * i / 8);
    await page.waitForTimeout(25);
  }
  await page.mouse.up();
}

test('drags a Stone Wall through the public UI and keeps invalid lines atomic', async ({ page, game }) => {
  await opening(page, game);
  await build(page, game, 'mill', 'Mill');
  await build(page, game, 'lumber_camp', 'Lumber Camp');

  await battlefieldKey(page, 'h');
  await page.getByRole('button', { name: 'Orders', exact: true }).click();
  await game.command('age', () => page.locator('#actions').getByRole('button', { name: /Feudal Age/ }).click());
  await expect.poll(async () => (await game.snapshot()).player.age).toBe(1);

  await battlefieldKey(page, '1');
  await page.getByRole('button', { name: 'Build', exact: true }).click();
  await page.locator('#actions').getByRole('button', { name: /Stone Wall/ }).click();
  await expect(page.locator('#mode-hint')).toContainText('click and drag a line');

  const before = await game.snapshot();
  const home = before.entities.find(entity => entity.owner === 1 && entity.type === 'town_center')!.position;
  const candidates = [-8, -6, 6, 8].map(offset => ({
    start: { x: home.x - 8, y: home.y + offset },
    end: { x: home.x + 8, y: home.y + offset },
  }));
  let result: import('@playwright/test').Response | undefined;
  let start: { x: number; y: number } | undefined;
  let end: { x: number; y: number } | undefined;
  let placementRequests = 0;
  const observePlacement = (request: import('@playwright/test').Request) => {
    if (request.method() === 'POST' && request.url().endsWith('/placement')) placementRequests++;
  };
  page.on('request', observePlacement);
  for (const candidate of candidates) {
    start = await projectOpening(page, before, candidate.start);
    end = await projectOpening(page, before, candidate.end);
    const beforeRequests = placementRequests;
    const response = page.waitForResponse(r => r.request().method() === 'POST' && r.url().endsWith('/commands'));
    await pacedDrag(page, start, end);
    const attempt = await response;
    expect(placementRequests - beforeRequests, 'wall preview requests should be throttled during a drag').toBeLessThanOrEqual(4);
    if (attempt.status() === 200) { result = attempt; break; }
  }
  page.off('request', observePlacement);
  expect(result, 'find an unobstructed wall line').toBeDefined();
  const command = await result!.json() as { accepted: boolean; command_id: string };
  expect(command.accepted).toBe(true);
  const sent = result!.request().postDataJSON() as { kind: string; product: string; position: { x: number; y: number }; end_position?: { x: number; y: number } };
  expect(sent.kind).toBe('build');
  expect(sent.product).toBe('wall');
  expect(sent.end_position).toBeDefined();
  expect(sent.end_position!.x).not.toBe(sent.position.x);

  await expect.poll(async () => (await game.snapshot()).entities.filter(entity => entity.owner === 1 && entity.type === 'wall').length).toBeGreaterThan(2);
  const after = await game.snapshot();
  const walls = after.entities.filter(entity => entity.owner === 1 && entity.type === 'wall');
  expect(after.player.resources.stone).toBe(before.player.resources.stone - walls.length * 5);

  await page.getByRole('button', { name: 'Build', exact: true }).click();
  await page.locator('#actions').getByRole('button', { name: /Stone Wall/ }).click();
  const unchanged = await game.snapshot();
  const invalidResponse = page.waitForResponse(r => r.request().method() === 'POST' && r.url().endsWith('/commands'));
  await drag(page, start!, end!);
  const invalid = await invalidResponse;
  expect(invalid.status()).toBeGreaterThanOrEqual(400);
  const rejected = await game.snapshot();
  expect(rejected.entities.filter(entity => entity.owner === 1 && entity.type === 'wall')).toHaveLength(walls.length);
  expect(rejected.player.resources.stone).toBe(unchanged.player.resources.stone);
});
