// Package nethttp is a dzcouriers.Transport over net/http, for regular Go programs.
package nethttp

import (
	"bytes"
	"io"
	"net/http"
	"time"

	"github.com/ZaouiAmine/dzcouriers"
)

// Transport implements dzcouriers.Transport.
type Transport struct {
	Client *http.Client
}

// New returns a Transport with a 30 second timeout.
func New() *Transport { return &Transport{Client: &http.Client{Timeout: 30 * time.Second}} }

// Do performs the request.
func (t *Transport) Do(r dzcouriers.Request) (dzcouriers.Response, error) {
	req, err := http.NewRequest(r.Method, r.URL, bytes.NewReader(r.Body))
	if err != nil {
		return dzcouriers.Response{}, err
	}
	for k, v := range r.Headers {
		req.Header.Set(k, v)
	}
	c := t.Client
	if c == nil {
		c = http.DefaultClient
	}
	res, err := c.Do(req)
	if err != nil {
		return dzcouriers.Response{}, err
	}
	defer res.Body.Close()
	b, err := io.ReadAll(io.LimitReader(res.Body, 20<<20))
	return dzcouriers.Response{Status: res.StatusCode, Body: b}, err
}
