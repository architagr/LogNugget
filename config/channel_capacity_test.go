//go:build testing

package config_test

import (
	"bytes"
	"log"
	"testing"

	"github.com/architagr/lognugget/config"
	"github.com/stretchr/testify/assert"
)

func resetCapacity(t *testing.T) {
	t.Helper()
	config.TestResetChannelCapacity()
	config.TestResetConfig()
}

func Test_DafaultLogBuffer_Value(t *testing.T) {
	assert.Equal(t, 1000, config.DafaultLogBuffer, "DafaultLogBuffer must be 1000")
}

func Test_SetChannelCapacity_DefaultIs1000(t *testing.T) {
	resetCapacity(t)
	assert.Equal(t, 1000, config.GetChannelCapacity())
}

func Test_SetChannelCapacity_ValidValue(t *testing.T) {
	resetCapacity(t)
	config.SetChannelCapacity(5000)
	assert.Equal(t, 5000, config.GetChannelCapacity())
	t.Cleanup(config.TestResetChannelCapacity)
}

func Test_SetChannelCapacity_ZeroClamped(t *testing.T) {
	resetCapacity(t)
	var buf bytes.Buffer
	log.SetOutput(&buf)
	t.Cleanup(func() { log.SetOutput(nil) })

	config.SetChannelCapacity(0)
	assert.Equal(t, 1000, config.GetChannelCapacity(), "zero must clamp to 1000")
	assert.Contains(t, buf.String(), "SetChannelCapacity")
}

func Test_SetChannelCapacity_NegativeClamped(t *testing.T) {
	resetCapacity(t)
	var buf bytes.Buffer
	log.SetOutput(&buf)
	t.Cleanup(func() { log.SetOutput(nil) })

	config.SetChannelCapacity(-5)
	assert.Equal(t, 1000, config.GetChannelCapacity(), "negative must clamp to 1000")
	assert.Contains(t, buf.String(), "SetChannelCapacity")
}

func Test_SetChannelCapacity_OverMaxClamped(t *testing.T) {
	resetCapacity(t)
	var buf bytes.Buffer
	log.SetOutput(&buf)
	t.Cleanup(func() { log.SetOutput(nil) })

	config.SetChannelCapacity(200_000)
	assert.Equal(t, 100_000, config.GetChannelCapacity(), "over-max must clamp to 100_000")
	assert.Contains(t, buf.String(), "SetChannelCapacity")
}
