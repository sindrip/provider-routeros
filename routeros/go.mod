// The generated typed view of RouterOS menus, and the conversions it calls.
//
// Its own module, stdlib only, and deliberately no dependency on rest: a
// caller that wants types should not be made to take a transport, and rest is
// pinned to a Go release candidate that nothing about a struct field needs.
// rest.Record is map[string]string underneath, so the two compose by passing
// one straight in.
//
// Separate from gen for the same reason in the other direction: gen imports
// schema to read the IR, and a consumer of the output has no business
// resolving that.
module github.com/sindrip/provider-routeros/routeros

go 1.26.5
