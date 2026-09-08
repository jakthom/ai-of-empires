import { test, expect } from './fixtures';
import { writeFile } from 'node:fs/promises';

test('sets up six settlements and safely changes to a solo map', async ({ page, game }, info) => {
  await game.start('skirmish', 'peaceful', 6);
  const six = await game.snapshot();
  expect(six.settlements).toBe(6); expect(six.opponents).toHaveLength(5); expect(six.map.width).toBe(108);
  await expect(page.locator('#rival-name')).toHaveText('5 other kingdoms');
  await page.getByRole('button', { name: 'Match menu', exact: true }).click();
  await page.getByRole('button', { name: 'Start a new match', exact: true }).click();
  await game.start('skirmish', 'peaceful', 1);
  const solo = await game.snapshot(); expect(solo.opponents).toHaveLength(0); expect(solo.status).toBe('running'); expect(solo.map.width).toBe(72);
  await expect(page.locator('#rival-name')).toHaveText('Solo settlement');
  await page.screenshot({ path: info.outputPath('solo-settlement.png') });
});

for (const width of [390, 800]) {
  test(`sets up and resumes a campaign at ${width}px`, async ({ page, game }, info) => {
    await page.setViewportSize({ width, height: 640 }); await page.goto('/');
    await expect(page.locator('#start-dialog')).toBeVisible();
    await page.getByRole('combobox', { name: 'Settlements', exact: true }).scrollIntoViewIfNeeded();
    await page.screenshot({ path: info.outputPath('compact-setup.png') });
    await game.start('sandbox', 'peaceful', 4, `A long evening campaign ${width} ${info.project.name} ${Date.now()}`);
    await page.getByRole('button', { name: 'Match menu', exact: true }).click();
    await expect(page.getByRole('button', { name: 'Save now', exact: true })).toBeVisible();
    await page.getByRole('button', { name: 'Save and leave', exact: true }).click();
    await expect(page.locator('#saved-games-panel')).toBeVisible();
    const name = page.locator('#saved-games-list .saved-game strong').filter({ hasText: `A long evening campaign ${width}` });
    await expect(name).toHaveCount(1);
    const row = name.locator('..').locator('..');
    await row.getByRole('button', { name: /^Resume / }).scrollIntoViewIfNeeded();
    await page.screenshot({ path: info.outputPath('compact-saved-games.png') });
    const overflow = await page.locator('#start-dialog').evaluate(d => d.scrollWidth > d.clientWidth);
    expect(overflow).toBe(false);
    await row.getByRole('button', { name: /^Resume / }).click();
    await expect(page.locator('#start-dialog')).toBeHidden();
    expect((await game.snapshot()).settlements).toBe(4);
    await page.getByRole('button', { name: 'Match menu', exact: true }).click();
    await game.command('resign', () => page.getByRole('button', { name: 'Resign this battle' }).click());
    await expect(page.locator('#result-title')).toHaveText('Your banner has fallen.');
  });
}

test('saves a named game and resumes its queues and history by name and ID', async ({ page, game }, info) => {
  const name = `Evening kingdom ${info.project.name} ${Date.now()}`;
  const seat = await game.start('sandbox', 'peaceful', 3, name);
  await game.command('train', () => page.locator('#actions').getByRole('button', { name: /Villager/ }).click());
  await game.command('pause', () => page.getByRole('button', { name: 'Pause match', exact: true }).click());
  const before = await game.snapshot();
  await page.getByRole('button', { name: 'Match menu', exact: true }).click();
  await expect(page.locator('#session-name')).toHaveText(name);
  await expect(page.getByLabel('Session ID', { exact: true })).toHaveValue(seat.match_id);
  const saving = page.waitForResponse(r => r.url().endsWith('/save'));
  await page.getByRole('button', { name: 'Save now', exact: true }).click();
  expect((await saving).status()).toBe(200);
  await expect(page.locator('#save-status')).toContainText('Saved');
  await page.getByRole('button', { name: 'Save and leave', exact: true }).click();
  await expect(page.locator('#saved-games-panel')).toBeVisible();
  await page.getByLabel('Game name or session ID', { exact: true }).fill(name);
  await expect(page.locator('#saved-games-list .saved-game')).toHaveCount(1);
  await page.screenshot({ path: info.outputPath('saved-games.png') });
  await page.getByRole('button', { name: 'Resume by name or ID', exact: true }).click();
  await expect(page.locator('#start-dialog')).toBeHidden();
  await expect(page.locator('#paused')).toBeVisible();
  const after = await game.snapshot();
  expect(after).toEqual(before);
  await page.getByRole('button', { name: 'Match menu', exact: true }).click();
  await page.getByRole('button', { name: 'Save and leave', exact: true }).click();
  await page.getByLabel('Game name or session ID', { exact: true }).fill(seat.match_id);
  await page.getByRole('button', { name: 'Resume by name or ID', exact: true }).click();
  await expect(page.locator('#start-dialog')).toBeHidden();
  expect((await game.snapshot()).tick).toBe(before.tick);
});

