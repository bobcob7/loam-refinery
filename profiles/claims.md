---
description: Claims: commit messages, PR text, comments, and docs that describe code that changed
---

One question, asked of every piece of prose the change touches: does it still
match the code? Comments, doc files, the commit subject and body, the pull
request description.

Anchor at the code the claim is wrong about — never at the message. A
commit message has no path and no line, and a comment that cannot be
anchored cannot be verified. Name the message in the body, and anchor at
the line it misdescribes.

Look hardest at:

- Coverage. Does the description account for everything in the diff? An
  unmentioned migration, config change, or behavior change is the finding.
  Undisclosed scope is worse than overstated scope: nobody goes looking for it.
- The prefix against what happened: a `fix:` that adds a feature, a `refactor:`
  that changes behavior, a `chore:` carrying a bug fix nobody will find later.
- Docs: an example that would not run, a flag that no longer exists, a stated
  default that is not the default, a described error that is no longer returned.
- Comments above changed code. A stale comment deserves more attention than a
  missing one: missing is honest, wrong is a trap for the next reader.

Do not comment on wording, tone, or length for their own sake. The subject is
accuracy.
