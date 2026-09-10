// Each silhouette names exactly one building throughout the command deck.
// The olive/brass field-manual style remains legible at the 32px action size.
const drawings: Record<string, string> = {
  bridge: '<path d="M3 18h34M4 18v-7m8 7V9m8 9V9m8 9V9m8 9v-7M3 12h34M5 18v14m30-14v14M8 31q12-23 24 0M2 35q4-3 9 0t9 0t9 0t9 0"/>',
  town_center: '<path d="M5 33V19l8-6h15l7 6v14ZM3 19h34M15 14V7l5-4 5 4v7M14 7h12M18 33v-9h5v9M8 23h4m15 0h4M20 3V1"/>',
  house: '<path d="M6 20 20 7l14 13M10 17v17h20V17M25 11V5h5v11M17 34V23h6v11M12 20h3"/>',
  mill: '<path d="m14 34 3-17h7l3 17ZM20 17V4m0 13L8 9m12 8L8 26m12-9 12 9m-12-9 12-8M7 8l5 1-2 4Zm25 0-5 1 2 4ZM8 26l5-1-2-4Zm24 0-5-1 2-4Z"/><circle cx="20" cy="17" r="2.5"/>',
  lumber_camp: '<path d="M4 17 15 8l10 9M7 17v9m15-9v7M4 17h22M8 28h18v7H8M28 4l-6 15m5-14 9 3-2 7-10-4"/><circle cx="8" cy="31.5" r="3.5"/><circle cx="26" cy="31.5" r="3.5"/>',
  mining_camp: '<path d="m4 20 12-9 12 9M7 19v13h16V19M3 33h27M23 6q6-5 13 2M30 5 20 24m8 6 4-7 5 6-2 6h-8ZM11 31l3-10h5l2 10"/>',
  farm: '<path d="m3 24 15-12 19 10-15 14ZM8 26l15-12m-9 15 15-12m-9 15 14-11M12 17V4m0 6L7 6m5 8 5-5m-5 8-5-5"/>',
  barracks: '<path d="M4 21 12 13h16l8 8M6 21h28v13H6ZM18 34v-9h5v9M10 24v5m19-5v5M15 3l11 11m-1-11L14 14M13 2l3 1-1 3m12-4-3 1 1 3"/>',
  archery_range: '<path d="M3 22V12l9-7 10 7v7M2 12h21M7 22V16h7v6M23 29l-4 6m10-6 4 6"/><circle cx="26" cy="23" r="9"/><circle cx="26" cy="23" r="5"/><circle cx="26" cy="23" r="1"/><path d="m15 15 11 8m-11-8 1 5m-1-5 5 1"/>',
  stable: '<path d="M4 18 20 7l16 11M7 17v17h26V17M13 34V22l6-7 3 1 4 7-4 3-3-2-1 10M19 15l1-4 3 6m1 5h1"/>',
  blacksmith: '<path d="M5 20 14 13h13l7 7M7 19v15h25V19M24 13V4h5v10M23 4h7M11 23h19l-5 5h-5v4h-6v-4l-4-2ZM26 3q-4-3 0-5"/>',
  market: '<path d="M4 17 8 8h24l4 9v4H4ZM8 21v13m24-13v13M6 34h28M14 9l-2 12m8-12v12m6-12 2 12M10 27h20v7H10"/>',
  tower: '<path d="M12 34h16l-2-20 3-3V5h-5v4h-8V5h-5v6l3 3ZM10 34h20M18 34v-7h4v7m-4-18h4v5h-4Z"/>',
  wall: '<path d="M4 34V12h6v6h7v-6h6v6h7v-6h6v22ZM4 25h32m-22-7v7m13-7v7M9 25v9m12-9v9m10-9v9"/>',
  gate: '<path d="M4 34V9h5V5h5v4h12V5h5v4h5v25M13 34V22a7 7 0 0 1 14 0v12M4 34h9m14 0h9M13 25h14m-10-9v18m6-18v18M7 13v5m25-5v5"/>',
  palisade: '<path d="M5 35V12l4-7 4 7v23m0 0V10l4-7 4 7v25m0 0V12l4-7 4 7v23m0 0V15l4-7 4 7v20M5 21h32M5 29h32"/>',
  castle: '<path d="M4 34V9h4V5h4v4h4v5h8V9h4V5h4v4h4v25ZM16 14V4h3V1h3v3h2v10M4 17h12m8 0h12M17 34v-9a3 3 0 0 1 6 0v9M9 22v4m22-4v4"/>',
  siege_workshop: '<path d="M3 19 13 10h17l7 9M6 19v15h29V19M14 34V22h13v12M8 8l6-5m-1 3 9 10M25 27l5-3 5 3-2 5h-5Z"/><circle cx="11" cy="31" r="4"/><path d="M11 27v8m-4-4h8"/>',
  monastery: '<path d="M6 34V20l9-8 9 8v14M3 20h24M27 34V12l5-8 5 8v22M26 12h12M32 4V1M29 2h6M12 34v-7a3 3 0 0 1 6 0v7"/><circle cx="15" cy="20" r="3"/>',
  university: '<path d="M5 20 20 11l15 9M8 20v14h24V20M12 22v10m8-10v10m8-10v10M4 35h32M12 4q4-2 8 1 4-3 8-1v6q-4-2-8 1-4-3-8-1ZM20 5v6"/>',
  dock: '<path d="M3 28h34M7 28v7m12-7v7m12-7v7M7 24h24M12 24V12h17v12M10 12l11-8 10 8M5 32q4-3 8 0t8 0t8 0t8 0M21 4V1"/>',
  wonder: '<path d="M3 35h34M6 31h28M9 27h22M11 27V14h18v13M9 14h22M12 11a8 8 0 0 1 16 0ZM16 17v7m8-7v7M20 3V1M18 29h4"/>',
};

export function buildingIcon(type: string): string {
  if (!Object.hasOwn(drawings, type)) return '';
  return `<svg class="building-icon" data-building-icon="${type}" viewBox="0 0 40 40" fill="none" stroke="currentColor" stroke-width="1.8" stroke-linecap="round" stroke-linejoin="round" aria-hidden="true">${drawings[type]}</svg>`;
}