test('closes the page and reconnects from browser storage with the same checkpoint', async ({ page, game }) => {
  const seat = await game.start();
  await game.command('pause', () => page.getByRole('button', { name: 'Pause match', exact: true }).click());
  const before = await game.snapshot(), context = page.context();
  await page.close();
  const reopened = await context.newPage();
  const read = reopened.waitForResponse(r => new URL(r.url()).pathname === `/api/v1/matches/${seat.match_id}`);
  await reopened.goto('/'); expect((await read).status()).toBe(200);
  await expect(reopened.locator('#start-dialog')).toBeHidden();
  await expect(reopened.locator('#paused')).toBeVisible();
  expect((await game.snapshot()).tick).toBe(before.tick);
  await reopened.close();
});

test('reconnects the battlefield after browser back navigation', async ({ page, game }) => {
  const seat = await game.start();
  const before = await game.snapshot();
  await page.goto('about:blank');
  const restored = page.waitForResponse(r => new URL(r.url()).pathname === `/api/v1/matches/${seat.match_id}`);
  await page.goBack(); expect((await restored).status()).toBe(200);
  await expect(page.locator('#connection')).toBeHidden();
  await expect(page.locator('#start-dialog')).toBeHidden();
  await expect.poll(async () => (await game.snapshot()).tick).toBeGreaterThan(before.tick);
  await game.command('pause', () => page.getByRole('button', { name: 'Pause match', exact: true }).click());
  await expect(page.locator('#paused')).toBeVisible();
});

test('reports periodic autosaves and keeps the game open when saving fails', async ({ page, game }) => {
  await game.start();
  await page.getByRole('button', { name: 'Match menu', exact: true }).click();
  const initial = await page.locator('#save-status').textContent();
  await expect.poll(() => page.locator('#save-status').textContent(), { timeout: 15_000 }).not.toBe(initial);
  await page.route('**/leave', route => route.fulfill({ status: 503, contentType: 'application/json', body: JSON.stringify({ error: { code: 'save_failed', message: 'Save failed. Try again.' } }) }), { times: 1 });
  await page.getByRole('button', { name: 'Save and leave', exact: true }).click();
  await expect(page.locator('#menu-dialog')).toBeVisible(); await expect(page.locator('#start-dialog')).toBeHidden();
  await expect(page.locator('#menu-dialog #save-error')).toContainText('Save failed');
  await expect(page.locator('#notice')).toContainText('Save failed');
  await page.getByRole('button', { name: 'Save and leave', exact: true }).click();
  await expect(page.locator('#saved-games-panel')).toBeVisible();
  await page.getByLabel('Game name or session ID', { exact: true }).fill('missing kingdom');
  await page.getByRole('button', { name: 'Resume by name or ID', exact: true }).click();
  await expect(page.locator('#sessions-error')).toContainText('No saved game');
});

test('shows the building type on hover without issuing an order', async ({ page, game }, info) => {
  await game.start();
  const bounds = await page.locator('#world canvas').boundingBox(); if (!bounds) throw new Error('No battlefield');
  const commands: string[] = []; page.on('request', r => { if (r.url().endsWith('/commands')) commands.push(r.method()); });
  // Home centers the visible Town Center. Hover its roof above the ground anchor.
  await page.mouse.move(bounds.x + bounds.width / 2, bounds.y + bounds.height / 2 - 20);
  await expect(page.getByRole('tooltip')).toContainText('Town Center #');
  await page.screenshot({ path: info.outputPath('building-hover.png') });
  await page.mouse.move(2, 2); await expect(page.getByRole('tooltip')).toBeHidden();
  expect(commands).toEqual([]);
});

test('releases browser render state when switching campaigns', async ({ page, game }, info) => {
  test.setTimeout(90_000);
  await game.start();
  const cdp = await page.context().newCDPSession(page); await cdp.send('Performance.enable');
  const measure = async () => {
    await cdp.send('HeapProfiler.collectGarbage');
    const { metrics } = await cdp.send('Performance.getMetrics');
    return Object.fromEntries(metrics.filter((m: { name: string }) => ['JSHeapUsedSize', 'Nodes', 'Documents', 'TaskDuration'].includes(m.name)).map((m: { name: string; value: number }) => [m.name, m.value]));
  };
  const samples = [];
  for (let i = 0; i < 7; i++) {
    await page.getByRole('button', { name: 'Match menu', exact: true }).click();
    await page.getByRole('button', { name: 'Start a new match', exact: true }).click();
    await game.start('skirmish', 'peaceful', i % 2 ? 2 : 6);
    samples.push(await measure());
  }
  // Compare equally sized, warmed maps, after garbage collection.
  expect(samples[6].JSHeapUsedSize).toBeLessThan(samples[2].JSHeapUsedSize * 1.35 + 2_000_000);
  expect(samples[6].Nodes).toBeLessThan(samples[2].Nodes + 300);
  await info.attach('browser-memory.json', { body: JSON.stringify(samples, null, 2), contentType: 'application/json' });
  await writeFile(info.outputPath('browser-memory.json'), JSON.stringify(samples, null, 2));
  await cdp.detach();
});
