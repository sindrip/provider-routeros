module telemetry

go 1.27rc2

require (
	github.com/sindrip/provider-routeros/gen v0.0.0
	github.com/sindrip/provider-routeros/rest v0.0.0
)

require github.com/sindrip/provider-routeros/schema v0.0.0 // indirect

replace github.com/sindrip/provider-routeros/rest => ../../rest

replace github.com/sindrip/provider-routeros/gen => ../../gen

replace github.com/sindrip/provider-routeros/schema => ../../schema
