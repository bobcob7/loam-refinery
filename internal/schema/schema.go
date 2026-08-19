// Package schema embeds the authoritative review schema, compiles it for
// draft 2020-12 validation, and serves both its annotated and minimal forms.
package schema

import (
	"bytes"
	_ "embed"
	"encoding/json"
	"fmt"
	"math"
	"math/big"
	"strconv"
	"strings"

	"github.com/santhosh-tekuri/jsonschema/v6"
	"github.com/santhosh-tekuri/jsonschema/v6/kind"
	"golang.org/x/text/language"
	"golang.org/x/text/message"
)

var printer = message.NewPrinter(language.English)

//go:embed review.schema.json
var annotated []byte

const (
	resourceURL = "https://github.com/bobcob7/refinery/review.schema.json"
	// fieldNamespace is the lens namespace field entries live in, used when a
	// field can only be named unambiguously in qualified form.
	fieldNamespace = "field"
)

// annotationKeys are the keywords stripped from the minimal form. None of them
// affects what the schema accepts.
var annotationKeys = []string{"title", "description", "examples", "$comment"}

// Annotated returns the source schema, descriptions and examples intact.
func Annotated() []byte {
	return annotated
}

// Minimal returns the schema with every annotation stripped. It validates
// identically to the annotated source.
func Minimal() ([]byte, error) {
	var doc any
	if err := json.Unmarshal(annotated, &doc); err != nil {
		return nil, fmt.Errorf("decoding embedded schema: %w", err)
	}
	strip(doc)
	out, err := json.MarshalIndent(doc, "", "  ")
	if err != nil {
		return nil, fmt.Errorf("encoding minimal schema: %w", err)
	}
	return append(out, '\n'), nil
}

// Version is the format version this schema enforces.
func Version() string {
	doc := decodeAnnotated()
	properties, _ := doc["properties"].(map[string]any)
	version, _ := properties["version"].(map[string]any)
	value, _ := version["const"].(string)
	return value
}

func decodeAnnotated() map[string]any {
	var doc map[string]any
	if err := json.Unmarshal(annotated, &doc); err != nil {
		panic("embedded schema is not valid JSON: " + err.Error())
	}
	return doc
}

func strip(node any) {
	object, ok := node.(map[string]any)
	if !ok {
		return
	}
	for _, key := range annotationKeys {
		delete(object, key)
	}
	for _, key := range []string{"properties", "$defs"} {
		if group, ok := object[key].(map[string]any); ok {
			for _, child := range group {
				strip(child)
			}
		}
	}
	for _, key := range []string{"items", "if", "then", "else", "not", "additionalProperties"} {
		if child, ok := object[key]; ok {
			strip(child)
		}
	}
	for _, key := range []string{"allOf", "anyOf", "oneOf"} {
		if group, ok := object[key].([]any); ok {
			for _, child := range group {
				strip(child)
			}
		}
	}
}

// Failure is one schema conformance failure.
type Failure struct {
	// Path is a JSON Pointer into the instance.
	Path string
	// Message is a one-line rendering, already carrying a did-you-mean when
	// an unknown property is close to a real field.
	Message string
	// Field is the lens name for the field the failure is about: the field's
	// final segment where that is unambiguous, its full JSON path where it is
	// not. Empty when no field can be named.
	Field string
}

// Validator checks documents against the embedded schema.
type Validator struct {
	compiled *jsonschema.Schema
	document map[string]any
	// unique reports whether exactly one field path ends in a segment, which
	// is what decides whether a lens name can be written short.
	unique map[string]bool
}

// NewValidator compiles the embedded schema.
func NewValidator() (*Validator, error) {
	resource, err := jsonschema.UnmarshalJSON(bytes.NewReader(annotated))
	if err != nil {
		return nil, fmt.Errorf("decoding embedded schema: %w", err)
	}
	compiler := jsonschema.NewCompiler()
	if err := compiler.AddResource(resourceURL, resource); err != nil {
		return nil, fmt.Errorf("adding embedded schema: %w", err)
	}
	compiled, err := compiler.Compile(resourceURL)
	if err != nil {
		return nil, fmt.Errorf("compiling embedded schema: %w", err)
	}
	document := decodeAnnotated()
	return &Validator{compiled: compiled, document: document, unique: uniqueSegments(document)}, nil
}

// Validate returns one failure per leaf violation, in instance order.
func (v *Validator) Validate(instance any) []Failure {
	err := v.compiled.Validate(instance)
	if err == nil {
		return nil
	}
	validationErr, ok := err.(*jsonschema.ValidationError)
	if !ok {
		return []Failure{{Message: err.Error()}}
	}
	failures := []Failure{}
	seen := map[string]bool{}
	v.collect(validationErr, &failures, seen)
	return failures
}

func (v *Validator) collect(err *jsonschema.ValidationError, failures *[]Failure, seen map[string]bool) {
	if len(err.Causes) > 0 {
		for _, cause := range err.Causes {
			v.collect(cause, failures, seen)
		}
		return
	}
	if _, ok := err.ErrorKind.(*kind.Schema); ok {
		return
	}
	failure := v.failure(err)
	key := failure.Path + "\x00" + failure.Message
	if seen[key] {
		return
	}
	seen[key] = true
	*failures = append(*failures, failure)
}

