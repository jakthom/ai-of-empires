package matches

import "crowns/internal/game"

func (a *Access) Statistics(all bool) (game.StatisticsReport, error) {
	m := a.match
	m.mu.Lock()
	defer m.mu.Unlock()
	p, err := a.valid(true)
	if err != nil {
		return game.StatisticsReport{}, err
	}
	id := 1
	if p != nil {
		id = p.PlayerID
	}
	if all && m.world.Status() != "finished" {
		if err = a.snapshotOwner(); err != nil {
			return game.StatisticsReport{}, err
		}
	}
	return m.world.Statistics(id, all), nil
}
func (a *Access) ObserverView() (game.Snapshot, error) {
	m := a.match
	m.mu.Lock()
	defer m.mu.Unlock()
	p, err := a.valid(true)
	if err != nil {
		return game.Snapshot{}, err
	}
	if err = a.snapshotOwner(); err != nil {
		return game.Snapshot{}, err
	}
	v := m.world.ObserverView(p.PlayerID)
	v.ControlRevision = m.room.Revision
	return v, nil
}
