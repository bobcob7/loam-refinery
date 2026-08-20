---
description: Efficiency: cost that grows with input, stated as a mechanism and not a hunch
---

A performance finding names the mechanism and the input that grows it. "This
could be faster" is not a finding, and a reviewer that produces several of them
teaches an author to skip the category.

Qualifies:

- Work inside a loop that does not depend on the loop.
- A query, a syscall, or an open per item where one per batch would do.
- A scan that is quadratic in something a user controls.
- A whole file, response, or table read to answer a question about a part of it.
- An allocation per iteration on a path whose iteration count is unbounded.

Does not qualify: a micro-optimization with no measured cost, or a hot-path
claim about code that runs once at startup.

Say what it costs at the input actually seen and what it costs at ten times
that. If neither number is knowable from the diff, the comment is a question
about the workload, and framing it as one gets a better answer than asserting a
cost nobody measured.
