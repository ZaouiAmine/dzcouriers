// Package dzcouriers talks to Algerian delivery companies (Yalidine and its clones, the EcoTrack
// platform, ZR Express, NOEST, Procolis and Maystro) through one small API.
//
// The package does no networking of its own: callers supply a Transport. That keeps it usable
// from TinyGo/WASI runtimes where net/http is unavailable. Plain Go programs can use
// adapters/nethttp.
package dzcouriers

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/url"
	"strings"
	"time"
)

// Request is one HTTP call a driver wants made.
type Request struct {
	Method  string
	URL     string
	Headers map[string]string
	Body    []byte
}

// Response is what the Transport got back. Status is 0 when the transport cannot tell
// (some serverless HTTP clients hide it); drivers then judge the body alone.
type Response struct {
	Status int
	Body   []byte
}

// Transport performs HTTP requests.
type Transport interface {
	Do(Request) (Response, error)
}

// Account is one merchant account at one provider.
type Account struct {
	Provider    string            // catalog key, e.g. "yalidine", "packers", "zr", "noest"
	Credentials map[string]string // field names come from Provider.Fields
	BaseURL     string            // optional override of the catalog base URL
}

// Delivery is where the parcel ends up.
type Delivery string

const (
	Home Delivery = "home"
	Desk Delivery = "desk"
)

// Product is one line of the shipment.
type Product struct {
	Name     string
	Quantity int
}

// Shipment is the provider-neutral parcel to create.
type Shipment struct {
	Reference    string // merchant order id
	FirstName    string
	LastName     string
	Phone        string
	Phone2       string
	WilayaCode   int    // 1..69
	Wilaya       string // Latin name, used by providers that match on names
	Commune      string // Latin name
	CommuneID    string // provider commune id, when the provider needs one (Maystro)
	Address      string
	Delivery     Delivery
	DeskID       string // provider desk / station / hub id when Delivery == Desk
	FromWilaya   int    // sender wilaya (Yalidine)
	Products     []Product
	COD          int64 // amount to collect, in dinars
	DeclaredDA   int64 // declared value, in dinars (0 = COD)
	WeightKg     float64
	Exchange     bool
	CanOpen      bool
	FreeShipping bool // merchant pays the delivery fee
	Note         string
}

// FullName joins first and last name.
func (s Shipment) FullName() string {
	return strings.TrimSpace(s.FirstName + " " + s.LastName)
}

// ProductText is a one-line description of the products.
func (s Shipment) ProductText() string {
	parts := make([]string, 0, len(s.Products))
	for _, p := range s.Products {
		if p.Name == "" {
			continue
		}
		if p.Quantity > 1 {
			parts = append(parts, fmt.Sprintf("%s x%d", p.Name, p.Quantity))
		} else {
			parts = append(parts, p.Name)
		}
	}
	if len(parts) == 0 {
		return "Produit"
	}
	return strings.Join(parts, ", ")
}

// Parcel is a created shipment.
type Parcel struct {
	Tracking   string
	ProviderID string // provider-side id when it differs from the tracking (ZR, Maystro)
	LabelURL   string
}

// Event is one tracking step.
type Event struct {
	Status Status
	Raw    string
	At     time.Time
	Where  string
}

// Tracking is the history of one parcel, oldest event first.
type Tracking struct {
	Tracking string
	Status   Status
	Raw      string
	Events   []Event
}

// DeskInfo is a pickup point.
type DeskInfo struct {
	ID         string
	Name       string
	WilayaCode int
	Commune    string
	Address    string
}

// Errors returned by drivers. ProviderError wraps anything else a provider said.
var (
	ErrUnsupported = errors.New("dzcouriers: operation not supported by this provider")
	ErrAuth        = errors.New("dzcouriers: credentials rejected")
	ErrNotFound    = errors.New("dzcouriers: parcel not found")
	ErrBusy        = errors.New("dzcouriers: rate limited")
	ErrUnknown     = errors.New("dzcouriers: unknown provider")
)

// InvalidError is a missing or rejected field.
type InvalidError struct{ Field, Reason string }

func (e *InvalidError) Error() string { return "dzcouriers: invalid " + e.Field + ": " + e.Reason }

// ProviderError carries a provider's own message.
type ProviderError struct {
	Provider string
	Status   int
	Message  string
}

