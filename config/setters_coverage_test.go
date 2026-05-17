//go:build testing

package config

import (
	"bytes"
	"testing"
	"time"

	pipelineStage "github.com/architagr/lognugget/pipeline_stage"
)

// Test_SetOutput verifies SetOutput replaces the output writer and
// Output() returns the new value.
func Test_SetOutput(t *testing.T) {
	t.Cleanup(func() { TestResetConfig() })

	var buf bytes.Buffer
	SetOutput(&buf)
	if got := GetConfig().Output(); got != &buf {
		t.Errorf("Output() = %v; want buf", got)
	}
}

// Test_SetLogBufferMaxSize verifies the setter and accessor round-trip.
func Test_SetLogBufferMaxSize(t *testing.T) {
	t.Cleanup(func() { TestResetConfig() })

	SetLogBufferMaxSize(50)
	if got := GetConfig().LogBuffer(); got != 50 {
		t.Errorf("LogBuffer() = %d; want 50", got)
	}
	// Zero/negative → defaults to 20.
	SetLogBufferMaxSize(0)
	if got := GetConfig().LogBuffer(); got != 20 {
		t.Errorf("LogBuffer() after 0 = %d; want 20 (default)", got)
	}
}

// Test_SetRate verifies the setter and Rate() accessor round-trip.
func Test_SetRate(t *testing.T) {
	t.Cleanup(func() { TestResetConfig() })

	SetRate(500 * time.Millisecond)
	if got := GetConfig().Rate(); got != 500*time.Millisecond {
		t.Errorf("Rate() = %v; want 500ms", got)
	}
}

// Test_RegisterAndDeRegisterHook verifies hooks can be registered and removed.
func Test_RegisterAndDeRegisterHook(t *testing.T) {
	t.Cleanup(func() { TestResetConfig() })

	proc := pipelineStage.NewUnsetLogEventPostProcessor(time.Second, 10, &bytes.Buffer{})
	RegisterHook(0, proc) // LevelUnSet = 0

	// DeRegister must not panic.
	DeRegisterHook(0, proc.Name())

	// DeRegister unknown hook must be no-op.
	DeRegisterHook(0, "nonexistent")
}

// Test_AddAndRemovePreProcessors verifies the add/remove pre-processor API.
func Test_AddAndRemovePreProcessors(t *testing.T) {
	t.Cleanup(func() { TestResetConfig() })

	obs := pipelineStage.EventPreProcessorObj
	AddPreProcessors(obs)
	if _, ok := EventPreProcessors[obs.Name()]; !ok {
		t.Error("AddPreProcessors: observer not registered")
	}

	RemovePreProcessor(obs.Name())
	if _, ok := EventPreProcessors[obs.Name()]; ok {
		t.Error("RemovePreProcessor: observer still present")
	}
}
