import { test, expect } from '@playwright/test';
import { ObservationClock, ObservedMotion } from '../../src/interpolation';
import { changedMapCells, observationTime, SnapshotAssembler } from '../../src/snapshot-stream';
import type { Snapshot, SnapshotDelta } from '../../src/api.generated';

// Pure presentation checks. These fixtures never enter the live renderer or
// grant a browser game authority; public gameplay is tested by the UI suite.
const sampleSnapshot = (): Snapshot => ({
	marketplace: {markets: [], offers: [], shipments: [], merchants: {region_id:-1,biome:'temperate',production:{food:0,wood:0,gold:0,stone:0},demand:{food:0,wood:0,gold:0,stone:0},supply_state:'preparing',supply_interval:90,next_supply_in:90,stock: {food:0,wood:0,gold:0,stone:0}, revision:0, actions:[]}, reserved: {food:0,wood:0,gold:0,stone:0}, capacity:500, max_lots:20, max_offers:12},
  version: 'test', difficulty: { id: 'peaceful', name: 'Practice', description: '' }, settlements: 1, world: {},
  tick: 0, time: 0, speed: 1, paused: false, status: 'running', winner: 0, treaty_remaining: 0,
  player: { id: 1, name: 'Player', civilization: 'britons', resources: { food: 0, wood: 0, gold: 0, stone: 0 },
    production: { rates: { food: 0, wood: 0, gold: 0, stone: 0 }, history: [], sample_seconds: 5, window_seconds: 60 },
    age: 0, age_name: 'Dark Age', population: 0, capacity: 5, limit: 200, idle: 0, workers: 0, military: 0, technologies: [], defeated: false, kills: 0 },
  map: { width: 2, height: 1, biome: 'temperate', tiles: [{ terrain: 'grass', elevation: 0 }, { terrain: 'unknown', elevation: 0 }], fog: [2, 0] },
  entities: [], events: [], event_cursor: 0, opponents: [], projectiles: [], effects: [], build_options: [],
});
const sampleDelta = (values: Partial<SnapshotDelta> = {}): SnapshotDelta => ({
  control_revision: 0, tick: 1, time: .05, speed: 1, paused: false, status: 'running', winner: 0,
  treaty_remaining: 0, event_cursor: 0, projectiles: [], effects: [], ...values,
});

test('stream assembly preserves unchanged views and never changes an older map', () => {
  const assembler = new SnapshotAssembler(), snapshot = sampleSnapshot();
  const first = assembler.apply({ sequence: 1, base: 0, sample_ms: 0, snapshot }, 10);
  const second = assembler.apply({ sequence: 2, base: 1, sample_ms: 50, delta: sampleDelta() }, 60);
  expect(second.map).toBe(first.map); expect(second.entities).toBe(first.entities); expect(second.player).toBe(first.player);
  expect(changedMapCells(second.map, first.map)).toEqual([]);
  const third = assembler.apply({ sequence: 3, base: 2, sample_ms: 100, delta: sampleDelta({ cells: [{ index: 1, tile: { terrain: 'water', elevation: -.2 }, fog: 2 }] }) }, 120);
  expect(first.map.fog).toEqual([2, 0]); expect(first.map.tiles[1].terrain).toBe('unknown');
  expect(third.map.fog).toEqual([2, 2]); expect(changedMapCells(third.map, second.map)).toEqual([1]);
  const fourth = assembler.apply({ sequence: 4, base: 3, sample_ms: 150, delta: sampleDelta({ cells: [{ index: 0, tile: { terrain: 'grass', elevation: 0 }, fog: 1 }] }) }, 160);
  expect(changedMapCells(fourth.map, third.map)).toEqual([0]);
  expect(changedMapCells(fourth.map, first.map)).toBeUndefined(); // skipped presentation needs a full comparison
  expect(observationTime(fourth)?.sampleMS).toBe(150);
});

test('stream gaps require a fresh baseline and paused commands do not depend on tick changes', () => {
  const assembler = new SnapshotAssembler();
  expect(() => assembler.apply({ sequence: 2, base: 1, sample_ms: 50, delta: sampleDelta() })).toThrow('baseline');
  const initial = assembler.apply({ sequence: 1, base: 0, sample_ms: 0, snapshot: sampleSnapshot() });
  const paused = assembler.apply({ sequence: 2, base: 1, sample_ms: 50, delta: sampleDelta({ tick: 0, paused: true, status: 'paused' }) });
  expect(paused.tick).toBe(initial.tick); expect(paused.paused).toBe(true);
  expect(() => assembler.apply({ sequence: 4, base: 3, sample_ms: 150, delta: sampleDelta() })).toThrow('baseline');
  const replacement = sampleSnapshot(); replacement.player.id = 2;
  const fresh = new SnapshotAssembler().apply({ sequence: 1, base: 0, sample_ms: 0, snapshot: replacement });
  expect(fresh.player.id).toBe(2); expect(observationTime(fresh)?.epoch).not.toBe(observationTime(initial)?.epoch);
});

test('movement follows observed turns and stops at the last known position during a stalled stream', () => {
  const motion = new ObservedMotion(), out = { x: 0, y: 0, z: 0 };
  motion.add(0, { x: 0, y: 0, z: 0 });
  motion.add(50, { x: 10, y: 0, z: 0 });
  motion.add(100, { x: 10, y: 0, z: 10 });
  motion.sample(25, out); expect(out).toEqual({ x: 5, y: 0, z: 0 });
  motion.sample(75, out); expect(out).toEqual({ x: 10, y: 0, z: 5 });
  expect(motion.sample(5000, out)).toBe(false); expect(out).toEqual({ x: 10, y: 0, z: 10 });
  motion.add(6000, { x: 30, y: 2, z: 40 }, true);
  motion.sample(0, out); expect(out).toEqual({ x: 30, y: 2, z: 40 }); // rejoin has no old path
});

test('buffered clock tolerates delivery jitter without restarting movement on every packet', () => {
  const clock = new ObservationClock(), motion = new ObservedMotion(), epoch = {};
  const arrivals = [0, 65, 110, 155, 218, 258, 315, 352, 415, 460, 506, 552, 610, 654, 705, 756, 808, 858, 908, 962, 1006];
  let packet = 0, previous = 0, previousTime = 0, frozenFrames = 0;
  for (let now = 0; now <= 1000; now += 10) {
    while (packet < arrivals.length && arrivals[packet] <= now) {
      const sample = packet * 50, reset = clock.observe(sample, arrivals[packet], epoch);
      motion.add(sample, { x: sample, y: 0, z: 0 }, reset); packet++;
    }
    const time = clock.time(now), out = { x: 0, y: 0, z: 0 };
    expect(time).toBeGreaterThanOrEqual(previousTime); previousTime = time;
    motion.sample(time, out);
    expect(out.x).toBeGreaterThanOrEqual(previous);
    expect(out.x).toBeLessThanOrEqual((packet - 1) * 50);
    if (now > 250 && out.x === previous) frozenFrames++;
    previous = out.x;
  }
  expect(frozenFrames).toBe(0);
  expect(clock.observe(0, 2000, {})).toBe(true); expect(clock.time(2010)).toBe(0);
});
