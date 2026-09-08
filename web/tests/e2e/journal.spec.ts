import { test, expect, battlefieldKey } from './fixtures';
import type { EventPage } from '../../src/api.generated';
import type { Page } from '@playwright/test';

async function openGlobalHistory(page: Page, name = 'Town Center #1') {
  if (await page.locator('#log-toggle').getAttribute('aria-expanded') === 'false') await page.locator('#log-toggle').click();
  await page.getByRole('button', { name: `View history of ${name}`, exact: true }).last().click();
}

test('streams actions into a tray that resizes and collapses to one line', async ({ page, game }, info) => {
  await game.start();
  const tray = page.locator('#event-tray');
  expect((await tray.boundingBox())!.height).toBe(35);
  await game.command('train', () => page.locator('#actions').getByRole('button', { name: /Villager/ }).click());
  await expect(page.locator('#activity')).toContainText(/Villager/);
  await page.getByRole('button', { name: 'Expand event log', exact: true }).click();
  await expect(page.locator('#event-entries')).toContainText('Queued Villager');
  const handle = page.getByRole('separator', { name: 'Resize event log' });
  const start = (await handle.boundingBox())!;
  const before = (await tray.boundingBox())!.height;
  await page.mouse.move(start.x + start.width / 2, start.y + start.height / 2);
  await page.mouse.down();
  await page.mouse.move(start.x + start.width / 2, start.y - 90, { steps: 8 });
  await page.mouse.up();
  expect((await tray.boundingBox())!.height).toBeGreaterThan(before + 80);
  const world = (await page.locator('#world').boundingBox())!;
  const deck = (await page.locator('.command-deck').boundingBox())!;
  expect(world.y + world.height).toBeCloseTo(deck.y, 0);
  expect(deck.y + deck.height).toBeCloseTo((await tray.boundingBox())!.y, 0);
  await page.screenshot({ path: info.outputPath('expanded-event-tray.png') });
  await handle.focus();
  await page.keyboard.press('Home');
  expect((await tray.boundingBox())!.height).toBe(35);
  await expect(page.locator('#event-log-body')).toBeHidden();
  await page.keyboard.press('ArrowUp');
  expect((await tray.boundingBox())!.height).toBe(67);
  await page.getByRole('button', { name: 'Collapse event log', exact: true }).click();
  expect((await tray.boundingBox())!.height).toBe(35);
});

test('shows backend activity and a worker history that survives removal and reload', async ({ page, game, request }, info) => {
  const session = await game.start();
  await game.command('speed', () => page.locator('#speed').click());
  await page.mouse.click(629, 382);
  await expect(page.locator('#selected-name')).toHaveText('Villager');
  await page.getByRole('button', { name: /Give order/ }).click();
  const order = await game.command('interact', () => page.mouse.click(675, 527));
  const workerID = order.request().postDataJSON().entity_ids[0] as number;
  await expect(page.locator('#selected-status')).toContainText('Logging');
  await openGlobalHistory(page, `Villager #${workerID}`);
  await expect(page.locator('#log-title')).toHaveText(`Villager #${workerID}`);
  await expect(page.locator('#log-status')).toContainText('Logging');
  await expect(page.locator('#event-entries')).toContainText('Logging');
  await expect.poll(async () => {
    const response = await request.get(`/api/v1/matches/${session.match_id}/entities/${workerID}/history`, { headers: { Authorization: `Bearer ${session.token}` } });
    const history = await response.json() as EventPage;
    return history.events.some(e => e.kind === 'delivery' && e.resource === 'wood');
  }, { timeout: 16_000 }).toBe(true);
  await expect(page.locator('#event-entries')).toContainText(/Delivered .* wood/);
  await page.screenshot({ path: info.outputPath('worker-history.png') });
  await page.locator('#actions').getByRole('button', { name: /Delete/ }).click();
  await game.command('delete', () => page.getByRole('button', { name: 'Confirm removal', exact: true }).click());
  await expect(page.locator('#log-status')).toHaveText('Destroyed');
  await expect(page.locator('#event-entries')).toContainText('Removed from the battlefield');
  await page.reload();
  await page.getByRole('button', { name: 'Expand event log', exact: true }).click();
  await page.getByRole('button', { name: `View history of Villager #${workerID}`, exact: true }).last().click();
  await expect(page.locator('#log-status')).toHaveText('Destroyed');
  await expect(page.locator('#event-entries')).toContainText('Villager created');
  await expect(page.locator('#event-entries')).toContainText(/Delivered .* wood/);
});

