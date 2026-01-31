package server

import (
	_ "github.com/armon/go-socks5"
	_ "github.com/hashicorp/yamux"
)

// Package server implements the RSK server components including
// the main server, registry, SOCKS5 manager, and connection handler.
