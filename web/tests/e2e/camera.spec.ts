import type { Page } from '@playwright/test';
import type { Command } from '../../src/api.generated';
import { test, expect, battlefieldKey } from './fixtures';

async function field(page: Page) {
  const canvas = page.locator('#world canvas');
  const bounds = await canvas.boundingBox();
  if (!bounds) throw new Error('No battlefield bounds.');
  return { canvas, ...bounds };
}

async function drag(page: Page, from: { x: number; y: number }, to: { x: number; y: number }, hold = 220) {
  await page.mouse.move(from.x, from.y);
  await page.mouse.down();
  if (hold) await page.waitForTimeout(hold);
  await page.mouse.move(to.x, to.y, { steps: 10 });
  await page.mouse.up();
  await page.mouse.move(2, 2);
}

async function orbit(page: Page, from: { x: number; y: number }, to: { x: number; y: number }) {
  await page.mouse.move(from.x, from.y);
  await page.mouse.down({ button: 'left' });
  await page.mouse.down({ button: 'right' });
  await page.mouse.move(to.x, to.y, { steps: 10 });
  await page.mouse.up({ button: 'right' });
  await page.mouse.up({ button: 'left' });
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
      await orbit(page, start, { x: start.x + 90, y: start.y + 40 });
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

test('rotates and tilts while holding both mouse buttons, preserves selection, and resets the view', async ({ page, game }, info) => {
  await game.start();
  await game.command('pause', () => page.getByRole('button', { name: 'Pause match', exact: true }).click());
  await expect(page.locator('#paused')).toBeVisible();
  const f = await field(page), minimap = page.locator('#minimap');
  const commands = commandsFrom(page);
  await f.canvas.focus();
  const before = await f.canvas.screenshot({ path: info.outputPath('world-original.png') }), mapBefore = await minimap.screenshot();
  const start = { x: f.x + f.width * .72, y: f.y + f.height * .57 };
  await orbit(page, start, { x: start.x + 140, y: start.y });
  await expect.poll(async () => (await f.canvas.screenshot()).equals(before)).toBe(false);
  await expect.poll(async () => (await minimap.screenshot()).equals(mapBefore)).toBe(false);
  const rotated = await f.canvas.screenshot();
  await page.screenshot({ path: info.outputPath('world-rotated.png') });
  await orbit(page, start, { x: start.x, y: start.y + 75 });
  await expect.poll(async () => (await f.canvas.screenshot()).equals(rotated)).toBe(false);
  await page.screenshot({ path: info.outputPath('world-tilted.png') });
  await expect(page.locator('#selected-name')).toHaveText('Town Center');
  await expect(page.locator('#mode-hint')).toBeHidden();
  await battlefieldKey(page, 'r');
  await page.mouse.move(2, 2);
  await f.canvas.screenshot({ path: info.outputPath('world-reset.png') });
  // The battlefield contains animated flags and windmills, so a full canvas
  // byte comparison is unstable after the keyboard event. The minimap remains
  // a stable public projection of the reset camera footprint.
  await expect.poll(async () => (await minimap.screenshot()).equals(mapBefore)).toBe(true);
  expect(commands).toEqual([]);
});

test('keeps the grabbed ground point beneath the cursor when panning a rotated and zoomed view', async ({ page, game }, info) => {
  await game.start();
  const f = await field(page);
  const start = { x: f.x + f.width * .72, y: f.y + f.height * .57 };
  await orbit(page, start, { x: start.x + 140, y: start.y + 40 });
  await battlefieldKey(page, '+');
  await battlefieldKey(page, '+');
  const anchor = { x: f.x + f.width * .68, y: f.y + f.height * .6 };
  await page.getByRole('button', { name: 'Give order' }).click();
  const first = await game.command('rally', () => page.mouse.click(anchor.x, anchor.y));
  const firstPosition = (first.request().postDataJSON() as Command).position!;
  await battlefieldKey(page, 'p');
  const movedAnchor = { x: anchor.x + 80, y: anchor.y + 40 };
  await drag(page, anchor, movedAnchor);
  await battlefieldKey(page, 'p');
  await page.getByRole('button', { name: 'Give order' }).click();
  const second = await game.command('rally', () => page.mouse.click(movedAnchor.x, movedAnchor.y));
  const secondPosition = (second.request().postDataJSON() as Command).position!;
  expect(secondPosition.x).toBeCloseTo(firstPosition.x, 5);
  expect(secondPosition.y).toBeCloseTo(firstPosition.y, 5);
  await page.screenshot({ path: info.outputPath('world-panned-after-rotation.png') });
});

test('pans after a short left hold and keeps selection and perspective unchanged', async ({ page, game }, info) => {
  await game.start();
  const f = await field(page), minimap = page.locator('#minimap');
  const start = { x: f.x + f.width * .72, y: f.y + f.height * .56 };
  const end = { x: start.x + 80, y: start.y + 36 };
  const before = await minimap.screenshot();
  await page.getByRole('button', { name: 'Give order' }).click();
  const first = await game.command('rally', () => page.mouse.click(start.x, start.y));
  const commands = commandsFrom(page);
  await drag(page, start, end);
  await expect(page.locator('#selected-name')).toHaveText('Town Center');
  await expect(page.locator('#mode-hint')).toBeHidden();
  expect(commands).toEqual([]);
  const panned = await minimap.screenshot();
  expect(panned.equals(before)).toBe(false);
  // Reset only restores angle/zoom. Its footprint stays the same after a pan.
  await battlefieldKey(page, 'r');
  expect((await minimap.screenshot()).equals(panned)).toBe(true);
  await page.getByRole('button', { name: 'Give order' }).click();
  const second = await game.command('rally', () => page.mouse.click(end.x, end.y));
  expect(second.request().postDataJSON().position.x).toBeCloseTo(first.request().postDataJSON().position.x, 5);
  expect(second.request().postDataJSON().position.y).toBeCloseTo(first.request().postDataJSON().position.y, 5);
  await page.screenshot({ path: info.outputPath('primary-drag-pan.png') });
});

test('pans an armed Give order after the hold delay without issuing a command', async ({ page, game }) => {
  await game.start();
  const f = await field(page), minimap = page.locator('#minimap');
  const worker = await game.point('villager');
  await page.mouse.click(worker.x, worker.y);
  await expect(page.locator('#selected-name')).toHaveText('Villager');
  const start = { x: f.x + f.width * .68, y: f.y + f.height * .56 };
  const end = { x: start.x + 90, y: start.y + 35 };
  const before = await minimap.screenshot();
  const commands = commandsFrom(page);
  await page.getByRole('button', { name: 'Give order' }).click();
  await expect(page.locator('#mode-hint')).toContainText('Give order');
  await drag(page, start, end);
  expect((await minimap.screenshot()).equals(before)).toBe(false);
  await expect(page.locator('#selected-name')).toHaveText('Villager');
  await expect(page.locator('#mode-hint')).toContainText('Give order');
  expect(commands).toEqual([]);
});

for (const viewport of [{ name: 'desktop', width: 1440, height: 960 }, { name: 'compact', width: 390, height: 640 }]) {
  for (const selection of ['villager', 'scout', 'group'] as const) {
    test(`ordinary hold-pan keeps the ${selection} selection at ${viewport.name} size`, async ({ page, game }) => {
      await page.setViewportSize(viewport);
      await game.start();
      const f = await field(page), minimap = page.locator('#minimap');
      if (selection === 'group') {
        const workers = await Promise.all([0, 1, 2].map(index => game.point('villager', index)));
        await page.mouse.click(workers[0].x, workers[0].y);
        await page.keyboard.down('Shift');
        for (const point of workers.slice(1)) await page.mouse.click(point.x, point.y);
        await page.keyboard.up('Shift');
        await expect(page.locator('#selection-count')).toHaveText('3 selected');
      } else {
        const point = await game.point(selection);
        await page.mouse.click(point.x, point.y);
        await expect(page.locator('#selection-count')).toHaveText('1 selected');
        await expect(page.locator('#selected-name')).toContainText(selection === 'scout' ? 'Scout' : 'Villager');
      }
      const before = await minimap.screenshot(), commands = commandsFrom(page);
      await drag(page, { x: f.x + f.width * .68, y: f.y + f.height * .56 }, { x: f.x + f.width * .78, y: f.y + f.height * .61 });
      await expect.poll(async () => (await minimap.screenshot()).equals(before)).toBe(false);
      expect(commands).toEqual([]);
      if (selection === 'group') await expect(page.locator('#selection-count')).toHaveText('3 selected');
      else await expect(page.locator('#selection-count')).toHaveText('1 selected');
    });
  }
}

for (const viewport of [{ name: 'desktop', width: 1440, height: 960 }, { name: 'compact', width: 390, height: 640 }]) {
  for (const action of ['Give order', 'Move'] as const) {
    test(`armed ${action} hold-pan keeps selection and remains armed at ${viewport.name} size`, async ({ page, game }, info) => {
      await page.setViewportSize(viewport);
      await game.start();
      const f = await field(page), minimap = page.locator('#minimap');
      const worker = await game.point('villager');
      const workerID = (await game.snapshot()).entities.find(entity => entity.type === 'villager' && entity.owner === 1)!.id;
      await page.mouse.click(worker.x, worker.y);
      await expect(page.locator('#selected-name')).toHaveText('Villager');
      const before = await minimap.screenshot(), commands = commandsFrom(page);
      const button = action === 'Give order' ? page.locator('#actions').getByRole('button', { name: /^Give order/ }) : page.locator('#actions').getByRole('button', { name: /^.*Move Command$/ });
      await button.click();
      await expect(page.locator('#mode-hint')).toContainText(action);
      await drag(page, { x: f.x + f.width * .68, y: f.y + f.height * .56 }, { x: f.x + f.width * .78, y: f.y + f.height * .61 });
      await expect.poll(async () => (await minimap.screenshot()).equals(before)).toBe(false);
      await expect(page.locator('#selected-name')).toHaveText('Villager');
      await expect(page.locator('#mode-hint')).toContainText(action);
      expect(commands).toEqual([]);
      await page.screenshot({ path: info.outputPath('armed-pan-selection.png') });
      const target = { x: f.x + f.width * .78, y: f.y + f.height * .72 };
      const response = await game.command('move', () => page.mouse.click(target.x, target.y));
      expect((response.request().postDataJSON() as Command).entity_ids).toEqual([workerID]);
      expect(commands.filter(command => command.kind === 'move')).toHaveLength(1);
      await expect(page.locator('#mode-hint')).toBeHidden();
    });
  }
}

test('a long stationary armed click still issues its intended order', async ({ page, game }) => {
  await game.start();
  const f = await field(page);
  const worker = await game.point('villager');
  await page.mouse.click(worker.x, worker.y);
  await page.locator('#actions').getByRole('button', { name: /^Give order/ }).click();
  const target = { x: f.x + f.width * .55, y: f.y + f.height * .48 };
  await game.command('move', async () => {
    await page.mouse.move(target.x, target.y);
    await page.mouse.down();
    await page.waitForTimeout(260);
    await page.mouse.up();
  });
  await expect(page.locator('#mode-hint')).toBeHidden();
});

test('an armed drag before the pan hold delay sends no command', async ({ page, game }) => {
  await game.start();
  const f = await field(page), commands = commandsFrom(page);
  const worker = await game.point('villager');
  await page.mouse.click(worker.x, worker.y);
  await page.locator('#actions').getByRole('button', { name: /^Give order/ }).click();
  await drag(page, { x: f.x + f.width * .68, y: f.y + f.height * .56 }, { x: f.x + f.width * .78, y: f.y + f.height * .61 }, 40);
  expect(commands).toEqual([]);
  await expect(page.locator('#mode-hint')).toContainText('Give order');
});

test('selects on a single click, then pans on a held second click without rotating', async ({ page, game }) => {
  await game.start();
  const minimap = page.locator('#minimap'), before = await minimap.screenshot();
  const commands = commandsFrom(page);
  const workerPoint = await game.point('villager');
  await page.mouse.click(workerPoint.x, workerPoint.y);
  await expect(page.locator('#selected-name')).toHaveText('Villager');
  expect((await minimap.screenshot()).equals(before)).toBe(true);
  await page.mouse.down({ clickCount: 2 });
  await page.waitForTimeout(220);
  await page.mouse.move(725, 420, { steps: 8 });
  await page.mouse.up({ clickCount: 2 });
  await expect(page.locator('#selected-name')).toHaveText('Villager');
  const panned = await minimap.screenshot();
  expect(panned.equals(before)).toBe(false);
  await battlefieldKey(page, 'r');
  expect((await minimap.screenshot()).equals(panned)).toBe(true);
  expect(commands).toEqual([]);
});

test('ignores click motion before the pan hold delay and highlights the pressed element', async ({ page, game }) => {
  await game.start();
  const clockEpoch = new Date('2030-01-01T00:00:00.000Z');
  await page.clock.install({ time: clockEpoch });
  await page.clock.pauseAt(new Date(clockEpoch.getTime() + 1_000));
  const minimap = page.locator('#minimap'), before = await minimap.screenshot();
  const commands = commandsFrom(page);
  const workerPoint = await game.point('villager');
  await page.mouse.move(workerPoint.x, workerPoint.y);
  await page.mouse.down();
  await page.clock.runFor(100);
  await page.mouse.move(workerPoint.x + 15, workerPoint.y + 10);
  await page.mouse.up();
  await page.clock.runFor(250);
  await expect(page.locator('#selected-name')).toHaveText('Villager');
  expect((await minimap.screenshot()).equals(before)).toBe(true);
  expect(commands).toEqual([]);
});

for (const firstPress of ['left', 'right'] as const) {
  for (const firstRelease of ['left', 'right'] as const) {
    test(`rotates with ${firstPress} pressed first and stops when ${firstRelease} releases`, async ({ page, game }) => {
      await game.start();
      const minimap = page.locator('#minimap'), before = await minimap.screenshot();
      const commands = commandsFrom(page);
      const workerPoint = await game.point('villager');
  await page.mouse.move(workerPoint.x, workerPoint.y);
      await page.mouse.down({ button: firstPress });
      await page.waitForTimeout(220);
      expect(commands).toEqual([]);
      await page.mouse.down({ button: firstPress === 'left' ? 'right' : 'left' });
      await page.mouse.move(725, 420, { steps: 8 });
      await expect(page.locator('#selected-name')).toHaveText('Town Center');
      const rotated = await minimap.screenshot();
      expect(rotated.equals(before)).toBe(false);
      await page.mouse.up({ button: firstRelease });
      await page.mouse.move(775, 450, { steps: 5 });
      await page.waitForTimeout(220);
      await page.mouse.move(805, 460, { steps: 5 });
      expect((await minimap.screenshot()).equals(rotated)).toBe(true);
      await page.mouse.up({ button: firstRelease === 'left' ? 'right' : 'left' });
      await expect(page.locator('#selected-name')).toHaveText('Town Center');
      expect(commands).toEqual([]);
      // Consuming the chord must not suppress the next ordinary right click.
      await game.command('rally', () => page.mouse.click(900, 375, { button: 'right' }));
      expect(commands.map(command => command.kind)).toEqual(['rally']);
    });
  }
}

test('selects with a slightly unsteady click and keeps the camera still', async ({ page, game }) => {
  await game.start();
  const f = await field(page), mapBefore = await page.locator('#minimap').screenshot();
  // Visually verified first settler in the fixed opening, with two pixels of
  // trackpad motion between press and release (below the drag threshold).
  const point = await game.point('villager');
  await drag(page, point, { x: point.x + 2, y: point.y + 1 }, 0);
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
  await page.waitForTimeout(220);
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
  await expect(page.locator('#mode-hint')).toContainText('Give order');
  expect(commands).toEqual([]);
  expect((await game.snapshot()).player.idle).toBe(3);
});

test('can rotate and reset on a compact layout', async ({ page, game }, info) => {
  await page.setViewportSize({ width: 390, height: 640 });
  await game.start();
  const f = await field(page), minimap = page.locator('#minimap');
  const before = await minimap.screenshot();
  const point = { x: f.x + f.width * .65, y: f.y + f.height * .35 };
  await orbit(page, point, { x: point.x + 55, y: point.y + 45 });
  await expect.poll(async () => (await minimap.screenshot()).equals(before)).toBe(false);
  await page.screenshot({ path: info.outputPath('compact-rotated-world.png') });
  await battlefieldKey(page, 'r');
  await expect.poll(async () => (await minimap.screenshot()).equals(before)).toBe(true);
  await expect(page.locator('#selected-name')).toHaveText('Town Center');
});

test('rotates and tilts with Shift and arrows for trackpads, with ordinary arrows still panning', async ({ page, game }) => {
  await page.setViewportSize({ width: 390, height: 640 });
  await game.start();
  const f = await field(page), minimap = page.locator('#minimap');
  const before = await minimap.screenshot(), commands = commandsFrom(page);
  await f.canvas.focus();
  for (const arrow of ['ArrowRight', 'ArrowDown']) {
    const previous = await minimap.screenshot();
    await page.keyboard.down('Shift'); await page.keyboard.down(arrow);
    await page.waitForTimeout(250);
    await page.keyboard.up(arrow); await page.keyboard.up('Shift');
    expect((await minimap.screenshot()).equals(previous)).toBe(false);
  }
  await battlefieldKey(page, 'r');
  expect((await minimap.screenshot()).equals(before)).toBe(true);
  await f.canvas.focus(); await page.keyboard.down('ArrowRight');
  await page.waitForTimeout(200); await page.keyboard.up('ArrowRight');
  const panned = await minimap.screenshot();
  expect(panned.equals(before)).toBe(false);
  await battlefieldKey(page, 'r');
  expect((await minimap.screenshot()).equals(panned)).toBe(true);
  await expect(page.locator('#selected-name')).toHaveText('Town Center');
  expect(commands).toEqual([]);
});

test('keeps lower-screen ground targeting available at minimum tilt and maximum zoom out', async ({ page, game }, info) => {
  await game.start();
  const f = await field(page);
  const point = { x: f.x + f.width * .72, y: f.y + f.height * .6 };
  await orbit(page, point, { x: point.x, y: f.y + f.height * .1 });
  for (let i = 0; i < 8; i++) await battlefieldKey(page, '-');
  const minimap = page.locator('#minimap'), bounds = await minimap.boundingBox();
  if (!bounds) throw new Error('No minimap bounds.');
  // Focus near the map's corner so the lower edge of this wide view lands
  // inside the actual map, where Go can accept a rally point.
  await minimap.click({ position: { x: bounds.width * .05, y: bounds.height * .05 } });
  await page.getByRole('button', { name: 'Give order' }).click();
  const response = await game.command('rally', () => f.canvas.click({ position: { x: f.width / 2, y: f.height * .93 } }));
  const position = (response.request().postDataJSON() as Command).position!;
  expect(position.x).toBeGreaterThan(0);
  expect(position.y).toBeGreaterThan(0);
  expect(position.x).toBeLessThan(72);
  expect(position.y).toBeLessThan(72);
  await page.screenshot({ path: info.outputPath('world-low-angle-wide-view.png') });
});
