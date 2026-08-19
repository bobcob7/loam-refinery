package schema

import (
	"bytes"
	"encoding/json"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestMinimalStripsAnnotationsAndStillValidates(t *testing.T) {
	t.Parallel()
	minimal, err := Minimal()
	require.NoError(t, err)
	for _, key := range annotationKeys {
		assert.NotContains(t, string(minimal), `"`+key+`"`, "minimal schema still carries %s", key)
	}
	assert.Contains(t, string(minimal), `"additionalProperties": false`)
	assert.Less(t, len(minimal), len(Annotated())/2, "minimal should be a fraction of the annotated source")
	var decoded any
	require.NoError(t, json.Unmarshal(minimal, &decoded))
}

func TestEveryObjectRejectsUnknownFields(t *testing.T) {
	t.Parallel()
	var document map[string]any
	require.NoError(t, json.Unmarshal(Annotated(), &document))
	objects := 0
	var walk func(node any)
	walk = func(node any) {
		object, ok := node.(map[string]any)
		if !ok {
			return
		}
		if object["type"] == "object" {
			objects++
			assert.Equal(t, false, object["additionalProperties"], "an object schema allows unknown fields")
		}
		for _, key := range []string{"properties", "$defs"} {
			if group, ok := object[key].(map[string]any); ok {
				for _, child := range group {
					walk(child)
				}
			}
		}
		if items, ok := object["items"]; ok {
			walk(items)
		}
	}
	walk(document)
	assert.Equal(t, 4, objects, "root, comment, anchor and suggestion")
}

func TestVersionIsTheFormatVersion(t *testing.T) {
	t.Parallel()
	assert.Equal(t, "1", Version())
}

func TestValidateReportsEveryLeafFailure(t *testing.T) {
	t.Parallel()
	validator, err := NewValidator()
	require.NoError(t, err)
	tests := []struct {
		name    string
		source  string
		path    string
		message string
		field   string
	}{
		{
			name:    "an unknown field names the nearest real one",
			source:  `{"file":"a.go","end-line":3}`,
			path:    "/comments/0/anchors/0",
			message: `unknown field "end-line" — did you mean "end_line"?`,
			field:   "end_line",
		},
		{
			name:    "a priority above the band is named with its maximum",
			source:  `{"id":"a-1","priority":12,"category":"style","body":"` + strings.Repeat("b", 30) + `","anchors":[],"suggestions":[]}`,
			path:    "/comments/0/priority",
			message: "12 is greater than the maximum of 10",
			field:   "priority",
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			instance := document(t, test.source, test.path)
			failures := validator.Validate(instance)
			found := false
			for _, failure := range failures {
				if failure.Path == test.path && failure.Message == test.message {
					found = true
					assert.Equal(t, test.field, failure.Field)
				}
			}
			assert.True(t, found, "wanted %q at %q, got %+v", test.message, test.path, failures)
		})
	}
}

func TestValidateAcceptsTheExampleDocument(t *testing.T) {
	t.Parallel()
	validator, err := NewValidator()
	require.NoError(t, err)
	instance := decode(t, valid)
	assert.Empty(t, validator.Validate(instance))
}

func TestNearestFieldStaysWithinEditDistanceTwo(t *testing.T) {
	t.Parallel()
	allowed := []string{"file", "ref", "line", "end_line"}
	nearest, ok := nearestField("end-line", allowed)
	assert.True(t, ok)
	assert.Equal(t, "end_line", nearest)
	_, ok = nearestField("completely-different", allowed)
	assert.False(t, ok)
}

// document splices a fragment into an otherwise valid review at the given path,
// so one test case exercises exactly one failure.
func document(t *testing.T, fragment, path string) any {
	t.Helper()
	var value any
	require.NoError(t, json.Unmarshal([]byte(fragment), &value))
	root := decode(t, valid).(map[string]any)
	comments := root["comments"].([]any)
	comment := comments[0].(map[string]any)
	switch {
	case strings.Contains(path, "/anchors/"):
		comment["anchors"] = []any{value}
	default:
		comments[0] = value
	}
	return root
}

func decode(t *testing.T, source string) any {
	t.Helper()
	decoder := json.NewDecoder(bytes.NewReader([]byte(source)))
	decoder.UseNumber()
	var value any
	require.NoError(t, decoder.Decode(&value))
	return value
}

const valid = `{
  "version": "1",
  "verdict": "request_changes",
  "ref": "4f2c1a9e8b3d7c5a1f0e2d4b6a8c9e1f3a5b7c9d",
  "summary": "The retry loop is sound, but the context deadline is not propagated downstream.",
  "comments": [
    {
      "id": "dropped-context-1",
      "priority": 9,
      "category": "correctness",
      "body": "The retry loop calls c.do(context.Background(), req) rather than the caller's ctx.",
      "anchors": [{"file": "internal/fetch/client.go", "line": 88, "end_line": 94}],
      "suggestions": [
        {"summary": "Pass the caller's context through", "effort": "trivial", "scope": "line",
         "pros": ["Deadlines propagate"], "cons": ["Behaviour change"]}
      ]
    }
  ]
}`
