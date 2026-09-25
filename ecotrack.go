package dzcouriers

import (
	"encoding/json"
	"fmt"
	"net/url"
	"strconv"
	"strings"
)

// EcoTrack: one API shared by ~80 tenants at https://<tenant>.ecotrack.dz/api/v1. Bearer token.
// Single-order endpoints take query parameters, not JSON. Created orders must be validated
// (valid/order) before the courier sees them; Create does both. 50 requests/minute.
type ecotrack struct{}

func (ecotrack) headers(c *call) map[string]string {
	return map[string]string{"Authorization": "Bearer " + c.cred("api_token")}
}

// ecoResult decodes {success, message, tracking} and the 200-with-error shape.
type ecoResult struct {
	Success  any    `json:"success"`
	Message  string `json:"message"`
	Tracking string `json:"tracking"`
	Error    any    `json:"error"`
	Errors   any    `json:"errors"`
}

func (r ecoResult) ok() bool {
	switch v := r.Success.(type) {
	case bool:
		return v
	case float64:
		return v == 1
	case string:
		return v == "true" || v == "1"
	}
	return false
}

func (e ecotrack) create(c *call, s Shipment) (Parcel, error) {
	q := url.Values{}
	q.Set("reference", s.Reference)
	q.Set("nom_client", s.FullName())
	q.Set("telephone", LocalPhone(s.Phone))
	if p2 := LocalPhone(s.Phone2); p2 != "" {
		q.Set("telephone_2", p2)
	}
	q.Set("adresse", orDefault(s.Address, s.Commune))
	q.Set("code_wilaya", strconv.Itoa(s.WilayaCode))
	q.Set("commune", s.Commune)
	q.Set("montant", strconv.FormatInt(s.COD, 10))
	q.Set("produit", s.ProductText())
	typ := "1"
	if s.Exchange {
		typ = "2"
	}
	q.Set("type", typ)
	if s.Delivery == Desk {
		q.Set("stop_desk", "1")
		if s.DeskID != "" {
			q.Set("code_postal", s.DeskID)
		}
	} else {
		q.Set("stop_desk", "0")
	}
	if s.WeightKg > 0 {
		q.Set("weight", strconv.Itoa(int(s.WeightKg+0.999)))
	}
	if s.Note != "" {
		q.Set("remarque", s.Note)
	}
	res, err := c.do("POST", "/api/v1/create/order", q, e.headers(c), nil)
	if err != nil {
		return Parcel{}, err
	}
	var r ecoResult
	_ = json.Unmarshal(res.Body, &r)
	if !r.ok() || r.Tracking == "" {
		return Parcel{}, &ProviderError{Provider: c.prov.Key, Message: message(res.Body)}
	}
	p := Parcel{Tracking: r.Tracking}
	vq := url.Values{"tracking": {r.Tracking}, "ask_collection": {"1"}}
	vres, err := c.do("POST", "/api/v1/valid/order", vq, e.headers(c), nil)
	if err != nil {
		return p, fmt.Errorf("dzcouriers: created %s but validation failed: %w", r.Tracking, err)
	}
	var v ecoResult
	if json.Unmarshal(vres.Body, &v) == nil && v.Success != nil && !v.ok() {
		return p, &ProviderError{Provider: c.prov.Key, Message: "created " + r.Tracking + " but not validated: " + message(vres.Body)}
	}
	return p, nil
}

func (e ecotrack) track(c *call, trackings []string) ([]Tracking, error) {
	var out []Tracking
	for start := 0; start < len(trackings); start += 100 {
		end := start + 100
		if end > len(trackings) {
			end = len(trackings)
		}
		q := url.Values{}
		want := map[string]bool{}
		for _, t := range trackings[start:end] {
			q.Add("trackings[]", t)
			want[t] = true
		}
		res, err := c.do("GET", "/api/v1/get/trackings/info", q, e.headers(c), nil)
		if err != nil {
			return nil, err
		}
		rows := keyedRows(res.Body)
		for _, t := range trackings[start:end] {
			row, ok := rows[t]
			if !ok {
				continue
			}
			out = append(out, ecoTracking(t, row))
		}
	}
	return out, nil
}