func (e *ProviderError) Error() string {
	return fmt.Sprintf("dzcouriers: %s: %s", e.Provider, e.Message)
}

// Client runs operations against any provider in the catalog.
type Client struct {
	Transport Transport
	Now       func() time.Time
}

type driver interface {
	create(c *call, s Shipment) (Parcel, error)
	track(c *call, trackings []string) ([]Tracking, error)
	label(c *call, tracking string) ([]byte, string, error) // pdf bytes or url
	desks(c *call, wilaya int) ([]DeskInfo, error)
	check(c *call) error
	cancel(c *call, tracking string) error
}

// call binds a driver to one account.
type call struct {
	cl   *Client
	acc  Account
	prov Provider
	base string
}

func (cl *Client) bind(acc Account) (*call, driver, error) {
	p, ok := Lookup(acc.Provider)
	if !ok {
		return nil, nil, ErrUnknown
	}
	for _, f := range p.Fields {
		if strings.TrimSpace(acc.Credentials[f]) == "" {
			return nil, nil, &InvalidError{Field: f, Reason: "required"}
		}
	}
	base := strings.TrimRight(acc.BaseURL, "/")
	if base == "" {
		base = p.BaseURL
	}
	d := drivers[p.Family]
	if d == nil {
		return nil, nil, ErrUnsupported
	}
	return &call{cl: cl, acc: acc, prov: p, base: base}, d, nil
}

// Create sends a new parcel to the provider.
func (cl *Client) Create(acc Account, s Shipment) (Parcel, error) {
	c, d, err := cl.bind(acc)
	if err != nil {
		return Parcel{}, err
	}
	if err := validate(s); err != nil {
		return Parcel{}, err
	}
	return d.create(c, s)
}

// Track returns the history of each tracking number the provider knows.
func (cl *Client) Track(acc Account, trackings ...string) ([]Tracking, error) {
	c, d, err := cl.bind(acc)
	if err != nil {
		return nil, err
	}
	if len(trackings) == 0 {
		return nil, nil
	}
	return d.track(c, trackings)
}

// Label returns the label as PDF bytes, or a URL when the provider only hands out links.
func (cl *Client) Label(acc Account, tracking string) (pdf []byte, link string, err error) {
	c, d, err := cl.bind(acc)
	if err != nil {
		return nil, "", err
	}
	return d.label(c, tracking)
}

// Desks lists pickup points, optionally for one wilaya (0 = all).
func (cl *Client) Desks(acc Account, wilaya int) ([]DeskInfo, error) {
	c, d, err := cl.bind(acc)
	if err != nil {
		return nil, err
	}
	return d.desks(c, wilaya)
}

// Check verifies the credentials with a read-only call.
func (cl *Client) Check(acc Account) error {
	c, d, err := cl.bind(acc)
	if err != nil {
		return err
	}
	return d.check(c)
}

// Cancel deletes a parcel that has not been picked up yet.
func (cl *Client) Cancel(acc Account, tracking string) error {
	c, d, err := cl.bind(acc)
	if err != nil {
		return err
	}
	return d.cancel(c, tracking)
}

func validate(s Shipment) error {
	switch {
	case s.FullName() == "":
		return &InvalidError{Field: "name", Reason: "required"}
	case LocalPhone(s.Phone) == "":
		return &InvalidError{Field: "phone", Reason: "not an Algerian number"}
	case s.WilayaCode < 1 || s.WilayaCode > 69:
		return &InvalidError{Field: "wilaya", Reason: "out of range"}
	case s.Delivery == Home && strings.TrimSpace(s.Commune) == "":
		return &InvalidError{Field: "commune", Reason: "required"}
	case s.Delivery != Home && s.Delivery != Desk:
		return &InvalidError{Field: "delivery", Reason: "home or desk"}
	case s.COD < 0:
		return &InvalidError{Field: "cod", Reason: "negative"}
	}
	return nil
}

// LocalPhone returns the 10-digit 0XXXXXXXXX form, or "" if the number is not Algerian.
func LocalPhone(p string) string {
	var b strings.Builder
	for _, r := range p {
		if r >= '0' && r <= '9' {
			b.WriteRune(r)
		}
	}
	d := b.String()
	switch {
	case strings.HasPrefix(d, "00213"):
		d = "0" + d[5:]
	case strings.HasPrefix(d, "213") && len(d) == 12:
		d = "0" + d[3:]
	case len(d) == 9:
		d = "0" + d
	}
	if len(d) != 10 || d[0] != '0' {
		return ""
	}
	return d
}

