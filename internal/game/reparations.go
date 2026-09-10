package game

import (
	"context"
	"fmt"
	"math"
	"slices"

	"github.com/open-ships/statemachine"
)

type peaceState string
type peaceEvent string

const (
	peaceDraft     peaceState = "draft"
	peaceOffered   peaceState = "offered"
	peaceAccepted  peaceState = "accepted"
	peaceDeclined  peaceState = "declined"
	peaceWithdrawn peaceState = "withdrawn"
	peaceExpired   peaceState = "expired"
	peaceSubmit    peaceEvent = "submit"
	peaceAccept    peaceEvent = "accept"
	peaceDecline   peaceEvent = "decline"
	peaceWithdraw  peaceEvent = "withdraw"
	peacePulse     peaceEvent = "pulse"
)

type peaceOffer struct {
	ID, From, To  int
	Gold, Expires float64
	lifecycle     *statemachine.Instance[peaceState, peaceEvent, *peaceContext]
}
type peaceContext struct {
	World *World
	Offer *peaceOffer
	Actor int
}
type PeaceOfferView struct {
	ID          int     `json:"id"`
	From        int     `json:"from"`
	To          int     `json:"to"`
	Gold        float64 `json:"gold"`
	ExpiresIn   float64 `json:"expires_in"`
	State       string  `json:"state"`
	CanAccept   bool    `json:"can_accept"`
	CanDecline  bool    `json:"can_decline"`
	CanWithdraw bool    `json:"can_withdraw"`
}

var peaceMachine = statemachine.MustCompile([]statemachine.Transition[peaceState, peaceEvent, *peaceContext]{
	{From: peaceDraft, Event: peaceSubmit, To: peaceOffered, Guard: canOfferPeace, Do: reservePeacePayment},
	{From: peaceOffered, Event: peaceAccept, To: peaceAccepted, Guard: canAcceptPeace, Do: payForPeace},
	{From: peaceOffered, Event: peaceDecline, To: peaceDeclined, Guard: peaceRecipient, Do: refundPeacePayment},
	{From: peaceOffered, Event: peaceWithdraw, To: peaceWithdrawn, Guard: peaceSender, Do: refundPeacePayment},
	{From: peaceOffered, Event: peacePulse, To: peaceExpired, Guard: peaceOfferExpired, Do: refundPeacePayment},
	{From: peaceOffered, Event: peacePulse, To: peaceAccepted, Guard: aiAcceptsPeace, Do: payForPeace},
	{From: peaceOffered, Event: peacePulse, To: peaceDeclined, Guard: aiDeclinesPeace, Do: refundPeacePayment},
	{From: peaceOffered, Event: peacePulse, To: peaceOffered},
})

