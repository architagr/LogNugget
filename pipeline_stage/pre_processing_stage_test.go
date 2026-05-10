//go:build testing

// Package pipelineStage tests for the event pre-processor observer.
// Build tag "testing" required because tests import test/support doubles.
package pipelineStage

import (
	"testing"

	"github.com/architagr/lognugget/enum"
	"github.com/architagr/lognugget/test/support"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// hookSet converts a slice of FakePostProcessor records into a set of
// string payloads for order-independent equality assertions (SC8).
func hookSet(got [][]byte) map[string]struct{} {
	s := make(map[string]struct{}, len(got))
	for _, b := range got {
		s[string(b)] = struct{}{}
	}
	return s
}

// Test_PreProcess_UnsetGetsAll verifies that a hook registered at
// LevelUnSet receives events published for every concrete level.
func Test_PreProcess_UnsetGetsAll(t *testing.T) {
	obj := newEventPreProcessingObserver()
	hook := support.NewFakePostProcessor("unset-catch-all")
	obj.RegisterHook(enum.LevelUnSet, hook)

	levels := []enum.LogLevel{
		enum.LevelDebug, enum.LevelInfo, enum.LevelWarn, enum.LevelError, enum.LevelFatal,
	}
	payloads := [][]byte{
		[]byte("debug-msg"), []byte("info-msg"), []byte("warn-msg"),
		[]byte("error-msg"), []byte("fatal-msg"),
	}

	for i, lvl := range levels {
		obj.PreProcess(lvl, payloads[i])
	}

	got := hookSet(hook.Got())
	for _, p := range payloads {
		assert.Contains(t, got, string(p),
			"LevelUnSet hook must receive payload for every level")
	}
}

// Test_PreProcess_LevelOnly verifies that a hook registered at
// LevelDebug only receives Debug-level events and not events for other
// levels.
func Test_PreProcess_LevelOnly(t *testing.T) {
	obj := newEventPreProcessingObserver()
	debugHook := support.NewFakePostProcessor("debug-only")
	obj.RegisterHook(enum.LevelDebug, debugHook)

	obj.PreProcess(enum.LevelDebug, []byte("debug-payload"))
	obj.PreProcess(enum.LevelError, []byte("error-payload"))
	obj.PreProcess(enum.LevelInfo, []byte("info-payload"))

	got := debugHook.Got()
	require.Len(t, got, 1, "debug hook must receive exactly one event")
	assert.Equal(t, []byte("debug-payload"), got[0])
}

// Test_PreProcess_DeRegister verifies that after DeRegisterHook the
// removed hook receives no further events.
func Test_PreProcess_DeRegister(t *testing.T) {
	obj := newEventPreProcessingObserver()
	hook := support.NewFakePostProcessor("removable")
	obj.RegisterHook(enum.LevelInfo, hook)

	obj.PreProcess(enum.LevelInfo, []byte("before-remove"))
	obj.DeRegisterHook(enum.LevelInfo, hook.Name())
	obj.PreProcess(enum.LevelInfo, []byte("after-remove"))

	got := hookSet(hook.Got())
	assert.Contains(t, got, "before-remove",
		"hook must have received the pre-deregister event")
	assert.NotContains(t, got, "after-remove",
		"deregistered hook must not receive subsequent events")
}

// Test_PreProcess_NameUniqueness verifies that registering a second
// hook with the same Name() silently overwrites the first (documented
// behavior: names are unique keys per level).
func Test_PreProcess_NameUniqueness(t *testing.T) {
	obj := newEventPreProcessingObserver()

	first := support.NewFakePostProcessor("shared-name")
	second := support.NewFakePostProcessor("shared-name")

	obj.RegisterHook(enum.LevelWarn, first)
	obj.RegisterHook(enum.LevelWarn, second) // overwrites first

	obj.PreProcess(enum.LevelWarn, []byte("warn-payload"))

	assert.Empty(t, first.Got(),
		"first hook must be overwritten and receive nothing")
	require.Len(t, second.Got(), 1,
		"second (overwriting) hook must receive the event")
	assert.Equal(t, []byte("warn-payload"), second.Got()[0])
}

// Test_PreProcess_SingletonInit verifies that EventPreProcessorObj is
// not nil after package init (D-3: the (&sync.Once{}).Do pattern
// created a transient Once that never guards the assignment).
func Test_PreProcess_SingletonInit(t *testing.T) {
	require.NotNil(t, EventPreProcessorObj,
		"EventPreProcessorObj must be initialised by init()")
}
