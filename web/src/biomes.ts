// Scenery only. Tile elevations, walkability, resources and fog come from Go.
const palettes: Record<string, Record<string, string>> = {
  temperate: { grass: '#899d60', cliff: '#8b907b', water: '#4c8584', shallows: '#8bb4a2', earth: '#796d50', sky: '#82988a' },
  desert: { grass: '#cbb184', cliff: '#aa8868', water: '#397f83', shallows: '#8db4a2', earth: '#a87d55', sky: '#b9b095' },
  alpine: { grass: '#c6d1d0', cliff: '#818d99', water: '#466d87', shallows: '#91b8c5', earth: '#6e797d', sky: '#a3b8c5' },
  tropical: { grass: '#719d58', cliff: '#888e73', water: '#357e85', shallows: '#83c1ae', earth: '#806847', sky: '#91b5a9' },
};
export const biomePalette = (biome?: string) => palettes[biome ?? 'temperate'] ?? palettes.temperate;
