package dzcouriers

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"net/url"
	"strings"
	"testing"
)

// fake answers by "METHOD path" prefix and records every request.
type fake struct {
	routes map[string]string
	seen   []Request
}

func (f *fake) Do(r Request) (Response, error) {
	f.seen = append(f.seen, r)
	u, _ := url.Parse(r.URL)
	key := r.Method + " " + u.Path
	best := ""
	for k := range f.routes {
		if strings.HasPrefix(key, k) && len(k) > len(best) {
			best = k
		}
	}
	if best == "" {
		return Response{Status: 404, Body: []byte(`{"message":"no route"}`)}, nil
	}
	return Response{Status: 200, Body: []byte(f.routes[best])}, nil
}

func (f *fake) last(prefix string) Request {
	for i := len(f.seen) - 1; i >= 0; i-- {
		u, _ := url.Parse(f.seen[i].URL)
		if strings.HasPrefix(f.seen[i].Method+" "+u.Path, prefix) {
			return f.seen[i]
		}
	}
	return Request{}
}

func shipment() Shipment {
	return Shipment{Reference: "A100", FirstName: "Amina", LastName: "B", Phone: "0555 12 34 56",
		WilayaCode: 16, Commune: "Bab Ezzouar", Address: "Cité 1", Delivery: Home, FromWilaya: 31,
		Products: []Product{{Name: "Robe", Quantity: 2}}, COD: 4500, CommuneID: "1601"}
}

func TestCatalog(t *testing.T) {
	if n := len(Providers()); n < 90 {
		t.Fatalf("catalog has %d providers", n)
	}
	p, ok := Lookup("dhd")
	if !ok || p.BaseURL != "https://platform.dhd-dz.com" || len(p.AltURLs) != 1 {
		t.Fatalf("dhd: %+v", p)
	}
	if p, _ := Lookup("packers"); p.BaseURL != "https://packers.ecotrack.dz" {
		t.Fatal(p.BaseURL)
	}
	if _, err := (&Client{Transport: &fake{}}).Create(Account{Provider: "nope"}, shipment()); err != ErrUnknown {
		t.Fatal(err)
	}
	_, err := (&Client{Transport: &fake{}}).Create(Account{Provider: "yalidine"}, shipment())
	if e, ok := err.(*InvalidError); !ok || e.Field != "api_id" {
		t.Fatal(err)
	}
}

func TestWilayas(t *testing.T) {
	if WilayaName(16) != "Alger" || WilayaName(69) != "El Abiodh Sidi Cheikh" || WilayaName(70) != "" || WilayaName(0) != "" {
		t.Fatal(WilayaName(16), WilayaName(69))
	}
	if WilayaCode("bejaia") != 6 || WilayaCode("Bou Saada") != 68 || WilayaCode("nowhere") != 0 {
		t.Fatal("codes")
	}
}

func TestPhones(t *testing.T) {
	for in, want := range map[string]string{"0555123456": "0555123456", "+213 555 12 34 56": "0555123456",
		"00213555123456": "0555123456", "555123456": "0555123456", "12345": ""} {
		if got := LocalPhone(in); got != want {
			t.Errorf("%s: %s", in, got)
		}
	}
	if IntlPhone("0555123456") != "+213555123456" {
		t.Fatal("intl")
	}
}

func TestNormalize(t *testing.T) {
	cases := []struct {
		fam, raw string
		want     Status
	}{
		{FamilyYalidine, "Sorti en livraison", OutForDelivery},
		{FamilyYalidine, "Tentative échouée", DeliveryAttempted},
		{FamilyYalidine, "Livré", Delivered},
		{FamilyYalidine, "Retourné au vendeur", Returned},
		{FamilyEcotrack, "livred", Delivered},
		{FamilyEcotrack, "Return_received", Returned},
		{FamilyEcotrack, "payé_et_archivé", Delivered},
		{FamilyNoest, "fdr_activated", OutForDelivery},
		{FamilyProcolis, "SD - En Attente du Client", AtHub},
		{FamilyProcolis, "Livrée [ Encaisser ]", Delivered},
		{FamilyMaystro, "41", Delivered},
		{FamilyZR, zrState("OutForDelivery"), OutForDelivery},
		{FamilyZR, "returned to seller", Returned},
		{FamilyZR, "", Unknown},
	}
	for _, c := range cases {
		if got := Normalize(c.fam, c.raw); got != c.want {
			t.Errorf("%s %q: %s, want %s", c.fam, c.raw, got, c.want)
		}
	}
	if !Delivered.IsFinal() || InTransit.IsFinal() || !DeliveryAttempted.NeedsAction() {
		t.Fatal("flags")
	}
}

