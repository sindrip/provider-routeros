package rest

import (
	"cmp"
	"encoding/json/v2"
	"errors"
	"fmt"
	"io"
	"net/http"
)

// ErrAddressRejected reports that the router accepted the connection and then
// closed it without answering. RouterOS does this when the source address is
// not permitted for the www/www-ssl service: the handshake completes, so this
// is neither a refusal nor a status code, and it must not be read as "menu
// absent" or "router down".
var ErrAddressRejected = errors.New("rest: router closed the connection without answering; source address is probably not permitted for the service")

// ErrNotFound reports that a path or row does not exist.
var ErrNotFound = errors.New("rest: not found")

// Error is a failure the router described in the response body.
//
// RouterOS reports errors as a JSON object that decodes exactly like a record,
// so the body alone cannot be trusted to be data. Code is the router's numeric
// "error" field, which is why a response cannot simply be unmarshalled into
// map[string]string: that fails on the number before any check can run.
type Error struct {
	Op      string // "GET /ip/address"
	Status  int    // HTTP status, 0 when the body alone reported the failure
	Code    int    // RouterOS "error"
	Message string // RouterOS "message", e.g. "Bad Request"
	Detail  string // RouterOS "detail", e.g. "no such command or directory"
}

func (e *Error) Error() string {
	// Detail is the useful one ("no such command or directory (routerboard)");
	// Message is the generic status text the router echoes.
	msg := cmp.Or(e.Detail, e.Message, http.StatusText(e.Status))
	return fmt.Sprintf("rest: %s: %s (error %d, http %d)", e.Op, msg, e.Code, e.Status)
}

// Is lets errors.Is(err, ErrNotFound) work for the router's own 404.
func (e *Error) Is(target error) bool {
	return target == ErrNotFound && (e.Status == http.StatusNotFound || e.Code == http.StatusNotFound)
}

// errorBody is the router's error shape. Code is a pointer so that a record
// which happens to carry a string field named "error" is not mistaken for one.
type errorBody struct {
	Code    *int   `json:"error"`
	Message string `json:"message"`
	Detail  string `json:"detail"`
}

// asError reports the failure described by body, or nil when body is data.
//
// Status alone is not sufficient — see ErrAddressRejected for the case with no
// status at all — and neither is the body, since a successful singleton read
// has the same shape. Both are consulted.
func asError(op string, status int, body []byte) error {
	var eb errorBody
	// A non-object body (an array of records) fails to unmarshal here, which
	// is the answer: arrays are never errors.
	if err := json.Unmarshal(body, &eb); err == nil && eb.Code != nil {
		return &Error{Op: op, Status: status, Code: *eb.Code, Message: eb.Message, Detail: eb.Detail}
	}
	if status >= http.StatusBadRequest {
		return &Error{Op: op, Status: status, Message: http.StatusText(status)}
	}
	return nil
}

// transportError translates a failed round-trip. A connection the router
// accepted and then closed gracefully, without an HTTP response, is it
// rejecting the source address.
//
// Only the graceful close counts. That is what RouterOS was observed to do,
// and it arrives as io.EOF. A reset is deliberately *not* included: RST is an
// ordinary network condition — a reboot, a restarted service, a middlebox —
// and reporting those as an address rejection would send the reader to check
// the service's address list while the real fault lay elsewhere. This error is
// only worth raising because it is specific.
func transportError(op string, err error) error {
	if errors.Is(err, io.EOF) || errors.Is(err, io.ErrUnexpectedEOF) {
		return fmt.Errorf("%s: %w", op, ErrAddressRejected)
	}
	return fmt.Errorf("%s: %w", op, err)
}
