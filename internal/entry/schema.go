package entry

import (
	"encoding/json"
	"fmt"
	"sort"
	"strings"
)

// SchemaProvider contributes field:* entries, read from the annotated schema so
// explanation cannot drift from enforcement.
type SchemaProvider struct {
	document map[string]any
}

// NewSchemaProvider reads the annotated schema.
func NewSchemaProvider(annotated []byte) (*SchemaProvider, error) {
	var document map[string]any
	if err := json.Unmarshal(annotated, &document); err != nil {
		return nil, fmt.Errorf("decoding annotated schema: %w", err)
	}
	return &SchemaProvider{document: document}, nil
}

// Name identifies the provider.
func (p *SchemaProvider) Name() string {
	return "schema"
}

type objectNode struct {
	prefix string
	schema map[string]any
}

// Entries walks the schema breadth-first and names every field by its JSON
// path — comments.suggestions.effort, not effort — because field names are not
// unique on their own and picking a winner silently is how a caller ends up
// reading about the wrong field. A bare final segment still resolves when only
// one field ends with it; the registry decides that, not this walk.
func (p *SchemaProvider) Entries() ([]Entry, error) {
	entries := []Entry{}
	queue := []objectNode{{schema: p.document}}
	for len(queue) > 0 {
		node := queue[0]
		queue = queue[1:]
		properties, ok := node.schema["properties"].(map[string]any)
		if !ok {
			continue
		}
		for _, name := range sortedKeys(properties) {
			property, ok := properties[name].(map[string]any)
			if !ok {
				continue
			}
			path := name
			if node.prefix != "" {
				path = node.prefix + "." + name
			}
			if child, ok := p.arrayItems(property); ok {
				queue = append(queue, objectNode{prefix: path, schema: child})
			}
			entries = append(entries, Entry{
				Name:      path,
				Namespace: NamespaceField,
				Title:     text(property["title"]),
				Body:      text(property["description"]),
				Related:   listValue(property, "related"),
				Example:   example(property),
			})
		}
	}
	sort.Slice(entries, func(i, j int) bool { return entries[i].Name < entries[j].Name })
	return entries, nil
}

// arrayItems returns the object schema an array property holds, resolving a $ref.
func (p *SchemaProvider) arrayItems(property map[string]any) (map[string]any, bool) {
	items, ok := property["items"].(map[string]any)
	if !ok {
		return nil, false
	}
	return p.resolve(items), true
}

func (p *SchemaProvider) resolve(node map[string]any) map[string]any {
	ref, ok := node["$ref"].(string)
	if !ok || !strings.HasPrefix(ref, "#/") {
		return node
	}
	current := any(p.document)
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

func listValue(property map[string]any, key string) []string {
	value, found := comments(property)[key]
	if !found {
		return nil
	}
	parts := strings.Split(value, ",")
	names := make([]string, 0, len(parts))
	for _, part := range parts {
		if trimmed := strings.TrimSpace(part); trimmed != "" {
			names = append(names, trimmed)
		}
	}
	return names
}

// comments parses the "key: value; key: value" mini-format carried in $comment,
// which the minimal schema strips.
func comments(property map[string]any) map[string]string {
	parsed := map[string]string{}
	for _, part := range strings.Split(text(property["$comment"]), ";") {
		key, value, found := strings.Cut(part, ":")
		if !found {
			continue
		}
		parsed[strings.TrimSpace(key)] = strings.TrimSpace(value)
	}
	return parsed
}

func example(property map[string]any) string {
	examples, ok := property["examples"].([]any)
	if !ok || len(examples) == 0 {
		return ""
	}
	encoded, err := json.Marshal(examples[0])
	if err != nil {
		return ""
	}
	return string(encoded)
}

func text(value any) string {
	s, _ := value.(string)
	return s
}

func sortedKeys(object map[string]any) []string {
	keys := make([]string, 0, len(object))
	for key := range object {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	return keys
}
