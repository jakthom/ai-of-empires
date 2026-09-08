import { test, expect, battlefieldKey } from './fixtures';

test('defaults to return fire and persists a chosen military stance', async ({ page, game }, info) => {
  await game.start('skirmish', 'easy', 3);
  const initial = await game.snapshot();
  const scout = initial.entities.find(e => e.owner === initial.player.id && e.type === 'scout')!;
  expect(scout.stance).toBe('defensive');
  const actions = page.locator('#actions');
  // The Town Center and trained troops use the same server-issued stances.
  await expect(actions.getByRole('button', { name: /^Return fire/ })).toHaveAttribute('aria-pressed', 'true');
  await page.getByRole('button', { name: 'Expand event log', exact: true }).click();
  await page.getByRole('searchbox', { name: 'Search event log' }).fill(`${scout.name} #${scout.id}`);
  await page.getByRole('button', { name: `Locate ${scout.name} #${scout.id}`, exact: true }).first().click();
  await page.getByRole('button', { name: 'Collapse event log', exact: true }).click();
  await page.getByRole('button', { name: 'Orders', exact: true }).click();
  await expect(page.locator('#selected-name')).toHaveText(scout.name);
  await expect(actions.getByRole('button', { name: /^Return fire/ })).toHaveAttribute('aria-pressed', 'true');
  await actions.getByRole('button', { name: /^Hold fire/ }).focus();
  const response = await game.command('stance', () => page.keyboard.press('Enter'));
  expect(response.request().postDataJSON()).toMatchObject({ entity_ids: [scout.id], product: 'passive' });
  await expect(actions.getByRole('button', { name: /^Hold fire/ })).toHaveAttribute('aria-pressed', 'true');
  await expect(actions.getByRole('button', { name: /^Hold fire/ })).toBeFocused();
  await expect(actions.getByRole('button', { name: /^Return fire/ })).toHaveAttribute('aria-pressed', 'false');
  await game.command('pause', () => page.getByRole('button', { name: 'Pause match', exact: true }).click());
  await page.reload();
  await expect(page.locator('#paused')).toBeVisible();
  expect((await game.snapshot()).entities.find(e => e.id === scout.id)?.stance).toBe('passive');
  await page.getByRole('button', { name: 'Resume battle', exact: true }).click();
  await battlefieldKey(page, '.');
  await game.command('stance', () => actions.getByRole('button', { name: /^Aggressive/ }).click());
  await expect(actions.getByRole('button', { name: /^Aggressive/ })).toHaveAttribute('aria-pressed', 'true');
  await actions.getByRole('button', { name: /^Aggressive/ }).hover();
  await expect(page.locator('#action-help')).toContainText('start a conflict');
  await game.command('stance', () => actions.getByRole('button', { name: /^Return fire/ }).click());
  await expect(actions.getByRole('button', { name: /^Return fire/ })).toHaveAttribute('aria-pressed', 'true');
  await page.screenshot({ path: info.outputPath('return-fire.png') });
});

for (const width of [1440, 800, 640, 390]) {
  test(`shows peaceful relationships and different kingdom preferences at ${width}px`, async ({ page, game }, info) => {
    await page.setViewportSize({ width, height: width === 1440 ? 960 : 640 });
    await game.start('skirmish', 'easy', 6);
    const snapshot = await game.snapshot();
    expect(snapshot.opponents.every(o => o.relation === 'peaceful')).toBe(true);
    expect(new Set(snapshot.opponents.map(o => o.temperament)).size).toBe(3);
    const summary = page.locator('#relationships summary');
    await expect(summary).toBeInViewport({ ratio: 1 });
    await expect(page.locator('#rival-status')).toHaveText('5 at peace · 0 in conflict');
    await summary.focus();
    await page.keyboard.press('Enter');
    const list = page.locator('#kingdom-relations');
    await expect(list.locator('li')).toHaveCount(5);
    for (const kingdom of snapshot.opponents) {
      const row = list.locator('li').filter({ hasText: kingdom.name });
      await expect(row).toContainText(`At peace · ${kingdom.temperament}`);
    }
    await page.screenshot({ path: info.outputPath('peaceful-kingdoms.png') });
    await list.locator('li').last().scrollIntoViewIfNeeded();
    await expect(list.locator('li').last()).toBeInViewport({ ratio: 1 });
    const panel = (await page.locator('#relationships').boundingBox())!;
    const topbar = (await page.locator('.topbar').boundingBox())!;
    const deck = (await page.locator('.command-deck').boundingBox())!;
    expect(panel.y).toBeGreaterThanOrEqual(topbar.y + topbar.height + 8);
    expect(panel.y + panel.height).toBeLessThanOrEqual(deck.y);
    expect(await page.evaluate(() => document.documentElement.scrollWidth > innerWidth)).toBe(false);
    await page.screenshot({ path: info.outputPath('peaceful-kingdoms-scrolled.png') });
    await summary.click();
    await expect(list).toBeHidden();
  });
}
