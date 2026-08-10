//go:generate mdatagen metadata.yaml

// Package routerosreceiver scrapes a MikroTik RouterOS device over its REST
// API and emits OTLP metrics.
//
// The transport and every value conversion come from the sibling rest and
// routeros modules, so this package holds only the mapping from menus to
// metrics. That is the whole reason it lives in this repo: the previous
// version, in the deployment repo, carried its own RouterOS client — the
// third copy in these codebases, and the one with no tests.
//
// It supersedes homelab/images/routeros-collector, which is still the one
// deployed. Retire that one when this ships; until then the deployed
// behaviour is the old code's.
package routerosreceiver
