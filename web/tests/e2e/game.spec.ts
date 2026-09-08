import { test, expect, battlefieldKey } from './fixtures';

test('renders the Three.js world and reconnects to the same Go match', async ({ page, game }, info) => {
  const session = await game.start();
  const initial = await game.snapshot();
  expect(initial.player.civilization).toBe('britons');
  await expect(page.locator('#res-food')).toHaveText(String(initial.player.resources.food));
  await expect(page.locator('#population')).toHaveText(`${initial.player.population} / ${initial.player.capacity}`);
  const graphics = await page.locator('#world canvas').evaluate((canvas: HTMLCanvasElement) => {
    const gl = canvas.getContext('webgl2');
    return { webgl2: !!gl, lost: gl?.isContextLost(), width: canvas.width, height: canvas.height };
  });
  expect(graphics.webgl2).toBe(true);
  expect(graphics.lost).toBe(false);
  expect(graphics.width).toBeGreaterThan(500);
  expect(graphics.height).toBeGreaterThan(200);
  await page.screenshot({ path: info.outputPath('battlefield.png') });
  await page.getByRole('button', { name: 'Pause match', exact: true }).click();
  await expect(page.locator('#paused')).toBeVisible();
  await page.screenshot({ path: info.outputPath('battlefield-paused.png') });
  const read = page.waitForResponse(r => r.request().method() === 'GET' && new URL(r.url()).pathname === `/api/v1/games/${session.match_id}/snapshot`);
  await page.reload();
  expect((await read).status()).toBe(200);
  await expect(page.locator('#start-dialog')).toBeHidden();
  await expect(page.locator('#paused')).toBeVisible();
  expect((await game.snapshot()).player.resources).toEqual(initial.player.resources);
  await page.getByRole('button', { name: 'Resume battle', exact: true }).click();
  await expect(page.locator('#paused')).toBeHidden();
});

test('queues and cancels production through the server', async ({ page, game }) => {
  await game.start();
  const food = (await game.snapshot()).player.resources.food;
  const train = page.locator('#actions').getByRole('button', { name: /Villager/ });
  await game.command('train', () => train.click());
  await expect(page.locator('#queue button')).toHaveCount(1);
  await expect(page.locator('#res-food')).toHaveText(String(food - 50));
  expect((await game.snapshot()).entities.find(e => e.type === 'town_center' && e.owner === 1)?.tasks).toHaveLength(1);
  await game.command('cancel', () => page.locator('#queue button').click());
  await expect(page.locator('#queue button')).toHaveCount(0);
  await expect(page.locator('#res-food')).toHaveText(String(food));
});

test('moves selected villagers using a canvas gesture and backend positions', async ({ page, game }) => {
  await game.start();
  await battlefieldKey(page, '.');
  await expect(page.locator('#selection-count')).toHaveText('3 selected');
  const before = await game.snapshot();
  await page.locator('#actions').getByRole('button', { name: /^.*Move Command$/ }).click();
  await expect(page.locator('#mode-hint')).toContainText('Move:');
  const canvas = await page.locator('#world canvas').boundingBox();
  if (!canvas) throw new Error('No battlefield bounds.');
  const response = await game.command('move', () => page.mouse.click(canvas.x + canvas.width * .64, canvas.y + canvas.height * .48));
  const command = response.request().postDataJSON();
  expect(command.entity_ids).toHaveLength(3);
  expect(command.position).toBeDefined();
  await expect(page.locator('#mode-hint')).toBeHidden();
  await expect.poll(async () => {
    const current = await game.snapshot();
    return current.entities.some(e => command.entity_ids.includes(e.id) && before.entities.some(b => b.id === e.id && Math.hypot(e.position.x - b.position.x, e.position.y - b.position.y) > .2));
  }).toBe(true);
});

test('preserves contextual keyboard cancellation and ignores held pause repeats', async ({ page, game }) => {
  await game.start();
  await battlefieldKey(page, '.');
  await page.getByRole('button', { name: 'Build', exact: true }).click();
  await page.locator('#actions').getByRole('button', { name: /House/ }).click();
  await expect(page.locator('#mode-hint')).toBeVisible();
  await page.keyboard.press('Escape');
  await expect(page.locator('#mode-hint')).toBeHidden();
  await page.getByRole('button', { name: 'Orders', exact: true }).click();
  await page.locator('#actions').getByRole('button', { name: /Delete/ }).click();
  await game.command('delete', () => page.keyboard.press('Delete'));
  await expect(page.locator('#idle-count')).toHaveText('0');
  await page.locator('#world canvas').focus();
  await game.command('pause', () => page.keyboard.down('Space'));
  await expect(page.locator('#paused')).toBeVisible();
  await page.keyboard.down('Space');
  await page.keyboard.up('Space');
  expect((await game.snapshot()).paused).toBe(true);
  await game.command('pause', () => page.keyboard.press('Space'));
  await expect(page.locator('#paused')).toBeHidden();
});

