import type { Page, Response } from '@playwright/test';
import type { Snapshot } from '../../src/api.generated';
import { test, expect, projectOpening } from './fixtures';

type FrameMetrics = {
  elapsedMs: number;
  frameCount: number;
  p50Ms: number;
  p95Ms: number;
  p99Ms: number;
  maxMs: number;
  over50Ms: number;
  longTasks: number[];
};

function workload(snapshot: Snapshot) {
  return {
    entities: snapshot.entities.length,
    ownedEntities: snapshot.entities.filter(entity => entity.owner === snapshot.player.id).length,
    units: snapshot.entities.filter(entity => entity.kind === 'unit').length,
    buildings: snapshot.entities.filter(entity => entity.kind === 'building').length,
    resources: snapshot.entities.filter(entity => entity.kind === 'resource').length,
  };
}

async function measureFrames(page: Page, durationMs: number): Promise<FrameMetrics> {
  return page.evaluate(duration => new Promise<FrameMetrics>(resolve => {
    const frameTimes: number[] = [];
    const longTasks: number[] = [];
    const started = performance.now();
    let observer: PerformanceObserver | undefined;
    if (PerformanceObserver.supportedEntryTypes.includes('longtask')) {
      observer = new PerformanceObserver(list => {
        for (const entry of list.getEntries()) longTasks.push(entry.duration);
      });
      observer.observe({ type: 'longtask' });
    }
    const sample = (now: number) => {
      frameTimes.push(now);
      if (now - started < duration) requestAnimationFrame(sample);
      else {
        observer?.disconnect();
        const deltas = frameTimes.slice(1).map((time, index) => time - frameTimes[index]).sort((a, b) => a - b);
        const percentile = (fraction: number) => deltas[Math.min(deltas.length - 1, Math.floor(deltas.length * fraction))] ?? 0;
        resolve({ elapsedMs: now - started, frameCount: frameTimes.length, p50Ms: percentile(.50), p95Ms: percentile(.95), p99Ms: percentile(.99), maxMs: deltas.at(-1) ?? 0, over50Ms: deltas.filter(delta => delta > 50).length, longTasks });
      }
    };
    requestAnimationFrame(sample);
  }), durationMs);
}

async function moveSelectedVillager(page: Page, game: { command: (kind: string, action: () => Promise<unknown>) => Promise<Response> }) {
  const canvas = await page.locator('#world canvas').boundingBox();
  if (!canvas) throw new Error('No battlefield bounds.');
  const button = page.locator('#actions').getByRole('button', { name: /^Give order/ });
  await expect(button).toBeVisible();
  const response = await game.command('move', () => button.click().then(() => page.mouse.click(canvas.x + canvas.width * .78, canvas.y + canvas.height * .72)));
  return (response.request().postDataJSON() as { entity_ids?: number[] }).entity_ids ?? [];
}

