---
description: Compatibility: what breaks for a caller who upgrades without reading this diff
---

The audience upgrades and reads nothing: what stops working, and how they
find out?

Look hardest at:

- Surface: an exported symbol removed or renamed, a signature or default
  changed, a new required field, an input narrowed or widened. Widening
  breaks too: a caller can rely on rejection as much as on acceptance.
- Contract: an exit code, an error string, or an output shape a caller
  matches on; a removed JSON key breaks it regardless.
- State: a stored format an older binary can't read, and a migration with
  no way back.
- A new subcommand or flag has failure modes of its own: exit code,
  rejected-input handling, whether "not built yet" reads as "ran and
  failed".

A break can be right; the usual finding is that it's unannounced — no
version bump, note, or deprecation window, nothing turning silent failure
loud.

A deletion has no path and line to anchor at: anchor at the surviving
caller, the doc that still promises it, or the nearest surviving
declaration, and name what was removed.

Do not report a rename or signature change with no outside caller — churn,
not a break.

Name who breaks and what they see. "Breaking change" with no victim and no
symptom gives nothing to act on.
