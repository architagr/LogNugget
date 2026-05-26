//go:build testing

// Tests for V4-P5 chain methods: Err and Any.
package entry_test

import (
	"context"
	"errors"
	"testing"

	"github.com/architagr/lognugget/config"
	"github.com/architagr/lognugget/entry"
	"github.com/architagr/lognugget/enum"
	"github.com/architagr/lognugget/test/support"
)

func resetAndSpyForP5(t *testing.T, name string) *support.FakePreProc {
	t.Helper()
	config.TestResetConfig()
	support.NewConfigBuilder(t).
		MinLevel(enum.LevelDebug).
		Encoder(enum.EncoderJSON).
		Build()
	spy := support.NewFakePreProc("p5-spy-" + name)
	config.InitPreProcessors(spy)
	return spy
}

func Test_LogEntry_Err(t *testing.T) {
	t.Run("Err_NonNil_AppendsErrorField", func(t *testing.T) {
		spy := resetAndSpyForP5(t, "err-nonnnil")

		entry.NewLogEntry().Err(errors.New("something went wrong")).Info(context.Background(), "err-msg")

		payload := drainSpy(t, spy)
		m := parseRaw(t, payload)

		if rawStr(t, m, "error") != "something went wrong" {
			t.Errorf("error field want %q got %q; payload: %s", "something went wrong", rawStr(t, m, "error"), payload)
		}
	})

	t.Run("Err_Nil_NoField", func(t *testing.T) {
		spy := resetAndSpyForP5(t, "err-nil")

		entry.NewLogEntry().Err(nil).Info(context.Background(), "no-err-msg")

		payload := drainSpy(t, spy)
		m := parseRaw(t, payload)

		if _, ok := m["error"]; ok {
			t.Errorf("error field must be absent when err is nil; payload: %s", payload)
		}
	})

	t.Run("Err_ChainWithStr", func(t *testing.T) {
		spy := resetAndSpyForP5(t, "err-chain")

		entry.NewLogEntry().
			Str("op", "delete").
			Err(errors.New("not found")).
			Info(context.Background(), "chain-err")

		payload := drainSpy(t, spy)
		m := parseRaw(t, payload)

		if rawStr(t, m, "op") != "delete" {
			t.Errorf("op want delete; payload: %s", payload)
		}
		if rawStr(t, m, "error") != "not found" {
			t.Errorf("error want 'not found'; payload: %s", payload)
		}
	})
}

func Test_LogEntry_Any(t *testing.T) {
	t.Run("Any_StringValue", func(t *testing.T) {
		spy := resetAndSpyForP5(t, "any-str")

		entry.NewLogEntry().Any("tag", "production").Info(context.Background(), "any-msg")

		payload := drainSpy(t, spy)
		m := parseRaw(t, payload)

		if rawStr(t, m, "tag") != "production" {
			t.Errorf("tag want production; payload: %s", payload)
		}
	})

	t.Run("Any_IntValue", func(t *testing.T) {
		spy := resetAndSpyForP5(t, "any-int")

		entry.NewLogEntry().Any("count", 42).Info(context.Background(), "any-int-msg")

		payload := drainSpy(t, spy)
		m := parseRaw(t, payload)

		if rawNum(t, m, "count") != "42" {
			t.Errorf("count want 42; payload: %s", payload)
		}
	})

	t.Run("Any_ChainWithTyped", func(t *testing.T) {
		spy := resetAndSpyForP5(t, "any-chain")

		entry.NewLogEntry().
			Str("host", "srv-1").
			Any("meta", "extra").
			Int("port", 8080).
			Info(context.Background(), "any-chain-msg")

		payload := drainSpy(t, spy)
		m := parseRaw(t, payload)

		if rawStr(t, m, "host") != "srv-1" {
			t.Errorf("host want srv-1; payload: %s", payload)
		}
		if rawStr(t, m, "meta") != "extra" {
			t.Errorf("meta want extra; payload: %s", payload)
		}
		if rawNum(t, m, "port") != "8080" {
			t.Errorf("port want 8080; payload: %s", payload)
		}
	})
}
