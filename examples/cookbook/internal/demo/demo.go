// Package demo holds helpers shared by the cookbook examples. It exists so the
// examples can print section headers in the same order as the log records they
// describe; production code does not need any of this.
package demo

import (
	"fmt"
	"time"

	"github.com/architagr/lognugget/v4/config"
)

// Immediate configures the collector to write every record as soon as it
// arrives, instead of batching up to 20 records or waiting for the 1 s ticker.
//
// This is a demo convenience, not a recommendation: batching is what keeps the
// caller off the IO path. See 06-tuning for how to choose real values.
func Immediate() {
	config.SetLogBufferMaxSize(1)
	config.SetRate(10 * time.Millisecond)
}

// Section prints a header after everything logged so far has been written, so
// the console output reads top to bottom.
func Section(title string) {
	Flush()
	fmt.Printf("\n── %s ──\n", title)
}

// Flush waits for the dispatcher to hand every published record to the
// collector, then gives the collector's flush goroutine a moment to write.
func Flush() {
	config.FlushDispatch(2 * time.Second)
	time.Sleep(20 * time.Millisecond)
}
