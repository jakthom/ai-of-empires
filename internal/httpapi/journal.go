package httpapi

import (
	"net/http"
	"strconv"
	"strings"
	"unicode/utf8"

	"crowns/internal/game"
)

func (s *Server) journal(w http.ResponseWriter, r *http.Request) {
	var readLog func(game.LogQuery) (game.EventPage, error)
	if strings.HasPrefix(r.URL.Path, "/api/v1/games/") {
		a := s.gameAccess(w, r)
		if a == nil {
			return
		}
		defer a.Release()
		readLog = a.Log
	} else {
		m := s.authorize(w, r)
		if m == nil {
			return
		}
		readLog = m.Log
	}
	query := game.LogQuery{Limit: 100}
	values := r.URL.Query()
	invalid := func() {
		writeError(w, 400, "invalid_cursor", "Use either after (0 or greater) or before (1 or greater), and a limit from 1 to 200.")
	}
	for key, list := range values {
		if (key != "after" && key != "before" && key != "limit" && key != "q" && key != "category") || len(list) != 1 {
			invalid()
			return
		}
		if key == "q" {
			if !utf8.ValidString(list[0]) || utf8.RuneCountInString(list[0]) > 200 {
				writeError(w, 400, "invalid_search", "Search must be at most 200 characters.")
				return
			}
			query.Search = list[0]
			continue
		}
		if key == "category" {
			if !game.ValidLogCategory(list[0]) {
				writeError(w, 400, "invalid_filter", "Choose an event category from the catalog.")
				return
			}
			query.Category = list[0]
			continue
		}
		n, err := strconv.Atoi(list[0])
		if err != nil || n < 0 {
			invalid()
			return
		}
		switch key {
		case "after":
			query.After, query.FromStart = n, true
		case "before":
			if n == 0 {
				invalid()
				return
			}
			query.Before = n
		case "limit":
			if n < 1 || n > 200 {
				invalid()
				return
			}
			query.Limit = n
		}
	}
	if query.FromStart && query.Before > 0 {
		invalid()
		return
	}
	if raw := r.PathValue("entity"); raw != "" {
		id, err := strconv.Atoi(raw)
		if err != nil || id <= 0 {
			writeError(w, 400, "invalid_entity_id", "Use a positive entity ID.")
			return
		}
		query.EntityID = id
	}
	page, err := readLog(query)
	if err != nil {
		domainError(w, err)
		return
	}
	respond(w, 200, page)
}
