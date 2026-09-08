import type { Page } from '@playwright/test';
import type { Command } from '../../src/api.generated';
import { test, expect, battlefieldKey } from './fixtures';

async function battlefield(page: Page) {
  const canvas = page.locator('#world canvas');
  const bounds = await canvas.boundingBox();
  if (!bounds) throw new Error('No battlefield bounds.');
  return { canvas, ...bounds };
}

function observeCommands(page: Page) {
  const commands: Command[] = [];
  page.on('request', request => {
    if (request.method() === 'POST' && request.url().endsWith('/commands')) commands.push(request.postDataJSON());
  });
  return commands;
}

for (const viewport of [{ width: 1440, height: 960 }, { width: 390, height: 640 }]) {
  test(`gathers using primary clicks and Go interact at ${viewport.width}x${viewport.height}`, async ({ page, game }, info) => {
    await page.setViewportSize(viewport);
    await game.start();
    const field = await battlefield(page);
    // Project the observed opening; generated homes no longer have fixed screen targets.
    // Shift-drag keeps the camera fixed.
    // The initial Town Center selection is replaced by the box selection.
    await expect(page.locator('#selected-name')).toHaveText('Town Center');
    const workers = await Promise.all([0,1,2].map(i => game.point('villager', i)));
    await page.keyboard.down('Shift');
    await page.mouse.move(Math.min(...workers.map(p => p.x)) - 10, Math.min(...workers.map(p => p.y)) - 10);
    await page.mouse.down();
    await page.mouse.move(Math.max(...workers.map(p => p.x)) + 10, Math.max(...workers.map(p => p.y)) + 16, { steps: 5 });
    await page.mouse.up();
    await page.keyboard.up('Shift');
    await expect(page.locator('#selection-count')).toHaveText('3 selected');
    const before = await game.snapshot();
    await page.getByRole('button', { name: 'Give order' }).click();
    await expect(page.locator('#give-order')).toHaveAttribute('aria-pressed', 'true');
    await page.screenshot({ path: info.outputPath('primary-order-targeting.png') });
    const target = await game.point('berries', 4);
    const response = await game.command('interact', () => page.mouse.click(target.x, target.y));
    const intent = response.request().postDataJSON() as Command;
    const resource = before.entities.find(e => e.id === intent.target_id)!;
    expect(resource.resource).toBe('food');
    expect(intent.entity_ids).toHaveLength(3);
    await expect(page.locator('#mode-hint')).toBeHidden();
    await expect(page.locator('#selection-count')).toHaveText('3 selected');
    await expect.poll(async () => (await game.snapshot()).entities.find(e => e.id === resource.id)?.amount).toBeLessThan(resource.amount!);
    await page.screenshot({ path: info.outputPath('primary-order-gathering.png') });
  });
}

for (const gesture of [
  { name: 'right-click', button: 'right' as const, modifiers: [] },
  { name: 'Mac Control-click', button: 'left' as const, modifiers: ['Control' as const] },
  { name: 'Option or Alt-click', button: 'left' as const, modifiers: ['Alt' as const] },
]) {
  test(`issues exactly one contextual move with ${gesture.name}`, async ({ page, game }) => {
    await game.start();
    await battlefieldKey(page, '.');
    const before = await game.snapshot();
    const commands = observeCommands(page);
    const field = await battlefield(page);
    const response = await game.command('move', () => field.canvas.click({ position: { x: field.width * .64, y: field.height * .48 }, button: gesture.button, modifiers: gesture.modifiers }));
    const intent = response.request().postDataJSON() as Command;
    expect(intent.entity_ids).toHaveLength(3);
    await expect(page.locator('#selection-count')).toHaveText('3 selected');
    await expect.poll(async () => (await game.snapshot()).entities.some(e => intent.entity_ids!.includes(e.id) && before.entities.some(b => b.id === e.id && Math.hypot(e.position.x - b.position.x, e.position.y - b.position.y) > .2))).toBe(true);
    await game.command('stop', () => battlefieldKey(page, 's'));
    expect(commands.filter(c => c.kind === 'move')).toHaveLength(1);
  });
}

test('queues primary-click orders and cancels targeting without sending another command', async ({ page, game }) => {
  await game.start();
  await battlefieldKey(page, '.');
  const commands = observeCommands(page);
  const field = await battlefield(page);
  await battlefieldKey(page, 'q');
  for (const x of [.64, .69]) {
    const response = await game.command('move', () => field.canvas.click({ position: { x: field.width * x, y: field.height * .48 }, modifiers: ['Shift'] }));
    expect(response.request().postDataJSON().queue).toBe(true);
    await expect(page.locator('#give-order')).toHaveAttribute('aria-pressed', 'true');
  }
  await page.getByRole('button', { name: 'Cancel', exact: true }).click();
  await expect(page.locator('#mode-hint')).toBeHidden();
  // Mac secondary click also cancels an existing building preview once.
  await page.getByRole('button', { name: 'Build', exact: true }).click();
  await page.locator('#actions').getByRole('button', { name: /House/ }).click();
  await field.canvas.click({ position: { x: field.width * .64, y: field.height * .48 }, modifiers: ['Control'] });
  await expect(page.locator('#mode-hint')).toBeHidden();
  await game.command('stop', () => battlefieldKey(page, 's'));
  expect(commands.map(c => c.kind)).toEqual(['move', 'move', 'stop']);
});

