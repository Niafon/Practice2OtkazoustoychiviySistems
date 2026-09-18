package web

import "embed"

// Files contains the client application served by every identical backend node.
//
//go:embed index.html styles.css app.js
var Files embed.FS
