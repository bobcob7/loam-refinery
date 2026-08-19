//go:build tools

// Package tools pins the versions of the Go tools this project generates with.
// It is never built into the binary; the tag keeps it out of every other build.
package tools

import (
	_ "github.com/matryer/moq"
	_ "mvdan.cc/gofumpt"
)
