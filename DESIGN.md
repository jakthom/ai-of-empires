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

Warm limestone buildings and terracotta roofs sit among sage meadows, deep green forests, and blue water. Blue identifies the player; red, violet, gold, teal, and orange identify the five possible rivals. HUD contrast supports extended play in ordinary indoor light. Temperate scenery retains sage grass (#899d60), mixed woodland and cool blue water. Desert uses sand (#cbb184) and palms; Alpine uses snow (#c6d1d0) and snow-capped evergreens; Tropical uses lush grass (#719d58) and broad canopies. Autumn uses mossy ground (#929064) and copper canopies; Savanna uses golden grass (#b6a36a), red earth and flat acacias. Mixed Regions uses server-provided tile biomes to combine all six landscapes, including matching tree types and minimap colors. Distance haze uses the selected overall biome, with a temperate fallback for mixed regions. These palettes express server-provided biome metadata without changing rules.

## Typography

Palatino titles evoke engraved campaign maps. System text and tabular numbers keep costs, orders, and populations readable. Small text remains at least 12px.

## Layout

The world occupies the viewport. Resources and match controls share the top strip. The minimap anchors the bottom left, selection details the center, and contextual actions the bottom right. At narrower widths the minimap remains beside a stacked selection and action area, with population, health, and queue cancellation retained. Instructions collapse; small devices receive an input recommendation without hiding the battlefield. Short viewports use a compact command deck.

## Elevation & Depth

A perspective camera makes foreground and distant objects read at different scales. Continuous meadow geometry follows the server’s terrain, with fourfold visual height emphasis, recessed water, and exposed banks and cliff faces. Warm directional light, cool sky fill, and restrained distance haze reveal the silhouettes. Buildings carry timber, window recesses, eaves, and stone foundations on multiple sides; mixed tree canopies break up the forest. Material families are specific to each building: coarse daub, thatch and timber mature into Feudal plaster and colored tiles, Castle masonry, and finer Imperial stonework and roof finishes. Farms gain age-specific furrows. The server supplies the observed owner’s appearance age; remembered buildings retain their last visible age. Procedural 128px textures with subtle bump detail are cached by building type, age and surface, never by entity. Existing models rebuild when their observed age changes. Archery targets, stable stalls, siege machinery, a university portico and book, and a domed Wonder reinforce distinct silhouettes. Flags and windmills remain animated. Roofs align with their rectangular eaves and clear the supporting walls. Level building floors sit on foundations fitted to the full terrain footprint, including during construction; farm beds and crops follow the slope. Units follow the rendered surface between server snapshots. Selection, hover, and order rings conform to the terrain while retaining depth testing so objects can occlude them. Static parts sharing a material are merged, and owned geometry is disposed when removed. Order markers are removed after their fade completes. HUD panels are opaque and framed with single subtle borders.

## Shapes

Rectangular compact controls, small clipped heraldic emblems, and clear selection rings. Buildings have different silhouettes and real world footprints. Each of the 21 building types has a dedicated outlined SVG in the Build deck and selection portrait. Icons remain 32px on desktop and 24px on narrow screens; below 360px, build choices use one column so the icon, name and cost remain readable.

## Components

Server-authored action buttons show costs, availability, and reasons. Queue rows show server progress. Selection groups, order markers, drag boxes, minimap, pause state, and connection errors have distinct visual states. Keyboard focus is gold.

The top bar names the current difficulty in brass beside the age and clock, moving onto its own line on phones. New-match difficulty choices and their plain-language descriptions come from the server catalog. During an initial peace period, a brass Peace timer sits 8px after the game clock, showing the remaining time from the server; it disappears at expiry.

A compact toolbar at the battlefield's lower left provides Give order, Pan view, zoom, and Reset view at every width. A single click selects an element. Holding the left button for 180 ms arms panning; subsequent movement beyond five pixels pans the world under the pointer. Movement before that delay leaves the camera still. Holding both left and right buttons rotates horizontally and tilts vertically, regardless of press order. Releasing either button stops rotation and consumes the remaining release, preserving selection and preventing an accidental order. Shift-drag selects groups. Shift with arrow keys provides rotation and tilt for trackpads; ordinary arrows pan. Pan view and middle-drag remain alternatives for panning. Camera gestures never issue gameplay commands. Scroll and trackpad pinch keep the actual terrain point under the cursor fixed while zooming; zoom buttons use the view center. The minimap outlines the actual visible ground area. Primary clicks cover contextual orders; mouse secondary clicks and Mac shortcuts remain fast alternatives. Active input tools use brass fill, an explicit instruction, and a visible Cancel button. Removal always has a clickable confirmation.

A chronicle tray anchors the bottom edge. Its default is one line showing the latest event; expanding or dragging its top edge raises the command deck and resizes the battlefield. Keyboard arrows resize it, Home collapses it, and End expands it. Timestamped rows use brass entity links and plain event descriptions. A History tab immediately after Research displays the selected entity's log inside the command panel, independently of the global tray. It follows the selection and retains the last entity's history after destruction until another selection is made. Older/Newer pages and Follow live let players read past events without losing their place. Entity links also filter the bottom tray, and All events returns to the kingdom's shared chronicle. Markets replace the inapplicable Build tab with Trade, keeping four visible tabs on compact screens. The Trade tab shows backend-authored cost → gain quotes and exact refusal reasons. Fishing Ships expose Fish and Trade Carts expose Trade route as targeted orders. Depleted farms expose Reseed farm with the server cost and available worker; empty fields lose their crops. Fish shoals use pale surface silhouettes and ripples that respect water depth. The compact layout keeps all four tabs, activity, history rows, queues, and resizing available. Speed cycles through the server's choices up to 32×, with direct selection in the match menu.

The expanded tray starts with a search field, event category selector, and Clear button. Searches include the full retained history, and the collapsed preview identifies an active search as filtered. Each entity row has a Locate button that centers the camera and opens its individual History tab. An entity outside current sight uses the event's recorded location with an explanatory notice; the UI does not infer a hidden position or death. Search, category, and entity filters share the same Older/Newer and Follow live controls.

When expansion leaves a very short battlefield, the redundant camera instruction line collapses to keep the camera buttons inside the world. Compact individual history uses a tighter toolbar and timestamps sized to their content so a complete short event remains readable.

The campaign dialog keeps New game and Saved games as adjacent navigation buttons. Setup uses two columns, Your kingdom and Your world, within a scrollable dialog up to 900px wide. Below 650px they stack. Kingdom fields include an optional name and a count of 1–6 settlements, explicitly including the player; world fields offer twelve layouts, seven biome choices and five independent sizes with server-authored descriptions. Huge (224×224) and Giant (288×288) extend the existing sizes; their descriptions state the area relative to Large. Mountain Lakes, Braided Wetlands, Northern Fjords and Scattered Archipelago extend the land and maritime layouts. A native Advanced world options disclosure holds natural abundance, starting separation, map reveal, initial peace and seed. Its three-column grid becomes two columns below 650px and one below 390px. Invalid seeds reopen the disclosure and receive focus with a recovery message. Input placeholders use #b4bea8. Saved games use compact rows with names, world/biome/size summaries, played time, difficulty, settlement count, save time, and a Resume action. Original saves retain an Original world label. A search field also accepts an exact name or session ID. The match menu exposes the current name, world summary and selectable session ID, last successful autosave, Save now, and Save and leave. Save failures retain the current battlefield with a recovery message.

Building hover adds a brass ground ring and a small opaque label with the backend's building name and activity. It clears during camera gestures and orders. Camera bounds and terrain capacity follow each snapshot's map dimensions when changing campaigns.

## Do's and Don'ts

- Keep the world dominant and combat readable.
- Use UI language about the player's task, not implementation details.
- Do not show predicted resource totals, combat outcomes, or hidden units.

## Private campaigns

Game creation includes a human friend-seat count alongside total settlements and opens a saved lobby. The lobby inherits the campaign dialog: a serif game name and world summary lead into compact kingdom rows, heraldic player colors, connection/readiness text and native editing controls. Optional Ready and owner-only Start sit below the roster with backend availability text. The owner can start with empty friend seats or unready players; the lobby explains that friends can join their reserved kingdoms later. Invitations belong to individual seats; consumed, revoked, replaced and expired links disappear. A join preview requires explicit confirmation before claiming.

Players & game management is available in the match menu and beside every game in the private library. Its disclosures expose personal rejoin codes, member-only game addresses, world settings before Start, immutable shared activity, and portable archives. Close, replacement invitations, moving and deletion use an inline confirmation that names the consequences and preserves keyboard focus. Delete names the game and its local saves/history. Close failures retain a frozen game with Retry and Cancel.

Live roster updates preserve open disclosures, drafts, confirmations and focus. Copy/download feedback lives inside the dialog, including keyboard-copy instructions when a LAN browser cannot use the clipboard API. Recovery and archive import remain in Saved games; completion receipts belong to the imported game. At phone widths, seat actions and invitation fields stack beneath their kingdom name without horizontal scrolling. Leaving remains available after request failures, and server-owned permission changes disable resume/speed controls for guests.
