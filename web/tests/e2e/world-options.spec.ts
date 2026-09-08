import { test, expect, battlefieldKey } from './fixtures';

test('opens advanced options and focuses an invalid seed before creating a game', async ({ page }) => {
  const exceptions: string[] = [];
  page.on('pageerror', error => exceptions.push(error.message));
  await page.goto('/');
  await page.locator('.advanced-world summary').click();
  await page.getByLabel('Map seed', { exact: true }).fill('0');
  await page.locator('.advanced-world summary').click();
  await page.getByRole('button', { name: 'Begin your reign' }).click();
  await expect(page.locator('.advanced-world')).toHaveAttribute('open', '');
  await expect(page.getByLabel('Map seed', { exact: true })).toBeFocused();
  await expect(page.locator('#start-error')).toContainText('whole-number map seed');
  await expect(page.locator('#start-dialog')).toBeVisible();
  expect(exceptions).toEqual([]);
});

for (const [type, name, biome] of [
  ['plains','Open Plains','temperate'], ['forest','Forest Marches','tropical'],
  ['highlands','Highland Relics','alpine'], ['rivers','River Kingdoms','temperate'],
  ['lakes','Twin Seas','temperate'], ['coast','Coastal Frontier','desert'],
  ['islands','Island Crowns','tropical'], ['protected','Walled Basin','temperate'],
  ['mountain_lakes','Mountain Lakes','mixed'], ['wetlands','Braided Wetlands','savanna'],
  ['fjords','Northern Fjords','alpine'], ['archipelago','Scattered Archipelago','autumn'],
] as const) {
  test(`creates ${name} with authoritative terrain and saves its world settings`, async ({ page, game }, info) => {
    await page.goto('/');
    await page.getByRole('combobox', { name: 'World type', exact: true }).selectOption(type);
    await page.getByRole('combobox', { name: 'Biome', exact: true }).selectOption(biome);
    await page.locator('.advanced-world summary').click();
    await page.getByRole('combobox', { name: 'Map reveal', exact: true }).selectOption('all');
    await page.getByRole('combobox', { name: 'Initial peace period', exact: true }).selectOption('5');
    const seat = await game.start();
    await game.command('pause', () => page.getByRole('button', { name: 'Pause match', exact: true }).click());
    const before = await game.snapshot();
    expect(before.world.type).toBe(type); expect(before.map.biome).toBe(biome);
    expect(before.map.width).toBe(96); expect(before.map.fog.every(f => f === 2)).toBe(true);
    expect(before.treaty_remaining).toBeGreaterThan(290);
    const terrains = new Set(before.map.tiles.map(t => t.terrain));
    if (['islands','coast','rivers','lakes','mountain_lakes','wetlands','fjords','archipelago'].includes(type)) expect(terrains.has('water')).toBe(true);
    if (type === 'rivers') expect(terrains.has('shallows')).toBe(true);
    if (type === 'highlands') expect(Math.max(...before.map.tiles.map(t => t.elevation))).toBeGreaterThan(1.8);
    if (type === 'protected') expect(before.entities.filter(e => e.owner === 1 && e.type === 'gate')).toHaveLength(4);
    await page.getByRole('button', { name: 'Match menu', exact: true }).click();
    await expect(page.locator('#session-world')).toContainText(name);
    await page.getByRole('button', { name: 'Leave game', exact: true }).click();
    await expect(page.locator('#saved-games-list')).toContainText(name);
    await page.getByRole('button', { name: `Resume ${seat.name}`, exact: true }).click();
    await expect(page.locator('#start-dialog')).toBeHidden();
    await expect(page.locator('#paused')).toBeVisible();
    expect(await game.snapshot()).toEqual(before);
    await page.getByRole('button', { name: 'Resume battle', exact: true }).click();
    await expect(page.locator('#paused')).toBeHidden();
    await expect(page.locator('#treaty-clock')).toBeVisible();
    await page.getByRole('button', { name: 'Zoom out', exact: true }).click({ clickCount: 3 });
    await page.screenshot({ path: info.outputPath(`${type}-${biome}.png`) });
  });
}

