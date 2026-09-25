package dzcouriers

import (
	"encoding/json"
	"strconv"
)

// Maystro Delivery (b.maystro-delivery.com/api). The Authorization header carries the raw access
// token. Wilaya and commune are numeric ids (Shipment.CommuneID); the parcel is identified by
// Maystro's order id, returned as Parcel.Tracking. Status codes are numeric.
type maystro struct{}

func (maystro) headers(c *call) map[string]string {
	return map[string]string{"Authorization": c.cred("api_token")}
}

func (m maystro) create(c *call, s Shipment) (Parcel, error) {
	commune, err := strconv.Atoi(s.CommuneID)
	if err != nil {
		return Parcel{}, &InvalidError{Field: "commune", Reason: "Maystro commune id required"}
	}
	delivery := 0
	if s.Delivery == Desk {
		delivery = 1
	}
	products := []map[string]any{}
	for _, p := range s.Products {
		q := p.Quantity
		if q < 1 {
			q = 1
		}
		products = append(products, map[string]any{"quantity": q, "logistical_description": p.Name})
	}
	body := map[string]any{
		"wilaya": s.WilayaCode, "commune": commune, "destination_text": orDefault(s.Address, s.Commune),
		"customer_phone": LocalPhone(s.Phone), "customer_name": s.FullName(), "product_price": s.COD,
		"delivery_type": delivery, "express": false, "note_to_driver": s.Note, "products": products,
		"source": 4, "external_order_id": s.Reference,
	}
	res, err := c.do("POST", "/orders/", nil, m.headers(c), body)
	if err != nil {
		return Parcel{}, err
	}
	var r map[string]any
	_ = json.Unmarshal(res.Body, &r)
	id := str(r["id"])
	if id == "" {
		return Parcel{}, &ProviderError{Provider: c.prov.Key, Message: message(res.Body)}
	}
	return Parcel{Tracking: id, ProviderID: str(r["display_id"])}, nil
}

func (m maystro) track(c *call, trackings []string) ([]Tracking, error) {
	var out []Tracking
	for _, id := range trackings {
		res, err := c.do("GET", "/orders/history_order/"+id, nil, m.headers(c), nil)
		if err == ErrNotFound {
			continue
		}
		if err != nil {
			return out, err
		}
		var v any
		_ = json.Unmarshal(res.Body, &v)
		if mm, ok := v.(map[string]any); ok {
			v = mm["results"]
			if v == nil {
				v = mm["history"]
			}
		}
		rows, _ := v.([]any)
		t := Tracking{Tracking: id}
		for _, r := range rows {
			e, ok := r.(map[string]any)
			if !ok {
				continue
			}
			code := str(e["status"])
			t.Events = append(t.Events, Event{Status: Normalize(FamilyMaystro, code), Raw: maystroLabel(code),
				At: parseTime(str(e["created_at"]))})
		}
		out = append(out, finish(t))
	}
	return out, nil
}

var maystroLabels = map[string]string{
	"4": "Créé", "5": "Ramassage demandé", "6": "En traitement", "8": "En attente de transit",
	"9": "En transit pour expédition", "10": "En transit pour retour", "11": "En attente",
	"12": "Rupture de stock", "15": "Prêt à expédier", "22": "Assigné", "31": "Expédié", "32": "Alerté",
	"41": "Livré", "42": "Reporté", "50": "Annulé", "51": "Prêt à retourner", "52": "Récupéré par le magasin",
	"53": "Non reçu",
}

func maystroLabel(code string) string {
	if l, ok := maystroLabels[code]; ok {
		return l
	}
	return code
}

func (m maystro) label(c *call, id string) ([]byte, string, error) {
	res, err := c.do("POST", "/starter_bordureau/", nil, m.headers(c), map[string]any{"orders_ids": []string{id}})
	if err != nil {
		return nil, "", err
	}
	if !isPDF(res.Body) {
		return nil, "", &ProviderError{Provider: c.prov.Key, Message: message(res.Body)}
	}
	return res.Body, "", nil
}

func (maystro) desks(*call, int) ([]DeskInfo, error) { return nil, ErrUnsupported }

func (m maystro) check(c *call) error {
	_, err := c.do("GET", "/base/wilayas/?language=fr&country=1", nil, m.headers(c), nil)
	return err
}

func (m maystro) cancel(c *call, id string) error {
	_, err := c.do("PATCH", "/orders/"+id+"/status/", nil, m.headers(c), map[string]any{"status": 50})
	return err
}
