package cli

import (
	"context"
	"io"

	"github.com/bobcob7/loam-refinery/internal/entry"
	"github.com/bobcob7/loam-refinery/internal/profile"
	"github.com/bobcob7/loam-refinery/internal/render"
	"github.com/bobcob7/loam-refinery/internal/review"
	"github.com/bobcob7/loam-refinery/internal/store"
	"github.com/bobcob7/loam-refinery/internal/validate"
)

//go:generate moq -out moq_test.go . documentValidator documentStore reviewStore profileSource headChecker HeadCheck

// documentValidator runs every check tier over one document.
type documentValidator interface {
	Validate(ctx context.Context, source []byte, options validate.Options) (*review.Result, error)
}

// StoreInput is what submit-review has learned about one run by the time storing
// happens (docs/config.md §5). Ref, Verdict, and Assessment are read straight
// off the submitted document, unvalidated — a documentStore applies whatever
// rules keep them safe to record, the same way it applies docs/config.md §4.8 to
// the repository name.
type StoreInput struct {
	// Dir is where repository identity is resolved from (docs/config.md
	// §4.2), the same directory verification already starts from.
	Dir string
	// Source is the exact bytes submitted, addressed and stored verbatim.
	Source []byte
	// Valid says whether this run exits 0 or 1 — which tree, if either,
	// keeps the bytes (docs/config.md §5). Meaningless when Precondition is
	// true: exit 3 keeps neither tree regardless of what Valid says, and a
	// documentStore must check Precondition first.
	Valid bool
	// Precondition says this run exits 3: the precondition (docs/cli.md
	// §2.3.1) fired before the document was examined, so there is nothing
	// to keep in either tree — a documentStore still records a row, with
	// no file (docs/config.md §5).
	Precondition bool
	// Ref, Verdict, and Assessment come from the document as submitted; each
	// is "" when the field was absent or not a string.
	Ref        string
	Verdict    string
	Assessment string
	// Comments, Errors, Advisories, and Skipped are the run's counts,
	// always known by the time storing happens.
	Comments   int
	Errors     int
	Advisories int
	Skipped    int
	// ToolVersion and SchemaVersion are recorded on every row.
	ToolVersion   string
	SchemaVersion string
}

// documentStore persists one run per docs/config.md §5: exit 0 keeps the
// review, exit 1 keeps the input unless it is oversized, exit 3 keeps
// neither — the precondition (docs/cli.md §2.3.1) fires before the document
// is examined, so there is nothing yet to keep — and every call records a
// row regardless. A nil error covers both a run that stored something and
// one with nothing to keep — store.enabled:false, most likely — so
// submit-review cannot tell the two apart and does not need to. A non-nil
// error means the store could not be established, written, or recorded to;
// the caller exits ExitTool with nothing on stdout (docs/config.md §5.1).
type documentStore interface {
	Save(ctx context.Context, in StoreInput) error
}

// renderer writes everything a command has to say. There is one implementation
// and one format: two of either is how the same run came to be described two
// different ways, which is the defect this interface no longer permits.
type renderer interface {
	Result(w io.Writer, result *review.Result) error
	Entries(w io.Writer, entries []entry.Entry) error
	Index(w io.Writer, groups []entry.Group) error
	Summary(w io.Writer, text string, groups []entry.Group) error
	// Profiles writes prime --list's profile index (docs/cli.md §2.1.5):
	// names and descriptions, no bodies. profiles is rendered as given, so
	// a non-nil empty slice renders "profiles":[] rather than null - the
	// same empty-directory guarantee internal/profile.Reader.List makes.
	Profiles(w io.Writer, profiles []profile.Profile) error
	// CollectReviews writes collect-reviews's JSON envelope
	// (docs/features/combined-reviews.md §8.1) — the one function that
	// decides what collect-reviews found (§8.3.1). Unlike every other
	// method here, its own value type lives in internal/render rather than
	// a domain package: internal/collect deliberately does not know about
	// repo, store.enabled, or head_check (internal/collect's own package
	// doc), so there is no third, lower package for both this interface
	// and internal/render to share it from the way review.Result serves
	// Result above.
	CollectReviews(w io.Writer, envelope render.CollectReviewsEnvelope) error
}

