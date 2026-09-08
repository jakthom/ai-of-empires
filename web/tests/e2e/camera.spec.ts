import type { Page } from '@playwright/test';
import type { Command } from '../../src/api.generated';
import { test, expect, battlefieldKey } from './fixtures';

async function field(page: Page) {
  const canvas = page.locator('#world canvas');
  const bounds = await canvas.boundingBox();
  if (!bounds) throw new Error('No battlefield bounds.');
  return { canvas, ...bounds };
}

async function drag(page: Page, from: { x: number; y: number }, to: { x: number; y: number }) {
  await page.mouse.move(from.x, from.y);
  await page.mouse.down();
  await page.mouse.move(to.x, to.y, { steps: 10 });
  await page.mouse.up();
  await page.mouse.move(2, 2);
}

function commandsFrom(page: Page) {
  const commands: Command[] = [];
  page.on('request', request => {
    if (request.method() === 'POST' && request.url().endsWith('/commands')) commands.push(request.postDataJSON());
  });
  return commands;
}

for (const view of [
  { name: 'default view', width: 1440, height: 960, rotate: false },
  { name: 'rotated and tilted view', width: 1440, height: 960, rotate: true },
  { name: 'compact view', width: 390, height: 640, rotate: true },
]) {
  test(`keeps the cursor over the same ground while zooming in a ${view.name}`, async ({ page, game }, info) => {
    await page.setViewportSize({ width: view.width, height: view.height });
    await game.start();
    const f = await field(page), minimap = page.locator('#minimap');
    if (view.rotate) {
      const start = { x: f.x + f.width * .7, y: f.y + f.height * .55 };
      await drag(page, start, { x: start.x + 90, y: start.y + 40 });
    }
    const cursor = { x: Math.round(f.x + f.width * .83), y: Math.round(f.y + f.height * .4) };
    const pointAtCursor = async () => {
      await page.getByRole('button', { name: 'Give order' }).click();
      const response = await game.command('rally', () => page.mouse.click(cursor.x, cursor.y));
      return (response.request().postDataJSON() as Command).position!;
    };
    const original = await pointAtCursor(), commands = commandsFrom(page);
    for (const delta of [-200, 200, -100_000, 100_000]) {
      const mapBefore = await minimap.screenshot(), count = commands.length;
      await page.mouse.move(cursor.x, cursor.y);
      await page.mouse.wheel(0, delta);
      await expect.poll(async () => (await minimap.screenshot()).equals(mapBefore)).toBe(false);
      expect(commands).toHaveLength(count);
      const after = await pointAtCursor();
      expect(after.x).toBeCloseTo(original.x, 5);
      expect(after.y).toBeCloseTo(original.y, 5);
    }
    const atLimit = await minimap.screenshot(), count = commands.length;
    await page.mouse.wheel(0, 1000);
    expect((await minimap.screenshot()).equals(atLimit)).toBe(true);
    expect(commands).toHaveLength(count);
    await expect(page.locator('#selected-name')).toHaveText('Town Center');
    await page.screenshot({ path: info.outputPath('cursor-anchored-zoom.png') });
  });
}

test('anchors trackpad pinch and line-based wheel events without browser page zoom', async ({ page, game }) => {
  await game.start();
  const f = await field(page), cursor = { x: Math.round(f.x + f.width * .83), y: Math.round(f.y + f.height * .4) };
  const pointAtCursor = async () => {
    await page.getByRole('button', { name: 'Give order' }).click();
    const response = await game.command('rally', () => page.mouse.click(cursor.x, cursor.y));
    return (response.request().postDataJSON() as Command).position!;
  };
  const original = await pointAtCursor(), commands = commandsFrom(page);
  for (const gesture of [{ ctrlKey: true, deltaMode: 0, deltaY: -80 }, { ctrlKey: false, deltaMode: 1, deltaY: 5 }]) {
    const before = await page.locator('#minimap').screenshot(), count = commands.length;
    // Chromium reports trackpad pinch as a Ctrl+wheel event. Dispatch that
    // public input shape; physical trackpad hardware is not simulated here.
    const allowed = await f.canvas.evaluate((canvas, event) => canvas.dispatchEvent(new WheelEvent('wheel', {
      clientX: event.cursor.x, clientY: event.cursor.y, ...event.gesture, bubbles: true, cancelable: true,
    })), { cursor, gesture });
    expect(allowed).toBe(false);
    await expect.poll(async () => (await page.locator('#minimap').screenshot()).equals(before)).toBe(false);
    expect(commands).toHaveLength(count);
    const after = await pointAtCursor();
    expect(after.x).toBeCloseTo(original.x, 5);
    expect(after.y).toBeCloseTo(original.y, 5);
  }
});

