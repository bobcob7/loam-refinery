title: The review store
aliases: storing
related: exit-codes
---
Every `submit-review` that reads a document writes it down: a clean review
as a review, a failing input as a rejected input, in separate trees, never
mixed, so the store answers "what was concluded about this commit" without
a failure polluting it. Exit 3 is the exception: it records a row and
writes no file.

Storing needs no flag: it is simply what `submit-review` always does.
`store.enabled: false` in the config file is the only way to turn it off.
The result object never mentions where a copy landed — that is
`loam-refinery reviews`'s job.

Resubmitting under the same profile supersedes rather than replaces: it
adds a new row. `collect-reviews` marks the earlier one `superseded_by` and
requalifies its ids from `profile:id` to `#N:id`, so an id from an earlier
collect can stop resolving.

A store that cannot be created or written — read-only home, full disk,
unreachable `store.path` — exits 101: the machine is at fault, not the
review. Stop and report it.

`loam-refinery reviews` reads it back: which reviews were stored for a
repository and commit, and, with `--failed`, which runs stored none.