test('pages older history without live updates moving the reader', async ({ page, game }) => {
  await game.start();
  await battlefieldKey(page, '.');
  const stop = page.locator('#actions').getByRole('button', { name: /Stop/ });
  for (let i = 0; i < 38; i++) await game.command('stop', () => stop.click());
  await page.getByRole('button', { name: 'Expand event log', exact: true }).click();
  await page.getByRole('button', { name: 'All events', exact: true }).click();
  const older = page.getByRole('button', { name: 'Older', exact: true });
  await expect(older).toBeEnabled();
  await older.click();
  await expect(page.locator('#log-follow')).toHaveText('Follow live');
  await expect(page.locator('#event-entries')).toContainText('Town Center created');
  const ids = await page.locator('.log-entry').evaluateAll(rows => rows.map(row => (row as HTMLElement).dataset.eventId));
  await game.command('stop', () => stop.click());
  await expect(page.locator('#activity')).toContainText('Order: stop');
  expect(await page.locator('.log-entry').evaluateAll(rows => rows.map(row => (row as HTMLElement).dataset.eventId))).toEqual(ids);
  await page.getByRole('button', { name: 'Follow live', exact: true }).click();
  await expect(page.locator('#log-follow')).toHaveText('Live');
  await expect(page.locator('.log-entry').last()).toContainText('Order: stop');
  const current = await page.locator('.log-entry').evaluateAll(rows => rows.map(row => (row as HTMLElement).dataset.eventId));
  expect(new Set(current).size).toBe(current.length);
});

test('recovers a failed history read without losing the game', async ({ page, game }) => {
  await game.start();
  await page.route('**/entities/*/history?*', route => route.fulfill({ status: 503, contentType: 'application/json', body: JSON.stringify({ error: { code: 'unavailable', message: 'History is temporarily unavailable.' } }) }), { times: 1 });
  await openGlobalHistory(page);
  await expect(page.locator('#log-feedback')).toContainText('History unavailable');
  await page.getByRole('button', { name: 'Retry', exact: true }).click();
  await expect(page.locator('#event-entries')).toContainText('Town Center created');
  await expect(page.locator('#log-feedback')).toBeHidden();
});

test('keeps entity status current while reading history and ignores unrelated new events', async ({ page, game }) => {
  await game.start();
  await openGlobalHistory(page);
  await expect(page.locator('#event-entries')).toContainText('Town Center created');
  await page.locator('#event-entries').focus();
  await expect(page.locator('#log-follow')).toHaveText('Follow live');
  const original = await page.locator('.log-entry').evaluateAll(rows => rows.map(row => (row as HTMLElement).dataset.eventId));
  await battlefieldKey(page, '.');
  await game.command('stop', () => page.locator('#actions').getByRole('button', { name: /Stop/ }).click());
  await expect(page.locator('#activity')).toContainText('Order: stop');
  await expect(page.locator('#log-newer')).toBeDisabled();
  expect(await page.locator('.log-entry').evaluateAll(rows => rows.map(row => (row as HTMLElement).dataset.eventId))).toEqual(original);
  await battlefieldKey(page, 'h');
  await page.locator('#actions').getByRole('button', { name: /Delete/ }).click();
  await game.command('delete', () => page.getByRole('button', { name: 'Confirm removal', exact: true }).click());
  await expect(page.locator('#log-status')).toHaveText('Destroyed');
  await expect(page.locator('#log-follow')).toHaveText('Follow live');
  expect(await page.locator('.log-entry').evaluateAll(rows => rows.map(row => (row as HTMLElement).dataset.eventId))).toEqual(original);
  await page.getByRole('button', { name: 'Newer', exact: true }).click();
  await expect(page.locator('#event-entries')).toContainText('Removed from the battlefield');
});

