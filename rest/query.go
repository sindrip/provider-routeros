package rest

import (
	"net/url"
	"slices"
	"strings"
)

// QueryOpt narrows a listing. Options are applied to a POST <path>/print body,
// so no caller value is URL-encoded.
type QueryOpt func(*query)

type query struct {
	filters []string
	props   []string
	count   bool
}

func (q *query) empty() bool { return len(q.filters) == 0 && len(q.props) == 0 && !q.count }

func newQuery(opts []QueryOpt) *query {
	q := &query{}
	for _, opt := range opts {
		opt(q)
	}
	return q
}

// Where keeps only rows whose field equals value.
//
// The value is placed in the request body verbatim. Comments containing
// spaces, ampersands and equals signs round-trip unharmed, which is the reason
// listings prefer POST over a GET query string.
func Where(field, value string) QueryOpt {
	return func(q *query) { q.filters = append(q.filters, field+"="+value) }
}

// WhereRaw adds a .query term the caller has already composed, for the
// operators Where does not model.
func WhereRaw(term string) QueryOpt {
	return func(q *query) { q.filters = append(q.filters, term) }
}

// Props limits the reply to the named fields.
//
// .id is always requested as well. POST <path>/print omits it unless it is
// asked for by name — unlike GET ?.proplist=, which includes it regardless —
// and a caller that cannot address the row it just read cannot then update it.
func Props(names ...string) QueryOpt {
	return func(q *query) {
		q.props = append(q.props, names...)
	}
}

// body renders the query as the JSON object POST <path>/print expects.
func (q *query) body() map[string]any {
	b := map[string]any{}
	if len(q.filters) > 0 {
		b[".query"] = q.filters
	}
	if len(q.props) > 0 {
		b[".proplist"] = withID(q.props)
	}
	if q.count {
		// The router answers {"ret":"0"} — a string, not a number.
		b["count-only"] = ""
	}
	return b
}

func withID(props []string) []string {
	if slices.Contains(props, IDField) {
		return props
	}
	return append([]string{IDField}, props...)
}

// escapeQuery percent-encodes a value for a GET query string, per RFC 3986.
//
// Listings use POST bodies and never reach this; it exists for the callers
// that build a GET filter by hand. The router is not doing anything unusual —
// "+" meaning space is an application/x-www-form-urlencoded convention rather
// than RFC 3986, and RouterOS simply decodes per the RFC. Go's
// url.QueryEscape emits the form convention, which is golang/go#4013.
//
// The single substitution is complete, not a patch aimed at spaces: escape()
// in net/url has exactly one non-%XX output path in query mode (' ' to '+'),
// and shouldEscape leaves only the RFC 3986 unreserved set unescaped. So the
// output alphabet is {unreserved} + {%XX} + {'+' iff the input held a space},
// and removing the last of those yields strict RFC 3986. url.PathEscape is not
// a substitute: it passes "=" and "&" through, which would split a filter.
func escapeQuery(v string) string {
	return strings.ReplaceAll(url.QueryEscape(v), "+", "%20")
}
