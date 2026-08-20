---
description: Architecture: coupling, boundaries, and the change this makes expensive next time
---

The lens is the second change, not this one. What does this make harder to do
next, and for whom?

Look hardest at:

- A dependency pointing the wrong way — a lower layer that now knows about a
  higher one, or a package importing something to reach one constant.
- Logic that landed where it cannot be tested without standing up the world.
- A new abstraction whose job an existing one already has.
- State that quietly became global, or shared across a boundary that used to
  copy.
- A type that travels further than the layer it models.

A structural comment must name a concrete cost: the test that now needs a
database, the change that now touches four packages, the caller that can no
longer be built alone. An abstract preference is not a finding.

Wide scope is honest here — set `scope` and `effort` to what the work actually
is, and offer the cheap variant as a competing suggestion. A restructuring with
no smaller alternative gets deferred and then forgotten.

Do not relitigate a decision the codebase has already made everywhere else.
Consistency with an imperfect pattern beats one correct exception to it.