func (v *Validator) failure(err *jsonschema.ValidationError) Failure {
	location := err.InstanceLocation
	path := pointer(location)
	message, field := v.describe(err.ErrorKind, location)
	if field == "" {
		field = fieldPath(location)
	}
	return Failure{Path: path, Message: message, Field: v.lens(field)}
}

// lens names a field the shortest way that still resolves: its final segment
// when nothing else in the schema ends the same way, its full path when the
// path itself distinguishes it, and the namespaced form for a root field whose
// name a nested field also ends in — "summary", which comments.suggestions
// also has.
func (v *Validator) lens(field string) string {
	if field == "" {
		return ""
	}
	segment := field
	dotted := false
	if i := strings.LastIndex(field, "."); i >= 0 {
		segment, dotted = field[i+1:], true
	}
	switch {
	case v.unique[segment]:
		return segment
	case dotted:
		return field
	default:
		return fieldNamespace + ":" + field
	}
}

func (v *Validator) describe(k jsonschema.ErrorKind, location []string) (message, field string) {
	switch e := k.(type) {
	case *kind.Type:
		return fmt.Sprintf("expected %s, got %s", strings.Join(e.Want, " or "), e.Got), ""
	case *kind.Enum:
		return "must be one of " + strings.Join(displayAll(e.Want), ", "), ""
	case *kind.Const:
		return "must be " + display(e.Want), ""
	case *kind.Required:
		return "missing required field " + quoteAll(e.Missing), missingField(e.Missing, location)
	case *kind.AdditionalProperties:
		return v.unknownProperties(e.Properties, location)
	case *kind.Minimum:
		return fmt.Sprintf("%s is less than the minimum of %s", rat(e.Got), rat(e.Want)), ""
	case *kind.Maximum:
		return fmt.Sprintf("%s is greater than the maximum of %s", rat(e.Got), rat(e.Want)), ""
	case *kind.MinLength:
		return fmt.Sprintf("is %d characters; needs at least %d", e.Got, e.Want), ""
	case *kind.MaxLength:
		return fmt.Sprintf("is %d characters; the maximum is %d", e.Got, e.Want), ""
	case *kind.MinItems:
		return fmt.Sprintf("has %d items; needs at least %d", e.Got, e.Want), ""
	case *kind.Pattern:
		return fmt.Sprintf("%q does not match %s", e.Got, e.Want), ""
	default:
		return k.LocalizedString(printer), ""
	}
}

func (v *Validator) unknownProperties(properties []string, location []string) (message, field string) {
	allowed := v.allowedAt(location)
	parts := make([]string, 0, len(properties))
	for _, property := range properties {
		part := fmt.Sprintf("unknown field %q", property)
		if nearest, ok := nearestField(property, allowed); ok {
			part += fmt.Sprintf(" — did you mean %q?", nearest)
			if field == "" {
				field = join(fieldPath(location), nearest)
			}
		}
		parts = append(parts, part)
	}
	return strings.Join(parts, "; "), field
}

// allowedAt walks the schema alongside the instance location and returns the
// property names valid at that point.
func (v *Validator) allowedAt(location []string) []string {
	node := v.document
	for _, token := range location {
		node = resolve(v.document, node)
		if _, err := strconv.Atoi(token); err == nil {
			items, ok := node["items"].(map[string]any)
			if !ok {
				return nil
			}
			node = items
			continue
		}
		properties, ok := node["properties"].(map[string]any)
		if !ok {
			return nil
		}
		child, ok := properties[token].(map[string]any)
		if !ok {
			return nil
		}
		node = child
	}
	node = resolve(v.document, node)
	properties, ok := node["properties"].(map[string]any)
	if !ok {
		return nil
	}
	names := make([]string, 0, len(properties))
	for name := range properties {
		names = append(names, name)
	}
	return names
}

func resolve(root, node map[string]any) map[string]any {
	ref, ok := node["$ref"].(string)
	if !ok || !strings.HasPrefix(ref, "#/") {
		return node
	}
	current := any(root)
	for _, token := range strings.Split(strings.TrimPrefix(ref, "#/"), "/") {
		object, ok := current.(map[string]any)
		if !ok {
			return node
		}
		current = object[token]
	}
	resolved, ok := current.(map[string]any)
	if !ok {
		return node
	}
	return resolved
}

func pointer(location []string) string {
	if len(location) == 0 {
		return ""
	}
	escaped := make([]string, 0, len(location))
	for _, token := range location {
		token = strings.ReplaceAll(token, "~", "~0")
		escaped = append(escaped, strings.ReplaceAll(token, "/", "~1"))
	}
	return "/" + strings.Join(escaped, "/")
}

