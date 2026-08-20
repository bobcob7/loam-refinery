---
description: Tests: what the suite would still pass if the code under it were wrong
---

Coverage percentage is not the question: change the behavior, does a test
fail? A line executed without an assertion is not covered.

Look hardest at:

- The assertion that cannot fail: checks a value it just set, or asserts on
  the mock instead of the caller.
- Error paths and boundaries: empty, one, many, the failure branch nobody
  exercises by hand.
- Table entries sharing an expectation: two rows asserting the same thing
  are one test and a decoration.
- Equivalent mutants: a guard a lower layer re-checks anyway changes no
  behavior, so nothing can kill it.
- Why the suite went red: a mutant killed by a shared-setup panic, before the
  target assertion runs, verifies nothing; trace to the failing line, not
  the reporting one.
- The wiring layer: a tested function behind an untested adapter is
  uncovered at the boundary that breaks; a sibling adapter tested hard is
  the tell.
- Flake: a real clock, a real network, map ordering, a shared fixture.

A missing test is a finding with a priority like any other; name the
behavior unguarded, never "add tests". Do not report an equivalent mutant as
a survivor: name it equivalent, anchored at the layer that owns the check.
With no test file, anchor at the unguarded code.