test('an in-flight live append preserves the page when the reader scrolls back', async ({ page, game }) => {
  await game.start();
  await page.getByRole('button', { name: 'Expand event log', exact: true }).click();
  await expect(page.locator('#event-entries')).toContainText('Your settlers await');
  let release!: () => void, requested!: () => void;
  const gate = new Promise<void>(resolve => { release = resolve; });
  const pending = new Promise<void>(resolve => { requested = resolve; });
  await page.route('**/log?*', async route => {
    if (new URL(route.request().url()).searchParams.has('after')) { requested(); await gate; }
    await route.continue();
  });
  try {
    await battlefieldKey(page, '.');
    await game.command('stop', () => page.locator('#actions').getByRole('button', { name: /Stop/ }).click());
    await pending;
    await page.locator('#event-entries').hover();
    await page.mouse.wheel(0, -150);
    await expect(page.locator('#log-follow')).toHaveText('Follow live');
    const ids = await page.locator('.log-entry').evaluateAll(rows => rows.map(row => (row as HTMLElement).dataset.eventId));
    const top = await page.locator('#event-entries').evaluate(list => list.scrollTop);
    expect(top).toBeGreaterThan(0);
    release();
    await expect(page.locator('#log-newer')).toBeEnabled();
    expect(await page.locator('.log-entry').evaluateAll(rows => rows.map(row => (row as HTMLElement).dataset.eventId))).toEqual(ids);
    expect(await page.locator('#event-entries').evaluate(list => list.scrollTop)).toBeCloseTo(top, 0);
  } finally { release(); }
});

test('offers 32× speed and advances the authoritative clock faster', async ({ page, game }) => {
  await game.start();
  await page.getByRole('button', { name: 'Match menu', exact: true }).click();
  await game.command('speed', () => page.getByRole('combobox', { name: 'Game speed', exact: true }).selectOption('32'));
  await page.getByRole('button', { name: 'Close menu', exact: true }).click();
  await expect(page.locator('#speed')).toHaveText('32×');
  const before = await game.snapshot();
  expect(before.speed).toBe(32);
  await expect.poll(async () => (await game.snapshot()).time - before.time, { timeout: 3000 }).toBeGreaterThan(20);
  await expect(page.locator('#activity')).toContainText('32×');
  await page.getByRole('button', { name: 'Expand event log', exact: true }).click();
  await expect(page.locator('#event-entries')).toContainText('Game speed set to 32×');
});

for (const viewport of [{ width: 800, height: 600 }, { width: 390, height: 640 }]) {
  test(`keeps entity activity and the event tray usable at ${viewport.width}×${viewport.height}`, async ({ page, game }, info) => {
    await page.setViewportSize(viewport);
    await game.start();
    await expect(page.locator('#selected-status')).toBeInViewport({ ratio: 1 });
    await expect(page.locator('#speed')).toBeInViewport({ ratio: 1 });
    await openGlobalHistory(page);
    await expect(page.locator('#event-entries')).toContainText('Town Center created');
    await expect(page.locator('#log-follow')).toBeInViewport({ ratio: 1 });
    await expect(page.locator('#history-tab')).toBeInViewport({ ratio: 1 });
    expect((await page.locator('#world').boundingBox())!.height).toBeGreaterThanOrEqual(120);
    expect(await page.evaluate(() => document.documentElement.scrollWidth > innerWidth)).toBe(false);
    await page.screenshot({ path: info.outputPath('compact-event-tray.png') });
    await page.getByRole('button', { name: 'Collapse event log', exact: true }).click();
    expect((await page.locator('#event-tray').boundingBox())!.height).toBe(35);
  });
}
