package dzcouriers

import (
	"encoding/json"
	"net/url"
	"strconv"
)

// NOEST Express (app.noest-dz.com/api/public). Bearer api_token, and user_guid (plus api_token)
// in every body or query. Orders are created then validated (valid/order); Create does both.
type noest struct{}

func (noest) headers(c *call) map[string]string {
	return map[string]string{"Authorization": "Bearer " + c.cred("api_token")}
}

func (n noest) body(c *call, m map[string]any) map[string]any {
	m["api_token"] = c.cred("api_token")
	m["user_guid"] = c.cred("user_guid")
	return m
}

func (n noest) query(c *call) url.Values {
	return url.Values{"api_token": {c.cred("api_token")}, "user_guid": {c.cred("user_guid")}}
}

func (n noest) create(c *call, s Shipment) (Parcel, error) {
	typ := 1
	if s.Exchange {
		typ = 2
	}
	stop := 0
	if s.Delivery == Desk {
		stop = 1
	}
	open := 0
	if s.CanOpen {
		open = 1
	}
	w := int(s.WeightKg + 0.999)
	if w < 1 {
		w = 1
	}
	m := map[string]any{
		"reference": s.Reference, "client": s.FullName(), "phone": LocalPhone(s.Phone),
		"adresse": orDefault(s.Address, s.Commune), "wilaya_id": s.WilayaCode, "commune": s.Commune,
		"montant": s.COD, "produit": s.ProductText(), "type_id": typ, "stop_desk": stop,
		"poids": w, "can_open": open,
	}
	if p2 := LocalPhone(s.Phone2); p2 != "" {
		m["phone_2"] = p2
	}
	if s.Note != "" {
		m["remarque"] = s.Note
	}
	if stop == 1 {
		m["station_code"] = s.DeskID
	}
	res, err := c.do("POST", "/api/public/create/order", nil, n.headers(c), n.body(c, m))
	if err != nil {
		return Parcel{}, err
	}
	var r struct {
		Success  bool   `json:"success"`
		Tracking string `json:"tracking"`
	}
	_ = json.Unmarshal(res.Body, &r)
	if !r.Success || r.Tracking == "" {
		return Parcel{}, &ProviderError{Provider: c.prov.Key, Message: message(res.Body)}
	}
	p := Parcel{Tracking: r.Tracking}
	vres, err := c.do("POST", "/api/public/valid/order", nil, n.headers(c), n.body(c, map[string]any{"tracking": r.Tracking}))
	if err != nil {
		return p, &ProviderError{Provider: c.prov.Key, Message: "created " + r.Tracking + " but not validated: " + err.Error()}
	}
	var v struct {
		Success *bool `json:"success"`
	}
	if json.Unmarshal(vres.Body, &v) == nil && v.Success != nil && !*v.Success {
		return p, &ProviderError{Provider: c.prov.Key, Message: "created " + r.Tracking + " but not validated: " + message(vres.Body)}
	}
	return p, nil
}

func (n noest) track(c *call, trackings []string) ([]Tracking, error) {
	res, err := c.do("POST", "/api/public/get/trackings/info", nil, n.headers(c), n.body(c, map[string]any{"trackings": trackings}))
	if err != nil {
		return nil, err
	}
	rows := keyedRows(res.Body)
	var out []Tracking
	for _, tr := range trackings {
		row, ok := rows[tr]
		if !ok {
			continue
		}
		t := Tracking{Tracking: tr}
		acts, _ := row["activity"].([]any)
		for _, a := range acts {
			m, ok := a.(map[string]any)
			if !ok {
				continue
			}
			key := orDefault(str(m["event_key"]), str(m["status"]))
			raw := orDefault(str(m["event"]), key)
			st := Normalize(FamilyNoest, key)
			if st == Unknown {
				st = Normalize(FamilyNoest, raw)
			}
			t.Events = append(t.Events, Event{Status: st, Raw: raw, At: parseTime(orDefault(str(m["date"]), str(m["created_at"]))),
				Where: orDefault(str(m["by"]), str(m["commune"]))})
		}
		out = append(out, finish(t))
	}
	return out, nil
}

func (n noest) label(c *call, tracking string) ([]byte, string, error) {
	q := n.query(c)
	q.Set("tracking", tracking)
	res, err := c.do("GET", "/api/public/get/order/label", q, n.headers(c), nil)
	if err != nil {
		return nil, "", err
	}
	if !isPDF(res.Body) {
		return nil, "", &ProviderError{Provider: c.prov.Key, Message: message(res.Body)}
	}
	return res.Body, "", nil
}

func (n noest) desks(c *call, wilaya int) ([]DeskInfo, error) {
	res, err := c.do("GET", "/api/public/desks", n.query(c), n.headers(c), nil)
	if err != nil {
		return nil, err
	}
	var v any
	_ = json.Unmarshal(res.Body, &v)
	var rows []map[string]any
	collect := func(x any) {
		if m, ok := x.(map[string]any); ok {
			rows = append(rows, m)
		}
	}
	switch t := v.(type) {
	case []any:
		for _, r := range t {
			collect(r)
		}
	case map[string]any:
		for k, r := range t {
			if m, ok := r.(map[string]any); ok {
				if _, has := m["code"]; !has {
					m["code"] = k
				}
			}
			collect(r)
		}
	}
	var out []DeskInfo
	for _, m := range rows {
		code := str(m["code"])
		w := num(m["wilaya_id"])
		if w == 0 && len(code) >= 2 {
			w, _ = strconv.Atoi(code[:2])
		}
		if wilaya > 0 && w != wilaya {
			continue
		}
		out = append(out, DeskInfo{ID: code, Name: str(m["name"]), Address: str(m["address"]), WilayaCode: w})
	}
	return out, nil
}

func (n noest) check(c *call) error {
	res, err := c.do("POST", "/api/public/get/trackings/info", nil, n.headers(c), n.body(c, map[string]any{"trackings": []string{"CHECK"}}))
	if err != nil {
		return err
	}
	var m map[string]any
	if json.Unmarshal(res.Body, &m) == nil {
		if msg := str(m["message"]); msg != "" && (m["success"] == false || m["error"] != nil) {
			return ErrAuth
		}
	}
	return nil
}

func (n noest) cancel(c *call, tracking string) error {
	res, err := c.do("POST", "/api/public/delete/order", nil, n.headers(c), n.body(c, map[string]any{"tracking": tracking}))
	if err != nil {
		return err
	}
	var r struct {
		Success *bool `json:"success"`
	}
	if json.Unmarshal(res.Body, &r) == nil && r.Success != nil && !*r.Success {
		return &ProviderError{Provider: c.prov.Key, Message: message(res.Body)}
	}
	return nil
}