for (const [size, tiles] of [['huge',224], ['giant',288]] as const) {
  test(`creates and resumes a ${size} world with six regional biomes`, async ({ page, game }, info) => {
    test.setTimeout(60_000);
    await page.goto('/');
    await page.getByRole('combobox', { name: 'World type', exact: true }).selectOption('mountain_lakes');
    await page.getByRole('combobox', { name: 'Biome', exact: true }).selectOption('mixed');
    await page.getByRole('combobox', { name: 'World size', exact: true }).selectOption(size);
    await page.screenshot({ path: info.outputPath(`${size}-creation.png`) });
    await page.locator('.advanced-world summary').click();
    await page.getByRole('combobox', { name: 'Map reveal', exact: true }).selectOption('all');
    const seat = await game.start('skirmish', 'peaceful', 6);
    // Measure the live rendered page without accessing game internals. These
    // samples diagnose large-map costs; they are not a machine-specific FPS gate.
    const frames = await page.evaluate(() => new Promise<number[]>(resolve => {
      const times: number[] = [], start = performance.now();
      function sample(now: number) { times.push(now); if (now - start < 3000) requestAnimationFrame(sample); else resolve(times); }
      requestAnimationFrame(sample);
    }));
    const deltas = frames.slice(1).map((time, i) => time - frames[i]).sort((a, b) => a - b);
    await info.attach('render-timing.json', { body: JSON.stringify({ size, frameCount: frames.length, medianMS: deltas[Math.floor(deltas.length / 2)], p95MS: deltas[Math.floor(deltas.length * .95)] }), contentType: 'application/json' });
    await game.command('pause', () => page.getByRole('button', { name: 'Pause match', exact: true }).click());
    const before = await game.snapshot();
    expect(before.map.width).toBe(tiles);
    expect(before.map.tiles).toHaveLength(tiles * tiles);
    expect(new Set(before.map.tiles.map(t => t.biome))).toEqual(new Set(['temperate','alpine','desert','autumn','savanna','tropical']));
    expect(before.settlements).toBe(6);
    await page.getByRole('button', { name: 'Match menu', exact: true }).click();
    await page.getByRole('button', { name: 'Leave game', exact: true }).click();
    await page.getByRole('button', { name: `Resume ${seat.name}`, exact: true }).click();
    await expect(page.locator('#start-dialog')).toBeHidden();
    expect(await game.snapshot()).toEqual(before);
    await page.getByRole('button', { name: 'Resume battle', exact: true }).click();
    await expect(page.locator('#paused')).toBeHidden();
    await battlefieldKey(page, 'h');
    await page.getByRole('button', { name: 'Zoom out', exact: true }).click({ clickCount: 4 });
    await page.screenshot({ path: info.outputPath(`${size}-mixed-regions.png`) });
  });
}

for (const width of [1440,390]) {
  test(`configures all advanced world settings at ${width}px`, async ({ page, game }, info) => {
    await page.setViewportSize({ width, height: width === 390 ? 740 : 960 });
    await page.goto('/');
    await page.getByRole('combobox', { name: 'World type', exact: true }).selectOption('islands');
    await page.getByRole('combobox', { name: 'Biome', exact: true }).selectOption('desert');
    await page.getByRole('combobox', { name: 'World size', exact: true }).selectOption('large');
    await page.screenshot({ path: info.outputPath('world-creation.png') });
    await page.locator('.advanced-world summary').click();
    await page.getByRole('combobox', { name: 'Natural resources', exact: true }).selectOption('scarce');
    await page.getByRole('combobox', { name: 'Starting separation', exact: true }).selectOption('far');
    await page.getByRole('combobox', { name: 'Map reveal', exact: true }).selectOption('explored');
    await page.getByRole('combobox', { name: 'Initial peace period', exact: true }).selectOption('20');
    await page.getByLabel('Map seed', { exact: true }).fill('82731');
    const dialog = page.locator('#start-dialog');
    expect(await dialog.evaluate(d => d.scrollWidth <= d.clientWidth)).toBe(true);
    await page.screenshot({ path: info.outputPath('advanced-options.png') });
    await game.start('skirmish','easy',6);
    const snapshot = await game.snapshot();
    expect(snapshot.world).toEqual({type:'islands',biome:'desert',size:'large',resources:'scarce',separation:'far',reveal:'explored',treaty_minutes:20});
    expect(snapshot.map.width).toBe(160); expect(snapshot.settlements).toBe(6);
    expect(snapshot.map.tiles.every(t => t.terrain !== 'unknown')).toBe(true);
    expect(snapshot.entities.every(e => e.owner === 0 || e.owner === 1)).toBe(true);
    expect(snapshot.entities.find(e => e.type === 'stone')?.amount).toBeCloseTo(245);
    await page.reload();
    await expect(page.locator('#start-dialog')).toBeHidden();
    expect((await game.snapshot()).world).toEqual(snapshot.world);
  });
}
