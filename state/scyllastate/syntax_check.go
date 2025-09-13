//go:build ignore
// +build ignore

package main

import (
	"context"

	"github.com/ipfs-cluster/ipfs-cluster/api"
	"github.com/ipfs-cluster/ipfs-cluster/state/scyllastate"
)

// This file is used to check syntax compilation of the WriteOnly methods
func main() {
	var s *scyllastate.ScyllaState
	var pin api.Pin
	var cid api.Cid
	ctx := context.Background()

	// Check Add method signature
	_ = s.Add(ctx, pin)

	// Check Rm method signature
	_ = s.Rm(ctx, cid)
}
