package transport

import "net/http"

// HTTPDoer abstracts HTTP request execution for testing.
type HTTPDoer interface {
	Do(req *http.Request) (*http.Response, error)
}
