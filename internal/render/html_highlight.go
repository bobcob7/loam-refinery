// internal/render/html_highlight.go — chroma's token API, used directly
// rather than chroma's own HTML formatter (§6.1: wrapping formatters/html's
// output in template.HTML would trust caller-influenced source text as
// markup, exactly the hole §4 exists to close). lexers.Match(anchor file),
// falling back to lexers.Fallback (§6.3), tokenizes a code excerpt; a
// fixed switch maps each chroma.TokenType to one of the six §6.2 CSS
// classes, using InSubCategory — never InCategory, which cannot tell
// LiteralString and LiteralNumber apart (§6.2's named trap) — so a
// numeric literal is never silently painted with the string color.
// Only the derived CSS class reaches an HTML attribute; t.Value, the
// source text itself, goes through the template as ordinary escaped text
// content, subject to the identical contextual autoescaping every other
// field on this page already gets.
//
// Bead .4 owns this file's content.
package render
