// SPDX-FileCopyrightText: 2025 SAP SE or an SAP affiliate company
// SPDX-License-Identifier: Apache-2.0

//go:build builtinassets

package ui

import (
	"embed"
	"io/fs"
	"net/http"
)

//go:embed static
var EmbedFS embed.FS

// Assets exposes all embedded static files (rooted at static/).
var Assets http.FileSystem = func() http.FileSystem {
	sub, err := fs.Sub(EmbedFS, "static")
	if err != nil {
		panic(err)
	}
	return http.FS(sub)
}()

// MantineUIAssets exposes the mantine-ui subdirectory directly,
// so /assets/foo.js maps to static/mantine-ui/assets/foo.js.
var MantineUIAssets http.FileSystem = func() http.FileSystem {
	sub, err := fs.Sub(EmbedFS, "static/mantine-ui")
	if err != nil {
		panic(err)
	}
	return http.FS(sub)
}()
