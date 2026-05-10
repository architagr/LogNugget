// Package entry provides the LogEntry type, which is the single unit of work
// for producing a structured log event. Callers obtain a *LogEntry from the
// package-level sync.Pool via NewLogEntry, call one of the level methods
// (Debug, Info, Warn, Error, Fatal, Panic), and the entry is automatically
// returned to the pool after the event is published.
//
// This package is responsible for: assembling the ordered list of key-value
// pairs that make up one log line, delegating encoding to the configured
// encoder, and dispatching the encoded bytes through config.PublishLog.
//
// This package is not responsible for: encoder implementation (see package
// encoder), config lifecycle (see package config), or pipeline post-processing
// (see package pipeline_stage).
//
// Key entry points: NewLogEntry, GenerateInitialPool.
package entry
