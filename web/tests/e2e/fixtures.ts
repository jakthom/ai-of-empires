import { test as base, expect, type Page, type Response } from '@playwright/test';
import type { Command, Session, Snapshot } from '../../src/api.generated';

type Game = {
  start: (mode?: 'skirmish' | 'sandbox', difficulty?: string, settlements?: number, name?: string) => Promise<Session>;
  snapshot: () => Promise<Snapshot>;
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
      if (response.request().method() === 'POST' && (path === '/api/v1/matches' && response.status() === 201 || path === '/api/v1/sessions/resume' && response.status() === 200)) {
        sessions.push(await response.json() as Session);
      }
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
        const response = page.waitForResponse(r => r.request().method() === 'POST' && new URL(r.url()).pathname === '/api/v1/matches');
        await page.getByRole('button', { name: 'Begin your reign' }).click();
        const result = await response;
        expect(result.status()).toBe(201);
        const session = await result.json() as Session;
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
        const response = await request.get(`/api/v1/matches/${session.match_id}`, { headers: { Authorization: `Bearer ${session.token}` } });
        expect(response.ok()).toBeTruthy();
        return response.json() as Promise<Snapshot>;
      },
      async command(kind, action) {
        const response = page.waitForResponse(r => {
          if (r.request().method() !== 'POST' || !r.url().endsWith('/commands')) return false;
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
        const response = await request.delete(`/api/v1/matches/${session.match_id}`, { headers: { Authorization: `Bearer ${session.token}` } });
        expect([204, 404]).toContain(response.status());
      }
      expect(pageErrors, 'uncaught browser exceptions').toEqual([]);
    }
  },
});

export { expect };

export async function battlefieldKey(page: Page, key: string) {
  await page.locator('#world canvas').focus();
  await page.keyboard.press(key);
}
