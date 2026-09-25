package dzcouriers

import (
	"encoding/json"
	"strconv"
)

// Procolis platform (ZR legacy, ABEX, Colilog, Flash Delivery, Leopard): procolis.com/api_v1 with
// Token and Key headers. The merchant chooses the tracking (we use the order reference).
type procolis struct{}

func (procolis) headers(c *call) map[string]string {
	return map[string]string{"token": c.cred("token"), "key": c.cred("key")}
}

func (p procolis) create(c *call, s Shipment) (Parcel, error) {
	mode := "0"
	if s.Delivery == Desk {
		mode = "1"
	}
	kind := "0"
	if s.Exchange {
		kind = "1"
	}
	colis := map[string]any{
		"Tracking": s.Reference, "TypeLivraison": mode, "TypeColis": kind, "Confrimee": "1",
		"Client": s.FullName(), "MobileA": LocalPhone(s.Phone), "MobileB": LocalPhone(s.Phone2),
		"Adresse": orDefault(s.Address, s.Commune), "IDWilaya": strconv.Itoa(s.WilayaCode), "Commune": s.Commune,
		"Total": strconv.FormatInt(s.COD, 10), "Note": s.Note, "Produit": s.ProductText(), "ID_Externe": s.Reference,
		"Source": "dzcouriers",
	}
	res, err := c.do("POST", "/add_colis", nil, p.headers(c), map[string]any{"Colis": []any{colis}})
	if err != nil {
		return Parcel{}, err
	}
	row := firstRow(res.Body, "Colis")
	tr := str(row["Tracking"])
	msg := str(row["MessageRetour"])
	if tr == "" || (msg != "" && msg != "Good") {
		return Parcel{}, &ProviderError{Provider: c.prov.Key, Message: orDefault(msg, message(res.Body))}
	}
	return Parcel{Tracking: tr}, nil
}

// firstRow reads [row], {key: [row]} or row.
func firstRow(body []byte, key string) map[string]any {
	var v any
	_ = json.Unmarshal(body, &v)
	for i := 0; i < 2; i++ {
		switch t := v.(type) {
		case []any:
			if len(t) > 0 {
				v = t[0]
				continue
			}
			return nil
		case map[string]any:
			if inner, ok := t[key]; ok {
				v = inner
				continue
			}
			return t
		}
	}
	m, _ := v.(map[string]any)
	return m
}

func (p procolis) track(c *call, trackings []string) ([]Tracking, error) {
	list := make([]any, 0, len(trackings))
	for _, t := range trackings {
		list = append(list, map[string]string{"Tracking": t})
	}
	res, err := c.do("POST", "/lire", nil, p.headers(c), map[string]any{"Colis": list})
	if err != nil {
		return nil, err
	}
	var v any
	_ = json.Unmarshal(res.Body, &v)
	if m, ok := v.(map[string]any); ok {
		if inner, ok := m["Colis"]; ok {
			v = inner
		} else {
			v = []any{m}
		}
	}
	rows, _ := v.([]any)
	var out []Tracking
	for _, r := range rows {
		m, ok := r.(map[string]any)
		if !ok || str(m["Tracking"]) == "" {
			continue
		}
		raw := str(m["Situation"])
		t := Tracking{Tracking: str(m["Tracking"]), Raw: raw, Status: Normalize(FamilyProcolis, raw)}
		t.Events = []Event{{Status: t.Status, Raw: raw, At: parseTime(str(m["DateH_Action"]))}}
		out = append(out, t)
	}
	return out, nil
}

func (procolis) label(*call, string) ([]byte, string, error) { return nil, "", ErrUnsupported }

func (procolis) desks(*call, int) ([]DeskInfo, error) { return nil, ErrUnsupported }

func (p procolis) check(c *call) error {
	res, err := c.do("GET", "/token", nil, p.headers(c), nil)
	if err != nil {
		return err
	}
	var m map[string]any
	_ = json.Unmarshal(res.Body, &m)
	if s := fold(str(m["Statut"])); s != "" && s != "acces active" && s != "accès activé" {
		return ErrAuth
	}
	return nil
}

func (procolis) cancel(*call, string) error { return ErrUnsupported }
