import type { Command } from '../../src/api.generated';
import { test, expect, battlefieldKey } from './fixtures';

// Project observed resource positions into the opening camera.
// Use actual pointer input and inspect only the UI session's public snapshots.
for (const resource of [
  { name: 'stone', type: 'stone', total: 'stone' as const },
  { name: 'wood', type: 'tree', total: 'wood' as const },
]) {
  test(`gathers and delivers ${resource.name} through primary-click orders`, async ({ page, game }, info) => {
    await game.start();
    await game.command('speed', () => page.locator('#speed').click());
    await expect(page.locator('#speed')).toHaveText('3.4×');
    const workerPoint = await game.point('villager');
    await page.mouse.click(workerPoint.x, workerPoint.y);
    await expect(page.locator('#selected-name')).toHaveText('Villager');
    await expect(page.locator('#selection-count')).toHaveText('1 selected');
    const before = await game.snapshot();
    await page.getByRole('button', { name: 'Give order' }).click();
    const targetPoint = await game.point(resource.type);
    const response = await game.command('interact', () => page.mouse.click(targetPoint.x, targetPoint.y));
    const intent = response.request().postDataJSON() as Command;
    const source = before.entities.find(e => e.id === intent.target_id)!;
    expect(source.type).toBe(resource.type);
    await expect(page.locator('#selected-name')).toHaveText('Villager');
    await expect.poll(async () => (await game.snapshot()).entities.find(e => e.id === source.id)?.amount, { timeout: 12_000 }).toBeLessThan(source.amount!);
    await expect.poll(async () => (await game.snapshot()).player.resources[resource.total], { timeout: 16_000 }).toBeGreaterThan(before.player.resources[resource.total]);
    await page.screenshot({ path: info.outputPath(`${resource.name}-delivered.png`) });
  });
}

test('resumes an unfinished farm with a primary-click order and starts farming', async ({ page, game }, info) => {
  await game.start();
  await game.command('speed', () => page.locator('#speed').click());
  const workerPoint = await game.point('villager');
  await page.mouse.click(workerPoint.x, workerPoint.y);
  await expect(page.locator('#selected-name')).toHaveText('Villager');
  const before = await game.snapshot();
  await page.getByRole('button', { name: 'Build', exact: true }).click();
  await page.locator('#actions').getByRole('button', { name: /Farm/ }).click();
  const check = page.waitForResponse(r => r.url().endsWith('/placement'));
  await page.mouse.move(690, 471);
  expect((await (await check).json()).valid).toBe(true);
  await game.command('build', () => page.mouse.click(690, 471));
  await game.command('stop', () => battlefieldKey(page, 's'));
  const farm = (await game.snapshot()).entities.find(e => e.type === 'farm')!;
  expect(farm.progress).toBeLessThan(1);
  // Wait for the construction and stop snapshots to reach the visible world
  // before clicking the new foundation; the HTTP receipt precedes SSE.
  await expect(page.locator('#selected-status')).toHaveText('Idle');
  await expect(page.locator('#res-wood')).toHaveText(String(before.player.resources.wood - 60));
  await page.screenshot({ path: info.outputPath('unfinished-farm.png') });
  await battlefieldKey(page, 'q');
  const resumed = await game.command('interact', () => page.mouse.click(690, 470));
  expect(resumed.request().postDataJSON().target_id).toBe(farm.id);
  await expect.poll(async () => (await game.snapshot()).entities.find(e => e.id === farm.id)?.progress, { timeout: 12_000 }).toBe(1);
  await expect.poll(async () => (await game.snapshot()).entities.find(e => e.id === farm.id)?.amount).toBeLessThan(farm.amount!);
  await expect.poll(async () => (await game.snapshot()).player.resources.food, { timeout: 14_000 }).toBeGreaterThan(before.player.resources.food);
  await page.screenshot({ path: info.outputPath('farm-delivered.png') });
});
