package entry

import (
	"testing"

	"github.com/bobcob7/loam-refinery/internal/review"
	"github.com/bobcob7/loam-refinery/internal/schema"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// fakeProvider lets a test build a registry holding exactly what it needs.
type fakeProvider struct {
	name    string
	entries []Entry
}

func (p *fakeProvider) Name() string { return p.name }

func (p *fakeProvider) Entries() ([]Entry, error) { return p.entries, nil }

func TestResolveFindsEntriesByEveryName(t *testing.T) {
	t.Parallel()
	registry, err := NewRegistry(&fakeProvider{name: "test", entries: []Entry{
		{Name: "comments.priority", Namespace: NamespaceField, Title: "Priority"},
		{Name: "id-unique", Namespace: NamespaceCheck, Title: "Duplicate id"},
		{Name: "ids", Namespace: NamespaceTopic, Aliases: []string{"slugs"}, Title: "Ids"},
	}})
	require.NoError(t, err)
	tests := map[string]string{
		"a full field path":         "comments.priority",
		"a qualified path":          "field:comments.priority",
		"a unique trailing segment": "priority",
	}
	for name, lens := range tests {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			resolved, err := registry.Resolve(lens)
			require.NoError(t, err)
			assert.Equal(t, "comments.priority", resolved.Name)
			assert.Equal(t, "test", resolved.Provider)
		})
	}
	t.Run("an alias resolves silently", func(t *testing.T) {
		t.Parallel()
		resolved, err := registry.Resolve("slugs")
		require.NoError(t, err)
		assert.Equal(t, "ids", resolved.Name)
	})
}

func TestResolveNeverGuesses(t *testing.T) {
	t.Parallel()
	registry, err := NewRegistry(&fakeProvider{name: "test", entries: []Entry{
		{Name: "comments.code", Namespace: NamespaceField},
		{Name: "comments.suggestions.code", Namespace: NamespaceField},
		{Name: "comments.priority", Namespace: NamespaceField},
		{Name: "anchors", Namespace: NamespaceTopic},
	}})
	require.NoError(t, err)
	t.Run("an unknown name is an error", func(t *testing.T) {
		t.Parallel()
		_, err := registry.Resolve("nonsense")
		var unknown *UnknownLensError
		require.ErrorAs(t, err, &unknown)
		assert.Equal(t, "nonsense", unknown.Name)
	})
	t.Run("a segment two fields end with lists both in full", func(t *testing.T) {
		t.Parallel()
		_, err := registry.Resolve("code")
		var ambiguous *AmbiguousLensError
		require.ErrorAs(t, err, &ambiguous)
		assert.Equal(t, []string{"comments.code", "comments.suggestions.code"}, ambiguous.Candidates)
	})
	t.Run("a full path is never ambiguous", func(t *testing.T) {
		t.Parallel()
		resolved, err := registry.Resolve("comments.suggestions.code")
		require.NoError(t, err)
		assert.Equal(t, "comments.suggestions.code", resolved.Name)
	})
	t.Run("a qualified name skips resolution entirely", func(t *testing.T) {
		t.Parallel()
		resolved, err := registry.Resolve("topic:anchors")
		require.NoError(t, err)
		assert.Equal(t, NamespaceTopic, resolved.Namespace)
	})
	t.Run("an empty name is unknown", func(t *testing.T) {
		t.Parallel()
		_, err := registry.Resolve("")
		require.Error(t, err)
	})
}

func TestIndexNamesEveryEntryTheShortestWayThatResolves(t *testing.T) {
	t.Parallel()
	registry := real(t)
	groups := registry.Index()
	require.Len(t, groups, 3)
	assert.Equal(t, NamespaceField, groups[0].Namespace)
	assert.Equal(t, NamespaceCheck, groups[1].Namespace)
	assert.Equal(t, NamespaceTopic, groups[2].Namespace)
	assert.Contains(t, groups[0].Names, "priority", "a unique segment is printed short")
	assert.Contains(t, groups[0].Names, "comments.code", "a shared segment is printed in full")
	assert.Contains(t, groups[0].Names, "field:summary", "a root field a nested one shadows needs its namespace")
	assert.Contains(t, groups[1].Names, "comment-flood")
	for _, group := range groups {
		for _, name := range group.Names {
			_, err := registry.Resolve(name)
			assert.NoError(t, err, "the index printed %q, which does not resolve", name)
		}
	}
}

func TestSchemaProviderReadsTheAnnotatedSchema(t *testing.T) {
	t.Parallel()
	registry := real(t)
	t.Run("a field entry carries its description and example", func(t *testing.T) {
		t.Parallel()
		resolved, err := registry.Resolve("priority")
		require.NoError(t, err)
		assert.Equal(t, "comments.priority", resolved.Name)
		assert.Equal(t, "Priority", resolved.Title)
		assert.Contains(t, resolved.Body, "9-10 must fix before merge")
		assert.Equal(t, "9", resolved.Example)
		assert.Contains(t, resolved.Related, "category")
	})
	t.Run("fields sharing a name keep separate entries", func(t *testing.T) {
		t.Parallel()
		comment, err := registry.Resolve("comments.code")
		require.NoError(t, err)
		suggestion, err := registry.Resolve("comments.suggestions.code")
		require.NoError(t, err)
		assert.NotEqual(t, comment.Body, suggestion.Body)
		root, err := registry.Resolve("field:summary")
		require.NoError(t, err)
		nested, err := registry.Resolve("comments.suggestions.summary")
		require.NoError(t, err)
		assert.NotEqual(t, root.Body, nested.Body)
	})
	t.Run("the anchor object carries no ref", func(t *testing.T) {
		t.Parallel()
		_, err := registry.Resolve("comments.anchors.ref")
		require.Error(t, err)
		resolved, err := registry.Resolve("ref")
		require.NoError(t, err)
		assert.Equal(t, "ref", resolved.Name)
	})
}

func TestTopicsProviderParsesItsHeader(t *testing.T) {
	t.Parallel()
	entry := parseTopic("title: Ids\naliases: slugs, comment-ids\nrelated: id\n---\nbody text\n")
	assert.Equal(t, "Ids", entry.Title)
	assert.Equal(t, []string{"slugs", "comment-ids"}, entry.Aliases)
	assert.Equal(t, []string{"id"}, entry.Related)
	assert.Equal(t, "body text", entry.Body)
}

func TestEveryRegisteredCheckIsExplainable(t *testing.T) {
	t.Parallel()
	registry := real(t)
	for _, resolved := range registry.All() {
		assert.NotEmpty(t, resolved.Title, "%s has no title", resolved.Qualified())
		assert.NotEmpty(t, resolved.Body, "%s has no body", resolved.Qualified())
	}
}

// real assembles the registry the binary ships with.
func real(t *testing.T) *Registry {
	t.Helper()
	schemaProvider, err := NewSchemaProvider(schema.Annotated())
	require.NoError(t, err)
	registry, err := NewRegistry(
		schemaProvider,
		NewChecksProvider([]review.Check{
			{Name: "comment-flood", Title: "Too many comments", Body: "body"},
		}),
		NewTopicsProvider(),
	)
	require.NoError(t, err)
	return registry
}