// entryRegistry resolves lens names to entries.
type entryRegistry interface {
	Resolve(name string) (entry.Entry, error)
	Index() []entry.Group
}

// reviewStore answers docs/config.md §6 queries against store.db. Every
// method only reads: a store that has not been created yet is answered as
// empty rather than being created (docs/config.md §2.2, §6.2), and the
// trees are opened only by ReadContent, for a file a caller asked for
// (docs/config.md §6.3).
type reviewStore interface {
	// RepoName resolves --repo's default the way verification finds a
	// repository, named per docs/config.md §4.2. ok is false outside a
	// repository, where reviews has no default to offer.
	RepoName(ctx context.Context, dir string) (name string, ok bool, err error)
	// Known reports whether the store has any row at all for repo, so a
	// mistyped repository can be told apart from one with nothing recent
	// (docs/config.md §6.2).
	Known(ctx context.Context, repo string) (bool, error)
	// ListReviews returns up to limit passing runs for repo, newest first,
	// and the total matching before limit was applied. ref, when non-empty,
	// restricts the result to one commit; limit of 0 means unlimited.
	ListReviews(ctx context.Context, repo, ref string, limit int) ([]store.Review, int, error)
	// ListFailedRuns is ListReviews' counterpart for runs that stored no
	// review (docs/config.md §6, --failed).
	ListFailedRuns(ctx context.Context, repo, ref string, limit int) ([]store.FailedRun, int, error)
	// ListRepos returns every repository the store has a row for, with its
	// review and failed-run counts, ordered by name (docs/config.md §6,
	// --list).
	ListRepos(ctx context.Context) ([]store.RepoCount, error)
	// ReadContent reads the file at path, as named by a Review's or
	// FailedRun's Path (docs/config.md §6.1, --content).
	ReadContent(path string) ([]byte, error)
	// DistinctDigests returns one row per distinct digest among passing
	// runs for repo and ref, oldest first — the enumeration
	// collect-reviews's own reader loops over
	// (docs/features/combined-reviews.md §5.3.1), mirroring
	// internal/store.Store.DistinctDigests. An empty slice and a nil error
	// answer a repo or ref the store has never heard of, the same
	// answered-as-empty posture every other method here already has.
	DistinctDigests(ctx context.Context, repo, ref string) ([]store.DigestRow, error)
	// ReviewPath resolves one distinct digest to the stored file it names
	// (docs/features/combined-reviews.md notes on refinery-uyb.11: "digest
	// to path is not on the reviewStore interface" until now), so
	// collect-reviews can turn DistinctDigests's rows into ReadContent
	// calls without collect-reviews knowing the store's own directory
	// layout. Mirrors internal/store.Store.ReviewPath's formula exactly;
	// unlike that method it can fail, because computing it here still
	// means resolving the store root from config first.
	ReviewPath(ctx context.Context, repo, ref, digest string) (string, error)
	// StoreEnabled reports store.enabled, read from the same config
	// submit-review's storing does (docs/config.md §3) — collect-reviews's
	// own store.enabled field (docs/features/combined-reviews.md §8.1),
	// reported rather than gated on, since disabling storing never gates
	// reading (§9).
	StoreEnabled(ctx context.Context) (bool, error)
}