func TestYalidine(t *testing.T) {
	f := &fake{routes: map[string]string{
		"POST /v1/parcels/": `{"A100":{"success":true,"order_id":"A100","tracking":"yal-123ABC","label":"https://x/label.pdf"}}`,
		"GET /v1/histories/": `{"has_more":false,"data":[
			{"tracking":"yal-123ABC","status":"Expédié","date_status":"2026-09-01 10:00:00"},
			{"tracking":"yal-123ABC","status":"Livré","date_status":"2026-09-02 15:00:00","center_name":"Bab Ezzouar"}]}`,
		"GET /v1/centers/": `{"has_more":false,"data":[{"center_id":160101,"name":"Agence Bab Ezzouar","wilaya_id":16}]}`,
	}}
	cl := &Client{Transport: f}
	acc := Account{Provider: "guepex", Credentials: map[string]string{"api_id": "id", "api_token": "tok"}}
	p, err := cl.Create(acc, shipment())
	if err != nil || p.Tracking != "yal-123ABC" || p.LabelURL == "" {
		t.Fatal(p, err)
	}
	req := f.last("POST /v1/parcels/")
	if !strings.HasPrefix(req.URL, "https://api.guepex.app/v1/") || req.Headers["X-API-TOKEN"] != "tok" {
		t.Fatal(req.URL, req.Headers)
	}
	var body []map[string]any
	_ = json.Unmarshal(req.Body, &body)
	if body[0]["from_wilaya_name"] != "Oran" || body[0]["to_wilaya_name"] != "Alger" || body[0]["contact_phone"] != "0555123456" {
		t.Fatal(body[0])
	}
	ts, err := cl.Track(acc, "yal-123ABC")
	if err != nil || len(ts) != 1 || ts[0].Status != Delivered || len(ts[0].Events) != 2 {
		t.Fatal(ts, err)
	}
	d, err := cl.Desks(acc, 16)
	if err != nil || len(d) != 1 || d[0].ID != "160101" {
		t.Fatal(d, err)
	}
	s := shipment()
	s.Delivery, s.DeskID = Desk, "abc"
	if _, err := cl.Create(acc, s); err == nil {
		t.Fatal("desk id must be numeric")
	}
}

func TestEcotrack(t *testing.T) {
	f := &fake{routes: map[string]string{
		"POST /api/v1/create/order":      `{"success":true,"tracking":"ECO-1"}`,
		"POST /api/v1/valid/order":       `{"success":true}`,
		"GET /api/v1/get/trackings/info": `{"ECO-1":{"activity":[{"status":"picked","date":"2026-09-01","time":"09:00"},{"status":"livred","date":"2026-09-03","time":"11:00"}]}}`,
		"GET /api/v1/get/order/label":    "%PDF-1.4 label",
		"GET /api/v1/validate/token":     `{"success":false,"message":"INVALID_TOKEN"}`,
	}}
	cl := &Client{Transport: f}
	acc := Account{Provider: "packers", Credentials: map[string]string{"api_token": "t"}}
	p, err := cl.Create(acc, shipment())
	if err != nil || p.Tracking != "ECO-1" {
		t.Fatal(p, err)
	}
	u, _ := url.Parse(f.last("POST /api/v1/create/order").URL)
	if u.Host != "packers.ecotrack.dz" || u.Query().Get("code_wilaya") != "16" || u.Query().Get("montant") != "4500" {
		t.Fatal(u)
	}
	ts, err := cl.Track(acc, "ECO-1", "ECO-404")
	if err != nil || len(ts) != 1 || ts[0].Status != Delivered {
		t.Fatal(ts, err)
	}
	pdf, _, err := cl.Label(acc, "ECO-1")
	if err != nil || !isPDF(pdf) {
		t.Fatal(err)
	}
	if err := cl.Check(acc); err != ErrAuth {
		t.Fatal(err)
	}
}

func TestZR(t *testing.T) {
	f := &fake{routes: map[string]string{
		"POST /api/v1/customers/individual": `{"id":"cust-1"}`,
		"POST /api/v1/territories/search":   `{"items":[{"id":"city-16","code":16,"level":"wilaya","name":"Alger"},{"id":"dist-1","parentId":"city-16","name":"Bab Ezzouar"}]}`,
		"POST /api/v1/parcels":              `{"id":"parcel-uuid"}`,
		"GET /api/v1/parcels/parcel-uuid":   `{"id":"parcel-uuid","trackingNumber":"ZR-9"}`,
		"GET /api/v1/parcels/ZR-9/state-history": `[{"newState":{"name":"Created"},"createdAt":"2026-09-01T10:00:00Z"},
			{"newState":{"name":"OutForDelivery","description":"En livraison"},"createdAt":"2026-09-02T10:00:00Z"}]`,
	}}
	cl := &Client{Transport: f}
	acc := Account{Provider: "zr", Credentials: map[string]string{"api_key": "k", "tenant_id": "t"}}
	p, err := cl.Create(acc, shipment())
	if err != nil || p.Tracking != "ZR-9" || p.ProviderID != "parcel-uuid" {
		t.Fatal(p, err)
	}
	var body map[string]any
	_ = json.Unmarshal(f.last("POST /api/v1/parcels").Body, &body)
	addr := body["deliveryAddress"].(map[string]any)
	if addr["cityTerritoryId"] != "city-16" || addr["districtTerritoryId"] != "dist-1" {
		t.Fatal(addr)
	}
	ts, err := cl.Track(acc, "ZR-9")
	if err != nil || len(ts) != 1 || ts[0].Status != OutForDelivery {
		t.Fatal(ts, err)
	}
}

