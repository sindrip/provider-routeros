module github.com/sindrip/provider-routeros/gen

go 1.26.5

require github.com/sindrip/provider-routeros/schema v0.0.0

// Spike debt: a published module must not carry a replace. It is here because
// schema is a sibling module in this repo and nothing is tagged yet.
replace github.com/sindrip/provider-routeros/schema => ../schema
