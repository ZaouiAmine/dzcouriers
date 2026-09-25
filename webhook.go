package dzcouriers

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"net/url"
	"strings"
)

// Update is one status change pushed by a provider.
type Update struct {
	EventID  string
	Tracking string
	Status   Status
	Raw      string
	At       string
	Type     string
}

// ErrSignature is returned when a webhook signature does not match.
var ErrSignature = errors.New("dzcouriers: bad webhook signature")

// WebhookChallenge answers a provider's subscription check. Yalidine calls the endpoint with
// GET ?subscribe=1&crc_token=X and expects X back. ok is false when the query is not a challenge.
func WebhookChallenge(query url.Values) (reply string, ok bool) {
	if t := query.Get("crc_token"); t != "" {
		return t, true
	}
	return "", false
}

// ParseWebhook verifies and decodes a webhook body for a provider family.
// Header names are matched case-insensitively.
//
// Yalidine family: X-YALIDINE-SIGNATURE = hex HMAC-SHA256(body, secret).
// ZR: "webhook-signature" (Svix-style "v1,<base64 HMAC-SHA256(id.timestamp.body)>") when present;
// ZR's payload shape is not publicly documented, so fields are read defensively.
func ParseWebhook(provider string, headers map[string]string, body []byte, secret string) ([]Update, error) {
	p, ok := Lookup(provider)
	if !ok {
		return nil, ErrUnknown
	}
	h := map[string]string{}
	for k, v := range headers {
		h[strings.ToLower(k)] = v
	}
	switch p.Family {
	case FamilyYalidine:
		if secret != "" && !hexHMAC(body, secret, h["x-yalidine-signature"]) {
			return nil, ErrSignature
		}
		return yalidineHook(body)
	case FamilyZR:
		if secret != "" && !svix(body, secret, h) {
			return nil, ErrSignature
		}
		return zrHook(body)
	}
	return nil, ErrUnsupported
}

func hexHMAC(body []byte, secret, got string) bool {
	m := hmac.New(sha256.New, []byte(secret))
	m.Write(body)
	want := hex.EncodeToString(m.Sum(nil))
	return hmac.Equal([]byte(want), []byte(strings.ToLower(strings.TrimSpace(got))))
}

func svix(body []byte, secret string, h map[string]string) bool {
	id, ts, sigs := h["webhook-id"], h["webhook-timestamp"], h["webhook-signature"]
	if sigs == "" {
		id, ts, sigs = h["svix-id"], h["svix-timestamp"], h["svix-signature"]
	}
	if sigs == "" {
		return false
	}
	key := []byte(secret)
	if strings.HasPrefix(secret, "whsec_") {
		if k, err := base64.StdEncoding.DecodeString(secret[6:]); err == nil {
			key = k
		}
	}
	m := hmac.New(sha256.New, key)
	m.Write([]byte(id + "." + ts + "."))
	m.Write(body)
	want := base64.StdEncoding.EncodeToString(m.Sum(nil))
	for _, s := range strings.Fields(sigs) {
		if i := strings.IndexByte(s, ','); i >= 0 {
			s = s[i+1:]
		}
		if hmac.Equal([]byte(s), []byte(want)) {
			return true
		}
	}
	return false
}

func yalidineHook(body []byte) ([]Update, error) {
	var in struct {
		Type   string `json:"type"`
		Events []struct {
			ID   string `json:"event_id"`
			At   string `json:"occurred_at"`
			Data struct {
				Tracking string  `json:"tracking"`
				Status   string  `json:"status"`
				Reason   *string `json:"reason"`
			} `json:"data"`
		} `json:"events"`
	}
	if err := json.Unmarshal(body, &in); err != nil {
		return nil, &InvalidError{Field: "body", Reason: "not JSON"}
	}
	var out []Update
	for _, e := range in.Events {
		if e.Data.Tracking == "" {
			continue
		}
		u := Update{EventID: e.ID, Tracking: e.Data.Tracking, At: e.At, Type: in.Type, Raw: e.Data.Status}
		switch in.Type {
		case "parcel_status_updated":
			u.Status = Normalize(FamilyYalidine, e.Data.Status)
		case "parcel_deleted":
			u.Status = Cancelled
		case "parcel_created":
			u.Status = Created
		default:
			continue
		}
		out = append(out, u)
	}
	return out, nil
}

func zrHook(body []byte) ([]Update, error) {
	var v any
	if err := json.Unmarshal(body, &v); err != nil {
		return nil, &InvalidError{Field: "body", Reason: "not JSON"}
	}
	root, _ := v.(map[string]any)
	if root == nil {
		return nil, nil
	}
	data, _ := root["data"].(map[string]any)
	if data == nil {
		data = root
	}
	tr := orDefault(str(data["trackingNumber"]), str(data["tracking"]))
	if tr == "" {
		return nil, nil
	}
	raw := ""
	if st, ok := data["state"].(map[string]any); ok {
		raw = str(st["name"])
	} else if st, ok := data["newState"].(map[string]any); ok {
		raw = str(st["name"])
	} else {
		raw = orDefault(str(data["state"]), str(data["status"]))
	}
	typ := orDefault(str(root["type"]), str(root["eventType"]))
	return []Update{{EventID: orDefault(str(root["id"]), str(root["eventId"])), Tracking: tr, Raw: raw,
		Status: Normalize(FamilyZR, zrState(raw)), At: orDefault(str(root["timestamp"]), str(root["createdAt"])), Type: typ}}, nil
}
