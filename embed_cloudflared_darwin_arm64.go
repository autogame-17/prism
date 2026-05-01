//go:build darwin && arm64

package main

import (
	_ "embed"

	"prism/backend/tunnel"
)

//go:embed resources/cloudflared/darwin-arm64/cloudflared
var embeddedCloudflared []byte

func init() { tunnel.SetEmbeddedBinary(embeddedCloudflared, "cloudflared") }
