package dzcouriers

import (
	"encoding/json"
	"net/url"
	"strings"
)

// ZR Express (new platform, api.zrexpress.app/api/v1). Headers X-Api-Key and X-Tenant.
// Addresses use territory UUIDs (city = wilaya level, district = commune); a customer record is
// created first; a single create returns a parcel id and the tracking number is read back.
type zr struct{}

func (zr) headers(c *call) map[string]string {
	return map[string]string{"X-Api-Key": c.cred("api_key"), "X-Tenant": c.cred("tenant_id")}
}

type zrTerritory struct {
	ID       string `json:"id"`
	Code     any    `json:"code"`
	Name     string `json:"name"`
	Level    string `json:"level"`
	ParentID string `json:"parentId"`
	Delivery struct {
		HasPickupPoint bool `json:"hasPickupPoint"`
	} `json:"delivery"`
}

func (z zr) search(c *call, keyword string, pickup bool, size int) ([]zrTerritory, error) {
	body := map[string]any{"keyword": keyword, "pageSize": size, "pageNumber": 1}
	if pickup {
		body["deliveryType"] = map[string]string{"value": "pickup-point"}
	}
	res, err := c.do("POST", "/api/v1/territories/search", nil, z.headers(c), body)
	if err != nil {
		return nil, err
	}
	var out struct {
		Items []zrTerritory `json:"items"`
	}
	if err := json.Unmarshal(res.Body, &out); err != nil {
		return nil, &ProviderError{Provider: c.prov.Key, Message: message(res.Body)}
	}
	return out.Items, nil
}

func (z zr) city(c *call, s Shipment) (zrTerritory, error) {
	name := orDefault(s.Wilaya, WilayaName(s.WilayaCode))
	for _, kw := range []string{name, str(float64(s.WilayaCode))} {
		items, err := z.search(c, kw, false, 50)
		if err != nil {
			return zrTerritory{}, err
		}
		for _, t := range items {
			if t.Level == "wilaya" && num(t.Code) == s.WilayaCode {
				return t, nil
			}
		}
	}
	return zrTerritory{}, &InvalidError{Field: "wilaya", Reason: "no ZR territory for " + name}
}

func (z zr) district(c *call, s Shipment, city zrTerritory) (string, error) {
	if s.Delivery == Desk && len(s.DeskID) == 36 {
		return s.DeskID, nil
	}
	items, err := z.search(c, s.Commune, s.Delivery == Desk, 50)
	if err != nil {
		return "", err
	}
	for _, t := range items {
		if t.ParentID == city.ID {
			return t.ID, nil
		}
	}
	return "", &InvalidError{Field: "commune", Reason: "no ZR territory for " + s.Commune}
}

func (z zr) create(c *call, s Shipment) (Parcel, error) {
	phone := map[string]string{"number1": IntlPhone(s.Phone)}
	if p2 := IntlPhone(s.Phone2); p2 != "" {
		phone["number2"] = p2
	}
	res, err := c.do("POST", "/api/v1/customers/individual", nil, z.headers(c),
		map[string]any{"name": s.FullName(), "phone": phone})
	if err != nil {
		return Parcel{}, err
	}
	var cust struct {
		ID string `json:"id"`
	}
	if json.Unmarshal(res.Body, &cust) != nil || cust.ID == "" {
		return Parcel{}, &ProviderError{Provider: c.prov.Key, Message: "customer: " + message(res.Body)}
	}
	city, err := z.city(c, s)
	if err != nil {
		return Parcel{}, err
	}
	district, err := z.district(c, s, city)
	if err != nil {
		return Parcel{}, err
	}
	products := []map[string]any{}
	for _, p := range s.Products {
		q := p.Quantity
		if q < 1 {
			q = 1
		}
		products = append(products, map[string]any{"productName": p.Name, "quantity": q, "unitPrice": 0, "stockType": "none"})
	}
	if len(products) == 0 {
		products = append(products, map[string]any{"productName": s.ProductText(), "quantity": 1, "unitPrice": s.COD, "stockType": "none"})
	}
	body := map[string]any{
		"customer":        map[string]any{"customerId": cust.ID, "name": s.FullName(), "phone": phone},
		"deliveryAddress": map[string]any{"cityTerritoryId": city.ID, "districtTerritoryId": district, "street": s.Address},
		"deliveryType":    "home", "amount": s.COD, "description": s.ProductText(),
		"externalId": s.Reference, "orderedProducts": products,
	}
	if s.Delivery == Desk {
		body["deliveryType"] = "pickup-point"
		body["hubId"] = district
	}
	res, err = c.do("POST", "/api/v1/parcels", nil, z.headers(c), body)
	if err != nil {
		return Parcel{}, err
	}
	var created struct {
		ID string `json:"id"`
	}
	if json.Unmarshal(res.Body, &created) != nil || created.ID == "" {
		return Parcel{}, &ProviderError{Provider: c.prov.Key, Message: message(res.Body)}
	}
	res, err = c.do("GET", "/api/v1/parcels/"+created.ID, nil, z.headers(c), nil)
	if err != nil {
		return Parcel{ProviderID: created.ID}, err
	}
	var parcel struct {
		TrackingNumber string `json:"trackingNumber"`
	}
	_ = json.Unmarshal(res.Body, &parcel)
	if parcel.TrackingNumber == "" {
		return Parcel{ProviderID: created.ID}, &ProviderError{Provider: c.prov.Key, Message: "parcel created, tracking not assigned yet"}
	}
	return Parcel{Tracking: parcel.TrackingNumber, ProviderID: created.ID}, nil
}

