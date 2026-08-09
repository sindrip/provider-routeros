// Package rest is a RouterOS REST client.
//
// It depends on nothing outside the standard library, and holds no package
// state, so a process may drive many routers concurrently. That is deliberate:
// the provider it grew out of serializes every router operation behind one
// process-global mutex, purely because the Terraform provider it wraps keeps
// the negotiated RouterOS version in a package variable.
//
// The http.Client is supplied by the caller rather than built here. An
// OpenTelemetry receiver already gets TLS, proxying, compression, connection
// pooling and auth extensions from confighttp; this package must not fight
// that.
//
// # What RouterOS does that a REST client would not expect
//
// Each of these is verified against CHR 7.23.2, and each has a test.
//
//   - Errors are a JSON object shaped like a record, so they cannot be
//     recognised by decoding alone. "error" is a JSON *number*, which is why
//     unmarshalling a response into map[string]string fails on a 401 before
//     any error check can run.
//
//   - A request from a source address the service does not permit completes
//     the TCP handshake and is then closed. That surfaces as io.EOF: neither a
//     status code nor a refusal, and emphatically not "menu absent" or "router
//     down". It is reported as ErrAddressRejected.
//
//   - Reaching REST at all needs both the "api" and "rest-api" policies.
//     read,rest-api alone answers 500 "std failure: not allowed (9)" on every
//     endpoint, because REST is a JSON wrapper over the binary API.
//
//   - Creation is PUT. POST is the console-command verb, and a POST to a menu
//     path creates nothing while reporting success.
//
//   - A singleton menu returns a bare object; a list menu returns an array.
//
//   - Settings singletons reject PATCH; they are written with POST <path>/set.
//
//   - Filters travel in the JSON body of POST <path>/print, so no caller value
//     is ever URL-encoded. Where a GET filter is used instead, values need
//     RFC 3986 escaping — see escapeQuery, and note that this is a Go stdlib
//     footgun rather than a RouterOS quirk.
//
//   - POST <path>/print with .proplist omits .id, while GET ?.proplist=
//     includes it. Props always requests .id back, because a caller that
//     cannot address the row it just read cannot update it.
//
//   - Every value is a string: {"cpu-load":"0"}, {"running":"true"},
//     {"rx-byte":"5577"}, {"uptime":"1m20s"}. Package scalar parses them, and
//     Record's accessors add the presence semantics that booleans need.
//
//   - Continuous commands are refused; monitor needs an "once" argument. POST
//     commands are capped at 60s router-side, which the caller's http.Client
//     timeout should account for.
//
// # Decoding
//
// Replies are decoded with encoding/json/v2 at its defaults, which reject both
// duplicate object names and invalid UTF-8. A malformed reply is an error
// rather than a silently repaired value: neither encoder can return the
// original bytes, and substituting U+FFFD into a comment — which is durable
// identity on this device — turns a visible failure into a row that quietly
// stops matching.
//
// # Inspecting failures
//
// A failure the router described in a body arrives as *Error, carrying its
// numeric code and detail:
//
//	if re, ok := errors.AsType[*rest.Error](err); ok && re.Code == 400 {
//	    // re.Detail is the useful half: "no such command or directory (…)"
//	}
//
// The sentinels answer the two cases with no useful body: errors.Is(err,
// rest.ErrNotFound) and errors.Is(err, rest.ErrAddressRejected).
package rest
