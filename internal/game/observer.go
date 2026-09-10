package game

import "slices"

// Administrative observation never changes fog, player knowledge or AI input.
// The match boundary must authenticate the owner before calling this method.
func (w *World) ObserverView(player int) Snapshot {
	v := w.View(player)
	v.GodMode = true
	v.Map.Tiles = slices.Clone(w.Tiles)
	v.Map.Fog = make([]int, len(w.Tiles))
	for i := range v.Map.Fog {
		v.Map.Fog[i] = 2
	}
	v.Entities = []EntityView{}
	for _, id := range w.IDs {
		if e := w.Entities[id]; e != nil && e.Container == 0 {
			v.Entities = append(v.Entities, w.entityView(e, player))
		}
	}
	v.Projectiles = []ProjectileView{}
	for _, p := range w.Projectiles {
		v.Projectiles = append(v.Projectiles, ProjectileView{ID: p.ID, Owner: p.Owner, Position: p.Position, Kind: p.Kind})
	}
	v.Effects = []BattlefieldEffectView{}
	for _, effect := range w.Aftermath {
		v.Effects = append(v.Effects, effect.View)
	}
	return v
}
