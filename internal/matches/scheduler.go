package matches

import (
	"context"
	"log/slog"
	"math"
	"sync"
	"time"

	"crowns/internal/game"
)

const schedulerInterval = 5 * time.Millisecond
const simulationBudget = 4 * time.Millisecond
const maxClockDebt = 250 * time.Millisecond

// Only fixed Go physics steps advance the world. Elapsed wall time determines
// how many are due; a bounded debt prevents a stalled host from accumulating an
// unbounded catch-up loop. Overloaded games slow down, without larger deltas.
func (m *Match) step(ctx context.Context, now time.Time) {
	elapsed := now.Sub(m.lastStepAt)
	if m.lastStepAt.IsZero() {
		elapsed = schedulerInterval
	}
	m.lastStepAt = now
	if m.available() != nil || m.world == nil {
		return
	}
	if m.world.Status() != "running" || m.room != nil && (m.room.Session.State() != sessionOpen || m.room.Runtime.State() != runtimeServing) {
		m.accumulator = 0
		return
	}
	m.accumulator = math.Min(m.accumulator+math.Max(0, elapsed.Seconds())*m.world.Speed, maxClockDebt.Seconds()*m.world.Speed)
	deadline := time.Now().Add(simulationBudget)
	for n := 0; n < 8 && m.accumulator+1e-9 >= game.Step && ctx.Err() == nil; n++ {
		m.world.Update()
		m.accumulator = math.Max(0, m.accumulator-game.Step)
		if time.Now().After(deadline) {
			break
		}
	}
}

// Each loaded game has one clock worker. A slow game or its final unload save
// cannot delay another game. The registry lock only covers worker admission.
func (s *Service) Run(ctx context.Context) {
	ctx, cancel := context.WithCancel(ctx)
	var workers sync.WaitGroup
	defer func() { cancel(); workers.Wait() }()
	ticker := time.NewTicker(schedulerInterval)
	defer ticker.Stop()
	active := map[*Match]bool{}
	finished := make(chan *Match, 16)
	for {
		select {
		case <-ctx.Done():
			return
		case <-s.Stopping():
			return
		case m := <-finished:
			delete(active, m)
		case <-ticker.C:
			s.mu.Lock()
			if s.lifecycle.State() != serviceServing {
				s.mu.Unlock()
				return
			}
			for _, m := range s.matches {
				if active[m] {
					continue
				}
				active[m] = true
				workers.Add(1)
				go func() {
					defer workers.Done()
					s.runMatch(ctx, m)
					select {
					case finished <- m:
					case <-ctx.Done():
					}
				}()
			}
			s.mu.Unlock()
		}
	}
}

func (s *Service) runMatch(ctx context.Context, m *Match) {
	ticker := time.NewTicker(schedulerInterval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-s.Stopping():
			return
		case <-ticker.C:
		}
		m.mu.Lock()
		now := time.Now()
		if m.available() != nil {
			m.mu.Unlock()
			return
		}
		m.collectAutosave(false)
		unload := false
		if now.Sub(m.lastMaintenance) >= 50*time.Millisecond {
			m.lastMaintenance = now
			if m.room != nil {
				unload = m.pulseRoom(ctx, now)
			} else if leaseExpired(nil, &leaseContext{m, now}) == nil {
				if err := m.saveContext(ctx, now); err != nil {
					slog.Error("checkpoint failed", "match", m.id, "error", err)
					m.lastAccess = now
				} else {
					m.fireLease(leasePulse, now)
					unload = m.lifecycle.State() == leaseClosed
				}
			}
		}
		if !unload {
			m.step(ctx, now)
			if m.db != nil && now.Sub(m.lastSaveAttempt) >= AutosaveInterval {
				m.startAutosave(ctx, now)
			}
		}
		m.mu.Unlock()
		if unload {
			s.mu.Lock()
			if s.matches[m.id] == m {
				delete(s.matches, m.id)
			}
			s.mu.Unlock()
			return
		}
	}
}
