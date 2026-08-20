# Seed reviewer profiles

Starting points, not defaults. Nothing here is compiled into the binary and
nothing installs itself: `loam-refinery` reads profiles only from
`$XDG_CONFIG_HOME/loam-refinery/profiles/` (see
[docs/config.md §2](../docs/config.md#2-locations)), and this directory is
material to copy there and then edit.

```sh
mkdir -p ~/.config/loam-refinery/profiles
cp -n profiles/*.md ~/.config/loam-refinery/profiles/
rm ~/.config/loam-refinery/profiles/README.md
```

`-n` because a profile you have edited is yours; nothing in this repository
should overwrite it. `README.md` is removed rather than skipped by the tool —
an uppercase stem is not a valid profile name, so it would be ignored anyway
([docs/cli.md §2.1.2](../docs/cli.md#212-the-profile-file)).

| Profile | Lens |
| --- | --- |
| `go` | Concurrency, error wrapping, context, lifetime, hot-path cost, tests |
| `typescript` | Type escapes, null handling, async boundaries |
| `tests` | Whether the suite would notice the code being wrong |
| `security` | Source, path, sink — and reachability as priority |
| `efficiency` | Cost that grows with input, named as a mechanism |
| `architecture` | Coupling and what the next change will cost |
| `claims` | Commit, PR, doc, and comment prose against the code |
| `compatibility` | What breaks for a caller who reads nothing |

## What belongs in one

A profile directs attention. It may not restate or amend the contract — nothing
in a profile is read by `validate`, and a profile that says "skip the priority
advisory" is asking for something it cannot have
([docs/cli.md §2.1.6](../docs/cli.md#216-what-a-profile-may-not-do)).

Every profile here does one thing, and a second when it needs to:

- **It says what not to report.** A lens that only adds is a lens that adds
  noise, and the fastest way to make a category ignored is to file three weak
  findings in it.
- **When its subjects anchor badly, it says where to anchor.** A coupling
  finding anchors at the import, a cost finding at the loop — no rule needed.
  A commit message, an absent test, or a deletion has no natural path and
  line, and every comment still needs one that verification can confirm
  ([docs/cli.md §2.3.1](../docs/cli.md#231-verifying-anchors)); those are
  exactly the subjects a profile has to give an anchoring rule for.

Keep the body near the size of the ones here, 210–290 tokens. It is pinned for a
whole session next to a `prime` that fought to stay at 250, and a profile that
costs more than the tool's own instructions is worth reading again.
