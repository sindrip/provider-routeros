module keyprobe

go 1.27rc2

require (
	github.com/sindrip/provider-routeros/rest v0.0.0
	github.com/sindrip/provider-routeros/schema v0.0.0
)

replace github.com/sindrip/provider-routeros/rest => ../../rest

replace github.com/sindrip/provider-routeros/schema => ../../schema
