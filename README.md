# dzcouriers

One Go API for Algerian delivery companies: create parcels, track them, fetch labels, list stop desks, and
receive webhooks. The provider-specific statuses are normalised into one set.

- **~96 providers, 6 API families**: Yalidine and its clones, the EcoTrack platform (~83 tenants), ZR Express,
  NOEST, Procolis (ZR legacy, ABEX, Colilog, Flash, Leopard), and Maystro.
- **No hosted relay.** Your credentials and customer data go straight from your server to the courier.
- **No networking inside.** You pass a `Transport`, so the package builds under TinyGo/WASI (Go 1.19, no
  `regexp`). Regular Go programs use `adapters/nethttp`.

```go
import (
	"github.com/ZaouiAmine/dzcouriers"
	"github.com/ZaouiAmine/dzcouriers/adapters/nethttp"
)

cl := &dzcouriers.Client{Transport: nethttp.New()}
acc := dzcouriers.Account{Provider: "yalidine", Credentials: map[string]string{
	"api_id": "...", "api_token": "...", "from_wilaya": "Oran",
}}
p, err := cl.Create(acc, dzcouriers.Shipment{
	Reference: "A100", FirstName: "Amina", LastName: "B.", Phone: "0555123456",
	WilayaCode: 16, Commune: "Bab Ezzouar", Address: "Cité 1", Delivery: dzcouriers.Home,
	Products: []dzcouriers.Product{{Name: "Robe", Quantity: 1}}, COD: 4500,
})
ts, err := cl.Track(acc, p.Tracking)       // ts[0].Status == dzcouriers.OutForDelivery ...
pdf, link, err := cl.Label(acc, p.Tracking) // PDF bytes or a URL, depending on the provider
```

## Coverage

| Family | Providers | Credentials | Create | Track | Label | Desks | Cancel | Webhook |
|---|---|---|---|---|---|---|---|---|
| Yalidine | yalidine, guepex, yalitec, economiqua, easyandspeed, wecan | `api_id`, `api_token` (+`from_wilaya`) | ✅ | ✅ batch | URL | ✅ | ✅ | ✅ signed |
| EcoTrack | 83 tenants (`dzcheck list`) | `api_token` | ✅ + validate | ✅ batch 100 | PDF | ✅ | ✅ | — |
| ZR Express | zr | `api_key`, `tenant_id` | ✅ | ✅ | URL | ✅ | ✅ | ✅ (payload unconfirmed) |
| NOEST | noest | `api_token`, `user_guid` | ✅ + validate | ✅ batch | PDF | ✅ | ✅ | — |
| Procolis | zrlegacy, abex, colilog, flashdelivery, leopard | `key`, `token` | ✅ | ✅ | — | — | — | — |
| Maystro | maystro | `api_token` | ✅ (needs `CommuneID`) | ✅ | PDF | — | ✅ | — |

**Status.** The request and response shapes come from the public references listed in [CREDITS.md](CREDITS.md),
and contract tests cover every family. A provider is marked **verified** here only after a read-only run of
`dzcheck test` with real merchant credentials. So far, none is. If you have an account, please report the
result in an issue.

Statuses: `created, pending_pickup, picked_up, in_transit, at_hub, out_for_delivery, delivery_attempted,
on_hold, delivered, returned, cancelled, lost, unknown`. `Normalize(family, raw)` maps a provider's wording to
one of these. It uses the exact tables first and falls back to keywords, so new wordings still land somewhere
sensible.

## Webhooks

```go
if reply, ok := dzcouriers.WebhookChallenge(r.URL.Query()); ok { w.Write([]byte(reply)); return } // Yalidine CRC
updates, err := dzcouriers.ParseWebhook("yalidine", headers, body, secret)
```

## dzcheck

```sh
go run ./cmd/dzcheck list
go run ./cmd/dzcheck ping ecotrack      # which tenant URLs answer (no credentials)
go run ./cmd/dzcheck test -p packers -c api_token=XXX -t TRACKING   # read-only, never creates parcels
```

## Notes per provider

- **EcoTrack**: single-order endpoints take query parameters. `Create` also calls `valid/order` with
  `ask_collection=1`. If validation fails, the parcel and an error are both returned, so keep the tracking.
  The stop desk is the commune `code_postal`. The rate limit is 50 requests a minute.
- **Yalidine**: the sender wilaya is required (`from_wilaya` credential or `Shipment.FromWilaya`). The desk ID is
  the numeric `center_id` from `Desks`.
- **ZR**: the addresses are territory UUIDs, resolved from the wilaya code and commune name. `Parcel.ProviderID`
  is the parcel UUID.
- **Maystro**: `Parcel.Tracking` is Maystro's order ID, and `Shipment.CommuneID` must be Maystro's commune ID.
- **Procolis**: the tracking is your `Reference`.

MIT licensed.
