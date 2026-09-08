# Product

<!-- impeccable:product-schema 1 -->

## Platform

web

## Users

Players of historical real-time strategy games, using a desktop browser with a Mac trackpad or mouse and optional keyboard shortcuts. Core orders must be available through primary clicks.

## Product Purpose

Build the game described in GAME_SPEC.md. Players develop a medieval economy, advance through four ages, and command armies.

## Capabilities and Constraints

The user's implementation requirements supersede the proposed Rust architecture in the spec: Go owns every simulation rule and serves a Three.js UI. The frontend is exclusively presentation and input. A versioned, documented API separates them. The existing specification is the long-term gameplay target; README.md records implemented scope accurately.

## Brand Commitments

Age of Empires is the explicit reference. Use a dimensional perspective battlefield and compact RTS controls. The user’s island-builder video reference informs terrain relief and readable geometry, while the game retains its medieval setting and original procedural assets. AI of Empires is the specification's working title. All render assets are original procedural Three.js geometry.

## Evidence on Hand

GAME_SPEC.md is the supplied product reference. There was no application code or incumbent visual system at implementation start.

## Product Principles

- Go owns truth; presentation never resolves gameplay.
- Commands describe intent; snapshots describe authorized observations.
- A match is playable without developer tools.
- Distinguish implemented mechanics from future specification scope.
