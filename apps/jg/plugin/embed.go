// Package plugin exposes the zsh and bash plugin scripts as Go strings
// embedded at compile time. internal/shell uses these to emit init output
// without duplicating the script content in Go source — the .zsh / .bash
// files in this directory are the single source of truth.
package plugin

import _ "embed"

//go:embed jg.plugin.zsh
var Zsh string

//go:embed jg.plugin.bash
var Bash string