// IntlPhone returns +213XXXXXXXXX, or "".
func IntlPhone(p string) string {
	l := LocalPhone(p)
	if l == "" {
		return ""
	}
	return "+213" + l[1:]
}

// ---- HTTP helpers shared by drivers ----

func (c *call) cred(k string) string { return strings.TrimSpace(c.acc.Credentials[k]) }

func (c *call) do(method, path string, q url.Values, headers map[string]string, body any) (Response, error) {
	u := c.base + path
	if len(q) > 0 {
		sep := "?"
		if strings.Contains(u, "?") {
			sep = "&"
		}
		u += sep + q.Encode()
	}
	var raw []byte
	h := map[string]string{"Accept": "application/json"}
	for k, v := range headers {
		h[k] = v
	}
	if body != nil {
		b, err := json.Marshal(body)
		if err != nil {
			return Response{}, err
		}
		raw = b
		h["Content-Type"] = "application/json"
	}
	if c.cl.Transport == nil {
		return Response{}, errors.New("dzcouriers: no transport")
	}
	res, err := c.cl.Transport.Do(Request{Method: method, URL: u, Headers: h, Body: raw})
	if err != nil {
		return res, err
	}
	switch {
	case res.Status == 401 || res.Status == 403:
		return res, ErrAuth
	case res.Status == 429:
		return res, ErrBusy
	case res.Status == 404:
		return res, ErrNotFound
	case res.Status >= 400:
		return res, &ProviderError{Provider: c.prov.Key, Status: res.Status, Message: message(res.Body)}
	}
	return res, nil
}

// message pulls a human message out of an error body.
func message(body []byte) string {
	var m map[string]any
	if json.Unmarshal(body, &m) == nil {
		for _, k := range []string{"message", "error", "detail", "title", "msg"} {
			switch v := m[k].(type) {
			case string:
				if v != "" {
					return v
				}
			case map[string]any:
				if s, ok := v["message"].(string); ok && s != "" {
					return s
				}
			}
		}
		if e, ok := m["errors"]; ok {
			b, _ := json.Marshal(e)
			return string(b)
		}
	}
	s := strings.TrimSpace(string(body))
	if len(s) > 200 {
		s = s[:200]
	}
	if s == "" {
		s = "empty response"
	}
	return s
}

func isPDF(b []byte) bool { return len(b) > 4 && string(b[:4]) == "%PDF" }

// str reads a JSON value as a string whatever its JSON type.
func str(v any) string {
	switch t := v.(type) {
	case string:
		return t
	case float64:
		if t == float64(int64(t)) {
			return fmt.Sprintf("%d", int64(t))
		}
		return fmt.Sprintf("%g", t)
	case bool:
		if t {
			return "1"
		}
		return "0"
	case json.Number:
		return t.String()
	}
	return ""
}

func num(v any) int {
	switch t := v.(type) {
	case float64:
		return int(t)
	case string:
		n := 0
		for _, r := range t {
			if r < '0' || r > '9' {
				return 0
			}
			n = n*10 + int(r-'0')
		}
		return n
	}
	return 0
}

var timeLayouts = []string{
	time.RFC3339Nano, "2006-01-02 15:04:05", "2006-01-02T15:04:05", "2006-01-02T15:04:05.000000Z",
	"02/01/2006 15:04", "02/01/2006 15:04:05", "2006-01-02",
}

func parseTime(s string) time.Time {
	s = strings.TrimSpace(s)
	for _, l := range timeLayouts {
		if t, err := time.Parse(l, s); err == nil {
			return t
		}
	}
	return time.Time{}
}

func finish(t Tracking) Tracking {
	sortEvents(t.Events)
	if n := len(t.Events); n > 0 {
		last := t.Events[n-1]
		t.Status, t.Raw = last.Status, last.Raw
	}
	if t.Status == "" {
		t.Status = Unknown
	}
	return t
}

func sortEvents(ev []Event) {
	for i := 1; i < len(ev); i++ {
		for j := i; j > 0 && ev[j].At.Before(ev[j-1].At); j-- {
			ev[j], ev[j-1] = ev[j-1], ev[j]
		}
	}
}
