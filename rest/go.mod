module github.com/sindrip/provider-routeros/rest

// Pinned to the 1.27 release candidate for encoding/json/v2, whose strict
// decoding this package depends on, and httptest.NewTestServer's in-memory
// network.
//
// Known cost, accepted deliberately: golangci-lint v2.12.2 is built with Go
// 1.26 and refuses a module targeting 1.27, so this module is currently
// unlinted. There is no fixed release to move to — one follows 1.27 final.
// The root module is unaffected: it stays on 1.26.5, and CI reads its go.mod.
// Revisit both when 1.27 ships.
go 1.27rc2