test('sets a building rally point with primary clicks on a compact display', async ({ page, game }, info) => {
  await page.setViewportSize({ width: 390, height: 640 });
  await game.start();
  const field = await battlefield(page);
  for (const name of ['Give order', 'Pan view', 'Zoom in', 'Zoom out', 'Reset view']) await expect(page.getByRole('button', { name })).toBeInViewport({ ratio: 1 });
  await page.getByRole('button', { name: 'Give order' }).click();
  await expect(page.getByRole('button', { name: 'Cancel', exact: true })).toBeInViewport({ ratio: 1 });
  await page.screenshot({ path: info.outputPath('compact-primary-order.png') });
  const response = await game.command('rally', () => field.canvas.click({ position: { x: field.width * .78, y: field.height * .72 } }));
  const intent = response.request().postDataJSON() as Command;
  await expect.poll(async () => (await game.snapshot()).entities.find(e => e.id === intent.entity_ids![0])?.rally).toEqual(intent.position);
  await expect(page.locator('#mode-hint')).toBeHidden();
});

test('pans with primary drag and zooms with buttons without ordering units', async ({ page, game }, info) => {
  await game.start();
  await game.command('pause', () => page.getByRole('button', { name: 'Pause match', exact: true }).click());
  await expect(page.locator('#paused')).toBeVisible();
  const commands = observeCommands(page);
  const field = await battlefield(page);
  await field.canvas.focus();
  const before = await field.canvas.screenshot();
  await page.getByRole('button', { name: 'Pan view', exact: true }).click();
  await page.mouse.move(field.width * .7, field.y + field.height * .6);
  await page.mouse.down();
  await page.mouse.move(field.width * .8, field.y + field.height * .7, { steps: 8 });
  await page.mouse.up();
  await expect(page.locator('#pan-view')).toHaveAttribute('aria-pressed', 'true');
  await page.getByRole('button', { name: 'Cancel', exact: true }).click();
  await expect.poll(async () => (await field.canvas.screenshot()).equals(before)).toBe(false);
  const panned = await field.canvas.screenshot();
  await page.getByRole('button', { name: 'Zoom in', exact: true }).click();
  await page.mouse.move(2, 2); // Keep toolbar hover out of the pixel comparison.
  await expect.poll(async () => (await field.canvas.screenshot()).equals(panned)).toBe(false);
  await page.getByRole('button', { name: 'Zoom out', exact: true }).click();
  await page.mouse.move(2, 2);
  await expect.poll(async () => (await field.canvas.screenshot()).equals(panned)).toBe(true);
  await expect(page.locator('#selected-name')).toHaveText('Town Center');
  expect(commands).toEqual([]);
  await page.screenshot({ path: info.outputPath('camera-controls.png') });
});

test('leaves browser and OS shortcuts out of game input', async ({ page, game }) => {
  await game.start();
  await battlefieldKey(page, '.');
  const commands = observeCommands(page);
  // Dispatch public keyboard events without opening native Save/Print dialogs
  // or closing Chrome. Uncancelled events remain available to the browser.
  const unhandled = await page.locator('#world canvas').evaluate(canvas => {
    return ['metaKey', 'ctrlKey', 'altKey'].flatMap(modifier => ['s', 'a', 'h', 'q', 'p', 'r', ' '].map(key =>
      canvas.dispatchEvent(new KeyboardEvent('keydown', { key, code: key === ' ' ? 'Space' : `Key${key.toUpperCase()}`, [modifier]: true, bubbles: true, cancelable: true }))));
  });
  expect(unhandled.every(Boolean)).toBe(true);
  await expect(page.locator('#mode-hint')).toBeHidden();
  await expect(page.locator('#selection-count')).toHaveText('3 selected');
  expect((await game.snapshot()).paused).toBe(false);
  expect(commands).toEqual([]);
});

for (const confirmation of ['button', 'Mac Backspace']) {
  test(`confirms removal with ${confirmation} and preserves Cmd control groups`, async ({ page, game }) => {
    await game.start();
    await battlefieldKey(page, '.');
    await battlefieldKey(page, 'Meta+1');
    await battlefieldKey(page, 'h');
    await expect(page.locator('#selected-name')).toHaveText('Town Center');
    await battlefieldKey(page, '1');
    await expect(page.locator('#selection-count')).toHaveText('3 selected');
    const commands = observeCommands(page);
    const remove = page.locator('#actions').getByRole('button', { name: /Delete/ });
    await remove.click();
    await page.getByRole('button', { name: 'Cancel', exact: true }).click();
    await battlefieldKey(page, 'Backspace');
    expect((await game.snapshot()).player.workers).toBe(3);
    expect(commands).toEqual([]);
    await remove.click();
    await game.command('delete', () => confirmation === 'button' ? page.getByRole('button', { name: 'Confirm removal', exact: true }).click() : battlefieldKey(page, 'Backspace'));
    await expect(page.locator('#mode-hint')).toBeHidden();
    await expect.poll(async () => (await game.snapshot()).player.workers).toBe(0);
    expect(commands.filter(c => c.kind === 'delete')).toHaveLength(1);
  });
}
