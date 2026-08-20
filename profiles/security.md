---
description: Security: untrusted input, the boundary it crosses, and what it reaches
---

Trace the input: a finding needs a source an attacker controls, a path, and
a sink that matters; without all three it is a preference, and reads as one.

Look hardest at:

- Sources: argv and environment, a path the caller names, anything parsed
  from a file, a network response, or a repository under review.
- Paths: traversal above a directory the tool owns, a symlink followed out
  of it, a stat-then-open with a window between them.
- Sinks: SQL, a shell, a template, a filesystem write, a log line.
- What leaks: secrets in errors and logs, private content in artifacts
  written with a permissive mode.
- What exhausts: an unbounded read into memory, recursion driven by input
  depth.

Priority ranks by reachability, not severity in the abstract: an issue
behind three conditions an attacker cannot satisfy is low, and a review
calling everything critical cannot say when something truly is.

A traced surface found closed is often the most useful output a pass
produces, and there is no field for it. Say so in `summary` for the review
as a whole, or as a low-priority comment anchored at the code that held,
when one boundary earns naming.

Do not report a missing mitigation that this threat model does not need.