// profileSource reads reviewer profiles for prime --profile and prime
// --list (docs/cli.md §2.1.1-§2.1.5). Load's ok separates "no such
// profile" - an unknown or malformed name, routed to ExitUsage without
// enumerating the directory (docs/cli.md §2.1.4) - from a non-nil error,
// which is the tool's own state (an unreadable or malformed profile file,
// or a directory that could not be resolved or read) and routed to
// ExitTool. Neither method is ever called by bare prime: the filesystem
// access either method makes happens only on --profile or --list.
type profileSource interface {
	// Load reads one profile by name.
	Load(name string) (profile.Profile, bool, error)
	// List returns every valid profile in the directory, sorted by name,
	// plus the name of every file that failed to parse (docs/cli.md
	// §2.1.5): --list degrades a broken file rather than failing the whole
	// call, naming it on stderr instead of in the index. A missing
	// directory is a non-nil empty profiles slice, a nil broken slice, and
	// a nil error. A non-nil error means the directory itself could not be
	// resolved or read - the tool's own state, routed to ExitTool - and
	// broken is not populated alongside one.
	List() (profiles []profile.Profile, broken []string, err error)
}

// DivergedAnchor is one entry in collect-reviews's head_check.diverged
// (docs/features/combined-reviews.md §4.3.1): one anchor a stored review
// verified once, at submission, that a headChecker's recheck at collection
// time found has since drifted from ref in the working tree. Comment is the
// qualified id (§6.1), never the origin id the underlying verification pass
// itself reports. File is the anchored path directly, never the JSON
// Pointer verify.Verifier's own Unverified type carries: a pointer only
// ever made sense into the one document submit-review was checking, and
// there is no single document here for one to point into.
type DivergedAnchor struct {
	Name    string
	Comment string
	File    string
	Message string
}

// headChecker resolves docs/features/combined-reviews.md §4.3's head_check
// question once per collect-reviews invocation: which repository, if any,
// answers ref, and whether ref names HEAD there. Both are per-invocation
// facts - every submission shares the one ref this command was asked about
// - so they are asked for exactly once, not once per submission
// (refinery-k3h), unlike the per-anchor divergence check the returned
// HeadCheck answers afterward. It is the one place this package asks
// anything about git at all, and per §4.3.2's routes 2+3, it must not pull
// internal/verify into this package to do it - the concrete implementation
// lives in cmd/loam-refinery, which is already free to import
// internal/verify the way its reviewsAdapter does for verify.Discover.
type headChecker interface {
	// Discover resolves the repository for dir and whether ref names HEAD
	// there, once. The returned HeadCheck answers Diverged for each
	// surviving submission afterward without repeating repository
	// discovery or the HEAD check itself.
	//
	// source is "repo", "none", or "unavailable", the same three values
	// verification.source uses, and is fixed on the returned HeadCheck
	// along with isHead, which is meaningful only when source == "repo".
	Discover(ctx context.Context, dir, ref string) (HeadCheck, error)
}

// HeadCheck is what one collect-reviews invocation's repository discovery
// resolved (§4.3, §4.3.2): Source and IsHead are invocation-level facts,
// fixed once headChecker.Discover returns; Diverged is the one part of the
// check that remains genuinely per-submission, since two submissions can
// carry different anchors even though they share Source and IsHead
// (refinery-k3h). Exported because cmd/loam-refinery's concrete
// implementation must name this type in its own method signature to
// satisfy headChecker above.
type HeadCheck interface {
	// Source reports "repo", "none", or "unavailable" (see headChecker).
	Source() string
	// IsHead reports whether ref, as resolved by Discover, names HEAD.
	// Meaningful only when Source() == "repo".
	IsHead() bool
	// Diverged re-runs anchor-worktree-diverged against every anchor doc
	// carries, fresh, reusing the repository state Discover already
	// resolved. qualifiedIDs maps doc's own origin comment ids to the
	// qualified id the combined output assigned them - only the
	// collect-assemble bead's code knows that mapping, so it is supplied
	// rather than re-derived here.
	//
	// diverged is non-nil - an empty slice when nothing has drifted - only
	// when IsHead() is also true, and nil otherwise: the check does not
	// apply to a non-HEAD ref, or outside a repository, so there is
	// nothing to report, and a caller must be able to tell that apart from
	// "checked, found nothing" (§4.3.1).
	Diverged(ctx context.Context, doc *review.Document, qualifiedIDs map[string]string) (diverged []DivergedAnchor, err error)
}
