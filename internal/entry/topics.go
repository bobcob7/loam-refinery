package entry

import (
	"embed"
	"fmt"
	"io/fs"
	"sort"
	"strings"
)

//go:embed topics/*.md
var topicFiles embed.FS

// TopicsProvider contributes topic:* entries from hand-written files compiled
// into the binary.
type TopicsProvider struct {
	files fs.FS
}

// NewTopicsProvider reads the embedded topic files.
func NewTopicsProvider() *TopicsProvider {
	return &TopicsProvider{files: topicFiles}
}

// Name identifies the provider.
func (p *TopicsProvider) Name() string {
	return "topics"
}

// Entries returns one entry per topic file, named after the file.
func (p *TopicsProvider) Entries() ([]Entry, error) {
	names, err := fs.Glob(p.files, "topics/*.md")
	if err != nil {
		return nil, fmt.Errorf("listing topics: %w", err)
	}
	sort.Strings(names)
	entries := make([]Entry, 0, len(names))
	for _, name := range names {
		content, err := fs.ReadFile(p.files, name)
		if err != nil {
			return nil, fmt.Errorf("reading %s: %w", name, err)
		}
		entry := parseTopic(string(content))
		entry.Name = strings.TrimSuffix(strings.TrimPrefix(name, "topics/"), ".md")
		entry.Namespace = NamespaceTopic
		entries = append(entries, entry)
	}
	return entries, nil
}

// parseTopic reads the header of key: value lines, then the body after "---".
func parseTopic(content string) Entry {
	entry := Entry{}
	header, body, found := strings.Cut(content, "\n---\n")
	if !found {
		entry.Body = strings.TrimSpace(content)
		return entry
	}
	entry.Body = strings.TrimSpace(body)
	for _, line := range strings.Split(header, "\n") {
		key, value, found := strings.Cut(line, ":")
		if !found {
			continue
		}
		switch strings.TrimSpace(key) {
		case "title":
			entry.Title = strings.TrimSpace(value)
		case "aliases":
			entry.Aliases = splitList(value)
		case "related":
			entry.Related = splitList(value)
		}
	}
	return entry
}

func splitList(value string) []string {
	items := []string{}
	for _, part := range strings.Split(value, ",") {
		if trimmed := strings.TrimSpace(part); trimmed != "" {
			items = append(items, trimmed)
		}
	}
	return items
}
