import type { WorldCatalog, WorldOptions } from './api.generated';

const select = (id: string) => document.getElementById(id) as HTMLSelectElement;
export function initializeWorldSetup(catalog: WorldCatalog) {
  const fields = [
    ['type', catalog.types, 'world-description'], ['biome', catalog.biomes, 'biome-description'],
    ['size', catalog.sizes, 'size-description'], ['resources', catalog.resources, 'resources-description'],
    ['separation', catalog.separations, 'separation-description'], ['reveal', catalog.reveals, 'reveal-description'],
  ] as const;
  for (const [key, choices, description] of fields) {
    const input = select(`world-${key}`);
    input.replaceChildren(...choices.map(choice => new Option(choice.name, choice.id)));
    input.value = catalog.defaults[key] ?? choices[0].id;
    const describe = () => {
      const choice = choices.find(item => item.id === input.value);
      document.getElementById(description)!.textContent = choice ? `${'tiles' in choice ? `${choice.tiles} × ${choice.tiles} tiles. ` : ''}${choice.description}` : '';
    };
    input.onchange = describe; describe();
  }
  select('world-treaty').replaceChildren(...catalog.treaty_minutes.map(minutes => new Option(minutes ? `${minutes} game minutes` : 'None', String(minutes))));
  select('world-treaty').value = String(catalog.defaults.treaty_minutes ?? 0);
}
export function readWorldOptions(): WorldOptions {
  return { type: select('world-type').value, biome: select('world-biome').value, size: select('world-size').value,
    resources: select('world-resources').value, separation: select('world-separation').value,
    reveal: select('world-reveal').value, treaty_minutes: Number(select('world-treaty').value) };
}
export function worldDescription(catalog: WorldCatalog, options?: WorldOptions) {
  if (!options?.type) return 'Original world';
  return [catalog.types.find(c => c.id === options.type)?.name, catalog.biomes.find(c => c.id === options.biome)?.name,
    catalog.sizes.find(c => c.id === options.size)?.name].filter(Boolean).join(' · ');
}
