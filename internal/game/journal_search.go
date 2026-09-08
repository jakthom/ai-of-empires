package game

import (
	"regexp"
	"strconv"
	"strings"
)

type LogFilter struct {
	ID   string `json:"id"`
	Name string `json:"name"`
}

var logFilters = []LogFilter{
	{"", "All categories"}, {"orders", "Orders"}, {"activity", "Activity"},
	{"economy", "Resources & trade"}, {"combat", "Combat & healing"},
	{"production", "Building & production"}, {"world", "World events"},
}

var logCategories = map[string]string{
	"order": "orders", "order_rejected": "orders",
	"activity": "activity", "retargeted": "activity",
	"harvest": "economy", "delivery": "economy", "trade": "economy", "exchange": "economy", "relic": "economy",
	"attack": "combat", "damage": "combat", "healed": "combat", "converted": "combat",
	"queued": "production", "completed": "production", "cancelled": "production", "production": "production", "life": "production", "siege": "production",
	"created": "world", "discovered": "world", "destroyed": "world", "notice": "world",
}

func ValidLogCategory(category string) bool {
	for _, filter := range logFilters {
		if category == filter.ID {
			return true
		}
	}
	return false
}

var identitySpacing = regexp.MustCompile(`([[:alpha:]])([0-9])`)

func searchWords(text string) []string {
	text = strings.ToLower(strings.NewReplacer("#", " #", "_", " ").Replace(text))
	words := strings.Fields(identitySpacing.ReplaceAllString(text, "$1 $2"))
	terms := []string{}
	for i := 0; i < len(words); i++ {
		word := words[i]
		if word == "#" && i+1 < len(words) {
			i++
			word += words[i]
		}
		explicitID := strings.HasPrefix(word, "#")
		id, err := strconv.Atoi(strings.TrimPrefix(word, "#"))
		if err == nil && id > 0 && (explicitID || len(words) == 1 || followsEntityName(words[:i])) {
			word = "#" + strconv.Itoa(id)
		}
		terms = append(terms, word)
	}
	return terms
}

func followsEntityName(words []string) bool {
	prefix := " " + strings.Join(words, " ")
	for _, definition := range definitions {
		if strings.HasSuffix(prefix, " "+strings.ToLower(definition.Name)) || strings.HasSuffix(prefix, " "+strings.ReplaceAll(definition.ID, "_", " ")) {
			return true
		}
	}
	return false
}

type journalSearch struct {
	player, entity, scanned int
	search, category        string
	indices                 []int
}

// Cache a bounded number of derived indexes, never event copies. Live searches
// inspect only newly appended records; switching queries still searches the
// complete authorized history. Cache eviction cannot remove any history.
func (j *eventJournal) filtered(player int, query LogQuery, source []int) []int {
	words := searchWords(query.Search)
	if len(words) == 0 && query.Category == "" {
		return source
	}
	key := strings.Join(words, " ")
	var cached *journalSearch
	for _, search := range j.searches {
		if search.player == player && search.entity == query.EntityID && search.search == key && search.category == query.Category {
			cached = search
			break
		}
	}
	if cached == nil {
		cached = &journalSearch{player: player, entity: query.EntityID, search: key, category: query.Category}
		if len(j.searches) == 8 {
			j.searches = j.searches[1:]
		}
		j.searches = append(j.searches, cached)
	}
	for _, index := range source[cached.scanned:] {
		if matchesLog(j.records[index], words, query.Category) {
			cached.indices = append(cached.indices, index)
		}
	}
	cached.scanned = len(source)
	return cached.indices
}

func matchesLog(event Event, words []string, category string) bool {
	if category != "" && logCategories[event.Kind] != category {
		return false
	}
	if len(words) == 0 {
		return true
	}
	text := strings.ToLower(strings.Join([]string{event.Message, event.EntityName, event.EntityType, event.Activity, event.Resource, event.State, event.Kind}, " "))
	text = strings.ReplaceAll(text, "_", " ")
	for _, word := range words {
		id, err := strconv.Atoi(strings.TrimPrefix(word, "#"))
		if strings.HasPrefix(word, "#") && err == nil {
			// An identity query must not also select #30 or a different actor
			// merely mentioning #3 in its message or target.
			if event.EntityID != id {
				return false
			}
		} else if !strings.Contains(text, word) {
			return false
		}
	}
	return true
}
