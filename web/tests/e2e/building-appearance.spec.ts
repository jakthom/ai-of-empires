import { createHash } from 'node:crypto';
import * as THREE from 'three';
import type { Catalog, EntityView } from '../../src/api.generated';
import { makeModel } from '../../src/models';
import { test, expect, battlefieldKey } from './fixtures';
import { opening, build, select } from './economy-helpers';

function surfaces(model: THREE.Group) {
  const found = new Map<string, THREE.MeshStandardMaterial>();
  model.traverse(object => {
    if (!(object instanceof THREE.Mesh)) return;
    const material = object.material as THREE.MeshStandardMaterial;
    if (material.userData.surface) found.set(material.userData.surface, material);
  });
  return found;
}

function signature(model: THREE.Group) {
  const hash = createHash('sha256');
  for (const [surface, material] of [...surfaces(model)].sort(([a], [b]) => a.localeCompare(b))) {
    hash.update(surface);
    hash.update((material.map as THREE.DataTexture).image.data as Uint8Array);
  }
  return hash.digest('hex');
}

function dispose(model: THREE.Group) {
  model.traverse(object => {
    if (object instanceof THREE.Mesh && object.userData.privateGeometry) object.geometry.dispose();
    if (object instanceof THREE.InstancedMesh) object.dispose();
  });
}

test('every build choice has its own silhouette at desktop and compact widths', async ({ page, game }, info) => {
  await game.start();
  const options = (await game.snapshot()).build_options;
  await battlefieldKey(page, '.');
  await page.getByRole('button', { name: 'Build', exact: true }).click();
  const icons = page.locator('#actions .building-icon');
  await expect(icons).toHaveCount(options.length);
  expect((await icons.evaluateAll(nodes => nodes.map(n => n.getAttribute('data-building-icon')))).sort()).toEqual(options.map(o => o.product).sort());
  const silhouettes = await icons.evaluateAll(nodes => nodes.map(n => n.innerHTML));
  expect(new Set(silhouettes).size).toBe(options.length);
  for (const width of [1440, 390]) {
    await page.setViewportSize({ width, height: width === 390 ? 740 : 960 });
    expect(await page.locator('.command-deck').evaluate(e => e.scrollWidth <= e.clientWidth)).toBe(true);
    await expect(icons.first()).toBeVisible();
    await page.screenshot({ path: info.outputPath(`building-icons-${width}.png`) });
  }
});

test('all catalog buildings have distinct age textures and share materials across entities', async ({ request, game }) => {
  await game.start();
  const response = await request.get('/api/v1/catalog');
  expect(response.ok()).toBe(true);
  const catalog = await response.json() as Catalog;
  const town = (await game.snapshot()).entities.find(e => e.type === 'town_center' && e.owner === 1)!;
  const distinctBuildings = new Set<string>();
  for (const definition of catalog.definitions.filter(d => d.kind === 'building')) {
    const ageTextures = new Set<string>();
    for (let age = 0; age < 4; age++) {
      // Render-only specimens exercise every catalog footprint. They cannot
      // create buildings or alter any authoritative game state.
      const entity: EntityView = { ...town, type: definition.id, radius: definition.radius, appearance_age: age };
      const model = makeModel(entity), another = makeModel({ ...entity, id: town.id + 1000, owner: 2 });
      try {
        expect(surfaces(model).size, definition.id).toBeGreaterThan(0);
        for (const [surface, material] of surfaces(model)) {
          expect(material).toBe(surfaces(another).get(surface));
          expect(material.userData.appearanceAge).toBe(age);
          expect(new Set((material.map as THREE.DataTexture).image.data).size).toBeGreaterThan(16);
        }
        ageTextures.add(signature(model));
      } finally { dispose(model); dispose(another); }
    }
    expect(ageTextures.size, `${definition.id} changes in all four ages`).toBe(4);
    for (const hash of ageTextures) {
      expect(distinctBuildings.has(hash), `${definition.id} has its own material family`).toBe(false);
      distinctBuildings.add(hash);
    }
  }
});

test('existing buildings visibly mature through four ages during public gameplay', async ({ page, game }, info) => {
  test.setTimeout(120_000);
  await opening(page, game);
  const town = (await game.snapshot()).entities.find(e => e.type === 'town_center' && e.owner === 1)!;
  async function capture(age: number) {
    await expect.poll(async () => (await game.snapshot()).entities.find(e => e.id === town.id)?.appearance_age ?? 0).toBe(age);
    await battlefieldKey(page, 'h');
    await page.locator('#world canvas').focus();
    for (let i = 0; i < 3; i++) await page.keyboard.press('+');
    await expect(page.locator('#age')).toHaveText(['Dark Age', 'Feudal Age', 'Castle Age', 'Imperial Age'][age]);
    expect((await game.snapshot()).entities.filter(e => e.owner === 1 && e.kind === 'building').every(e => (e.appearance_age ?? 0) === age)).toBe(true);
    await page.screenshot({ path: info.outputPath(`settlement-age-${age}.png`) });
    await battlefieldKey(page, 'r');
  }
  async function advance(name: string, age: number) {
    await battlefieldKey(page, 'h');
    await page.getByRole('button', { name: 'Orders', exact: true }).click();
    await game.command('age', () => page.locator('#actions').getByRole('button', { name: new RegExp(name) }).click());
    await expect.poll(async () => (await game.snapshot()).player.age, { timeout: 20_000 }).toBe(age);
    await capture(age);
  }
  await capture(0);
  await build(page, game, 'mill', 'Mill');
  await build(page, game, 'lumber_camp', 'Lumber Camp');
  await advance('Feudal Age', 1);
  await build(page, game, 'market', 'Market');
  await build(page, game, 'blacksmith', 'Blacksmith');
  await page.getByRole('button', { name: 'Match menu', exact: true }).click();
  await game.command('speed', () => page.locator('#game-speed').selectOption('1'));
  await page.getByRole('button', { name: 'Close menu', exact: true }).click();
  await select(page, game, 'market');
  await page.getByRole('button', { name: 'Trade', exact: true }).click();
  // Buy food for age costs and population upkeep. Wait for each observed
  // transaction before buying again so the next click uses its new quote.
  for (let i = 0; i < 5; i++) {
    const gold = (await game.snapshot()).player.resources.gold;
    await game.command('market_buy', () => page.locator('#actions').getByRole('button', { name: /Buy food/ }).click());
    await expect(page.locator('.resource.gold strong')).not.toHaveText(Math.floor(gold).toLocaleString());
  }
  await page.getByRole('button', { name: 'Match menu', exact: true }).click();
  await game.command('speed', () => page.locator('#game-speed').selectOption('32'));
  await page.getByRole('button', { name: 'Close menu', exact: true }).click();
  await advance('Castle Age', 2);
  await build(page, game, 'monastery', 'Monastery');
  await build(page, game, 'university', 'University');
  await advance('Imperial Age', 3);
});