// keyedRows accepts {tracking: row}, {data: {tracking: row}} or [{tracking, ...}].
func keyedRows(body []byte) map[string]map[string]any {
	rows := map[string]map[string]any{}
	var v any
	if json.Unmarshal(body, &v) != nil {
		return rows
	}
	switch t := v.(type) {
	case []any:
		for _, r := range t {
			if m, ok := r.(map[string]any); ok {
				if k := str(m["tracking"]); k != "" {
					rows[k] = m
				}
			}
		}
	case map[string]any:
		if d, ok := t["data"].(map[string]any); ok {
			t = d
		} else if d, ok := t["data"].([]any); ok {
			return keyedRows(mustJSON(d))
		}
		for k, r := range t {
			if m, ok := r.(map[string]any); ok {
				rows[k] = m
			}
		}
	}
	return rows
}

func mustJSON(v any) []byte { b, _ := json.Marshal(v); return b }

func ecoTracking(tracking string, row map[string]any) Tracking {
	t := Tracking{Tracking: tracking}
	acts, _ := row["activity"].([]any)
	for _, a := range acts {
		m, ok := a.(map[string]any)
		if !ok {
			continue
		}
		raw := str(m["status"])
		when := strings.TrimSpace(str(m["date"]) + " " + str(m["time"]))
		t.Events = append(t.Events, Event{Status: Normalize(FamilyEcotrack, raw), Raw: raw, At: parseTime(when),
			Where: orDefault(str(m["station"]), str(m["scanLocation"]))})
	}
	t = finish(t)
	if len(t.Events) == 0 {
		if raw := str(row["status"]); raw != "" {
			t.Raw, t.Status = raw, Normalize(FamilyEcotrack, raw)
		}
	}
	return t
}

func (e ecotrack) label(c *call, tracking string) ([]byte, string, error) {
	res, err := c.do("GET", "/api/v1/get/order/label", url.Values{"tracking": {tracking}}, e.headers(c), nil)
	if err != nil {
		return nil, "", err
	}
	if !isPDF(res.Body) {
		return nil, "", &ProviderError{Provider: c.prov.Key, Message: message(res.Body)}
	}
	return res.Body, "", nil
}

// desks: on EcoTrack the stop desk is the commune; code_postal identifies it.
func (e ecotrack) desks(c *call, wilaya int) ([]DeskInfo, error) {
	q := url.Values{}
	if wilaya > 0 {
		q.Set("wilaya_id", strconv.Itoa(wilaya))
	}
	res, err := c.do("GET", "/api/v1/get/communes", q, e.headers(c), nil)
	if err != nil {
		return nil, err
	}
	var rows []map[string]any
	var v any
	_ = json.Unmarshal(res.Body, &v)
	switch t := v.(type) {
	case []any:
		for _, r := range t {
			if m, ok := r.(map[string]any); ok {
				rows = append(rows, m)
			}
		}
	case map[string]any:
		for _, r := range t {
			if m, ok := r.(map[string]any); ok {
				rows = append(rows, m)
			}
		}
	}
	var out []DeskInfo
	for _, m := range rows {
		if num(m["has_stop_desk"]) != 1 {
			continue
		}
		w := num(m["wilaya_id"])
		if wilaya > 0 && w != wilaya {
			continue
		}
		out = append(out, DeskInfo{ID: str(m["code_postal"]), Name: str(m["nom"]), Commune: str(m["nom"]), WilayaCode: w})
	}
	return out, nil
}

func (e ecotrack) check(c *call) error {
	q := url.Values{"api_token": {c.cred("api_token")}}
	res, err := c.do("GET", "/api/v1/validate/token", q, e.headers(c), nil)
	if err != nil {
		return err
	}
	var r ecoResult
	_ = json.Unmarshal(res.Body, &r)
	if r.ok() && r.Message == "VALID_TOKEN" {
		return nil
	}
	if r.Message == "TOKEN_NOT_ALLOWED" {
		return &ProviderError{Provider: c.prov.Key, Message: "API access is disabled for this account"}
	}
	return ErrAuth
}

func (e ecotrack) cancel(c *call, tracking string) error {
	res, err := c.do("DELETE", "/api/v1/delete/order", url.Values{"tracking": {tracking}}, e.headers(c), nil)
	if err != nil {
		return err
	}
	var r map[string]any
	_ = json.Unmarshal(res.Body, &r)
	if str(r["delete"]) == "fail" || r["success"] == false {
		return &ProviderError{Provider: c.prov.Key, Message: "order can no longer be deleted"}
	}
	return nil
}
