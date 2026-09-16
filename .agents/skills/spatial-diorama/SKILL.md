---
name: spatial-diorama
description: Explain local runtime data locality with a rotatable 3D diorama—where each piece of state lives and when it moves. Use when the user wants to understand live vs durable state, memory vs disk, or how pending buffers become persisted facts. Do not use for ordinary implementation, bugfixes, ablation review, or API walkthroughs.
---

# Spatial Diorama

Build a **notional machine** the user can orbit. The model is spatial: state is a place, flow is an object moving.

Read the code first (codegraph / source). Map only confirmed fields and write points. Do not invent furniture from docs.

## When

The question is “where does this live, and when does it move?” Typical targets: live objects, pending buffers, drafts, ledgers, event bells.

Skip this skill for feature work, refactors, ablation, or “what function calls what.”

## Scene rules

- **Volatile up, durable down.** Memory / live objects float. Disk ledgers and records sit on the floor. Notifications are a wall signal that does not store.
- **Tokens in places.** Each runtime field is a physical object in one place at a time. Movement is relocation, not an arrow on a flowchart.
- **One step, one move.** Steppable. Caption says what moved and why.
- **Human diorama, not debug boxes.** Three.js + OrbitControls (drag rotate, wheel zoom). Recognizable objects, readable plaques. No CSS fake-3D. No HTML that is really a document with sections and tables.

Metaphor comes from this code. Do not reuse the Runner kitchen unless this code is the kitchen.

## Output

Write a single self-contained HTML file, default `docs/learn/`. Do not put it in `STATUS.md` or the design doc.

Before handing it over, the user should be able to answer: what is in the air, what is on the floor, and which step makes a token change place.