func TestNoest(t *testing.T) {
	f := &fake{routes: map[string]string{
		"POST /api/public/create/order":       `{"success":true,"tracking":"NO-1"}`,
		"POST /api/public/valid/order":        `{"success":true}`,
		"POST /api/public/get/trackings/info": `{"NO-1":{"OrderInfo":{"tracking":"NO-1"},"activity":[{"event_key":"upload","date":"2026-09-01 08:00:00"},{"event_key":"livred","event":"Order delivered","date":"2026-09-02 08:00:00"}]}}`,
		"GET /api/public/desks":               `{"16A":{"code":"16A","name":"Alger centre"},"31A":{"code":"31A","name":"Oran"}}`,
	}}
	cl := &Client{Transport: f}
	acc := Account{Provider: "noest", Credentials: map[string]string{"api_token": "t", "user_guid": "g"}}
	if p, err := cl.Create(acc, shipment()); err != nil || p.Tracking != "NO-1" {
		t.Fatal(p, err)
	}
	var body map[string]any
	_ = json.Unmarshal(f.last("POST /api/public/create/order").Body, &body)
	if body["user_guid"] != "g" || body["wilaya_id"] != float64(16) {
		t.Fatal(body)
	}
	ts, err := cl.Track(acc, "NO-1")
	if err != nil || ts[0].Status != Delivered {
		t.Fatal(ts, err)
	}
	d, err := cl.Desks(acc, 16)
	if err != nil || len(d) != 1 || d[0].ID != "16A" {
		t.Fatal(d, err)
	}
}

func TestProcolisAndMaystro(t *testing.T) {
	f := &fake{routes: map[string]string{
		"POST /api_v1/add_colis":            `{"Colis":[{"Tracking":"A100","MessageRetour":"Good"}]}`,
		"POST /api_v1/lire":                 `{"Colis":[{"Tracking":"A100","Situation":"Livrée"}]}`,
		"POST /api/orders/":                 `{"id":987,"display_id":"MY-987"}`,
		"GET /api/orders/history_order/987": `[{"status":31,"created_at":"2026-09-01T10:00:00Z"},{"status":41,"created_at":"2026-09-02T10:00:00Z"}]`,
	}}
	cl := &Client{Transport: f}
	pa := Account{Provider: "abex", Credentials: map[string]string{"key": "k", "token": "t"}}
	if p, err := cl.Create(pa, shipment()); err != nil || p.Tracking != "A100" {
		t.Fatal(p, err)
	}
	if ts, err := cl.Track(pa, "A100"); err != nil || ts[0].Status != Delivered {
		t.Fatal(ts, err)
	}
	if _, _, err := cl.Label(pa, "A100"); err != ErrUnsupported {
		t.Fatal(err)
	}
	ma := Account{Provider: "maystro", Credentials: map[string]string{"api_token": "Token x"}}
	p, err := cl.Create(ma, shipment())
	if err != nil || p.Tracking != "987" {
		t.Fatal(p, err)
	}
	if ts, err := cl.Track(ma, "987"); err != nil || ts[0].Status != Delivered || ts[0].Raw != "Livré" {
		t.Fatal(ts, err)
	}
}

func TestErrors(t *testing.T) {
	cl := &Client{Transport: &fake{}}
	acc := Account{Provider: "packers", Credentials: map[string]string{"api_token": "t"}}
	if _, err := cl.Track(acc, "X"); err != ErrNotFound {
		t.Fatal(err)
	}
	s := shipment()
	s.Phone = "12"
	if _, err := cl.Create(acc, s); err == nil {
		t.Fatal("bad phone accepted")
	}
}

func TestYalidineWebhook(t *testing.T) {
	body := []byte(`{"type":"parcel_status_updated","events":[{"event_id":"e1","occurred_at":"2022-04-28 00:01:26","data":{"tracking":"yal-111AAA","status":"Sorti en livraison","reason":null}}]}`)
	m := hmac.New(sha256.New, []byte("sec"))
	m.Write(body)
	sig := hex.EncodeToString(m.Sum(nil))
	ups, err := ParseWebhook("yalidine", map[string]string{"X-Yalidine-Signature": sig}, body, "sec")
	if err != nil || len(ups) != 1 || ups[0].Status != OutForDelivery || ups[0].EventID != "e1" {
		t.Fatal(ups, err)
	}
	if _, err := ParseWebhook("yalidine", map[string]string{"X-Yalidine-Signature": "00"}, body, "sec"); err != ErrSignature {
		t.Fatal(err)
	}
	if r, ok := WebhookChallenge(url.Values{"subscribe": {"1"}, "crc_token": {"abc"}}); !ok || r != "abc" {
		t.Fatal(r)
	}
	ups, err = ParseWebhook("zr", nil, []byte(`{"type":"parcel.state.updated","data":{"trackingNumber":"ZR-1","state":{"name":"Delivered"}}}`), "")
	if err != nil || len(ups) != 1 || ups[0].Status != Delivered {
		t.Fatal(ups, err)
	}
}
