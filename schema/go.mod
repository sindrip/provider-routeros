module github.com/sindrip/provider-routeros/schema

// Deliberately not on rest/'s 1.27 release candidate: nothing here needs
// encoding/json/v2, and staying on the released toolchain keeps this module
// lintable.
go 1.26.5