func (z zr) track(c *call, trackings []string) ([]Tracking, error) {
	var out []Tracking
	for _, tr := range trackings {
		res, err := c.do("GET", "/api/v1/parcels/"+url.PathEscape(tr)+"/state-history", nil, z.headers(c), nil)
		if err == ErrNotFound {
			continue
		}
		if err != nil {
			return out, err
		}
		var hist []struct {
			NewState struct {
				Name        string `json:"name"`
				Description string `json:"description"`
			} `json:"newState"`
			CreatedAt string `json:"createdAt"`
		}
		if json.Unmarshal(res.Body, &hist) != nil {
			continue
		}
		t := Tracking{Tracking: tr}
		for _, h := range hist {
			t.Events = append(t.Events, Event{Status: Normalize(FamilyZR, zrState(h.NewState.Name)),
				Raw: orDefault(h.NewState.Description, h.NewState.Name), At: parseTime(h.CreatedAt)})
		}
		out = append(out, finish(t))
	}
	return out, nil
}

// zrState splits camelCase / kebab state names so the keyword matcher can read them.
func zrState(s string) string {
	var b strings.Builder
	for i, r := range s {
		if r >= 'A' && r <= 'Z' && i > 0 {
			b.WriteByte(' ')
		}
		b.WriteRune(r)
	}
	return b.String()
}

func (z zr) label(c *call, tracking string) ([]byte, string, error) {
	res, err := c.do("POST", "/api/v1/parcels/labels/individual/pdf", nil, z.headers(c),
		map[string]any{"trackingNumbers": []string{tracking}, "format": "a6"})
	if err != nil {
		return nil, "", err
	}
	var out struct {
		Files []struct {
			FileURL string `json:"fileUrl"`
		} `json:"parcelLabelFiles"`
	}
	_ = json.Unmarshal(res.Body, &out)
	if len(out.Files) == 0 || out.Files[0].FileURL == "" {
		return nil, "", &ProviderError{Provider: c.prov.Key, Message: message(res.Body)}
	}
	return nil, out.Files[0].FileURL, nil
}

func (z zr) desks(c *call, wilaya int) ([]DeskInfo, error) {
	items, err := z.search(c, "", true, 500)
	if err != nil {
		return nil, err
	}
	var out []DeskInfo
	for _, t := range items {
		if !t.Delivery.HasPickupPoint {
			continue
		}
		w := num(t.Code)
		if w > 69 {
			w = 0
		}
		if wilaya > 0 && w != wilaya {
			continue
		}
		out = append(out, DeskInfo{ID: t.ID, Name: t.Name, WilayaCode: w})
	}
	return out, nil
}

func (z zr) check(c *call) error {
	_, err := z.search(c, "Alger", false, 1)
	return err
}

func (z zr) cancel(c *call, tracking string) error {
	res, err := c.do("POST", "/api/v1/parcels/bulk/by-tracking-number", nil, z.headers(c),
		map[string]any{"trackingNumbers": []string{tracking}})
	if err != nil {
		return err
	}
	var out struct {
		SuccessCount int `json:"successCount"`
	}
	if json.Unmarshal(res.Body, &out) == nil && out.SuccessCount != 1 {
		return &ProviderError{Provider: c.prov.Key, Message: "parcel can no longer be deleted"}
	}
	return nil
}
