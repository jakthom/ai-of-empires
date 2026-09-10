import type { ProductionView, Resources } from './api.generated';

// A projection of server samples: the browser never infers income from changes
// in stockpiles, or advances samples while a match is paused/disconnected.
const compactRate = new Intl.NumberFormat(undefined, { notation: 'compact', maximumFractionDigits: 0 });
export function renderProduction(production: ProductionView) {
  for (const resource of ['food', 'wood', 'gold', 'stone'] as (keyof Resources)[]) {
    const chart = document.getElementById(`production-${resource}`)!;
    const samples = production.history;
    const rate = Math.round(production.rates[resource]);
    const label = `${resource}: +${rate.toLocaleString()} per game minute. Rolling ${production.window_seconds} seconds; five minutes of history. Harvest deliveries and relics; excludes trade, purchases and refunds.`;
    chart.setAttribute('aria-label', label); chart.title = label;
    chart.querySelector('small')!.textContent = `+${compactRate.format(rate)}/min`;
    const end = samples.at(-1)?.time ?? 0;
    const max = Math.max(1, ...samples.map(sample => sample.rates[resource]));
    const points = samples.map(sample => `${Math.max(0, 120 - (end - sample.time) * .4).toFixed(2)},${(19 - 17 * sample.rates[resource] / max).toFixed(2)}`);
    // Empty and zero income have a visible baseline, not fabricated history.
    chart.querySelector('.production-line')!.setAttribute('d', points.length ? `M${points.join('L')}` : '');
  }
}
