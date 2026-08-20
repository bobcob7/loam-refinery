# Configuration and the review store

Tool specification. Version 0.2 (draft).

Companion to [cli.md](cli.md), which specifies the commands, and
[review-document.md](review-document.md), which specifies the format they read.

## 1. What this adds

Until now `loam-refinery` kept nothing. A run read a document, said whether it
was usable, and exited; the same document validated twice produced the same
answer twice and no trace either time. That is still the whole of `validate`'s
contract, and the reason the tool needs no setup.

What it cost was memory. A review that passed is a conclusion about a commit,
and the only copy of it lived in whatever pipe the agent happened to be holding.
Nothing could answer *what was concluded about `4f2c1a9`* an hour later, on
another machine, or in another session — not because the answer was hard, but
because nobody wrote it down.

This document specifies the writing down:

- **Where settings live**, and the deliberately small set of things they may
  change ([§2](#2-locations), [§3](#3-the-config-file)).
- **The store** — files holding what was submitted, passing reviews and
  rejected inputs alike, and a database holding a row per run
  ([§4](#4-the-store)).
- **Storing**, which `validate` does on every run that reads a document
  ([§5](#5-storing-a-review)).
- **`loam-refinery reviews`**, which gets them back out
  ([§6](#6-reading-the-store)).

Three things stay out, and are not deferrals — they are the boundary that keeps
this addition from turning the tool into something else:

- **No network.** The store is a directory on the machine that wrote it. It is
  not synced, served, or fetched, and nothing here opens a socket.
- **No aggregation.** Reviews are stored side by side, not merged, ranked, or
  reconciled. Merging is still [cli.md §8](cli.md#8-future-considerations).
- **No writes inside the repository.** The store lives under the user's home
  directory. `loam-refinery` remains read-only about any checkout it is pointed
  at.

### 1.1 What this changes about the design principles

[cli.md §1](cli.md#1-overview) listed "storing or aggregating reviews across
runs" as explicitly out of scope, and its seventh design principle promised no
config file. Both are amended here rather than quietly contradicted:

| Principle | Standing |
| --- | --- |
| Offline, no network | **Unchanged.** A directory read is not a fetch, and reading `remote.origin.url` is a config lookup, not a connection. |
| Read-only about the repository | **Unchanged.** Nothing is written under the checkout. |
| No config file *required* | **Amended.** Nothing has to be configured, but the first `validate` with anything to keep creates the config file and the store rather than requiring either ([§2.2](#22-first-use-creates-what-it-needs)). |
| Storing across runs is out of scope | **Amended.** Storing is in scope, on by default, and never changes whether a document is valid. |
| Aggregating across runs is out of scope | **Unchanged.** |

Two load-bearing rules survive all of it. Configuration may not change whether a
document is valid ([§3.1](#31-what-configuration-may-not-do)), and storing may
not change the *verdict* on one — only whether the command as a whole could be
carried out ([§5.1](#51-when-storing-fails)).

## 2. Locations

Two directories, because they hold two different kinds of thing.

| Holds | Path | Override |
| --- | --- | --- |
| Settings | `$XDG_CONFIG_HOME/loam-refinery/config.json` | `$XDG_CONFIG_HOME`, or `$LOAM_REFINERY_HOME` |
| The store | `$XDG_DATA_HOME/loam-refinery/` | `$XDG_DATA_HOME`, `$LOAM_REFINERY_HOME`, or `store.path` |

With neither XDG variable set, those resolve to
`~/.config/loam-refinery/config.json` and `~/.local/share/loam-refinery/`.

**The store is a directory, not a file.** `<store>` throughout this document
means that directory, and it holds `store.db`, `reviews/`, and `rejected/`
([§4.1](#41-layout)) — plus, transiently, `store.db-wal` and `store.db-shm`.
WAL mode ([§4.6](#46-concurrency-and-locking)) keeps those two beside the
database for as long as any connection is open, and removes them on the last
clean close. A store inspected between runs holds only the first three; a
store caught mid-run, or one a crash or a `kill -9` left without a clean
close, holds all five. Nothing else belongs there.

**Config and data are split** because one is a file a person writes and wants
backed up, and the other is bulk output a machine produces and can regenerate
by re-validating. Dotfile repositories, `rsync` snapshots, and machine setup
scripts all want the first and not the second; a single directory holding
both makes every one of those choices worse. The split is what XDG is for, and
following it costs nothing here.

**`$LOAM_REFINERY_HOME` collapses the split for anyone who wants it collapsed.**
When it is set, config is `$LOAM_REFINERY_HOME/config.json` and the store is
`$LOAM_REFINERY_HOME` itself, and the XDG variables are not consulted. One
directory, one thing to point at a scratch path in a test or a container. It
exists because "everything under `~/.local/loam-refinery/`" is a reasonable
thing to want and should not require two environment variables to express.

In that mode `config.json` sits inside the store directory. Nothing this tool
does deletes a store wholesale, and if that ever changes it removes `store.db`,
`store.db-wal`, `store.db-shm`, `reviews/`, and `rejected/` by name rather than
the directory containing them ([§7](#7-growth-and-retention)) — so the
collapse can never cost somebody their settings.

**macOS uses the same paths as Linux.** Apple's convention is
`~/Library/Application Support`, and it is rejected here: this is a terminal
tool that sits beside `~/.gitconfig`, `~/.ssh/config`, and `~/.config/gh`, and a
developer looking for its settings will look where the rest of them are.
`~/Library` is also hostile to the things people actually do with config —
`ls`, `grep`, symlinking into a dotfile repository — for no benefit a CLI
collects.

### 2.1 Precedence

For any setting that more than one source can supply:

```
flag  >  environment  >  config file  >  built-in default
```

No merging, no partial override: the first source that speaks wins for that
setting, and settings do not interact. `LOAM_REFINERY_HOME` beats a `store.path`
in the config file, which beats the XDG default.

**No flag currently sets a store setting**, and the flag tier is in the ladder
for the shape rather than for a member. The store deliberately has no per-run
flags ([§5](#5-storing-a-review)); a future one would land at the top of this
ladder rather than beside the config file.

The rule matters most for the failure it prevents. A caller reading a surprising
result must be able to work out *why* from the environment and one file, and a
scheme where two sources blend turns that into an investigation. Environment
above config is what lets a container override a baked-in image setting without
rewriting the file.

### 2.2 First use creates what it needs

Because storing is on by default ([§5](#5-storing-a-review)), a first run has to
work on a machine that has never seen this tool. It does: the first `validate`
with anything to keep creates the config directory, writes a `config.json`
holding the defaults, and creates the store — and if it cannot, the command
fails ([§5.1](#51-when-storing-fails)).

Writing the config file rather than merely working without one is the point.
The alternative leaves settings that exist but are invisible: a user who wants
to change where the store lives has to already know the file's path, its format,
and its key names. A file on disk holding the defaults is discoverable with `ls`
and editable without reading a specification.

Three things bound it:

- **It is lazy.** Nothing is created by a run with nothing to keep — a usage
  error that never read a document, or a machine whose config says
  `store.enabled: false`. A passing review and a rejected input both count as
  something to keep ([§5](#5-storing-a-review)), so an agent that has never
  produced a valid review still leaves a record of its attempts, which is
  exactly where that record is most wanted.
- **It is `validate` only.** Reading never creates anything.
  `loam-refinery reviews` on a machine with no store reports an empty one and
  exits 0 without leaving a `~/.local/share/loam-refinery/` behind to mark the
  visit, and `prime`, `describe`, and `schema` never consult either path at all.
- **It never overwrites.** An existing `config.json` is read, never rewritten,
  never merged with new defaults, and never reformatted. The file belongs to
  whoever edited it.

Both directories are created with mode `0700`, and the config file `0600`. A
review names files, line numbers, and findings about private code, and the
default umask is not a good enough reason to let another account on the machine
read it.

## 3. The config file

JSON, one object, at the path in [§2](#2-locations).

```json
{
  "version": "1",
  "store": {
    "enabled": true,
    "path": "~/reviews",
    "repos": {
      "/Users/me/src/refinery": "github.com/bobcob7/loam-refinery"
    }
  }
}
```

The file written on first use ([§2.2](#22-first-use-creates-what-it-needs)) is
the defaults, spelled out rather than left implicit:

```json
{
  "version": "1",
  "store": {
    "enabled": true
  }
}
```

`store.path` and `store.repos` are absent rather than written with their
computed values. Writing the resolved store path into the file would freeze a
default that should follow `$XDG_DATA_HOME`, and pin it to whatever the first
run happened to see.

| Key | Type | Default | Meaning |
| --- | --- | --- | --- |
| `version` | string | required | Config format version. Always `"1"`. |
| `store.enabled` | bool | `true` | Store passing reviews. `false` is the only way to turn storing off — see [§5](#5-storing-a-review). |
| `store.path` | string | [§2](#2-locations) | The store directory, holding `store.db`, `reviews/`, and `rejected/`. `~` expands; relative paths are rejected. |
| `store.repos` | object | `{}` | Absolute path → repository name. Matched against the worktree root, or the working directory when there is no worktree. See [§4.2](#42-repository-identity). |

**JSON, not TOML or YAML.** The tool already carries a JSON parser and a JSON
Schema compiler for the review document, and every command already emits JSON.
A second format is a second parser, a second set of failure messages, and a
second place for a spec to drift. The cost is honest — JSON has no comments —
and it is a config file with four keys.

**Unknown keys are rejected**, for the reason
[review-document.md §3](review-document.md#3-root-object) rejects them in the
document: a misspelled key that silently does nothing is the failure this
project exists to catch. `{"store": {"enable": true}}` must not read as a store
that is quietly off. The message names the offending key and the file.

**A missing file is not an error.** Defaults apply and nothing is reported.

**Anything else wrong with the file is exit 101**, the tool-error code
([cli.md §4](cli.md#4-exit-codes)): unreadable, unparseable, an unknown key, or
a value that is not the right shape. All four name the path and the offending
detail.

101 rather than 2 because of what a caller can do about it. Exit 2 says *fix the
command*, and the command is not wrong — the same invocation is correct and will
keep failing until something outside it changes. That is the test the tool-error
band exists for.

### 3.1 What configuration may not do

**No key may change whether a document is valid.** `strict`, `disable`,
`warn_only`, and `require_verification` are flags and only flags. They are named
explicitly in the rejected-key message rather than being reported as unknown, so
the caller learns where they went.

The reason is the one this format is built around. A document's validity is a
property of the document, and the moment a file in someone's home directory can
change it, the same review passes on one machine and fails on another with the
same command line. Every failure report becomes unreproducible without also
knowing the reporter's config; CI diverges from the laptop silently; and the
contract stops being a contract, because two parties running the same tool no
longer agree about what the tool said.

Storing is safe to configure precisely because it changes nothing about the
answer — only whether a copy of it is kept. That is the test any future key has
to pass.

## 4. The store

A store is two things doing different jobs: **files holding what was
submitted** — a passing review kept whole, a rejected input kept up to its
first megabyte — and **one database holding what gets queried**: when, which
repository, which ref, what verdict, and how many comments.

Neither half is an interface. [§4.9](#49-the-store-is-not-a-contract) says so at
length, and that is what lets this split be revisited later:
`loam-refinery reviews` is the contract, the layout underneath it is not.

### 4.1 Layout

```
<store>/
  store.db
  reviews/
    github.com/bobcob7/loam-refinery/
      4f2c1a9e3b7d5f0c8a1e2d4b6c8f0a2e4d6b8c0f/
        3f9a1c2b7e04d19c….json
      9a3f0c1d5e7b2a4c6e8f0b2d4a6c8e0f2b4d6a8c/
        b7e04d193f9a1c2b….json
    local/scratch/
      …
  rejected/
    github.com/bobcob7/loam-refinery/
      44136fa355b3678a….json      the input that failed, whatever it was
```

**The files carry the bulk; the database carries the questions.** A review is
several kilobytes of prose that nothing searches by content, and a row about it
is a couple of hundred bytes that everything searches. Putting the prose in a
database makes it opaque for no gain; putting the questions in the filesystem
makes every query a directory walk that grows with the store. The split puts
each where its access pattern already points, and it is why "which reviews of
this repo requested changes last month" is a query rather than a program.

Keeping `<repo>/<ref>/` in the reviews tree, when the database could answer
both, is deliberate. Nothing rebuilds a lost database today
([§4.7](#47-when-the-database-is-lost)), and the path is what keeps that
buildable later: a tree whose structure carries the repository and the commit
can be walked back into rows, where a flat pile of hashes could not be. The
rejected tree has no ref level because a rejected input often has no usable
ref — that is frequently why it was rejected.

### 4.2 Repository identity

The first path component names the repository, and choosing that name is the
only genuinely hard decision here. The candidates:

| Candidate | Stable across | Fails when |
| --- | --- | --- |
| Directory basename | Nothing | Two checkouts share a name; a checkout is renamed |
| Remote URL | Rename, re-clone, moving the checkout | No remote; several remotes; a fork points elsewhere |
| Root commit SHA | Everything above | Nobody can type it; a walk to the root commit is not cheap |

**The name is the normalized `origin` remote when there is one.**

```
git@github.com:bobcob7/loam-refinery.git   ->  github.com/bobcob7/loam-refinery
https://github.com/bobcob7/loam-refinery    ->  github.com/bobcob7/loam-refinery
ssh://git@example.com:2222/team/svc.git     ->  example.com/team/svc
```

Normalization lowercases the host, drops userinfo, port, a trailing `.git`, and
any leading or trailing `/`. It is a string transformation on a value read from
`git config`; nothing is resolved and nothing is contacted.

**With no `origin`, the name is `local/<basename of the worktree root>`.** The
`local/` prefix is not decoration. It keeps unanchored names in a separate
keyspace, so a directory that happens to be called `github.com` can never shadow
a real one, and it makes a listing say plainly which names are backed by a
remote and which are a guess about a directory.

**With no repository at all, the name is `no-repo`.** `validate` does not need a
checkout — it reads a document from a path or from stdin, and
[cli.md §2.3.1](cli.md#231-verifying-anchors) calls running outside a repository
ordinary rather than exceptional. A CI step validating a review artifact, an
agent working in a scratch directory, a container with only the JSON mounted:
all of them still have a document worth keeping and no repository to file it
under.

`no-repo` is a single reserved segment, and single is what makes it safe. A
remote-derived name always begins with a host and a `local/` name always carries
its prefix, so neither derivation can produce a one-segment name; a derived name
that would equal `no-repo` is rejected and falls back to the derivation below
it. Everything unrooted therefore shares one bucket rather than inventing
identity from a working directory — `local/tmp` and `local/yourusername` look
like identity and are not, and the person reading the store six months later is
the one who would pay for that.

**`store.repos` overrides all three**, keyed by an absolute path: the worktree
root when there is one, the working directory when there is not. This is the
answer for a fork that should file under upstream's name, a repository whose
remote is not called `origin`, two unrelated checkouts that normalize to the
same thing, and an unrooted directory that deserves better than the shared
bucket. It is explicit, per-machine, and the only place a human is asked to make
this judgment — which matters more than it looks, because `validate` has no
store flags, so this file is the *only* way to correct a name.

**The root commit is not used**, though it is the only truly intrinsic answer.
`git rev-list --max-parents=0 HEAD` walks the entire history, which on a large
repository is seconds — paid on every stored review, in a tool whose seventh
design principle is that it is cheap to call. It also is not unique: histories
with grafts or merged-in projects have several roots, so the "intrinsic"
identifier needs a disambiguation rule of its own.

**The name is a label, and nothing more.** Nothing else is recorded about which
checkout produced a review — no remote URL, no working directory — because
neither is worth carrying for the life of a store, and a store is meant to
outlive the machine layout that filled it.

That has a consequence worth being plain about: two unrelated checkouts that
normalize to the same name are **merged**, not merely suspected of colliding.
Their reviews land in one tree and no query can separate them afterwards. The
fix is to prevent it rather than to detect it — `store.repos` names either one
explicitly — and the case is rare enough that paying two columns on every row
forever to diagnose it after the fact is the wrong trade.

### 4.3 The ref component

The second path component is the document's `ref`: a full 40-character lowercase
commit SHA, exactly as [review-document.md §5.1](review-document.md#51-refs)
requires. It is validated as such before it is used as a path component, and a
value that is not 40 hex characters never reaches the filesystem.

**Every document that reaches the store has one.** `ref` is a required field, so
a review with nothing to file under fails structurally long before storing is
considered, and the store never has to decide what to do with a review it
cannot file.

The requirement is not the store's doing — verification needs the ref for
reasons that predate any of this
([review-document.md §5.1](review-document.md#51-refs)) — but the store is what
makes the consequence concrete. A commit is the only thing a review can be
retrieved *by* an hour later, and a format that let the key be absent would have
produced a store with a hole in it.

### 4.4 The stored files

**A review file is the document, byte for byte.** Not re-serialized, not
re-indented, not decorated with a field of the tool's own. What was validated is
what is on disk; `sha256sum` on it reproduces the digest that names it; and
`loam-refinery validate` on it passes unchanged, because it is still exactly a
review document.

That guarantee is scoped to the reviews tree. The rejected tree, described in
[§4.4.1](#441-rejected-inputs) below, keeps only the first megabyte of an
input larger than that — so the byte-exact, self-verifying claim above does
not extend to it, and a caller that needs to know which applies checks which
tree a `path` names.

That fidelity is the point of keeping files at all. Everything else about a
review can be recomputed, migrated, or re-derived; the reviewer's bytes cannot
be, and a migration that has to rewrite them to move them is a migration that
can corrupt them. Files that are pure content are the easiest thing in this
design to move, copy, checksum, and leave alone.

**The filename is the full SHA-256 of those bytes**, plus `.json`. No timestamp:
the time a review was stored is a fact about the run, it lives in the database
where it can be queried, and putting it in the name only made two identical
documents look like two reviews.

Content addressing makes deduplication a filesystem primitive. Storing a review
that is already stored is an `O_EXCL` create that fails with `EEXIST` — no
directory scan, no digest-prefix comparison, and no race between two agents
storing the same document at the same moment. One of them creates the file, the
other is told it exists, and both record a run.

Nothing else goes in the tree. There is no sidecar and no index file: metadata
that lived beside a review would be a second thing to keep consistent with it,
and consistency is what the database is for.

#### 4.4.1 Rejected inputs

**A run that exits 1 keeps its input too**, under `rejected/<repo>/`, addressed
by the same SHA-256 of the same submitted bytes.

The reason is the one the run log cannot serve. A row saying a run failed with
three errors is a statistic; the thing that actually explains an agent's
behaviour is what it emitted. An agent that submits `{}` is not making three
mistakes, it is making one, and no count of diagnostics conveys that as directly
as four bytes on disk. Recording *how* it failed is deferred
([§4.5.1](#451-what-it-holds)); recording *what was submitted* is not, because
that is the part nothing can reconstruct later.

Content addressing does most of the work here. An agent looping on the same
broken output writes one file and fifty rows: the pathological case, the one
where a failure repeats until something changes, costs a single copy. It is the
reason this can be on by default rather than sampled or capped.

Three differences from a review file:

- **No ref level in the path.** A rejected input frequently has no usable `ref`;
  that is often why it was rejected. `<repo>/<digest>.json` is the whole layout.
- **`.json` is a claim, not a guarantee.** The bytes are stored exactly as
  submitted, and a document that failed `document-unparseable` is not JSON at
  all. The extension records what the caller submitted it as, which is also
  precisely what a person opening the file wants to see.
- **It is not a review document**, and nothing pretends otherwise. Feeding one
  back to `validate` reproduces the failure that put it there, which is the
  useful behaviour.

Exit 1 is the whole of it. An exit 2 run mistyped a flag and usually never read
a document; an exit 101 run failed at the store itself and has nowhere to put
one.

**An input over 1 MiB is truncated to its first 1 MiB, not dropped.** The
reviews tree is bounded by documents that passed a schema; the rejected tree is
bounded by nothing at all, and a caller that pipes a log file or a tarball at
`validate` should not silently fill a home directory with it. A legitimate
review is far under the cap — the format caps a body at 4,000 characters and a
summary at 1,500 — so the limit only ever catches something that was never going
to validate.

That is the split this section leaves the store with, stated plainly: **every
document that passed a schema is kept whole, in the reviews tree; the first
megabyte of every input that did not is kept in the rejected tree.** Bounding
one and not the other is what justifies capping either at all — a legitimate
review never approaches the cap, and an unbounded rejected tree is the one the
store cannot afford to keep whole.

The cap bounds only the copy this section writes. `validate` still reads and
parses the entire submitted input before any of this runs; truncating what
reaches the parser would change what gets validated, which is a worse defect
than a large file on disk, not a smaller one.

Truncation is what keeps every exit-1 row showing a `path` now
([§6.1](#61-output)): there is no longer a case where a run is recorded and its
file is not, so an oversized input needs no visible omission to signal it.

**The digest still names the full input, not the truncated bytes.** It is
computed before truncation, so the filename identifies what was actually
submitted: an agent looping on the same broken output still dedupes to one
file regardless of size, and two different oversized inputs that happen to
share a first megabyte are never conflated under one name. The cost is that a
truncated file's own hash no longer matches the name it is stored under — and
that is not a defect to route around, it is the signal. A mismatch between a
file's bytes and its name already means something under content addressing
([§6.3](#63-missing-and-foreign-files)): ordinarily it means tampering, and
for a rejected file over the cap it means truncation instead. Nothing else
records which happened — no `stored` column, no `truncated` flag — because
hashing the stored file and comparing the result to its own name already
tells the two apart.

The cap is fixed rather than configurable, because a knob here would be a knob
nobody sets correctly and a fifth key in a four-key file.

### 4.5 The database

One SQLite file at `<store>/store.db`, holding a row per **run** — not per
stored review. An exit-0 run has a row pointing at a file under `reviews/`; an
exit-1 run has a row pointing at a file under `rejected/`, truncated or not
([§4.4.1](#441-rejected-inputs)); only a run that wrote nothing at all — exit
101 — has a row with no file.

That difference is the second reason the database exists. A store of passing
reviews answers "what was concluded about this commit". A log of runs also
answers "what does this agent keep getting wrong", which is a question about the
prompt driving it rather than about any one review, and it cannot be asked of a
directory of successes. An agent tripping `id-unique` on a third of its attempts
has a prompt problem that no individual review reveals.

#### 4.5.1 What it holds

One table, one row per run, and every row about two hundred bytes.

```sql
CREATE TABLE runs (
  id             INTEGER PRIMARY KEY,
  at             TEXT    NOT NULL,  -- RFC 3339 UTC
  repo           TEXT    NOT NULL,  -- normalized name, §4.2
  ref            TEXT,              -- 40 hex; NULL when the input never parsed
  digest         TEXT    NOT NULL,  -- sha256 of the submitted bytes
  exit_code      INTEGER NOT NULL,
  verdict        TEXT             CHECK (verdict IN ('approve', 'request_changes', 'comment')),
  num_comments   INTEGER,
  num_errors     INTEGER,
  num_advisories INTEGER,
  num_skipped    INTEGER,
  tool_version   TEXT    NOT NULL,
  schema_version TEXT    NOT NULL
) STRICT;

CREATE INDEX runs_repo_ref ON runs(repo, ref);
CREATE INDEX runs_repo_at  ON runs(repo, at DESC);
CREATE INDEX runs_digest   ON runs(digest);
```

**The counters carry a `num_` prefix** so a column name says what it is without
the reader consulting its type. `comments` and `errors` read as collections;
`num_comments` and `num_errors` cannot be read as anything but counts, and the
cost of the prefix is four characters in a file nobody types by hand.

**Nothing records what a run found, only how much.** Two earlier drafts did:
the first stored the whole result object, several kilobytes against a
two-hundred-byte row, to carry messages and JSON pointers describing a document
the row does not hold; the second stored just its `lenses` field, the
deduplicated check names ([cli.md §5.2](cli.md#52-the-result-object)). Neither
is here.

What that costs is the difference between *this agent fails often* and *this
agent keeps tripping `id-unique`*. The counts and the timestamps still show
failure rates per repository over time, which is enough to tell whether a prompt
change helped; they cannot say which check to change it about. That is the
sharper half of the question, and it is deferred rather than answered.

Deferring it is not symmetrical with the other omissions, and it is worth being
plain about why. A column can be added later; the runs that already happened
cannot be re-recorded. Everything here is recoverable except history, and this
is the one column whose absence is only felt in hindsight.

**There is no `stored` or `path` column.** `exit_code` says which tree a run's
file is in and the rest of the row says where in it:

| `exit_code` | File |
| --- | --- |
| 0 | `<store>/reviews/<repo>/<ref>/<digest>.json` |
| 1 | `<store>/rejected/<repo>/<digest>.json` |
| 101 | none — the store is what failed |

Both are computed on read, which is always right; a stored path would go stale
the moment `store.path` changed, and a stored flag would restate a rule the
specification already fixes. Restating a rule is how the two come to disagree.

And no `remote` or `worktree`. Both were recorded as evidence for which checkout
a name referred to; neither is worth carrying for the life of a store, and
[§4.2](#42-repository-identity) says plainly what identity the name does and
does not establish.

**Nothing records whether the anchors were verified.** The result object says
([cli.md §5.2](cli.md#52-the-result-object)) and the row does not, so a store
cannot currently distinguish a review whose claims were checked against a
repository from one validated outside any repository at all. That is a real gap
and a deliberate one: it is held back until there is a question being asked of
it, on the same reasoning as the check table. It returns as a nullable column
whenever that changes, and rows written before then read as NULL — unknown,
which is exactly what they are.

#### 4.5.2 Constrained values

**SQLite has no `ENUM` type, and no `BOOLEAN` type either.** In a `STRICT` table
the only column types are `INT`, `INTEGER`, `REAL`, `TEXT`, `BLOB`, and `ANY` —
writing `BOOLEAN` is rejected at `CREATE TABLE`. Outside `STRICT`, SQLite
accepts the word as a column type with numeric affinity and stores `true` and
`false` as 1 and 0, which looks like a boolean and is not one. The idiom, and
what this schema would use if it needed a flag, is `INTEGER NOT NULL CHECK (x IN
(0, 1))`.

For closed sets of strings the idiom is the same `CHECK` — which `verdict`
carries, and is the only column that does. A lookup table with a foreign key is
the other option and is not worth it for three values.

Two details make it work as intended:

- **`STRICT` is what makes the column types real.** SQLite's normal type
  affinity accepts a string in an `INTEGER` column and stores it. Without
  `STRICT` the `CHECK` constraints would be guarding columns that had already
  let the wrong kind of value in.
- **A nullable enum needs no special case.** `verdict` is NULL when the document
  never parsed, and `NULL IN ('approve', …)` evaluates to NULL rather than
  false. A `CHECK` fails only on false, so NULL passes and the constraint still
  rejects every wrong string.

**`exit_code` is deliberately unconstrained.** SQLite cannot alter a `CHECK`, so
every constrained set is a table rebuild away from every addition to it — and
the exit codes are the one set here designed to grow:
[cli.md §4](cli.md#4-exit-codes) reserves 102–125 for tool errors that do not
exist yet. A constraint there would mean a schema migration to ship a new
failure mode, which is a tax on exactly the change that should be cheap.

That is the line for anything added later. Constrain a set that a
**specification** defines, because changing it is already a versioned event and
a migration is the honest expression of that — `verdict` comes from
[review-document.md §3](review-document.md#3-root-object) and is API. Leave
unconstrained a set the **implementation** reserves room to extend. When the two
are confused, either the database accepts values the format forbids, or shipping
a patch release needs a migration.

#### 4.5.3 Why SQLite

The requirement is a single embedded file, no server, no cgo, and a real query
language. Three candidates were weighed:

| | Verdict |
| --- | --- |
| **SQLite** (`modernc.org/sqlite`, pure Go) | **Chosen.** SQL, indexes, transactions, one file, no cgo, and the most widely understood on-disk format there is. |
| **bbolt** | Rejected. A key-value store has no query language, so every question above becomes a full scan written by hand — which is the work the database was added to avoid. |
| **Dolt** | Rejected. A versioned SQL database is a fine thing and the wrong shape here: embedding it pulls in a SQL engine and storage layer measured in hundreds of megabytes of dependencies, for a binary whose seventh design principle is that it is cheap to call. Its version control also duplicates what the reviews tree and `at` already give — immutable content, addressed by hash, with a timestamp. |

This is the largest dependency the project takes on, and
[cli.md §7.3](cli.md#73-dependencies) asks for a shallow tree. The pure-Go
SQLite driver is a transpiled C library, and it costs what one costs: binary
size **and** startup time, not the first instead of the second. An earlier
draft of this section claimed the opposite, measured against a scratch
`main` with a blank import that never opened a database. Measured instead
against the real binary — `aedb291`, the last commit before the store was
wired into `validate`, against this branch's `HEAD` — the binary grew from
5,754,098 to 12,285,026 bytes, **+6.2 MiB, +114 percent**. A clean
`validate`'s median startup, 50 runs after a five-run warmup, went from
22.5ms to 38.2ms — **+15.7ms, +70 percent** — from paging in a binary that
more than doubled and from opening, migrating, and writing to a real SQLite
file on every call, not from linking alone.

[cli.md §1](cli.md#design-principles)'s seventh design principle is "cheap
to call, fast startup," and fifteen milliseconds is a real cost against it,
not a rounding error. It is also fifteen milliseconds against an agent's
inference time, which is measured in seconds on every call this tool is
invoked from — the reason [cli.md §6](cli.md#6-token-economy) budgets in
tokens rather than milliseconds is that tokens are the cost an agent
actually pays per call, and a few milliseconds of local disk I/O does not
compete with it. The alternative was never free: bbolt turns every query in
[§4.5.1](#451-what-it-holds) into a hand-written scan, and Dolt is a SQL
engine and storage layer measured in hundreds of megabytes, against the
same seventh principle, for capability this store does not need. SQLite
stays chosen. What changes here is the claim made about its price: it costs
milliseconds, stated plainly, not sub-millisecond, stated before anyone
measured it against the wired binary.

#### 4.5.4 sqlc

Queries are written as SQL and compiled to Go by
[sqlc](https://sqlc.dev), which reads the schema and the queries and generates
typed methods against them.

```
internal/store/sql/schema.sql     the table above, and every migration
internal/store/sql/query.sql      every query, named, with sqlc annotations
internal/store/sqlc/              generated: models, params, methods
sqlc.yaml                         engine sqlite, output into internal/store/sqlc
```

The reason is the same one behind "the schema is the documentation"
([cli.md §1](cli.md#design-principles)). A hand-written query is a string that
the compiler cannot check against the table it reads, so a renamed column, a
dropped index, or a scan into the wrong type is a runtime error found by a test
if there happens to be one. sqlc makes all three a build failure, and it makes
`schema.sql` the single statement of the shape — which is what
[§4.9](#49-the-store-is-not-a-contract) needs in order for the shape to be
allowed to change.

It joins `moq` and `gofumpt` in `internal/tools/tools.go` and runs under
`make generate`, which stays the one entry point for code generation. Generated
code is committed, as `moq_test.go` already is, so a build never depends on the
tool being installed.

**sqlc does not do migrations**, and pretending otherwise is where projects get
hurt. `schema.sql` is embedded with `//go:embed` and executed on a database that
does not exist yet; `PRAGMA user_version` records which revision a file is at;
and a later revision ships an explicit migration alongside the schema it
produces. sqlc generates against the *current* schema and has no opinion about
how a file got there, so that ordering has to be the store's own code and has to
be tested against a database written by the previous version.

### 4.6 Concurrency and locking

The database is opened in **WAL mode** with a **busy timeout**. WAL lets readers
run while a writer commits, so a `reviews` query never blocks behind a
`validate`, and the busy timeout makes two concurrent writers wait rather than
fail.

Serializing writers is acceptable here in a way it would not be in a
long-running service: a `validate` holds the write transaction for the time it
takes to insert one row, having already done its parsing, verification, and file
write outside it. Agents run these commands in loops, but each command is brief,
and brief is what makes a lock cheap.

A busy timeout that **expires** is a tool error, exit 101
([§5.1](#51-when-storing-fails)). It means something is holding the database far
longer than any run of this tool does — an interactive `sqlite3` session, a
backup, a hung process — and inventing a fallback for it would mean writing a
run that later contradicts the file beside it.

The file is written **before** the row that accounts for it, and the two are not
one atomic act. The order makes the incomplete state the harmless one: a file no
row accounts for is invisible to every query, and is reclaimed by the next store
of the same bytes, which finds it already present. A row whose file is missing
is the state that would have to be repaired, and it cannot arise from an
interrupted run.

### 4.7 When the database is lost

The database is the source of truth for everything in it. Losing it loses that,
and **nothing rebuilds it**. That is a real cost of choosing a database over
metadata-on-disk, and it is stated here rather than discovered.

What survives is everything submitted to the reviews tree, and the first
megabyte of everything submitted to the rejected tree
([§4.4.1](#441-rejected-inputs)). The trees are untouched, and their structure
still carries meaning a person can read: `<repo>/<ref>/` says which project and
which commit, and the filename is the checksum of what was submitted —
`sha256sum` still verifies a review file, but not a rejected file that was
truncated, since its name commits to bytes beyond what is on disk. What is
gone is what the runs *found* — the counts, the exit codes, the timestamps,
and every record of a run that produced no file.

Reconstruction is deliberately deferred rather than half-built
([§8](#8-future-considerations)). A rebuild that silently invented a
`tool_version` would poison the one column that exists to say which binary
reached a verdict, and one that guessed `at` from a file's modification time
would produce a history no query could distinguish from a real one. Both are
solvable; neither is solvable in passing, and a recovery path that quietly
degrades the data it recovers is worse than an honest gap.

Two consequences to design around until it exists:

- **A store is worth backing up as a unit.** `store.db` and the trees are one
  artifact; copying the trees alone preserves the reviews and loses the log.
- **The trees stay legible on purpose.** [§4.1](#41-layout) keeps
  `<repo>/<ref>/` in the path when the database could have replaced it, and this
  is why: it is the precondition that keeps a rebuild buildable later.

### 4.8 Path safety

Every path component is validated before it touches the filesystem, because a
repository name is derived from a remote URL, and a remote URL is attacker-
adjacent data in any workflow that reviews code it did not write.

- Each segment matches `^[a-z0-9][a-z0-9._-]*$`, is at most 64 characters, and
  is never `.` or `..`.
- A repository name has 1 to 3 segments and is at most 200 characters total.
- A ref is exactly 40 lowercase hex characters.
- `no-repo` is reserved ([§4.2](#42-repository-identity)); a derived name may
  not equal it.

**A derived name is normalized to fit**, in this order, per segment: lowercase;
replace every character outside `[a-z0-9._-]` with `-`; collapse runs of `-`;
trim any leading or trailing character that is not `[a-z0-9]`; truncate to 64.
The order matters — lowercasing first is what keeps `My_Repo` from becoming
`-y-repo`, and trimming last is what keeps the result matching the leading-
character anchor. A segment left empty by all of that, or a whole name left
empty, falls back to `no-repo`.

Normalization is lossy and nothing records what was lost. That is a deliberate
consequence of [§4.2](#42-repository-identity) keeping no remote URL: two
remotes that normalize alike are indistinguishable afterwards, and `store.repos`
is the prevention rather than the cure.

**A name that comes from a person is never normalized.** A `store.repos` value
that does not fit is a config error and exits 101; a `--repo` value that does
not fit is a usage error and exits 2. Neither performs a lookup first, so a
traversal attempt never reaches the filesystem to be tested.

This is the same rule `verify` already applies to anchor paths: a component that
can climb out of the tree it names is rejected on its shape, not discovered by
its effect. It applies to the database too: a repository name reaches SQL only
as a bound parameter, never as concatenated text.

### 4.9 The store is not a contract

The layout in [§4.1](#41-layout), the filenames in
[§4.4](#44-the-stored-files), and the schema in
[§4.5.1](#451-what-it-holds) are **this tool's internal state**, not an
interface anything else may depend on. They can change, a later version may
migrate what is on disk, and `PRAGMA user_version`
([§4.5.4](#454-sqlc)) is what makes that orderly.

The supported way to read a store is `loam-refinery reviews`
([§6](#6-reading-the-store)). That command's *output* is the contract; what sits
behind it is not. Opening `store.db` with `sqlite3` to answer a question nobody
built a flag for is expected and fine — that is much of why it is SQL — but a
script that depends on a column name is a script a migration will break, and it
will not be warned first.

This is deliberately weaker than every other promise in these specifications.
Check names are API; lens names are API; the result object is API. The store is
an audit trail whose job is to still be there later, holding enough for features
that do not exist yet to do something with it. Freezing its shape now, before
any of those features exist, would be committing to a design decided by the
first thing that needed it.

One thing inside it is *not* free to change: a review file stays the submitted
bytes. Migrations may rewrite the schema, move directories, and rebuild indexes;
they may not rewrite a review. That is the one guarantee an audit trail cannot
renegotiate.

## 5. Storing a review

```
loam-refinery validate [path]
```

No flag. Every review that validates clean is stored, and every input that does
not is stored too.

**Exit 0 writes a review; exit 1 writes a rejected input.** They go in separate
trees ([§4.4](#44-the-stored-files)) and are never mixed, because a store whose
reviews include the failures stops being the answer to "what was concluded about
this commit". Keeping both is what lets the store answer that question and
"what did this agent actually emit" without either one contaminating the other.
The one exception is size: a rejected input over 1 MiB is recorded and kept,
truncated to its first 1 MiB ([§4.4.1](#441-rejected-inputs)).

**Every run records a row**, including the ones that wrote no file
([§4.5](#45-the-database)). That is the second question a store can answer: not
"what was concluded" but "what does this agent keep getting wrong", which is a
fact about the prompt driving it rather than about any one review. A directory
of successes cannot be asked it.

**None of it appears in `validate` output.** The result object describes the
review, and where a copy of it landed is not a fact about the review — a caller
that wants to know asks `loam-refinery reviews`, which is the command for
questions about the store. Keeping it out is also what holds a clean `validate`
at its original 80-token ceiling ([cli.md §6.1](cli.md#61-budgets)), on the one
path every loop pays.

**Any run with something to keep creates the store**, a failure as readily as a
success ([§2.2](#22-first-use-creates-what-it-needs)). An earlier draft reserved
that for passing reviews, so a machine that had never produced a valid one
recorded nothing about the attempts — which withholds the record exactly where
it is most wanted, since an agent that never succeeds is the one worth
inspecting. The tradeoff it was buying is now paid for openly: a repository of
nothing but failing reviews does leave a directory behind, and on a machine
where `validate` cannot write, an exit-1 run fails with 101 the same as an
exit-0 one would ([§5.1](#51-when-storing-fails)).

The only runs that create nothing are the ones with nothing to create it for: an
exit 2 that never read a document, and a machine with `store.enabled: false`.

**There is no per-run opt-out.** `store.enabled: false` in the config file turns
storing off for a machine, and that is the whole of it — a decision made once,
where the rest of the machine's settings live, rather than a flag every caller
has to remember not to forget.

Dropping the flag is what makes the store trustworthy rather than merely
available. A store that any invocation can silently skip cannot answer "what was
concluded about this commit" — only "what was concluded, on the runs where
somebody left the flag alone", which is not a question anyone asks. The same
reasoning already made verification run by default with no flag
([cli.md §2.3.1](cli.md#231-verifying-anchors)): a tool whose value depends on
being asked will not be asked.

The cost is that `validate` writes outside the repository on essentially every
run, and on a machine where it cannot, the command fails
([§5.1](#51-when-storing-fails)). [§5.2](#52-turning-it-off) is how an
environment that cannot accept that says so.

### 5.1 When storing fails

**A store that cannot be established or written fails the command, with exit
101.** A missing config directory is created; a missing store is created; and if
creating or writing either one fails — a read-only home, a full disk, a
`store.path` pointing somewhere the process cannot write — the run exits 101
with the reason on stderr.

Exit **101**, and not 1 or 2, because it is a third kind of problem
([cli.md §4](cli.md#4-exit-codes)). Exit 1 means revise the review, and a full
disk is not a defect in the review; an agent told "revise" enters a loop it
cannot exit. Exit 2 means fix the command, and the command was right; an agent
told that re-reads its own flags and finds nothing wrong with them. The only
useful response is to stop and report the machine, which is what a code of its
own lets a caller do.

**Nothing is written to stdout**, only the reason on stderr, exactly as every
other failing exit behaves ([cli.md §5.2](cli.md#52-the-result-object)).

An earlier draft made this an exception and printed the result object anyway, on
the reasoning that the verdict and the machine failure are independent facts and
a caller should get both. That only held while the object could *say* the store
had failed. With no `stored` block in it, the object a failed run would print is
indistinguishable from a successful one — `valid: true`, and no field mentioning
what went wrong — and publishing a clean-looking result for a run that failed is
worse than publishing nothing. The exit code carries the failure and stderr
carries the reason, which is what the rest of the tool already does.

**A failure never leaves a store to repair.** The review file is written before
the row that points at it ([§4.6](#46-concurrency-and-locking)), so a run that
fails partway leaves at most a file no query can see — reclaimed by the next
store of the same document, which finds it already there. There is no state in
which the database refers to something that is not on disk.

### 5.2 Turning it off

An environment where `validate` must not write — a read-only container, a CI
sandbox, a machine with no writable `$HOME` — has two answers, and both are
arranged **before** the run rather than during it:

1. **Point the store somewhere writable.** `LOAM_REFINERY_HOME=/tmp/loam` or
   `XDG_DATA_HOME=/tmp` needs no file and no flag, and is the answer for a
   container that is willing to store but not into `$HOME`.
2. **Ship a config file** containing `{"version":"1","store":{"enabled":false}}`.
   Storing is off, and nothing is created or recorded.

The second one has a bootstrapping edge worth stating plainly, because it is the
sharpest consequence of removing the flag. The key that disables storing lives
in a file that first use would otherwise create — so on a filesystem that is
already read-only, a machine cannot be *told* not to store unless the file is
put there in advance, by whoever builds the image. There is no in-line escape.
That is a real operational constraint, not an oversight: it is the price of a
store that no individual invocation can opt out of, and the first environment
variable above is why it is rarely the one you need.

## 6. Reading the store

```
loam-refinery reviews [--repo=NAME] [--ref=SHA] [--limit=N] [--content]
                      [--failed] [--list] [--format json]
```

A fifth content command, and the first one that reads something the tool wrote.
It fetches nothing, resolves nothing, and **writes nothing** — every form is a
read, including on a machine that has no store yet
([§2.2](#22-first-use-creates-what-it-needs)).

It is a **query**, not a walk. Every form answers from `store.db`
([§4.5](#45-the-database)); the trees are opened only for files `--content`
asked for. That is what keeps a listing the same cost at ten thousand stored
reviews as at ten.

**It is deliberately small.** This is the surface that proves end to end that a
review was stored, a rejection was recorded, and both come back — and the
surface a person uses to look. It is not a query API, and several obvious
conveniences are absent on purpose ([§8](#8-future-considerations)) because a
small command is cheaper to change than a designed one.

| Flag | Default | Meaning |
| --- | --- | --- |
| `--repo=NAME` | inferred from the CWD's repository | Which repository's reviews. |
| `--ref=SHA` | all refs | Which commit. The full 40-character SHA; abbreviations are a usage error. |
| `--limit=N` | 10 | Most recent N. `0` means no limit. |
| `--content` | off | Include each stored file, not just its row. |
| `--failed` | off | List runs that stored no review instead of ones that did. |
| `--list` | off | Print the repositories the store knows, and nothing else. |

**`--repo` is inferred the same way verification finds a repository** — walking
up from the working directory, then naming what it finds per
[§4.2](#42-repository-identity). Running `loam-refinery reviews` inside a
checkout asks about that checkout, which is the question a caller standing
there almost always has. Outside a repository, with no `--repo`, it is a usage
error naming `--list` as the way to find out what is there.

**`--ref` takes the whole SHA.** An earlier draft resolved unique prefixes of
seven or more characters, which meant a prefix query, an ambiguity check, a
candidate listing, and an exit-2 path that exists nowhere else — a good amount
of surface bought with typing convenience. The full SHA is one equality match,
and it is what
[review-document.md §5.1](review-document.md#51-refs) already requires
everywhere else. Prefix resolution is the first thing to add back if querying by
hand turns out to matter.

**Flags do not silently compose.** `--list` answers a different question from
every other form and takes no company: combining it with `--repo`, `--ref`,
`--limit`, `--content`, or `--failed` is a usage error rather than a flag that
is quietly ignored. Everything else composes freely — `--failed --content
--limit=1` is a normal thing to type.

### 6.1 Output

Default is an **index**, newest first — what was stored, not what it said:

```json
{
  "repo": { "name": "github.com/bobcob7/loam-refinery", "known": true },
  "total": 14,
  "reviews": [
    {
      "at": "2026-08-19T14:30:05Z",
      "ref": "4f2c1a9e3b7d5f0c8a1e2d4b6c8f0a2e4d6b8c0f",
      "digest": "3f9a1c2b7e04d19c…",
      "verdict": "request_changes",
      "counts": { "comments": 6, "errors": 0, "advisories": 2, "skipped": 0 },
      "path": "/Users/me/.local/share/…/reviews/github.com/bobcob7/loam-refinery/4f2c…/3f9a1c2b7e04d19c….json"
    }
  ]
}
```

`at` is the column of the same name ([§4.5.1](#451-what-it-holds)), spelled the
same way in every output this command produces. `total` is the number of rows
matching the query before `--limit`, so a caller can tell a truncated answer
from a complete one. `path` is absolute, so anything that wants the file can
open it without reconstructing the layout; it is computed from the row rather
than stored, and its presence is not a claim that the file is still there.

**`--failed` lists the other half of the log** — runs that produced no review —
with the same row shape:

```json
{
  "at": "2026-08-19T14:22:41Z",
  "ref": "4f2c1a9e3b7d5f0c8a1e2d4b6c8f0a2e4d6b8c0f",
  "exit_code": 1,
  "counts": { "comments": 3, "errors": 2, "advisories": 1, "skipped": 0 },
  "path": "/Users/me/.local/share/loam-refinery/rejected/github.com/bobcob7/loam-refinery/44136fa355b3678a….json"
}
```

`ref` is **omitted** when the run has none — an input that never parsed has no
commit to name, and a null would invite a consumer to render it. `path` is
always present on an exit-1 row: every rejected input is kept, truncated to
its first 1 MiB when it was larger ([§4.4.1](#441-rejected-inputs)), so there
is no case left where the row has a run but no file to point at.

The row shown above is not the whole answer — it is wrapped exactly like the
default index: `repo`, `total`, then the array, named `failed` in place of
`reviews`:

```json
{
  "repo": { "name": "github.com/bobcob7/loam-refinery", "known": true },
  "total": 3,
  "failed": [ /* … rows like the one above … */ ]
}
```

The specification this implements never showed that envelope, only the bare
row, on the assumption that a reader would carry the wrapper over from the
default form. It is worth stating outright: nothing about `--failed` changes
the shape *around* the rows, only what is in them.

It answers when, against which commit, how much — and `path` is the submitted
input, truncated to its first 1 MiB when it was larger
([§4.4.1](#441-rejected-inputs)), which is the answer to the question the
counts cannot reach. It does not answer *which check* fired,
because the row does not record one ([§4.5.1](#451-what-it-holds)); reading the
file is what replaces that, and it is a better answer than a list of names for
anyone asking why an agent keeps failing.

`--content` works here too, and returns the input verbatim rather than a
document — including for the inputs that are not JSON at all, which are
delivered as a JSON string rather than as parsed structure. The field it adds
is named `review` on this form too — the same key the default index above
uses for a passing document — which is an odd name for a rejected input that
was never a review, sometimes not even parseable as one. It is kept anyway so
a caller reading `--content` output never has to branch on which form it
asked for to find the content.

**Returning an index rather than documents is the same argument as `describe`.**
A repository with a hundred stored reviews holds several hundred kilobytes of
prose that a caller asking "what reviews exist for this commit" did not ask for.
The index is the cheap answer with enough in it to choose; `--content` is the
expensive one, requested deliberately, for the caller that has chosen.

**`--content` is explicitly unbudgeted.** Every other command in
[cli.md §6.1](cli.md#61-budgets) has a ceiling, and this one cannot: it returns
documents the caller wrote, at whatever size the caller wrote them. Saying so is
better than inventing a ceiling that truncates a review, and `--limit` is the
control a caller actually has. `--limit=1 --content` is the ordinary way to read
one review.

What it adds to each row is a `review` field holding the stored document
verbatim, read from the file the row's `path` names. The row is the database's
answer and the review is the file's, joined in the output rather than on disk —
which is the right place for it, because the command's output is a contract and
the storage behind it is not ([§4.9](#49-the-store-is-not-a-contract)). On this
form `review` is the document itself, embedded as JSON, because that is what
was stored; on `--failed` the same key holds a JSON string instead, because
what was stored there is not guaranteed to parse as anything.

`unreadable` sits on the envelope, next to `total`, not on any one row — one
count for the whole answer, not a flag per file
([§6.3](#63-missing-and-foreign-files)). It appears only when `--content`
asked a file to be opened; a listing that answered entirely from the
database never touched a file and has nothing to report, so the key is
omitted rather than printed as zero.

**`--list` names the repositories the store knows**, with what each holds:

```json
{
  "repos": [
    { "name": "github.com/bobcob7/loam-refinery", "reviews": 14, "failed": 3 },
    { "name": "no-repo", "reviews": 0, "failed": 7 }
  ]
}
```

Names alone would answer the `known: false` question in
[§6.2](#62-empty-answers), and the counts are what make the answer useful for
choosing what to ask next rather than merely enumerating. Ordered by name, so
the output is stable between runs.

This is the one form whose size grows with the **breadth** of a store rather
than with `--limit`, which is why its budget is per repository rather than flat
([cli.md §6.1](cli.md#61-budgets)). A flat ceiling on something that scales is
the failure that section already solves for `describe --lens` by budgeting per
entry.

### 6.2 Empty answers

An empty result is not an error and exits 0. A query that matched nothing and a
query that named something that does not exist are, however, **different
answers**, and the result says which:

| Case | Result | Exit |
| --- | --- | --- |
| Repository known, ref has no reviews | `known: true`, `reviews: []` | 0 |
| Repository not in the store | `known: false`, `reviews: []` | 0 |
| Store does not exist at all | `known: false`, `reviews: []` | 0 |
| Malformed `--repo` or `--ref`, or `--list` with company | usage error | 2 |
| Database exists but cannot be opened or read | tool error on stderr | 101 |

`known: false` is the whole point of the distinction. A mistyped repository name
returning a bare empty list is indistinguishable from a repository whose reviews
were never stored, and a caller cannot tell that it asked the wrong question —
the same failure `skipped` and `verification` exist to prevent on the `validate`
side. `--list` is the follow-up the caller makes when `known` is false.

### 6.3 Missing and foreign files

The trees are directories a person can write to, so a row will eventually name a
file that has been deleted or is unreadable. Under `--content`, such a file is
**skipped and counted**, not fatal:

```json
"unreadable": 2
```

One bad file must not make a hundred good ones unreachable, and a silent skip
would let a store quietly lose reviews.

**Only `--content` can report this**, because only `--content` opens a file. A
default listing answers entirely from the database and never touches a tree, so
its `path` values are computed addresses rather than verified ones and no
`unreadable` count appears. That asymmetry is the price of listings that do not
slow down as a store grows, and a caller that needs certainty asks for the
content.

Verifying each file against the digest that names it was specified in an earlier
draft and is not here. Content addressing makes tampering *detectable* — a
review file whose bytes no longer hash to its own name has been altered — but
re-hashing every file a caller reads is machinery this command does not need
yet, and [§8](#8-future-considerations) is where it waits.

A rejected file is a partial exception to what a mismatch means: it is not
necessarily tampering, because a file over 1 MiB is truncated to its first
megabyte by design while its name still names the full input
([§4.4.1](#441-rejected-inputs)). The same hash-against-name check that would
catch tampering doubles as the only signal that a rejected file was truncated
rather than kept whole — and this command runs it no more for that than it
does for the reviews tree above.

Files in a tree that no row accounts for are ignored entirely rather than
counted: the litter editors and backup tools leave behind, and the file of a run
that died before recording its row ([§4.6](#46-concurrency-and-locking)) — an
interrupted write rather than damage, since nothing was lost. Nothing adopts
them, and nothing reports them; that is part of what a rebuild would do
([§4.7](#47-when-the-database-is-lost)).

## 7. Growth and retention

A stored six-comment review runs roughly 4–8 KB. A thousand of them is under 10
MB, and a thousand stored reviews is a great deal of reviewing. `store.db` adds
roughly 200 bytes per run, and rejected inputs add whatever an agent emitted —
but only once each, because they are content-addressed
([§4.4.1](#441-rejected-inputs)). An agent looping fifty times on one broken
document costs one file and fifty rows, which is why the failure case does not
need a cap.

**Nothing is pruned automatically, in this version.** A tool that deletes a
user's reviews on a schedule they did not set is a worse failure than a
directory that grew, and the layout makes manual pruning targeted and obvious:
one `rm -rf` per repository or per commit, with the paths right there in the
`reviews` output. Retention is [§8](#8-future-considerations).

Manual pruning does leave the database describing files that are gone. That is
the state [§6.3](#63-missing-and-foreign-files) already handles, and it is
survivable rather than tidy — which is the honest trade for having no
`--prune` yet.

**Nothing this tool does deletes a store wholesale.** If that ever changes, it
removes `store.db`, `store.db-wal`, `store.db-shm`, `reviews/`, and `rejected/`
by name rather than the directory containing them — because under
`$LOAM_REFINERY_HOME` ([§2](#2-locations)) that directory also holds
`config.json`, and a clear-my-store command must never be able to take a
person's settings with it. The two sidecar files are usually already gone by
the time such a command runs — WAL mode removes them on a clean close
([§4.6](#46-concurrency-and-locking)) — but naming five files instead of
three is what stops a delete that follows a crash or a kill from leaving
`store.db-wal` behind, orphaned from the database it belonged to, which is
the one piece of SQLite litter capable of confusing a later open.

Query cost does not grow with either number in the way a directory walk would:
the indexes in [§4.5.1](#451-what-it-holds) cover the listings `reviews` makes,
and the tree is opened only for a file a caller asked to read. Storage grows;
answering does not get slower.

## 8. Future considerations

Deliberately deferred, recorded so the design leaves room for them.

- **Rebuilding the database from the trees.** A `reviews --reindex` that walks
  both trees and reconstructs what the files support: `repo` and `ref` from the
  path, `digest` from the filename, `verdict` and `num_comments` by parsing.
  It cannot recover what the runs found, and the two honest problems it has to
  solve first are what to write for `tool_version`, which exists to say which
  binary reached a verdict, and how to mark an `at` guessed from a modification
  time so no query mistakes it for a real one. Both want a schema decision
  rather than a flag ([§4.7](#47-when-the-database-is-lost)).
- **Reporting orphans.** Files in a tree that no row accounts for. It is the
  diagnostic half of the same feature and is deferred with it.
- **Querying by ref prefix.** `--ref` takes the full SHA
  ([§6](#6-reading-the-store)); resolving a unique prefix is the first
  convenience to add if querying by hand becomes common.
- **Verifying stored files against their digests.** Content addressing makes
  tampering detectable; nothing checks for it yet
  ([§6.3](#63-missing-and-foreign-files)).
- **Run analytics.** `reviews --failed` lists; it does not summarize. The
  questions the run log exists to answer — which checks this agent trips most,
  whether that is improving, how a prompt change moved the numbers — want
  grouping and rates rather than rows. Deferred because the useful shape of that
  output is not knowable before there is a store with months in it, and because
  `sqlite3 store.db` answers all of it today for whoever needs it first.
- **Retention.** `reviews --prune --older-than=90d`, or a `store.retain` key.
  Deferred until there is evidence a store gets big enough to need it; the
  design that matters is that pruning must be an explicit act, never a side
  effect of validating. It has to delete rows and files together, which is the
  one place the split costs something.
- **Pruning rejected inputs separately.** The rejected tree is the half most
  likely to want a retention policy of its own: its value decays as soon as the
  agent that produced it is fixed, where a stored review's does not. Deferred
  with the rest of retention, and noted here because the two probably want
  different defaults.
- **Worktree redaction.** `repo.worktree` records an absolute local path, which
  is a small privacy consideration the moment anyone copies a store off the
  machine. A `store.record_worktree: false` key is one line; it is not here
  because the store is specified as local and adding keys ahead of a real need
  is how a four-key config becomes a forty-key one.
- **A project-local store.** `.loam-refinery/reviews/` inside the checkout,
  committed alongside the code, is what a team rather than an individual would
  want. It is a different feature: it needs a merge story, a review of what
  belongs in version control, and it breaks the "no writes inside the
  repository" line this document keeps.
- **Sharing.** Publishing a store, or reading one someone else published, is
  where the network arrives. The reviews tree is deliberately a plain directory
  of immutable content-addressed files, which is the easy half to sync with
  tools that already exist; the database is the half that would need a merge
  story, and neither belongs in this binary.
- **`reviews --diff`.** Given two refs, what changed between the reviews of
  them: which findings persisted, which were resolved, which are new. It reads
  the two review files rather than the database — comment IDs being stable
  slugs is what makes it tractable — and it is the most likely next command.
