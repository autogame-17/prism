//go:build windows && amd64

package main

import (
	_ "embed"

	"prism/backend/tunnel"
)

//go:embed resources/cloudflared/windows-amd64/cloudflared.exe
var embeddedCloudflared []byte

func init() { tunnel.SetEmbeddedBinary(embeddedCloudflared, "cloudflared.exe") }
