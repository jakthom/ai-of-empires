import type { MapView } from './api.generated';
import { biomePalette } from './biomes';
import { changedMapCells } from './snapshot-stream';

// Terrain and fog change independently of camera outlines and unit markers.
// Paint cells once at map resolution, then scale this cached bitmap per frame.
export class MinimapTerrain {
  private canvas = document.createElement('canvas');
  private map?: MapView;

  draw(ctx: CanvasRenderingContext2D, map: MapView, width: number, height: number) {
    if (map !== this.map) {
      const resized = this.canvas.width !== map.width || this.canvas.height !== map.height;
      if (resized) { this.canvas.width = map.width; this.canvas.height = map.height; }
      const paint = this.canvas.getContext('2d')!;
      const indices = !resized && this.map?.biome === map.biome ? changedMapCells(map, this.map) : undefined;
      const cell = (i: number) => {
        const x = i % map.width, y = Math.floor(i / map.width), tile = map.tiles[i];
        paint.globalAlpha = 1; paint.fillStyle = '#202d24'; paint.fillRect(x, y, 1, 1);
        if (map.fog[i]) {
          const palette = biomePalette(tile.biome || map.biome);
          paint.globalAlpha = map.fog[i] === 1 ? .45 : 1;
          paint.fillStyle = palette[tile.terrain] ?? palette.grass; paint.fillRect(x, y, 1, 1);
        }
      };
      if (indices) indices.forEach(cell);
      else for (let i = 0; i < map.tiles.length; i++) cell(i);
      this.map = map;
    }
    ctx.globalAlpha = 1; ctx.imageSmoothingEnabled = false;
    ctx.drawImage(this.canvas, 0, 0, width, height);
  }
}
