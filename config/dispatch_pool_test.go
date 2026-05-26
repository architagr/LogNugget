package config_test

import (
	"testing"

	"github.com/architagr/lognugget/config"
)

func TestGetDispatchBuf_LenZero(t *testing.T) {
	b := config.GetDispatchBuf()
	if len(b) != 0 {
		t.Errorf("GetDispatchBuf len=%d, want 0", len(b))
	}
}

func TestGetDispatchBuf_MinCap(t *testing.T) {
	b := config.GetDispatchBuf()
	if cap(b) < config.DefaultDispatchBufCap {
		t.Errorf("GetDispatchBuf cap=%d, want >= %d", cap(b), config.DefaultDispatchBufCap)
	}
}

func TestReturnDispatchBuf_OversizedNotRetained(t *testing.T) {
	// Put an oversized buffer — should be silently dropped, not pooled.
	big := make([]byte, 0, config.MaxDispatchBufRetainCap+1)
	config.ReturnDispatchBuf(big) // must not panic

	// Drain any pooled buf and verify it has cap in [DefaultDispatchBufCap, Max].
	got := config.GetDispatchBuf()
	if cap(got) > config.MaxDispatchBufRetainCap {
		t.Errorf("oversized buf leaked into pool: cap=%d", cap(got))
	}
}

func TestReturnDispatchBuf_RecycledOnNextGet(t *testing.T) {
	b := config.GetDispatchBuf()
	b = append(b, "test data"...)
	ptr := &b[0]

	config.ReturnDispatchBuf(b)

	got := config.GetDispatchBuf()
	if len(got) != 0 {
		t.Errorf("recycled buf len=%d, want 0", len(got))
	}
	// Pool may or may not return the exact same backing — only check len=0.
	_ = ptr
}
