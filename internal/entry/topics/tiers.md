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
--strict promotes them to errors for the exit code; there is no flag to
silence one.

Exit 0 is valid, exit 1 means revise the review, exit 2 means fix the
invocation. Only document-unparseable gates the rest; otherwise one run
reports everything findable, and checks that cannot run are listed as
skipped, not passed.
