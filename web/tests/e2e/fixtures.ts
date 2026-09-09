import { test as base, expect, type Page, type Response } from '@playwright/test';
import type { Command, MemberSession, Session, Snapshot } from '../../src/api.generated';
import * as THREE from 'three';
import { BattlefieldTerrain } from '../../src/terrain';

export type Game = {
  start: (mode?: 'skirmish' | 'sandbox', difficulty?: string, settlements?: number, name?: string) => Promise<Session>;
  snapshot: () => Promise<Snapshot>;
  point: (type: string, index?: number) => Promise<{x:number; y:number}>;
  command: (kind: string, action: () => Promise<unknown>) => Promise<Response>;
};

// Observe our own match-creation response; never reach into browser storage or
// application internals. All player intentions below go through the actual UI.
export const test = base.extend<{ game: Game }>({
  game: async ({ page, request }, use) => {
    const sessions: Session[] = [];
    const pageErrors: string[] = [];
    page.on('pageerror', error => pageErrors.push(error.message));
    const created = async (response: Response) => {
      const path = new URL(response.url()).pathname;
      if (response.request().method() === 'POST' && (path === '/api/v1/games' && response.status() === 201 || path === '/api/v1/memberships/rejoin' && response.status() === 200)) {
        sessions.push(await response.json() as Session);
      }
      if(response.request().method()==='GET'&&/^\/api\/v1\/games\/[^/]+\/session$/.test(path)&&response.ok()){const v=await response.json() as MemberSession;const old=sessions.find(s=>s.match_id===v.match_id);if(old)sessions.push(old);}
    };
    page.on('response', created);
    const game: Game = {
      async start(mode = 'skirmish', difficulty = 'peaceful', settlements = 2, name = '') {
        if (page.url() === 'about:blank') await page.goto('/');
        await expect(page.locator('#start-dialog')).toBeVisible();
        await page.getByRole('combobox', { name: 'Your civilization', exact: true }).selectOption('britons');
        await page.getByRole('combobox', { name: 'Difficulty', exact: true }).selectOption(difficulty);
        await page.getByRole('combobox', { name: 'Opening', exact: true }).selectOption(mode);
        await page.getByRole('combobox', { name: 'Settlements', exact: true }).selectOption(String(settlements));
        await page.getByLabel('Game name', { exact: true }).fill(name);
        const response = page.waitForResponse(r => r.request().method() === 'POST' && new URL(r.url()).pathname === '/api/v1/games');
        await page.getByRole('button', { name: 'Begin your reign' }).click();
        const result = await response;
        expect(result.status()).toBe(201);
        const session = await result.json() as Session;
        await expect(page.locator('#room-panel')).toBeVisible();
        await page.getByRole('button',{name:'Ready',exact:true}).click();
        await page.getByRole('button',{name:'Start game',exact:true}).click();
        await expect(page.locator('#start-dialog')).not.toBeVisible();
        await expect(page.locator('#connection')).toBeHidden();
        await expect(page.locator('#selected-name')).toHaveText('Town Center');
        await expect(page.locator('#world canvas')).toBeVisible();
        if (await page.getByRole('button', { name: 'Understood' }).isVisible()) await page.getByRole('button', { name: 'Understood' }).click();
        return session;
      },
      async snapshot() {
        const session = sessions.at(-1);
        if (!session) throw new Error('Start a UI session first.');
        const response = await request.get(`/api/v1/games/${session.match_id}/snapshot`, { headers: { Authorization: `Bearer ${session.token}` } });
        expect(response.ok()).toBeTruthy();
        return response.json() as Promise<Snapshot>;
      },
      async point(type, index = 0) {
        const snapshot = await game.snapshot();
        const entity = snapshot.entities.filter(e => e.type === type && (e.owner === 1 || e.owner === 0))[index];
        if (!entity) throw new Error(`No observed ${type}.`);
        return projectOpening(page, snapshot, entity.position, type === 'tree' ? 1.5 : .45);
      },
      async command(kind, action) {
        const response = page.waitForResponse(r => {
          if(r.request().method() !== 'POST') return false;
          if(kind==='pause')return /\/(pause|resume)$/.test(r.url());
          if(kind==='speed')return r.url().endsWith('/speed');
          if(!r.url().endsWith('/commands'))return false;
          return (r.request().postDataJSON() as Command).kind === kind;
        });
        await action();
        const result = await response;
        expect(result.status(), await result.text()).toBe(200);
        return result;
      },
    };
    try { await use(game); }
    finally {
      page.off('response', created);
      for (const session of new Map(sessions.map(s => [s.match_id, s])).values()) {
        const headers={Authorization:`Bearer ${session.token}`};
        const info=await request.get(`/api/v1/games/${session.match_id}`,{headers});
        if(info.status()===404)continue;
        expect(info.ok()).toBeTruthy();const g=await info.json();
        const response = await request.delete(`/api/v1/games/${session.match_id}`, {headers, data:{id:crypto.randomUUID(),revision:g.revision,confirm:true}});
        expect([204, 404]).toContain(response.status());
      }
      expect(pageErrors, 'uncaught browser exceptions').toEqual([]);
    }
  },
});

export { expect };

// Project observed positions into the default camera, without accessing or
// changing application state. The pointer still uses the real canvas picking.
export async function projectOpening(page: Page, snapshot: Snapshot, point: {x:number; y:number}, height = 0) {
  const box = await page.locator('#world canvas').boundingBox();
  if (!box) throw new Error('No battlefield bounds.');
  const home = snapshot.entities.find(e => e.owner === snapshot.player.id && e.type === 'town_center')!.position;
  const terrain = new BattlefieldTerrain(snapshot.map);
  try {
    const camera = new THREE.PerspectiveCamera(38, box.width / box.height, .1, 350);
    const target = new THREE.Vector3(home.x, terrain.height(home), home.y);
    const distance = 13 / Math.tan(THREE.MathUtils.degToRad(19)), tilt = Math.PI * .24;
    const horizontal = distance * Math.cos(tilt) / Math.SQRT2;
    camera.position.copy(target).add(new THREE.Vector3(horizontal, distance * Math.sin(tilt), horizontal));
    camera.lookAt(target); camera.updateMatrixWorld();
    const v = new THREE.Vector3(point.x, terrain.height(point) + height, point.y).project(camera);
    return { x: box.x + (v.x + 1) * box.width / 2, y: box.y + (1 - v.y) * box.height / 2 };
  } finally { terrain.dispose(); }
}

export async function battlefieldKey(page: Page, key: string) {
  await page.locator('#world canvas').focus();
  await page.keyboard.press(key);
}
