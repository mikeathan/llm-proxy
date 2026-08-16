// Package templates embeds the shipped default task templates so a fresh
// install has a template library without relying on the source tree at runtime
// (XDG relocation, Phase 7).
package templates

import "embed"

//go:embed *.md
var FS embed.FS
