import * as THREE from 'three';
import { BuildingFoundation, GroundRing, groundFarm } from '../../src/grounding';
import { BattlefieldTerrain } from '../../src/terrain';
import { makeModel } from '../../src/models';
import { test, expect } from './fixtures';

test('selection and expanding order rings follow slopes while remaining occluded by objects', async ({ page, game }) => {
  await page.goto('/');
  await page.getByRole('combobox', { name: 'World type', exact: true }).selectOption('highlands');
  await page.getByText('Advanced world options', { exact: false }).click();
  await page.getByRole('combobox', { name: 'Map reveal', exact: true }).selectOption('all');
  await game.start();
  const snapshot = await game.snapshot(), terrain = new BattlefieldTerrain(snapshot.map);
  const ring = new GroundRing(), marker = new GroundRing(.43 / .35, true);
  try {
    const slopeIndex = snapshot.map.tiles.findIndex((t, i) => t.terrain === 'grass' && t.elevation > .5 && snapshot.map.tiles[i+1]?.terrain === 'grass');
    expect(slopeIndex).toBeGreaterThan(-1);
    const slope = { position: { x: slopeIndex % snapshot.map.width + .5, y: Math.floor(slopeIndex / snapshot.map.width) + .5 } };
    for (const worker of [slope]) {
      for (const mesh of [ring, marker]) for (const radius of [.5, .8]) {
        mesh.place(worker.position, radius, terrain);
        const vertices = mesh.geometry.getAttribute('position'), heights: number[] = [];
        for (let i = 0; i < vertices.count; i++) {
          const ground = terrain.height({ x: vertices.getX(i) + mesh.position.x, y: vertices.getZ(i) + mesh.position.z });
          expect(vertices.getY(i) - ground).toBeGreaterThan(.01);
          expect(vertices.getY(i) - ground).toBeLessThan(.03);
          heights.push(vertices.getY(i));
        }
        expect(Math.max(...heights) - Math.min(...heights)).toBeGreaterThan(.01);
        expect(mesh.material.depthTest).toBe(true);
        expect(mesh.material.depthWrite).toBe(false);
      }
    }
  } finally { ring.dispose(); marker.dispose(); terrain.dispose(); }
});

test('building floors clear the hillside and their foundations reach the ground throughout construction', async ({ game }) => {
  await game.start();
  const snapshot = await game.snapshot(), terrain = new BattlefieldTerrain(snapshot.map);
  const town = snapshot.entities.find(entity => entity.type === 'town_center' && entity.owner === 1)!;
  const foundation = new BuildingFoundation(town);
  try {
    for (const constructionScale of [.2, .6, 1]) {
      const base = foundation.fit(town.position, terrain, constructionScale), vertices = foundation.geometry.getAttribute('position');
      for (let i = 0; i < vertices.count; i++) {
        const ground = terrain.height({ x: town.position.x + vertices.getX(i), y: town.position.y + vertices.getZ(i) });
        const height = base + vertices.getY(i) * foundation.scale.y * constructionScale;
        // Each segment has a level upper edge and a lower edge buried just
        // beneath the terrain; there must be no floating gap at either end.
        if (i % 6 === 0 || i % 6 === 3 || i % 6 === 5) expect(height).toBeGreaterThan(ground);
        else { expect(height).toBeLessThan(ground); expect(height).toBeGreaterThan(ground - .05); }
      }
    }
  } finally { foundation.geometry.dispose(); foundation.material.dispose(); terrain.dispose(); }
});

test('rectangular roofs align with their eaves and the Town Center tower ends below its upper roof', async ({ game }) => {
  await game.start();
  const town = (await game.snapshot()).entities.find(entity => entity.type === 'town_center' && entity.owner === 1)!;
  const model = makeModel(town);
  try {
    const roof = model.children.find(object => object instanceof THREE.Mesh && (object.material as THREE.MeshStandardMaterial).userData.surface === 'roof') as THREE.Mesh;
    const vertices = roof.geometry.getAttribute('position');
    const ys = Array.from({ length: vertices.count }, (_, i) => vertices.getY(i)), base = Math.min(...ys);
    const xs = new Set<number>(), zs = new Set<number>();
    for (let i = 0; i < vertices.count; i++) if (Math.abs(vertices.getY(i) - base) < 1e-5) {
      if (Math.abs(vertices.getX(i)) < 1e-5 && Math.abs(vertices.getZ(i)) < 1e-5) continue; // underside cap center
      xs.add(Math.round(vertices.getX(i) * 10000)); zs.add(Math.round(vertices.getZ(i) * 10000));
    }
    expect(xs.size).toBe(2); expect(zs.size).toBe(2);
    const stone = model.children.find(object => object instanceof THREE.Mesh && (object.material as THREE.MeshStandardMaterial).userData.surface === 'wall') as THREE.Mesh;
    stone.geometry.computeBoundingBox();
    const upperRoofCenter = ys.filter((_, i) => Math.abs(vertices.getX(i) - .9) < 1e-5 && Math.abs(vertices.getZ(i) - .5) < 1e-5);
    const upperEave = Math.min(...upperRoofCenter);
    expect(stone.geometry.boundingBox!.max.y).toBeLessThanOrEqual(upperEave + 1e-5);
  } finally {
    model.traverse(object => { if (object instanceof THREE.Mesh && object.userData.privateGeometry) object.geometry.dispose(); });
  }
});

test('farm soil and crops follow the same observed terrain as the workers', async ({ game }) => {
  await game.start();
  const snapshot = await game.snapshot(), terrain = new BattlefieldTerrain(snapshot.map);
  // A rendering specimen uses a real observed footprint; it cannot place a
  // farm or change any authoritative match state.
  const villager = snapshot.entities.find(entity => entity.type === 'villager' && entity.owner === 1)!;
  const model = makeModel({ ...villager, kind: 'building', type: 'farm', radius: 1.2, progress: 1, amount: 175 });
  try {
    groundFarm(model, villager.position, terrain);
    const base = terrain.height(villager.position), matrix = new THREE.Matrix4();
    const soil = model.children.find(object => object instanceof THREE.Mesh && (object.material as THREE.MeshStandardMaterial).userData.surface === 'soil') as THREE.Mesh;
    const vertices = soil.geometry.getAttribute('position');
    for (let i = 0; i < vertices.count; i++) {
      const ground = terrain.height({ x: villager.position.x + vertices.getX(i), y: villager.position.y + vertices.getZ(i) });
      expect(base + vertices.getY(i)).toBeGreaterThan(ground);
      expect(base + vertices.getY(i)).toBeLessThan(ground + .15);
    }
    const crops = model.children.find(object => object instanceof THREE.InstancedMesh) as THREE.InstancedMesh;
    for (let i = 0; i < crops.count; i++) {
      crops.getMatrixAt(i, matrix);
      const ground = terrain.height({ x: villager.position.x + matrix.elements[12], y: villager.position.y + matrix.elements[14] });
      expect(base + matrix.elements[13] - ground).toBeCloseTo(.23, 5);
    }
  } finally {
    model.traverse(object => { if (object instanceof THREE.Mesh && object.userData.privateGeometry) object.geometry.dispose(); if (object instanceof THREE.InstancedMesh) object.dispose(); });
    terrain.dispose();
  }
});
