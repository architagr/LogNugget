// Command quickstart is the smallest possible LogNugget program: import the
// facade, log, shut down.
//
// Run: go run ./01-quickstart
package main

import (
	"context"

	"github.com/architagr/lognugget/entry"
	"github.com/architagr/lognugget/lognugget"
)

func main() {
	// Importing lognugget is the whole setup. Its init builds the pipeline:
	// a collector that batches records and writes them to stdout.
	//
	// Shutdown drains both asynchronous stages — the dispatch ring and the
	// collector's buffer — so nothing is lost when main returns. Without it a
	// short-lived program can exit before its logs are written.
	defer lognugget.Shutdown()

	ctx := context.Background()

	entry.NewLogEntry().Info(ctx, "server started")
	entry.NewLogEntry().Warn(ctx, "cache warm-up skipped")
}
