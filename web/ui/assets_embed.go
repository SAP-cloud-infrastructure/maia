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

// Assets exposes the embedded static files as an http.FileSystem.
// The "static" prefix is stripped so that /mantine-ui/index.html is
// accessible at /mantine-ui/index.html (not /static/mantine-ui/index.html).
var Assets http.FileSystem = func() http.FileSystem {
	sub, err := fs.Sub(EmbedFS, "static")
	if err != nil {
		panic(err)
	}
	return http.FS(sub)
}()
