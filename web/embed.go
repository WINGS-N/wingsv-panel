// Package web embeds the built Vue SPA so the panel ships as a single binary.
//
// The directive captures the entire dist/ subtree (built by `pnpm build`).
// Files starting with "." (like .well-known/assetlinks.json) require the
// "all:" prefix to be included.
package web

import "embed"

//go:embed all:dist
var Dist embed.FS
