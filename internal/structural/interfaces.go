package structural

import "github.com/bobcob7/loam-refinery/internal/schema"

// schemaValidator checks a decoded document against the embedded JSON Schema.
type schemaValidator interface {
	Validate(instance any) []schema.Failure
}
