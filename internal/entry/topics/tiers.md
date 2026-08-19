title: Check tiers and exit codes
aliases: exit-codes, severity, strict
related: schema, ref-unknown, priority-flat
---
Checks come in three tiers answering different questions.

Structural checks ask "is this a document?" — schema conformance, unique ids,
safe paths, well-formed refs. Always run, always hard.

Verification checks ask "is this about anything real?" — does this path exist at
this ref, does the file have this many lines. They run whenever a repository is
found, and are errors when they do. --warn-only demotes them for a repository
that lacks the reviewed commit.

Advisory checks ask "is this a good review?" — a thin body, a suggestion with no
stated cost, every comment at priority 9. Always run, never fatal. --strict
promotes them, --disable silences named ones.

Exit 0 is valid, exit 1 means revise the review, exit 2 means fix the
invocation. No check gates another, so one run reports everything findable, and
checks that cannot run are listed as skipped rather than counted as passing.
