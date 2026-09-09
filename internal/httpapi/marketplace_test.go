package httpapi

import (
	"net/http/httptest"
	"reflect"
	"testing"

	"crowns/internal/game"
)

func TestMCPMarketplaceUsesAuthorizedRESTViewAndStrictIntent(t *testing.T) {
	f := newMCPGame(t, "hidden")
	f.start(t)
	host := httptest.NewServer(f.server)
	t.Cleanup(host.Close)
	client := connectMCP(t, host, f.owner)
	path := "/api/v1/games/" + f.owner.MatchID + "/marketplace"
	if r := request(t, f.server, "GET", path, "", nil); r.Code != 401 {
		t.Fatal("marketplace omitted authentication", r.Code)
	}
	credential := decodeResponse[AgentEndpoint](t, request(t, f.server, "POST", "/api/v1/games/"+f.owner.MatchID+"/agent", f.owner.Token, nil), 200)
	if r := request(t, f.server, "GET", path, credential.Token, nil); r.Code != 401 {
		t.Fatal("MCP token authorized REST", r.Code)
	}
	rest := decodeResponse[game.MarketplaceView](t, request(t, f.server, "GET", path, f.owner.Token, nil), 200)
	mcp := mcpOutput[game.MarketplaceView](t, client, "marketplace", struct{}{})
	if !reflect.DeepEqual(rest, mcp) || !reflect.DeepEqual(rest, f.view(t, f.owner).Marketplace) {
		t.Fatal("marketplace transports disagree")
	}
	before := f.view(t, f.owner).Player.Resources
	mcpRefusal(t, client, "command", game.Command{ID: "unavailable-market", Kind: "market_post", EntityIDs: []int{ownEntity(t, f.view(t, f.friend), "town_center").ID}, Offer: &game.TradeOfferIntent{GiveResource: "wood", GiveAmount: 100, WantResource: "gold", WantAmount: 100, Lots: 1}}, "market_required")
	invalid := callMCP(t, client, "command", map[string]any{"id": "forged-trade", "kind": "market_post", "entity_ids": []int{1}, "offer": map[string]any{"give_resource": "wood", "give_amount": 100, "want_resource": "gold", "want_amount": 100, "lots": 1, "owner": 2}})
	if !invalid.IsError {
		t.Fatal("unknown nested trade authority accepted")
	}
	if f.view(t, f.owner).Player.Resources != before {
		t.Fatal("refused trade changed resources")
	}
}
