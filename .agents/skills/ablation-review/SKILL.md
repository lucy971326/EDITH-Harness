---
name: ablation-review
description: Review an existing architecture or implementation by asking what concretely breaks when each layer, wrapper, state, queue, or dependency is deleted or merged. Use for simplification, code subtraction, first-principles architecture review, or reducing AI-generated complexity; do not invoke for ordinary feature work without a simplification question.
---

# Ablation Review

Find the smallest design that preserves current behavior and proven safeguards.

## Method

1. Read `AGENTS.md`, `STATUS.md`, and only the design or plan documents relevant to the target. Treat existing architecture as evidence to inspect, not as proof that it is necessary.
2. Establish the current behavior with code, call sites, and tests. Separate confirmed facts, reasonable inferences, and unknowns.
3. Inventory the target's layers, wrappers, registries, helpers, state fields, goroutines, queues, and dependencies.
4. For each item, test the counterfactual: delete it, merge it into its caller or owner, or replace it with a direct call. State the first observable failure, current requirement, or concrete safety property that would be lost.
5. Classify each item:
   - **Delete:** no current behavior or proven safeguard breaks.
   - **Merge:** the responsibility is necessary but a separate abstraction is not.
   - **Keep:** deletion has a concrete failure supported by a call path, test, or current requirement.
6. When the user authorized implementation, run a baseline first, make the deletions directly, and verify behavior proportionally. Add or retain a regression test only for a real invariant exposed by the deletion experiment.

## Decision Rules

- Hypothetical extensibility, symmetry, architectural fashion, and “might be useful later” do not justify an abstraction.
- A library must replace owned infrastructure. Do not keep the old implementation behind a compatibility adapter unless a real consumer requires it.
- Do not move complexity into UI, Product, or another layer and call that deletion. Follow state and responsibility to their correct owner.
- Do not use line count as the sole objective. Disconnect detection, backpressure, ordering, cancellation, and cleanup remain when tests or current behavior prove they matter.
- Stop subtracting when every remaining piece has a specific failure mode; summarize that failure plainly.
- Respect review-only requests: analyze without editing unless the user also asks for changes.

## Output

Lead with a compact ASCII mental model. Then use a short table:

```text
Item | If deleted | Decision
```

For implemented work, report what disappeared, what remains and why, verification performed, and whether anything was committed. Avoid defending complexity with jargon.
