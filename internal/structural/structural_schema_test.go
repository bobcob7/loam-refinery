package structural

import (
	"encoding/json"
	"testing"

	"github.com/bobcob7/loam-refinery/internal/schema"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestProfilePatternMatchesThePublishedSchema pins review.schema.json's own
// properties.profile.pattern against profilePattern, the regex profileFormat
// enforces (refinery-xlp.6). Deleting the schema's pattern line leaves this
// tool's own suite green: profileFormat runs the identical regex regardless
// of what the schema says, and Check's covered-dedup discards the schema
// failure at /profile whenever it would otherwise fire — an equivalent
// mutant from inside submit-review. But `loam-refinery schema` publishes
// that pattern for external validators, with nothing else pinning it, so a
// consumer generating code or validating documents against the published
// schema would silently lose the constraint the moment the two drifted.
//
// This reads schema.Annotated() — the same embedded bytes `loam-refinery
// schema` serves — rather than a literal retyped here, so it is the two
// artifacts staying in sync that keeps this green, not one copied from the
// other once and left to rot.
func TestProfilePatternMatchesThePublishedSchema(t *testing.T) {
	t.Parallel()
	var doc struct {
		Properties struct {
			Profile struct {
				Pattern string `json:"pattern"`
			} `json:"profile"`
		} `json:"properties"`
	}
	require.NoError(t, json.Unmarshal(schema.Annotated(), &doc), "decoding the embedded schema")
	require.NotEmpty(t, doc.Properties.Profile.Pattern, "review.schema.json's properties.profile has no pattern at all")
	assert.Equal(t, profilePattern.String(), doc.Properties.Profile.Pattern, "the published schema's profile pattern must match the one profileFormat enforces")
}
