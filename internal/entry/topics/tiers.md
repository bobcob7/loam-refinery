title: Check tiers and exit codes
aliases: exit-codes, severity, strict
related: schema, ref-unknown, priority-flat
---
Checks come in three tiers answering different questions.

Structural checks ask "is this a document?" — schema conformance, unique ids,
safe paths, well-formed refs. Always run, always hard.

Verification checks ask "is this about anything real?" — does this path exist at
this ref, does the file have this many lines. Always run, and cannot be
disabled or demoted: an anchor claim that goes unconfirmed fails the run.

Advisory checks ask "is this a good review?" — a thin body, a suggestion with no
stated cost, every comment at priority 9. Always run, never fatal outright.
--strict promotes them to errors; there is no flag to silence one.

Exit 0 is valid, exit 1 means revise the review, exit 2 means fix the
invocation, exit 3 means the reviewed state is not a commit — commit it, or
run `git stash create` and submit that SHA — and exit 101 means the tool
failed: stop and report the machine. Only document-unparseable gates the
rest; every other run reports everything findable, skipped checks included.
