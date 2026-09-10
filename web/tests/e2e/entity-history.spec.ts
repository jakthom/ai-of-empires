import { test, expect } from './fixtures';

test('shows individual history after Research without expanding or filtering the global tray', async ({ page, game }, info) => {
  const entityReads: string[] = [];
  page.on('request', request => { if (request.url().includes('/history?')) entityReads.push(request.url()); });
  await game.start();
  expect(entityReads).toHaveLength(0);
  const history = page.getByRole('button', { name: 'History', exact: true });
  expect(await history.evaluate(button => button.previousElementSibling?.textContent)).toBe('Research');
  await history.click();
  await expect(history).toHaveAttribute('aria-pressed', 'true');
  await expect(page.locator('#entity-history-panel')).toBeVisible();
  await expect(page.locator('#entity-event-entries')).toContainText('Town Center created');
  await expect(page.locator('#actions')).toBeHidden();
  expect((await page.locator('#event-tray').boundingBox())!.height).toBe(35);
  await expect(page.locator('#log-title')).toHaveText('Your kingdom');
  await page.getByRole('button', { name: 'Orders', exact: true }).click();
  await game.command('train', () => page.locator('#actions').getByRole('button', { name: /Villager/ }).click());
  await history.click();
  await expect(page.locator('#entity-event-entries')).toContainText('Queued Villager');
  await game.command('cancel', () => page.locator('#queue button').click());
  await expect(page.locator('#entity-event-entries')).toContainText('Cancelled Villager');
  await page.screenshot({ path: info.outputPath('history-tab.png') });
});

test('changing selection ignores a delayed response for the previous entity', async ({ page, game }) => {
  await game.start();
  let release!: () => void, requested!: () => void;
  const gate = new Promise<void>(resolve => { release = resolve; });
  const pending = new Promise<void>(resolve => { requested = resolve; });
  await page.route('**/entities/1/history?*', async route => { requested(); await gate; await route.continue(); });
  try {
    await page.getByRole('button', { name: 'History', exact: true }).click();
    await pending;
    const workerPoint = await game.point('villager');
  await page.mouse.click(workerPoint.x, workerPoint.y);
    await expect(page.locator('#selected-name')).toHaveText('Villager');
    await expect(page.locator('#entity-log-title')).toContainText('Villager');
    await expect(page.locator('#entity-event-entries')).toContainText('Villager created');
    const response = page.waitForResponse(r => r.url().includes('/entities/1/history?'));
    release(); await response;
    await expect(page.locator('#entity-log-title')).toContainText('Villager');
    await expect(page.locator('#entity-event-entries')).not.toContainText('Town Center created');
    const ids = await page.locator('[id]').evaluateAll(elements => elements.map(e => e.id));
    expect(new Set(ids).size).toBe(ids.length);
  } finally { release(); }
});

test('retains the last selected entity history after removal', async ({ page, game }) => {
  await game.start();
  const workerPoint = await game.point('villager');
  await page.mouse.click(workerPoint.x, workerPoint.y);
  await expect(page.locator('#selected-name')).toHaveText('Villager');
  await page.getByRole('button', { name: 'History', exact: true }).click();
  await expect(page.locator('#entity-event-entries')).toContainText('Villager created');
  await page.getByRole('button', { name: 'Orders', exact: true }).click();
  await page.locator('#actions').getByRole('button', { name: /Delete/ }).click();
  await game.command('delete', () => page.getByRole('button', { name: 'Confirm removal', exact: true }).click());
  await page.getByRole('button', { name: 'History', exact: true }).click();
  await expect(page.locator('#entity-log-status')).toHaveText('Destroyed');
  await expect(page.locator('#entity-event-entries')).toContainText('Removed from the battlefield');
});

for (const viewport of [{ width: 1024, height: 768 }, { width: 900, height: 650 }, { width: 800, height: 600 }, { width: 390, height: 640 }, { width: 320, height: 640 }]) {
  test(`keeps the History tab and its rows usable at ${viewport.width}×${viewport.height}`, async ({ page, game }, info) => {
    await page.setViewportSize(viewport);
    await game.start();
    await page.getByRole('button', { name: 'History', exact: true }).click();
    for (const name of ['Orders', 'Build', 'Research', 'History']) await expect(page.getByRole('button', { name, exact: true })).toBeInViewport({ ratio: 1 });
    await expect(page.locator('#entity-event-entries')).toContainText('Town Center created');
    await expect(page.locator('#entity-log-follow')).toBeInViewport({ ratio: 1 });
    await expect(page.locator('#entity-event-entries .log-entry').last()).toBeInViewport({ ratio: 1 });
    expect((await page.locator('#entity-event-entries').boundingBox())!.height).toBeGreaterThan(18);
    expect((await page.locator('#event-tray').boundingBox())!.height).toBe(35);
    expect(await page.evaluate(() => document.documentElement.scrollWidth > innerWidth)).toBe(false);
    await page.screenshot({ path: info.outputPath('compact-history-tab.png') });
  });
}
