package structural

import (
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"testing"

	"github.com/bobcob7/loam-refinery/internal/review"
	"github.com/bobcob7/loam-refinery/internal/schema"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestChecksReportEveryStructuralFailure(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name     string
		file     string
		want     []string
		messages []string
	}{
		{
			name: "a well-formed document raises nothing",
			file: "valid.json",
		},
		{
			name:     "a misspelled field is named with its correction",
			file:     "schema-unknown-field.json",
			want:     []string{"schema"},
			messages: []string{`unknown field "end-line" — did you mean "end_line"?`},
		},
		{
			name:     "two comments sharing an id",
			file:     "id-duplicate.json",
			want:     []string{"id-unique"},
			messages: []string{"declared by comments[0] and comments[1]"},
		},
		{
			name:     "a span that runs backwards and an end_line with no line",
			file:     "anchor-range-backwards.json",
			want:     []string{"anchor-range-ordered", "anchor-range-ordered"},
			messages: []string{"end_line 88 is before line 94", "end_line 12 without line"},
		},
		{
			name:     "paths that escape the repository",
			file:     "anchor-path-escape.json",
			want:     []string{"anchor-path-safe", "anchor-path-safe"},
			messages: []string{`file "../other/client.go" escapes the repository`, `file "/home/me/repo/client_test.go" is absolute; anchors are repository-relative`},
		},
		{
			name:     "a branch name is not a ref",
			file:     "ref-branch.json",
			want:     []string{"ref-format"},
			messages: []string{`ref "main" is not a 40-character lowercase commit SHA`},
		},
		{
			name:     "a profile containing a colon",
			file:     "profile-colon.json",
			want:     []string{"profile-format"},
			messages: []string{`profile "arch:v2" does not match ^[a-z0-9]+(-[a-z0-9]+)*$`},
		},
		{
			name: "a profile matching the grammar raises nothing",
			file: "profile-valid.json",
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			diagnostics := check(t, test.file)
			if len(test.want) == 0 {
				assert.Empty(t, diagnostics, "a well-formed document raises nothing at all")
			}
			assert.Equal(t, test.want, namesOf(diagnostics, test.want))
			for _, message := range test.messages {
				assert.Contains(t, messagesOf(diagnostics), message)
			}
			for _, diagnostic := range diagnostics {
				assert.Equal(t, review.SeverityError, diagnostic.Severity, "structural checks are hard")
			}
		})
	}
}

func TestSchemaFailuresDoNotStopOtherChecks(t *testing.T) {
	t.Parallel()
	diagnostics := check(t, "ill-typed.json")
	names := map[string]bool{}
	for _, diagnostic := range diagnostics {
		names[diagnostic.Name] = true
	}
	assert.True(t, names["schema"], "the ill-typed priority fails the schema")
	assert.NotEmpty(t, diagnostics, "checking continues past a schema failure")
}

func TestOneMistakeCostsOneDiagnostic(t *testing.T) {
	t.Parallel()
	diagnostics := check(t, "ref-branch.json")
	for _, diagnostic := range diagnostics {
		assert.NotEqual(t, "schema", diagnostic.Name,
			"a ref the pattern and ref-format both reject is reported once, by the check that explains it")
	}
}

func TestValidSHAAcceptsOnlyFullLowercaseHex(t *testing.T) {
	t.Parallel()
	assert.True(t, ValidSHA("4f2c1a9e8b3d7c5a1f0e2d4b6a8c9e1f3a5b7c9d"))
	assert.False(t, ValidSHA("4F2C1A9E8B3D7C5A1F0E2D4B6A8C9E1F3A5B7C9D"))
	assert.False(t, ValidSHA("4f2c1a9"))
	assert.False(t, ValidSHA("main"))
}

func check(t *testing.T, file string) []review.Diagnostic {
	t.Helper()
	source, err := os.ReadFile(filepath.Join("testdata", file))
	require.NoError(t, err)
	doc, err := review.Parse(source)
	require.NoError(t, err)
	validator, err := schema.NewValidator()
	require.NoError(t, err)
	log := slog.New(slog.NewJSONHandler(io.Discard, nil))
	return New(validator, log).Check(doc)
}

// namesOf keeps only the names the case is about, so an unrelated diagnostic
// elsewhere in the fixture does not make the assertion unreadable.
func namesOf(diagnostics []review.Diagnostic, wanted []string) []string {
	interesting := map[string]bool{}
	for _, name := range wanted {
		interesting[name] = true
	}
	names := []string{}
	for _, diagnostic := range diagnostics {
		if interesting[diagnostic.Name] {
			names = append(names, diagnostic.Name)
		}
	}
	if len(names) == 0 {
		return nil
	}
	return names
}

func messagesOf(diagnostics []review.Diagnostic) []string {
	messages := make([]string, 0, len(diagnostics))
	for _, diagnostic := range diagnostics {
		messages = append(messages, diagnostic.Message)
	}
	return messages
}