test('rotates and tilts by dragging directly, updates the minimap, and resets the view', async ({ page, game }, info) => {
  await game.start();
  await game.command('pause', () => page.getByRole('button', { name: 'Pause match', exact: true }).click());
  await expect(page.locator('#paused')).toBeVisible();
  const f = await field(page), minimap = page.locator('#minimap');
  const commands = commandsFrom(page);
  await f.canvas.focus();
  const before = await f.canvas.screenshot({ path: info.outputPath('world-original.png') }), mapBefore = await minimap.screenshot();
  const start = { x: f.x + f.width * .72, y: f.y + f.height * .57 };
  await drag(page, start, { x: start.x + 140, y: start.y });
  await expect.poll(async () => (await f.canvas.screenshot()).equals(before)).toBe(false);
  await expect.poll(async () => (await minimap.screenshot()).equals(mapBefore)).toBe(false);
  const rotated = await f.canvas.screenshot();
  await page.screenshot({ path: info.outputPath('world-rotated.png') });
  await drag(page, start, { x: start.x, y: start.y + 75 });
  await expect.poll(async () => (await f.canvas.screenshot()).equals(rotated)).toBe(false);
  await page.screenshot({ path: info.outputPath('world-tilted.png') });
  await expect(page.locator('#selected-name')).toHaveText('Town Center');
  await expect(page.locator('#mode-hint')).toBeHidden();
  await page.getByRole('button', { name: 'Reset view', exact: true }).click();
  await page.mouse.move(2, 2);
  await f.canvas.screenshot({ path: info.outputPath('world-reset.png') });
  await expect.poll(async () => (await f.canvas.screenshot()).equals(before)).toBe(true);
  await expect.poll(async () => (await minimap.screenshot()).equals(mapBefore)).toBe(true);
  expect(commands).toEqual([]);
});

test('keeps the grabbed ground point beneath the cursor when panning a rotated and zoomed view', async ({ page, game }, info) => {
  await game.start();
  const f = await field(page);
  const start = { x: f.x + f.width * .72, y: f.y + f.height * .57 };
  await drag(page, start, { x: start.x + 140, y: start.y + 40 });
  await page.getByRole('button', { name: 'Zoom in', exact: true }).click();
  await page.getByRole('button', { name: 'Zoom in', exact: true }).click();
  const anchor = { x: f.x + f.width * .68, y: f.y + f.height * .6 };
  await page.getByRole('button', { name: 'Give order' }).click();
  const first = await game.command('rally', () => page.mouse.click(anchor.x, anchor.y));
  const firstPosition = (first.request().postDataJSON() as Command).position!;
  await page.getByRole('button', { name: 'Pan view', exact: true }).click();
  const movedAnchor = { x: anchor.x + 80, y: anchor.y + 40 };
  await drag(page, anchor, movedAnchor);
  await page.getByRole('button', { name: 'Pan view', exact: true }).click();
  await page.getByRole('button', { name: 'Give order' }).click();
  const second = await game.command('rally', () => page.mouse.click(movedAnchor.x, movedAnchor.y));
  const secondPosition = (second.request().postDataJSON() as Command).position!;
  expect(secondPosition.x).toBeCloseTo(firstPosition.x, 5);
  expect(secondPosition.y).toBeCloseTo(firstPosition.y, 5);
  await page.screenshot({ path: info.outputPath('world-panned-after-rotation.png') });
});