test('validates and places a building from the canvas through Go', async ({ page, game }) => {
  await game.start();
  await battlefieldKey(page, '.');
  const before = await game.snapshot();
  const builder = before.entities.find(e => e.owner === 1 && e.type === 'villager')!;
  const cost = builder.actions.find(a => a.kind === 'build' && a.product === 'house')!.cost;
  await page.getByRole('button', { name: 'Build', exact: true }).click();
  await page.locator('#actions').getByRole('button', { name: /House/ }).click();
  const canvas = await page.locator('#world canvas').boundingBox();
  if (!canvas) throw new Error('No battlefield bounds.');
  let site: { x: number; y: number } | undefined;
  for (const [x, y] of [[.65, .6], [.6, .35], [.5, .65], [.35, .55], [.7, .45]]) {
    const check = page.waitForResponse(r => r.url().endsWith('/placement'));
    const point = { x: canvas.x + canvas.width * x, y: canvas.y + canvas.height * y };
    await page.mouse.move(point.x, point.y);
    const response = await check;
    expect(response.status()).toBe(200);
    if ((await response.json()).valid) { site = point; break; }
  }
  expect(site, 'a visible free site near the starting villagers').toBeDefined();
  await game.command('build', () => page.mouse.click(site!.x, site!.y));
  await expect(page.locator('#mode-hint')).toBeHidden();
  const after = await game.snapshot();
  expect(after.player.resources.wood).toBe(before.player.resources.wood - cost.wood);
  expect(after.entities.some(e => e.owner === 1 && e.type === 'house' && e.progress < 1)).toBe(true);
});

for (const viewport of [{ width: 800, height: 600 }, { width: 390, height: 640 }]) {
  test(`keeps population, selection and queue cancellation usable at ${viewport.width}x${viewport.height}`, async ({ page, game }, info) => {
    await page.setViewportSize(viewport);
    await game.start();
    await expect(page.locator('#population')).toBeInViewport();
    await expect(page.locator('#selected-health')).toBeInViewport();
    await game.command('train', () => page.locator('#actions').getByRole('button', { name: /Villager/ }).click());
    const cancel = page.locator('#queue button');
    await expect(cancel).toBeInViewport({ ratio: 1 });
    // A real click also checks clipping, overlays, and pointer hit testing.
    await game.command('cancel', () => cancel.click());
    await expect(page.locator('#queue button')).toHaveCount(0);
    const overflow = await page.evaluate(() => document.documentElement.scrollWidth > window.innerWidth);
    expect(overflow).toBe(false);
    await page.screenshot({ path: info.outputPath('compact-battlefield.png') });
  });
}

test('recovers from a failed catalog request with the persistent retry control', async ({ page, game }) => {
  await page.route('**/api/v1/catalog', route => route.fulfill({ status: 503, contentType: 'application/json', body: JSON.stringify({ error: { code: 'unavailable', message: 'The catalog is temporarily unavailable.' } }) }), { times: 1 });
  await page.goto('/');
  await expect(page.locator('#startup-error')).toBeVisible();
  await page.getByRole('button', { name: 'Try again', exact: true }).click();
  await expect(page.locator('#startup-error')).toBeHidden();
  await game.start();
});

test('shows match results and clears control groups when starting again', async ({ page, game }) => {
  await game.start();
  await battlefieldKey(page, '.');
  await battlefieldKey(page, 'Control+1');
  await page.getByRole('button', { name: 'Match menu', exact: true }).click();
  await game.command('resign', () => page.getByRole('button', { name: 'Resign this battle' }).click());
  await expect(page.locator('#result-dialog')).toBeVisible();
  await expect(page.locator('#result-title')).toHaveText('Your banner has fallen.');
  await page.getByRole('button', { name: 'Begin another chapter' }).click();
  await game.start();
  await battlefieldKey(page, '1');
  await expect(page.locator('#selection-count')).toHaveText('No selection');
});