func (w *World) peacePrice(from, to int) float64 {
	r := w.Relations[relationKey(from, to)]
	if r == nil {
		return 0
	}
	damage := r.DamageB
	if to == r.A {
		damage = r.DamageA
	}
	return math.Max(50, math.Ceil(damage*1.1))
}
func canOfferPeace(_ context.Context, c *peaceContext) error {
	w, o := c.World, c.Offer
	if c.Actor != o.From || w.Players[o.To] == nil || o.From == o.To || w.relation(o.From, o.To) != inConflict {
		return rule("peace_unneeded", "Choose a kingdom currently in conflict with you.")
	}
	if !finitePositive(o.Gold) || o.Gold != math.Floor(o.Gold) || o.Gold > 1e7 {
		return rule("invalid_payment", "Offer a whole number of gold from 1 to 10,000,000.")
	}
	if w.Players[o.From].Resources.Gold < o.Gold {
		return rule("insufficient_resources", "Gather enough gold for this peace payment.")
	}
	for _, old := range w.Reparations {
		if old.lifecycle.State() == peaceOffered && old.From == o.From && old.To == o.To {
			return rule("peace_pending", "Withdraw or resolve the existing offer first.")
		}
	}
	if len(w.Reparations) >= 1000 {
		return rule("peace_limit", "This campaign has reached its peace-offer limit.")
	}
	return nil
}
func finitePositive(x float64) bool { return x > 0 && !math.IsNaN(x) && !math.IsInf(x, 0) }
func peaceRecipient(_ context.Context, c *peaceContext) error {
	return applicable(c.Actor == c.Offer.To)
}
func peaceSender(_ context.Context, c *peaceContext) error {
	return applicable(c.Actor == c.Offer.From)
}
func canAcceptPeace(ctx context.Context, c *peaceContext) error {
	if err := peaceRecipient(ctx, c); err != nil {
		return err
	}
	return applicable(c.World.Time < c.Offer.Expires && c.World.relation(c.Offer.From, c.Offer.To) == inConflict && c.World.Players[c.Offer.From].lifecycle.State() != PlayerDefeated && c.World.Players[c.Offer.To].lifecycle.State() != PlayerDefeated)
}
func peaceOfferExpired(_ context.Context, c *peaceContext) error {
	o := c.Offer
	w := c.World
	return applicable(w.Time >= o.Expires || w.relation(o.From, o.To) != inConflict || w.Players[o.From].lifecycle.State() == PlayerDefeated || w.Players[o.To].lifecycle.State() == PlayerDefeated)
}
func aiAcceptsPeace(ctx context.Context, c *peaceContext) error {
	if !c.World.Players[c.Offer.To].AI || c.Offer.Gold < c.World.peacePrice(c.Offer.From, c.Offer.To) {
		return applicable(false)
	}
	return canAcceptPeace(ctx, c)
}
func aiDeclinesPeace(_ context.Context, c *peaceContext) error {
	return applicable(c.World.Players[c.Offer.To].AI && c.Offer.Gold < c.World.peacePrice(c.Offer.From, c.Offer.To))
}
func reservePeacePayment(_ context.Context, c *peaceContext) error {
	w, o := c.World, c.Offer
	w.Players[o.From].Resources.Gold -= o.Gold
	if w.Reparations == nil {
		w.Reparations = map[int]*peaceOffer{}
	}
	w.Reparations[o.ID] = o
	w.NextID++
	for _, id := range []int{o.From, o.To} {
		w.event(id, fmt.Sprintf("%s offered %s %.0f gold in reparations to restore peace.", w.Players[o.From].Name, w.Players[o.To].Name, o.Gold))
	}
	return nil
}
func refundPeacePayment(_ context.Context, c *peaceContext) error {
	o := c.Offer
	c.World.Players[o.From].Resources.Gold += o.Gold
	for _, id := range []int{o.From, o.To} {
		c.World.event(id, "The peace payment was returned without changing the relationship.")
	}
	return nil
}
func payForPeace(_ context.Context, c *peaceContext) error {
	w, o := c.World, c.Offer
	w.Players[o.To].Resources.Gold += o.Gold
	w.accountConsumption(o.From, Resources{Gold: o.Gold}, "reparations")
	r := w.Relations[relationKey(o.From, o.To)]
	mustFire(r.lifecycle, peacePurchased, &relationContext{World: w, Relation: r})
	return nil
}
func settlePeace(_ context.Context, c *relationContext) error {
	w, r := c.World, c.Relation
	r.DamageA = 0
	r.DamageB = 0
	r.QuietUntil = w.Time + peaceAfter
	r.PurchasedUntil = w.Time + peaceAfter
	for _, owner := range []int{r.A, r.B} {
		other := r.A
		if owner == other {
			other = r.B
		}
		for id, incident := range w.Incidents[owner] {
			if incident.Owner == other {
				delete(w.Incidents[owner], id)
			}
		}
		for _, e := range w.entities(owner, "") {
			e.Orders = slices.DeleteFunc(e.Orders, func(o Order) bool {
				t := w.Entities[o.Target]
				return (o.Kind == "attack" || o.Kind == "convert" || o.Kind == "attack_move") && (o.TargetPlayer == other || w.attackParticipants(t)[other])
			})
			target := w.Entities[e.Order.Target]
			threat := w.Entities[e.Order.Threat]
			if e.Order.TargetPlayer == other && (e.Order.Kind == "attack" || e.Order.Kind == "attack_move" || e.Order.Kind == "convert") || w.attackParticipants(target)[other] && (e.Order.Kind == "attack" || e.Order.Kind == "convert") {
				w.setOrder(e, Order{Kind: "idle"}, false)
			} else if e.Order.Kind == "guard" && threat != nil && threat.Owner == other {
				o := e.Order
				o.Threat = 0
				w.setOrder(e, o, false)
			}
		}
		w.event(owner, "Reparations accepted. Peace restored and mutual attacks recalled; AI kingdoms honor it for five game minutes unless attacked again.")
	}
	for i := range w.Projectiles {
		p := &w.Projectiles[i]
		target := w.Entities[p.Target]
		if p.flight.State() == Flying && target != nil && (p.Owner == r.A && w.attackParticipants(target)[r.B] || p.Owner == r.B && w.attackParticipants(target)[r.A]) {
			mustFire(p.flight, CancelFlight, &flightContext{World: w, Projectile: p})
		}
	}
	return nil
}
func (w *World) reparationCommand(player int, c Command) error {
	if c.Kind == "peace_offer" {
		o := &peaceOffer{ID: w.NextID, From: player, To: c.TargetPlayer, Gold: c.Value, Expires: w.Time + 120, lifecycle: statemachine.NewInstance(peaceMachine, peaceDraft)}
		return fire(o.lifecycle, peaceSubmit, &peaceContext{World: w, Offer: o, Actor: player})
	}
	o := w.Reparations[c.OfferID]
	if o == nil {
		return rule("peace_offer_missing", "This peace offer is unavailable.")
	}
	event := map[string]peaceEvent{"peace_accept": peaceAccept, "peace_decline": peaceDecline, "peace_withdraw": peaceWithdraw}[c.Kind]
	return fire(o.lifecycle, event, &peaceContext{World: w, Offer: o, Actor: player})
}
func (w *World) pulseReparations() {
	for _, id := range sortedTradeIDs(w.Reparations) {
		o := w.Reparations[id]
		if o.lifecycle.State() == peaceOffered {
			mustFire(o.lifecycle, peacePulse, &peaceContext{World: w, Offer: o, Actor: o.To})
		}
	}
}
func (w *World) peaceOffers(player int) []PeaceOfferView {
	views := []PeaceOfferView{}
	for _, id := range sortedTradeIDs(w.Reparations) {
		o := w.Reparations[id]
		if o.From != player && o.To != player {
			continue
		}
		open := o.lifecycle.State() == peaceOffered
		views = append(views, PeaceOfferView{o.ID, o.From, o.To, o.Gold, math.Max(0, o.Expires-w.Time), string(o.lifecycle.State()), open && o.To == player, open && o.To == player, open && o.From == player})
	}
	if len(views) > 100 {
		views = slices.Clone(views[len(views)-100:])
	}
	return views
}
func (w *World) accountDamage(c *entityContext) {
	hp := math.Min(c.Actor.HP, c.Amount)
	if p := w.Players[c.Actor.Owner]; p != nil {
		p.Economy.DamageTaken += hp
	}
	if p := w.Players[c.SourceOwner]; p != nil && c.SourceOwner != c.Actor.Owner {
		p.Economy.DamageDealt += hp
	}
	if r := w.Relations[relationKey(c.SourceOwner, c.Actor.Owner)]; r != nil {
		value := resourceValue(w.stats(c.Actor).Cost) * hp / w.stats(c.Actor).HP
		if c.Actor.Owner == r.A {
			r.DamageA += value
		} else {
			r.DamageB += value
		}
	}
}
