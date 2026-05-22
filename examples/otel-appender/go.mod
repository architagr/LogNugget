module github.com/architagr/lognugget/examples/otel-appender

go 1.22.0

toolchain go1.24.2

require (
	github.com/architagr/lognugget v0.0.0
	go.opentelemetry.io/otel/trace v1.35.0
)

require go.opentelemetry.io/otel v1.35.0 // indirect

replace github.com/architagr/lognugget => ../..