test('selects with a slightly unsteady click and keeps the camera still', async ({ page, game }) => {
  await game.start();
  const f = await field(page), mapBefore = await page.locator('#minimap').screenshot();
  // Visually verified first settler in the fixed opening, with two pixels of
  // trackpad motion between press and release (below the drag threshold).
  const point = { x: f.x + f.width / 2 - f.height * .14, y: f.y + f.height * .49 };
  await drag(page, point, { x: point.x + 2, y: point.y + 1 });
  await expect(page.locator('#selected-name')).toHaveText('Villager');
  await expect(page.locator('#selection-count')).toHaveText('1 selected');
  expect((await page.locator('#minimap').screenshot()).equals(mapBefore)).toBe(true);
});

test('cancels an in-progress camera drag and never orders on release', async ({ page, game }, info) => {
  await game.start();
  const f = await field(page), commands = commandsFrom(page);
  const point = { x: f.x + f.width * .72, y: f.y + f.height * .57 };
  await page.mouse.move(point.x, point.y);
  await page.mouse.down();
  await page.mouse.move(point.x + 80, point.y, { steps: 5 });
  await page.keyboard.press('Escape');
  const stopped = await page.locator('#minimap').screenshot({ path: info.outputPath('minimap-drag-cancelled.png') });
  await page.mouse.move(point.x + 160, point.y + 30, { steps: 5 });
  await page.mouse.up();
  await page.locator('#minimap').screenshot({ path: info.outputPath('minimap-after-release.png') });
  await expect.poll(async () => (await page.locator('#minimap').screenshot()).equals(stopped)).toBe(true);
  await expect(page.locator('#selected-name')).toHaveText('Town Center');
  // A drag in an armed order mode must not submit a destination either.
  await battlefieldKey(page, '.');
  await battlefieldKey(page, 'q');
  await drag(page, point, { x: point.x + 40, y: point.y + 10 });
  await expect(page.locator('#give-order')).toHaveAttribute('aria-pressed', 'true');
  expect(commands).toEqual([]);
  expect((await game.snapshot()).player.idle).toBe(3);
});

test('can rotate and reset on a compact trackpad layout', async ({ page, game }, info) => {
  await page.setViewportSize({ width: 390, height: 640 });
  await game.start();
  const f = await field(page), minimap = page.locator('#minimap');
  const before = await minimap.screenshot();
  const point = { x: f.x + f.width * .65, y: f.y + f.height * .35 };
  await drag(page, point, { x: point.x + 55, y: point.y + 45 });
  await expect.poll(async () => (await minimap.screenshot()).equals(before)).toBe(false);
  const reset = page.getByRole('button', { name: 'Reset view', exact: true });
  await expect(reset).toBeInViewport({ ratio: 1 });
  await page.screenshot({ path: info.outputPath('compact-rotated-world.png') });
  await reset.click();
  await expect.poll(async () => (await minimap.screenshot()).equals(before)).toBe(true);
  await expect(page.locator('#selected-name')).toHaveText('Town Center');
});

test('keeps lower-screen ground targeting available at minimum tilt and maximum zoom out', async ({ page, game }, info) => {
  await game.start();
  const f = await field(page);
  const point = { x: f.x + f.width * .72, y: f.y + f.height * .6 };
  await drag(page, point, { x: point.x, y: f.y + f.height * .1 });
  for (let i = 0; i < 8; i++) await page.getByRole('button', { name: 'Zoom out', exact: true }).click();
  const minimap = page.locator('#minimap'), bounds = await minimap.boundingBox();
  if (!bounds) throw new Error('No minimap bounds.');
  // Focus near the map's corner so the lower edge of this wide view lands
  // inside the actual map, where Go can accept a rally point.
  await minimap.click({ position: { x: bounds.width * .05, y: bounds.height * .05 } });
  await page.getByRole('button', { name: 'Give order' }).click();
  const response = await game.command('rally', () => f.canvas.click({ position: { x: f.width / 2, y: f.height * .93 } }));
  const position = (response.request().postDataJSON() as Command).position!;
  expect(position.x).toBeGreaterThan(50);
  expect(position.y).toBeGreaterThan(50);
  expect(position.x).toBeLessThan(72);
  expect(position.y).toBeLessThan(72);
  await page.screenshot({ path: info.outputPath('world-low-angle-wide-view.png') });
});
