// Package web embeds the SSR shell templates. Static assets (/web/static) are
// served from disk and are deliberately NOT embedded, so that JS/CSS edits are
// visible on page reload without rebuilding the binary.
//
// This package exists because //go:embed cannot reference paths outside its own
// package directory: an embed directive in internal/http could not reach
// /web/templates. It is one of exactly two public packages in this module (the
// other is app) and must stay free of logic — only the FS.
package web

import "embed"

//go:embed templates/*.html
var TemplatesFS embed.FS
