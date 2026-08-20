title: The review store
aliases: storing
related: exit-codes
---
Every `validate` that reads a document writes it down. A clean review is
stored as a review; an input that fails is stored as a rejected input — in
separate trees, never mixed, so the store can answer "what was concluded
about this commit" without a failure ever polluting it.

Storing needs no flag and produces no output: it is simply what `validate`
does, on every run with something to keep. The result object never mentions
it — asking where a copy landed is `loam-refinery reviews`'s job, not
`validate`'s.

`store.enabled: false` in the config file is the only way to turn storing
off. There is no per-run flag.

If the store cannot be created or written — a read-only home, a full disk, an
unreachable `store.path` — the command exits 101, not 1 or 2. That code means
the machine is at fault, not the review or the invocation: revising the
review will not fix it, and neither will re-checking flags. Stop and report
the machine.

`loam-refinery reviews` reads the store back: which reviews were stored for a
repository and commit, and, with `--failed`, which runs stored none.
