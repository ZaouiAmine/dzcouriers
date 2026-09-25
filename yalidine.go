package dzcouriers

import (
	"encoding/json"
	"net/url"
	"strconv"
	"strings"
)

// Yalidine and its clones (Guepex, Yalitec, Economiqua, Easy & Speed, WeCan): /v1 REST API,
// X-API-ID / X-API-TOKEN headers. Parcels are created in "En préparation" and can be edited or
// deleted until pickup.
type yalidine struct{}

func (yalidine) headers(c *call) map[string]string {
	return map[string]string{"X-API-ID": c.cred("api_id"), "X-API-TOKEN": c.cred("api_token")}
}

func (y yalidine) create(c *call, s Shipment) (Parcel, error) {
	from := c.cred("from_wilaya")
	if from == "" {
		from = WilayaName(s.FromWilaya)
	}
	if from == "" {
		return Parcel{}, &InvalidError{Field: "from_wilaya", Reason: "required"}
	}
	to := s.Wilaya
	if to == "" {
		to = WilayaName(s.WilayaCode)
	}
	first, last := s.FirstName, s.LastName
	if last == "" {
		last = first
	}
	declared := s.DeclaredDA
	if declared == 0 {
		declared = s.COD
	}
	weight := int(s.WeightKg + 0.999)
	if weight < 1 {
		weight = 1
	}
	p := map[string]any{
		"order_id": s.Reference, "from_wilaya_name": from, "firstname": first, "familyname": last,
		"contact_phone": LocalPhone(s.Phone), "address": orDefault(s.Address, s.Commune),
		"to_commune_name": s.Commune, "to_wilaya_name": to, "product_list": s.ProductText(),
		"price": s.COD, "do_insurance": false, "declared_value": declared,
		"length": 1, "width": 1, "height": 1, "weight": weight,
		"freeshipping": s.FreeShipping, "is_stopdesk": s.Delivery == Desk, "has_exchange": s.Exchange,
	}
	if s.Delivery == Desk {
		id, err := strconv.Atoi(s.DeskID)
		if err != nil {
			return Parcel{}, &InvalidError{Field: "desk", Reason: "Yalidine center id expected"}
		}
		p["stopdesk_id"] = id
	}
	res, err := c.do("POST", "/v1/parcels/", nil, y.headers(c), []any{p})
	if err != nil {
		return Parcel{}, err
	}
	var out map[string]struct {
		Success  bool   `json:"success"`
		Tracking string `json:"tracking"`
		Label    string `json:"label"`
		Message  string `json:"message"`
	}
	if err := json.Unmarshal(res.Body, &out); err != nil {
		return Parcel{}, &ProviderError{Provider: c.prov.Key, Message: message(res.Body)}
	}
	r, ok := out[s.Reference]
	if !ok {
		for _, v := range out {
			r = v
			break
		}
	}
	if !r.Success || r.Tracking == "" {
		return Parcel{}, &ProviderError{Provider: c.prov.Key, Message: orDefault(r.Message, message(res.Body))}
	}
	return Parcel{Tracking: r.Tracking, LabelURL: r.Label}, nil
}

func (y yalidine) track(c *call, trackings []string) ([]Tracking, error) {
	byT := map[string]*Tracking{}
	var order []string
	for start := 0; start < len(trackings); start += 50 {
		end := start + 50
		if end > len(trackings) {
			end = len(trackings)
		}
		q := url.Values{"tracking": {strings.Join(trackings[start:end], ",")}, "page_size": {"1000"}}
		res, err := c.do("GET", "/v1/histories/", q, y.headers(c), nil)
		if err != nil {
			return nil, err
		}
		var out struct {
			Data []struct {
				Tracking   string `json:"tracking"`
				Status     string `json:"status"`
				DateStatus string `json:"date_status"`
				Reason     string `json:"reason"`
				Center     string `json:"center_name"`
				Commune    string `json:"commune_name"`
			} `json:"data"`
		}
		if err := json.Unmarshal(res.Body, &out); err != nil {
			return nil, &ProviderError{Provider: c.prov.Key, Message: message(res.Body)}
		}
		for _, h := range out.Data {
			t := byT[h.Tracking]
			if t == nil {
				t = &Tracking{Tracking: h.Tracking}
				byT[h.Tracking] = t
				order = append(order, h.Tracking)
			}
			raw := h.Status
			if h.Reason != "" {
				raw += " (" + h.Reason + ")"
			}
			t.Events = append(t.Events, Event{Status: Normalize(FamilyYalidine, h.Status), Raw: raw,
				At: parseTime(h.DateStatus), Where: orDefault(h.Center, h.Commune)})
		}
	}
	out := make([]Tracking, 0, len(order))
	for _, k := range order {
		out = append(out, finish(*byT[k]))
	}
	return out, nil
}

func (y yalidine) label(c *call, tracking string) ([]byte, string, error) {
	res, err := c.do("GET", "/v1/parcels/"+url.PathEscape(tracking), nil, y.headers(c), nil)
	if err != nil {
		return nil, "", err
	}
	var out struct {
		Data []struct {
			Label string `json:"label"`
		} `json:"data"`
	}
	_ = json.Unmarshal(res.Body, &out)
	if len(out.Data) == 0 || out.Data[0].Label == "" {
		return nil, "", ErrNotFound
	}
	return nil, out.Data[0].Label, nil
}

func (y yalidine) desks(c *call, wilaya int) ([]DeskInfo, error) {
	var all []DeskInfo
	for page := 1; page <= 20; page++ {
		q := url.Values{"page": {strconv.Itoa(page)}, "page_size": {"1000"}}
		if wilaya > 0 {
			q.Set("wilaya_id", strconv.Itoa(wilaya))
		}
		res, err := c.do("GET", "/v1/centers/", q, y.headers(c), nil)
		if err != nil {
			return nil, err
		}
		var out struct {
			HasMore bool `json:"has_more"`
			Data    []struct {
				ID      any    `json:"center_id"`
				Name    string `json:"name"`
				Address string `json:"address"`
				Commune string `json:"commune_name"`
				Wilaya  any    `json:"wilaya_id"`
			} `json:"data"`
		}
		if err := json.Unmarshal(res.Body, &out); err != nil {
			return nil, &ProviderError{Provider: c.prov.Key, Message: message(res.Body)}
		}
		for _, d := range out.Data {
			all = append(all, DeskInfo{ID: str(d.ID), Name: d.Name, Address: d.Address, Commune: d.Commune,
				WilayaCode: num(d.Wilaya)})
		}
		if !out.HasMore {
			break
		}
	}
	return all, nil
}

func (y yalidine) check(c *call) error {
	res, err := c.do("GET", "/v1/wilayas/", url.Values{"page_size": {"1"}}, y.headers(c), nil)
	if err != nil {
		return err
	}
	if !strings.Contains(string(res.Body), "\"data\"") {
		return ErrAuth
	}
	return nil
}

func (y yalidine) cancel(c *call, tracking string) error {
	res, err := c.do("DELETE", "/v1/parcels/"+url.PathEscape(tracking), nil, y.headers(c), nil)
	if err != nil {
		return err
	}
	var out []struct {
		Deleted bool `json:"deleted"`
	}
	if json.Unmarshal(res.Body, &out) == nil && len(out) > 0 && !out[0].Deleted {
		return &ProviderError{Provider: c.prov.Key, Message: "parcel can no longer be deleted"}
	}
	return nil
}

func orDefault(v, d string) string {
	if strings.TrimSpace(v) == "" {
		return d
	}
	return v
}