// fieldPath is an instance location with its array indices dropped, which is
// exactly how the entry registry names a field: comments.suggestions.cons.
func fieldPath(location []string) string {
	segments := make([]string, 0, len(location))
	for _, token := range location {
		if _, err := strconv.Atoi(token); err == nil {
			continue
		}
		segments = append(segments, token)
	}
	return strings.Join(segments, ".")
}

func join(prefix, name string) string {
	if prefix == "" {
		return name
	}
	return prefix + "." + name
}

func missingField(missing []string, location []string) string {
	if len(missing) == 1 {
		return join(fieldPath(location), missing[0])
	}
	return ""
}

// uniqueSegments counts how many field paths end in each segment, so a lens
// name can be shortened only where the short form means one thing.
func uniqueSegments(document map[string]any) map[string]bool {
	counts := map[string]int{}
	var walk func(node map[string]any)
	walk = func(node map[string]any) {
		properties, ok := node["properties"].(map[string]any)
		if !ok {
			return
		}
		for name, child := range properties {
			counts[name]++
			object, ok := child.(map[string]any)
			if !ok {
				continue
			}
			items, ok := object["items"].(map[string]any)
			if !ok {
				continue
			}
			walk(resolve(document, items))
		}
	}
	walk(document)
	unique := map[string]bool{}
	for name, count := range counts {
		unique[name] = count == 1
	}
	return unique
}

// nearestField returns the closest valid field within edit distance 2.
func nearestField(property string, allowed []string) (string, bool) {
	best, bestDistance := "", 3
	for _, candidate := range allowed {
		d := distance(property, candidate)
		if d < bestDistance || (d == bestDistance && candidate < best) {
			best, bestDistance = candidate, d
		}
	}
	if bestDistance > 2 {
		return "", false
	}
	return best, true
}

func distance(a, b string) int {
	previous := make([]int, len(b)+1)
	current := make([]int, len(b)+1)
	for j := range previous {
		previous[j] = j
	}
	for i := 1; i <= len(a); i++ {
		current[0] = i
		for j := 1; j <= len(b); j++ {
			cost := 1
			if a[i-1] == b[j-1] {
				cost = 0
			}
			current[j] = min(previous[j]+1, current[j-1]+1, previous[j-1]+cost)
		}
		previous, current = current, previous
	}
	return previous[len(b)]
}

// ratBitCap bounds the numbers spelled out in full, at roughly 154 decimal
// digits. Past that a value says everything it has to say through its
// magnitude, and spelling it out lets a nine-byte field bury the report:
// "priority": 1e100000 rendered as 100,035 digits of a single number.
const ratBitCap = 512

func rat(value *big.Rat) string {
	if value.Num().BitLen() > ratBitCap || value.Denom().BitLen() > ratBitCap {
		return magnitude(value)
	}
	if value.IsInt() {
		return value.Num().String()
	}
	float, _ := value.Float64()
	return strconv.FormatFloat(float, 'f', -1, 64)
}

// magnitude renders a number too large to spell out, to six significant
// digits. It never converts the value to decimal, so the cost does not grow
// with the number: a document cannot buy unbounded work with one short field.
func magnitude(value *big.Rat) string {
	log := log10(value.Num()) - log10(value.Denom())
	exponent := math.Floor(log)
	// Round to six digits before normalising, not after: rounding is itself
	// what lifts a 9.999999 mantissa to 10, and the exponent has to follow.
	mantissa, _ := strconv.ParseFloat(strconv.FormatFloat(math.Pow(10, log-exponent), 'g', 6, 64), 64)
	if mantissa >= 10 {
		mantissa, exponent = mantissa/10, exponent+1
	}
	if mantissa < 1 {
		mantissa, exponent = mantissa*10, exponent-1
	}
	sign := ""
	if value.Sign() < 0 {
		sign = "-"
	}
	return fmt.Sprintf("%s%se%+d", sign, strconv.FormatFloat(mantissa, 'g', 6, 64), int(exponent))
}

// log10 approximates the base-ten logarithm of |value|. The top 64 bits fix
// the mantissa and the bit length fixes the exponent, so the result is exact
// to far more digits than the six magnitude prints.
func log10(value *big.Int) float64 {
	bits := value.BitLen()
	if bits == 0 {
		return 0
	}
	lead := new(big.Int).Abs(value)
	if bits > 64 {
		lead.Rsh(lead, uint(bits-64))
	}
	return (float64(bits-lead.BitLen()) + math.Log2(float64(lead.Uint64()))) * math.Log10(2)
}

func display(value any) string {
	switch v := value.(type) {
	case string:
		return strconv.Quote(v)
	default:
		return fmt.Sprint(v)
	}
}

func displayAll(values []any) []string {
	out := make([]string, 0, len(values))
	for _, value := range values {
		if s, ok := value.(string); ok {
			out = append(out, s)
			continue
		}
		out = append(out, fmt.Sprint(value))
	}
	return out
}

func quoteAll(values []string) string {
	out := make([]string, 0, len(values))
	for _, value := range values {
		out = append(out, strconv.Quote(value))
	}
	return strings.Join(out, ", ")
}
