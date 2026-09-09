import type { MapView, Snapshot, SnapshotFrame } from './api.generated';

const mapIDs = new WeakMap<MapView, number>();
let nextMapID = 0;
const mapID = (map: MapView) => {
  let id = mapIDs.get(map);
  if (id === undefined) { id = ++nextMapID; mapIDs.set(map, id); }
  return id;
};
// IDs avoid retaining a linked chain of old full maps through patch metadata.
const mapEdits = new WeakMap<MapView, { base: number; indices: number[] }>();
const observations = new WeakMap<Snapshot, { epoch: object; sampleMS: number; receivedMS: number }>();

export const observationTime = (snapshot: Snapshot) => observations.get(snapshot);
export function changedMapCells(map: MapView, previous?: MapView): readonly number[] | undefined {
  if (map === previous) return [];
  const edits = mapEdits.get(map);
  return edits && previous && edits.base === mapID(previous) ? edits.indices : undefined;
}

// Reassemble server observations only. Replacements include every entity
// field, so cleared optional values cannot survive in an older read model.
// One baseline per connection; a gap discards it and reconnects for a keyframe.
export class SnapshotAssembler {
  private sequence = 0;
  private snapshot?: Snapshot;
  private entities = new Map<number, Snapshot['entities'][number]>();
  private epoch = {};

  apply(frame: SnapshotFrame, receivedMS = performance.now()): Snapshot {
    if (!Number.isInteger(frame.sequence) || frame.sequence <= this.sequence || !Number.isFinite(frame.sample_ms)) throw new Error('Invalid stream sequence');
    if (frame.snapshot && !frame.delta && frame.base === 0) {
      this.epoch = {};
      this.snapshot = frame.snapshot;
      this.entities = new Map(frame.snapshot.entities.map(e => [e.id, e]));
    } else if (frame.delta && !frame.snapshot && this.snapshot && frame.base === this.sequence && frame.sequence === this.sequence + 1) {
      const old = this.snapshot, d = frame.delta;
      let map = old.map;
      if (d.cells?.length) {
        map = { ...map, tiles: map.tiles.slice(), fog: map.fog.slice() };
        const indices: number[] = [];
        for (const cell of d.cells) {
          if (!Number.isInteger(cell.index) || cell.index < 0 || cell.index >= map.tiles.length) throw new Error('Invalid map cell');
          map.tiles[cell.index] = cell.tile; map.fog[cell.index] = cell.fog; indices.push(cell.index);
        }
        mapEdits.set(map, { base: mapID(old.map), indices });
      }
      for (const id of d.removed_entities ?? []) this.entities.delete(id);
      for (const entity of d.entities ?? []) this.entities.set(entity.id, entity);
      this.snapshot = {
        ...old, control_revision: d.control_revision, tick: d.tick, time: d.time, speed: d.speed,
        paused: d.paused, status: d.status, winner: d.winner, treaty_remaining: d.treaty_remaining,
        event_cursor: d.event_cursor, projectiles: d.projectiles, map,
        player: d.player ?? old.player, opponents: d.opponents ?? old.opponents,
		marketplace: d.marketplace ?? old.marketplace,
        events: d.events ?? old.events, build_options: d.build_options ?? old.build_options,
        entities: d.entities?.length || d.removed_entities?.length ? [...this.entities.values()] : old.entities,
      };
    } else throw new Error('Stream baseline lost');
    this.sequence = frame.sequence;
    observations.set(this.snapshot, { epoch: this.epoch, sampleMS: frame.sample_ms, receivedMS });
    return this.snapshot;
  }
}
