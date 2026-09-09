import { test, expect, battlefieldKey } from './fixtures';

test('searches old events by exact identity, combines categories, and locates the entity without an order', async ({ page, game }, info) => {
  await game.start();
  await battlefieldKey(page, '.');
  for (let i = 0; i < 38; i++) await game.command('stop', () => page.locator('#actions').getByRole('button', { name: /Stop/ }).click());
  await battlefieldKey(page, 'h');
  await page.getByRole('button', { name: 'Expand event log', exact: true }).click();
  await page.getByRole('searchbox', { name: 'Search event log' }).fill('vILlaGer 3');
  await page.getByRole('combobox', { name: 'Event category' }).selectOption('world');
  const rows = page.locator('#event-entries .log-entry');
  await expect(rows).toHaveCount(1);
  await expect(rows).toContainText('Villager created');
  await expect(page.locator('#log-status')).toHaveText('Matching events · full history');
  const before = await game.snapshot();
  const minimap = await page.locator('#minimap').screenshot();
  const orders: string[] = [];
  page.on('request', request => { if (request.method() === 'POST' && request.url().endsWith('/commands')) orders.push(request.url()); });
  await page.getByRole('button', { name: 'Locate Villager #3', exact: true }).click();
  await expect(page.locator('#selected-name')).toHaveText('Villager');
  await expect(page.locator('#entity-log-title')).toHaveText('Villager #3');
  await expect(page.locator('#history-tab')).toHaveAttribute('aria-pressed', 'true');
  await expect.poll(async () => (await page.locator('#minimap').screenshot()).equals(minimap)).toBe(false);
  expect((await game.snapshot()).entities.find(e => e.id === 3)?.position).toEqual(before.entities.find(e => e.id === 3)?.position);
  expect(orders).toEqual([]);
  await page.screenshot({ path: info.outputPath('search-and-locate.png') });
  await page.getByRole('combobox', { name: 'Event category' }).selectOption('combat');
  await expect(rows).toHaveCount(0);
  await expect(page.locator('#log-feedback')).toContainText('No matching events');
  await page.getByRole('button', { name: 'Clear log filters' }).click();
  await expect(page.getByRole('searchbox', { name: 'Search event log' })).toHaveValue('');
  await expect(page.getByRole('combobox', { name: 'Event category' })).toHaveValue('');
  await expect(page.locator('#event-entries')).toContainText('Order: stop');
});

test('live filters receive matching events and retain the recorded location of removed entities', async ({ page, game }) => {
  await game.start();
  await page.getByRole('button', { name: 'Expand event log', exact: true }).click();
  await page.getByRole('searchbox', { name: 'Search event log' }).fill('villager 3');
  await expect(page.locator('#event-entries .log-entry')).toHaveCount(1);
  await page.getByRole('button', { name: 'Locate Villager #3', exact: true }).click();
  await page.getByRole('button', { name: 'Orders', exact: true }).click();
  await page.getByRole('combobox', { name: 'Event category' }).selectOption('orders');
  await expect(page.locator('#log-feedback')).toContainText('No matching events');
  await game.command('stop', () => page.locator('#actions').getByRole('button', { name: /Stop/ }).click());
  await expect(page.locator('#event-entries')).toContainText('Order: stop');
  await page.getByRole('combobox', { name: 'Event category' }).selectOption('');
  await page.locator('#actions').getByRole('button', { name: /Delete/ }).click();
  await game.command('delete', () => page.getByRole('button', { name: 'Confirm removal', exact: true }).click());
  await expect(page.locator('#event-entries')).toContainText('Removed from the battlefield');
  await page.getByRole('button', { name: 'Locate Villager #3', exact: true }).last().click();
  await expect(page.locator('#entity-log-title')).toHaveText('Villager #3');
  await expect(page.locator('#entity-log-status')).toHaveText('Destroyed');
  await expect(page.locator('#notice')).toContainText('recorded location');
  expect((await game.snapshot()).entities.some(e => e.id === 3)).toBe(false);
});

test('changing search discards a delayed response for the previous query', async ({ page, game }) => {
  await game.start();
  let release!: () => void, requested!: () => void;
  const gate = new Promise<void>(resolve => { release = resolve; });
  const pending = new Promise<void>(resolve => { requested = resolve; });
  await page.route('**/log?*', async route => {
    if (new URL(route.request().url()).searchParams.get('q') === 'tree') { requested(); await gate; }
    await route.continue();
  });
  try {
    await page.getByRole('button', { name: 'Expand event log', exact: true }).click();
    const search = page.getByRole('searchbox', { name: 'Search event log' });
    await search.fill('tree'); await search.press('Enter'); await pending;
    await search.fill('villager 3'); await search.press('Enter');
    await expect(page.locator('#event-entries .log-entry')).toHaveCount(1);
    await expect(page.locator('#event-entries')).toContainText('Villager #3');
    const response = page.waitForResponse(r => new URL(r.url()).searchParams.get('q') === 'tree');
    release(); await response;
    await expect(page.locator('#event-entries')).toContainText('Villager #3');
    await expect(page.locator('#event-entries')).not.toContainText('Tree');
  } finally { release(); }
});

for (const viewport of [{ width: 800, height: 600 }, { width: 390, height: 640 }, { width: 320, height: 640 }]) {
  test(`search and difficulty remain usable at ${viewport.width}×${viewport.height}`, async ({ page, game }, info) => {
    await page.setViewportSize(viewport);
    await game.start();
    await expect(page.locator('#match-difficulty')).toHaveText('Peaceful practice');
    await expect(page.locator('#match-difficulty')).toBeInViewport({ ratio: 1 });
    await expect(page.locator('#speed')).toBeInViewport({ ratio: 1 });
    await page.getByRole('button', { name: 'Expand event log', exact: true }).click();
    const search = page.getByRole('searchbox', { name: 'Search event log' });
    await search.fill('villager 3'); await search.press('Enter');
    await expect(page.getByRole('button', { name: 'Locate Villager #3', exact: true })).toBeInViewport({ ratio: 1 });
    expect((await page.locator('#event-entries').boundingBox())!.height).toBeGreaterThan(24);
    expect((await page.locator('#world').boundingBox())!.height).toBeGreaterThanOrEqual(120);
    await expect(page.locator('.battlefield-controls')).toHaveCount(0);
    expect(await page.evaluate(() => document.documentElement.scrollWidth > innerWidth)).toBe(false);
    await page.screenshot({ path: info.outputPath('compact-search.png') });
  });
}
