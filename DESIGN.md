---
name: AI of Empires
colors:
  background: "#171e1b"
  surface: "#222b25"
  raised: "#303b30"
  text: "#ede8d7"
  muted: "#b6bdac"
  accent: "#dec184"
  player: "#6198c7"
  enemy: "#ba6756"
typography:
  display:
    fontFamily: "Palatino, Palatino Linotype, Georgia, serif"
  body:
    fontFamily: "system-ui, sans-serif"
rounded:
  control: "3px"
spacing:
  small: "8px"
  medium: "16px"
  large: "24px"
---

## Overview

The living miniature battlefield leads. A familiar RTS command console uses dark olive surfaces, brass details, heraldic shields, and restrained serif titles. This follows the user's Age of Empires reference; it is an operating interface, not a promotional landing page.

## Colors

Warm limestone buildings and terracotta roofs sit among sage meadows, deep green forests, and blue water. Blue identifies the player; red, violet, gold, teal, and orange identify the five possible rivals. HUD contrast supports extended play in ordinary indoor light.

## Typography

Palatino titles evoke engraved campaign maps. System text and tabular numbers keep costs, orders, and populations readable. Small text remains at least 12px.

## Layout

The world occupies the viewport. Resources and match controls share the top strip. The minimap anchors the bottom left, selection details the center, and contextual actions the bottom right. At narrower widths the minimap remains beside a stacked selection and action area, with population, health, and queue cancellation retained. Instructions collapse; small devices receive an input recommendation without hiding the battlefield. Short viewports use a compact command deck.

## Elevation & Depth

Actual Three.js geometry, directional shadows, terrain, flags, and windmills carry visual depth. HUD panels are opaque and framed with single subtle borders.

## Shapes

Rectangular compact controls, small clipped heraldic emblems, and clear selection rings. Buildings have different silhouettes and real world footprints.

## Components

Server-authored action buttons show costs, availability, and reasons. Queue rows show server progress. Selection groups, order markers, drag boxes, minimap, pause state, and connection errors have distinct visual states. Keyboard focus is gold.

The top bar names the current difficulty in brass beside the age and clock, moving onto its own line on phones. New-match difficulty choices and their plain-language descriptions come from the server catalog.

A compact toolbar at the battlefield's lower left provides Give order, Pan view, zoom, and Reset view at every width. Dragging the world rotates and tilts it; a short click selects, while Shift-drag selects groups. Pan view moves across the map at the current camera angle. Scroll and trackpad pinch keep the ground under the cursor fixed while zooming; zoom buttons use the view center. The minimap outlines the actual visible ground area. Primary clicks cover contextual orders; mouse secondary clicks and Mac shortcuts remain fast alternatives. Active input tools use brass fill, an explicit instruction, and a visible Cancel button. Removal always has a clickable confirmation.

A chronicle tray anchors the bottom edge. Its default is one line showing the latest event; expanding or dragging its top edge raises the command deck and resizes the battlefield. Keyboard arrows resize it, Home collapses it, and End expands it. Timestamped rows use brass entity links and plain event descriptions. A History tab immediately after Research displays the selected entity's log inside the command panel, independently of the global tray. It follows the selection and retains the last entity's history after destruction until another selection is made. Older/Newer pages and Follow live let players read past events without losing their place. Entity links also filter the bottom tray, and All events returns to the kingdom's shared chronicle. The compact layout keeps all four tabs, activity, history rows, queues, and resizing available. Speed cycles through the server's choices up to 32×, with direct selection in the match menu.

The expanded tray starts with a search field, event category selector, and Clear button. Searches include the full retained history, and the collapsed preview identifies an active search as filtered. Each entity row has a Locate button that centers the camera and opens its individual History tab. An entity outside current sight uses the event's recorded location with an explanatory notice; the UI does not infer a hidden position or death. Search, category, and entity filters share the same Older/Newer and Follow live controls.

When expansion leaves a very short battlefield, the redundant camera instruction line collapses to keep the camera buttons inside the world. Compact individual history uses a tighter toolbar and timestamps sized to their content so a complete short event remains readable.

The campaign dialog keeps New game and Saved games as adjacent navigation buttons. Setup offers an optional name and a count of 1–6 settlements, explicitly including the player. Saved games use compact rows with names, played time, difficulty, settlement count, save time, and a Resume action. A search field also accepts an exact name or session ID. The match menu exposes the current name and selectable session ID, last successful autosave, Save now, and Save and leave. Save failures retain the current battlefield with a recovery message.

Building hover adds a brass ground ring and a small opaque label with the backend's building name and activity. It clears during camera gestures and orders. Camera bounds and terrain capacity follow each snapshot's map dimensions when changing campaigns.

## Do's and Don'ts

- Keep the world dominant and combat readable.
- Use UI language about the player's task, not implementation details.
- Do not show predicted resource totals, combat outcomes, or hidden units.
