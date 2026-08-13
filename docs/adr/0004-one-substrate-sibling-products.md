# One substrate, sibling products

The repo ships several products — crossplane provider, OTel collector,
maybe terraform — off one substrate: the REST client, harness, probes, IR,
generator. The substrate is the root module (`github.com/sindrip/routeros`,
the client is the root package); each published product is a sibling
module; generator emissions land in the product module. Modules split by
dependency cone, not taste: crossplane-runtime, the collector stack, and
terraform-plugin-framework never meet in one go.mod. The substrate imports
nothing downward. The repo name lags the module path; rename before the
first release anyone fetches by import path.

Nothing derived is committed. Observations and generated code are
re-derived on demand from the CHR image pinned in `lab/compose.yaml` — the
derivation is deterministic given that pin, so committing outputs would be
caching, a cost decision deferred until the cost exists. Consequence: CI
runs the pipeline, router included.

Tooling is Go-native: go 1.27rc2 via `GOTOOLCHAIN=auto`, tools pinned as
`tool` directives in a dedicated tools module — their requires would
otherwise be the substrate's, and the workspace exposes them to plain
`go tool` anyway — go-task (never make) as the verb menu with no logic in
it, golangci-lint near defaults. `lab/compose.yaml` is the lab's
single topology definition — humans and programs boot the same file. Unit
tests only at day zero; the first router-touching test lands in a
dedicated e2e module, keeping `go test ./...` docker-free everywhere.