test('records a reproducible 16x and 32x rendering baseline with movement and stream cadence', async ({ page, game }, info) => {
  test.setTimeout(120_000);
  await page.setViewportSize({ width: 1440, height: 960 });
  const cdp = await page.context().newCDPSession(page);
  await cdp.send('Network.enable');
  const streamRequests = new Set<string>();
  const streamBytes: { timestamp: number; bytes: number; encodedBytes: number }[] = [];
  cdp.on('Network.requestWillBeSent', event => {
    const url = new URL(event.request.url);
    if (url.pathname.endsWith('/events') && event.request.method === 'GET') streamRequests.add(event.requestId);
  });
  cdp.on('Network.dataReceived', event => {
    if (streamRequests.has(event.requestId)) streamBytes.push({ timestamp: event.timestamp, bytes: event.dataLength ?? 0, encodedBytes: event.encodedDataLength ?? 0 });
  });

  await page.goto('/');
  await page.getByRole('combobox', { name: 'World type', exact: true }).selectOption('mountain_lakes');
  await page.getByRole('combobox', { name: 'Biome', exact: true }).selectOption('mixed');
  await page.getByRole('combobox', { name: 'World size', exact: true }).selectOption('huge');
  await page.locator('.advanced-world summary').click();
  await page.getByRole('combobox', { name: 'Map reveal', exact: true }).selectOption('all');
  await page.getByLabel('Map seed', { exact: true }).fill('82731');
  const seat = await game.start('skirmish', 'peaceful', 6);
  const initial = await game.snapshot();
  expect(initial.world).toMatchObject({ type: 'mountain_lakes', biome: 'mixed', size: 'huge' });
  expect(initial.map.width).toBe(224);
  expect(initial.settlements).toBe(6);
  const selectedVillager = initial.entities.find(entity => entity.owner === initial.player.id && entity.type === 'villager');
  if (!selectedVillager) throw new Error('No owned villager available for movement workload.');
  const selectedPoint = await projectOpening(page, initial, selectedVillager.position);
  await page.mouse.click(selectedPoint.x, selectedPoint.y);
  await expect(page.locator('#selected-name')).toHaveText('Villager');

  await page.getByRole('button', { name: 'Match menu', exact: true }).click();
  await game.command('speed', () => page.getByRole('combobox', { name: 'Game speed', exact: true }).selectOption('16'));
  await page.getByRole('button', { name: 'Close menu', exact: true }).click();
  await expect(page.locator('#speed')).toHaveText('16×');
  // Public gameplay warmup gives all six settlements time to build activity
  // before measurement; no frontend or private simulation state is accessed.
  await page.waitForTimeout(8_000);

  const responseSizes: { path: string; status: number; contentLength?: number }[] = [];
  const recordResponse = (response: Response) => {
    const url = new URL(response.url());
    if (!url.pathname.startsWith('/api/v1/')) return;
    const contentLength = response.headers()['content-length'];
    responseSizes.push({ path: url.pathname, status: response.status(), ...(contentLength ? { contentLength: Number(contentLength) } : {}) });
  };
  page.on('response', recordResponse);
  const measure = async (speed: 16 | 32) => {
    if (speed === 32) {
      await page.getByRole('button', { name: 'Match menu', exact: true }).click();
      await game.command('speed', () => page.getByRole('combobox', { name: 'Game speed', exact: true }).selectOption('32'));
      await page.getByRole('button', { name: 'Close menu', exact: true }).click();
      await expect(page.locator('#speed')).toHaveText('32×');
    }
    const before = await game.snapshot();
    const movingEntityIDs = await moveSelectedVillager(page, game);
    const streamStart = streamBytes.length;
    const frames = await measureFrames(page, 15_000);
    const after = await game.snapshot();
    const moved = movingEntityIDs.map(id => {
      const from = before.entities.find(entity => entity.id === id)?.position;
      const to = after.entities.find(entity => entity.id === id)?.position;
      return from && to ? { id, distance: Math.hypot(to.x - from.x, to.y - from.y) } : { id, distance: 0 };
    });
    const arrivals = streamBytes.slice(streamStart);
    const intervals = arrivals.slice(1).map((event, index) => (event.timestamp - arrivals[index].timestamp) * 1000).sort((a, b) => a - b);
    const percentile = (fraction: number) => intervals[Math.min(intervals.length - 1, Math.floor(intervals.length * fraction))] ?? 0;
    return { speed, before: workload(before), after: workload(after), clock: { before: before.time, after: after.time, gameSeconds: after.time - before.time, wallSeconds: frames.elapsedMs / 1000, gameSecondsPerWallSecond: (after.time - before.time) / (frames.elapsedMs / 1000) }, movingEntityIDs, moved, frames, eventsStream: { bytes: arrivals.reduce((sum, event) => sum + event.bytes, 0), encodedBytes: arrivals.reduce((sum, event) => sum + event.encodedBytes, 0), arrivals: arrivals.length, p50ArrivalMs: percentile(.50), p95ArrivalMs: percentile(.95), maxArrivalMs: intervals.at(-1) ?? 0 } };
  };
  const segments = [await measure(16), await measure(32)];
  page.off('response', recordResponse);
  const resourceTimings = await page.evaluate(() => performance.getEntriesByType('resource').map(entry => entry as PerformanceResourceTiming).filter(entry => new URL(entry.name).pathname.startsWith('/api/v1/')).map(entry => ({ path: new URL(entry.name).pathname, transferSize: entry.transferSize, encodedBodySize: entry.encodedBodySize })).slice(-40));
  const result = { config: { world: initial.world, mapWidth: initial.map.width, settlements: initial.settlements, seed: 82731, warmupSeconds: 8, measuredSecondsPerSpeed: 15 }, segments, payloads: { responseHeaders: responseSizes, resourceTimings }, matchID: seat.match_id };
  await info.attach('high-speed-baseline.json', { body: JSON.stringify(result, null, 2), contentType: 'application/json' });
  console.log(`HIGH_SPEED_BASELINE ${JSON.stringify(result)}`);
  await page.screenshot({ path: info.outputPath('high-speed-huge-map.png') });
});
