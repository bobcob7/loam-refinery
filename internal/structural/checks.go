package structural

import "github.com/bobcob7/loam-refinery/internal/review"

// Checks returns the structural check registry: hard checks that decide whether
// the input is a document at all. They cannot be disabled or demoted.
func Checks() []review.Check {
	return []review.Check{
		{
			Name:    "document-unparseable",
			Tier:    review.TierStructural,
			Summary: "the input is not a single JSON object",
			Title:   "Unparseable document",
			Body: `Fires when the input never became a document: malformed JSON, a top-level
value that is not an object, or a second value after the first. It is the one
failure that stops the run, because there is nothing left to check — every
other check reports against a document, and here there is none.

No other diagnostic accompanies it. Do not read that quiet as approval of the
rest: nothing else ran.

The usual causes are prose wrapped around the JSON, a fenced code block left
in, and two documents concatenated into one stream. The message carries the
decoder's own complaint, which names the byte offset:

  loam-refinery submit-review review.json
  invalid character 'h' looking for beginning of value

This is a document to repair rather than a command line to fix, so it exits 1
like any other unusable document.`,
			Related: []string{"schema", "tiers", "verdict"},
		},
		{
			Name:    "schema",
			Tier:    review.TierStructural,
			Summary: "JSON Schema draft 2020-12 conformance against the embedded schema",
			Title:   "Schema conformance",
			Body: `Fires when the document does not conform to the embedded JSON Schema: a
missing required field, a value of the wrong type, an out-of-range priority, an
enum value that is not in the enum, or a field name the format does not define.
Each failure is reported with a JSON Pointer into your document, so the third
column tells you exactly where to look.

Unknown fields are rejected on purpose: the failure this format exists to catch
is the mistake that reads as correct, and a misspelled key is exactly that.
Write "end-line" and a permissive schema drops it silently, leaving a review
that validates clean while claiming a span you never meant. When the unknown
name is close to a real one the diagnostic says so:

  unknown field "end-line" — did you mean "end_line"?

Open the lens for the field the diagnostic names — for "12 is greater than the
maximum of 10" that is the priority scale, not JSON Schema.`,
			Related: []string{"version", "comments", "priority"},
		},
		{
			Name:    "id-unique",
			Tier:    review.TierStructural,
			Summary: "no two comments share an id",
			Title:   "Duplicate comment id",
			Body: `Fires when two comments carry the same id. Ids are how every consumer refers
to a finding — a diagnostic, an orchestrator saying "resolve missing-context-2",
a human reading a digest — so a duplicate makes one of the two findings
unaddressable, and nothing downstream can tell which one was meant.

Fix it by renumbering rather than renaming. The slug is the grouping mechanism,
so two findings of the same kind belong on the same slug and differ only in
suffix; inventing a second slug to dodge the collision throws away the grouping
that made the ids worth having.

  before: "missing-context-1", "missing-context-1"
  after:  "missing-context-1", "missing-context-2"`,
			Related: []string{"id", "id-grouping"},
		},
		{
			Name:    "anchor-range-ordered",
			Tier:    review.TierStructural,
			Summary: "end_line requires line and must be greater than or equal to it",
			Title:   "Anchor span out of order",
			Body: `Fires when an anchor carries end_line without line, or an end_line before its
line. A span that runs backwards names no lines at all, and an end_line with no
start has no anchor point, so neither can be resolved by anyone reading the
review later.

Drop end_line for a single-line anchor rather than repeating line; the span is
inclusive, so line 88 with end_line 94 is seven lines.

  before: { "file": "internal/fetch/client.go", "line": 94, "end_line": 88 }
  after:  { "file": "internal/fetch/client.go", "line": 88, "end_line": 94 }`,
			Related: []string{"line", "end_line", "anchors"},
		},
		{
			Name:    "anchor-path-safe",
			Tier:    review.TierStructural,
			Summary: "file must be a relative POSIX path inside the repository",
			Title:   "Unsafe anchor path",
			Body: `Fires when an anchor's file is not a plain repository-relative POSIX path: a
leading slash, a ".." segment, or a backslash anywhere. An absolute path names a
location on the reviewing machine, which nobody else has; a ".." segment names a
file outside the repository the ref belongs to, so no commit can confirm it; a
backslash is a Windows path that will not resolve in git.

Write the path exactly as git reports it — git ls-files output is always the
right shape.

  before: "/home/me/repo/internal/fetch/client.go", "../other/client.go"
  after:  "internal/fetch/client.go"`,
			Related: []string{"file", "anchors"},
		},
		{
			Name:    "ref-format",
			Tier:    review.TierStructural,
			Summary: "ref must be a full 40-character lowercase commit SHA",
			Title:   "Malformed ref",
			Body: `Fires when the document ref is not 40 lowercase hex characters. There is one
ref, at the root, and every anchor is read at it. Branches and tags are rejected because they name a moving
target: an anchor recorded against "main" means whatever main points at when
someone looks, which is not what was reviewed, and nothing in the document
records the difference. Abbreviations are rejected because they are unambiguous
only until the repository grows.

This is checkable without a repository, so it fires even when verification is
skipped. git rev-parse HEAD produces the correct value, and a reviewing agent is
holding a checkout already.

  before: "main", "4f2c1a9"
  after:  "4f2c1a9e8b3d7c5a1f0e2d4b6a8c9e1f3a5b7c9d"`,
			Related: []string{"ref", "ref-unknown"},
		},
	}
}
