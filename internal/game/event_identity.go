package game

import "slices"

// BindUser changes who controls a kingdom, never who owns its past events.
// Rebuild derived indexes once at membership changes; normal reads remain
// indexed and cannot reveal the former member's history after a seat reclaim.
func (w *World) BindUser(player int, user string) {
	if p := w.Players[player]; p != nil && p.UserID != user {
		p.UserID = user
		p.UserAliases = nil
		w.indexPrivateJournal()
	}
}

// AdoptUser is used only after proving the original owner's credential. It
// keeps that person's legacy identities readable without rewriting records.
func (w *World) AdoptUser(player int, user string) {
	if p := w.Players[player]; p != nil && p.UserID != user {
		p.UserAliases = append(p.UserAliases, p.UserID)
		p.UserID = user
		w.indexPrivateJournal()
	}
}

func (w *World) indexPrivateJournal() {
	old := w.journal
	w.journal = eventJournal{}
	for i, event := range old.records {
		var readers []int
		if p := w.Players[event.Player]; p != nil && (event.UserID == p.UserID || slices.Contains(p.UserAliases, event.UserID)) {
			readers = []int{p.ID}
		}
		w.journal.append(event, readers)
		w.journal.readers[i] = old.readers[i]
	}
}
