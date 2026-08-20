---
description: TypeScript: type escapes, null handling, and the runtime the types do not cover
---

`any`, `as`, and `!` are the three ways the type system stops checking. Every
occurrence is worth reading; report the ones hiding a real null or a real shape
mismatch, not the ones that are merely ugly. The finding is the runtime failure
the escape conceals, not the escape.

Look hardest at:

- Indexed reads. Under `noUncheckedIndexedAccess` every one is `T | undefined`.
  Anchor at the read that assumes otherwise, not at the declaration.
- Async. An unawaited promise, a boundary with no catch, a `Promise.all` that
  turns one failure into a total one, a `finally` that swallows a rejection.
- Unions that lost their discriminant, and switches over them with no
  exhaustiveness check — the bug arrives when a variant is added, not now.
- React: effect dependencies that lie, state derived in render that should be
  computed, array indices used as keys over a list that reorders.

Do not report formatting, import order, or a preference between two spellings
the compiler accepts equally.
