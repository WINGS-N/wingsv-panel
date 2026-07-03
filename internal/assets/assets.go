// Package assets embeds small operator-facing files the panel serves verbatim,
// such as the node connector shell script fetched by curl | sh.
package assets

import _ "embed"

//go:embed connect.sh
var ConnectScript []byte
