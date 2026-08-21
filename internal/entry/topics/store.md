title: The review store
aliases: storing
related: exit-codes
---
Every `submit-review` that reads a document writes it down: a clean review
as a review, a failing input as a rejected input, in separate trees, never
mixed, so the store can answer "what was concluded about this commit"
without a failure polluting it. Exit 3 is the exception: verification finds
the precondition itself and stops the rest of validation there, so it
records a row and writes no file.

Storing needs no flag and produces no output: it is simply what `submit-review`
always does. The result object never mentions it — asking where a copy
landed is `loam-refinery reviews`'s job, not `submit-review`'s.

`store.enabled: false` in the config file is the only way to turn storing
off. There is no per-run flag.

If the store cannot be created or written — a read-only home, a full disk,
an unreachable `store.path` — the command exits 101: the machine is at
fault, not the review or the invocation. Stop and report it; revising the
review won't help.

`loam-refinery reviews` reads it back: which reviews were stored for a
repository and commit, and, with `--failed`, which runs stored none.
