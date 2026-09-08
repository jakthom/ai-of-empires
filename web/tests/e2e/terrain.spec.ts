import * as THREE from 'three';
import { BattlefieldTerrain } from '../../src/terrain';
import { terrainAnchorOffset } from '../../src/camera';
import { test, expect } from './fixtures';

for (const settlements of [2,6]) {
  test(`terrain picking matches rendered triangles and exposed banks on a ${settlements}-settlement map`, async ({ page, game }) => {
    await game.start('sandbox','peaceful',settlements);
    const initial = (await game.snapshot()).map, terrain = new BattlefieldTerrain(initial);
    try {
      // The server initially withholds unknown terrain. Scout to the river
      // through the UI, then apply the newly authorized map to the same renderer.
      expect(initial.tiles.some(tile => tile.terrain === 'water')).toBe(false);
      await page.mouse.click(742,472);
      await expect(page.locator('#selected-name')).toHaveText('Scout Cavalry');
      await page.getByRole('button',{name:'Match menu',exact:true}).click();
      await game.command('speed',()=>page.getByRole('combobox',{name:'Game speed',exact:true}).selectOption('32'));
      await page.getByRole('button',{name:'Close menu',exact:true}).click();
      const minimap = await page.locator('#minimap').boundingBox();
      await page.locator('#minimap').click({position:{x:minimap!.width*42.5/initial.width,y:minimap!.height*40.5/initial.height}});
      await page.locator('#actions').getByRole('button',{name:/^Move\b/}).click();
      const field = await page.locator('#world canvas').boundingBox();
      await game.command('move',()=>page.mouse.click(field!.x+field!.width/2,field!.y+field!.height/2));
      await expect.poll(async ()=>(await game.snapshot()).map.tiles.some(tile=>tile.terrain === 'water'),{timeout:12000}).toBe(true);
      const map = (await game.snapshot()).map;
      terrain.update(map); terrain.group.updateMatrixWorld(true);
      const water = map.tiles.findIndex(tile=>tile.terrain === 'water');
      expect(terrain.height({x:water%map.width+.5,y:Math.floor(water/map.width)+.5})).toBeCloseTo(-.8,6);
      const rays: THREE.Raycaster[] = [];
      for (let z = 1; z < map.height; z += 7) for (let x = 1; x < map.width; x += 7) {
        const origin = new THREE.Vector3(x-5,8,z+4), destination = new THREE.Vector3(x,0,z);
        rays.push(new THREE.Raycaster(origin,destination.sub(origin).normalize()));
      }
      // Horizontal rays below the meadow hit the river banks, not the top.
      const banks = map.tiles.flatMap((tile,i) => tile.terrain === 'water' && i%map.width > 0 && map.tiles[i-1].terrain === 'grass' ? [i] : []);
      expect(banks.length).toBeGreaterThan(0);
      for (const river of banks) rays.push(new THREE.Raycaster(new THREE.Vector3(river%map.width+.5,-.4,Math.floor(river/map.width)+.5),new THREE.Vector3(-1,0,0)));
      for (const ray of rays) {
        const expected = ray.intersectObjects(terrain.group.children)[0]?.point ?? null;
        const actual = terrain.hit(ray);
        expect(Boolean(actual)).toBe(Boolean(expected));
        if (actual && expected) expect(actual.distanceTo(expected)).toBeLessThan(1e-6);
      }
      const bank = rays.at(-1)!;
      const hit = terrain.hit(bank);
      expect(hit).not.toBeNull();
      expect(hit!.y).toBeCloseTo(-.4,6);
      expect(hit!.x).toBeLessThan(bank.ray.origin.x);
    } finally { terrain.dispose(); }
  });
}

test('alternating hill and valley zooms preserves the cursor anchor without vertical camera drift', () => {
  const target = new THREE.Vector3(20,0,40), camera = new THREE.PerspectiveCamera(38,1.5,.1,350);
  const direction = new THREE.Vector3(1,1.3,1).normalize();
  let distance = 13 / Math.tan(THREE.MathUtils.degToRad(19));
  const positionCamera = () => { camera.position.copy(target).addScaledVector(direction,distance); camera.lookAt(target); camera.updateMatrixWorld(); };
  positionCamera();
  for (let cycle = 0; cycle < 30; cycle++) for (const [height,zoom] of [[-.8,5],[2.8,30]]) {
    const anchor = new THREE.Vector3(target.x+2,height,target.z-2);
    const screen = anchor.clone().project(camera);
    distance = zoom / Math.tan(THREE.MathUtils.degToRad(19)); positionCamera();
    const ray = new THREE.Raycaster(); ray.setFromCamera(new THREE.Vector2(screen.x,screen.y),camera);
    const offset = terrainAnchorOffset(ray.ray,anchor);
    expect(offset).not.toBeNull(); target.add(offset!); positionCamera();
    const after = anchor.clone().project(camera);
    expect(after.x).toBeCloseTo(screen.x,8); expect(after.y).toBeCloseTo(screen.y,8);
    expect(target.y).toBe(0);
    expect(camera.position.y).toBeGreaterThan(2.8);
  }
  distance = 13 / Math.tan(THREE.MathUtils.degToRad(19)); positionCamera();
  expect(camera.position.y).toBeGreaterThan(2.8);
});
