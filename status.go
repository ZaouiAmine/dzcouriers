package dzcouriers

import "strings"

// Status is the provider-neutral parcel state.
type Status string

const (
	Created           Status = "created"
	PendingPickup     Status = "pending_pickup"
	PickedUp          Status = "picked_up"
	InTransit         Status = "in_transit"
	AtHub             Status = "at_hub"
	OutForDelivery    Status = "out_for_delivery"
	DeliveryAttempted Status = "delivery_attempted"
	OnHold            Status = "on_hold"
	Delivered         Status = "delivered"
	Returned          Status = "returned"
	Cancelled         Status = "cancelled"
	Lost              Status = "lost"
	Unknown           Status = "unknown"
)

// IsFinal reports whether the parcel will not move again.
func (s Status) IsFinal() bool {
	return s == Delivered || s == Returned || s == Cancelled || s == Lost
}

// NeedsAction reports whether the merchant should call the customer.
func (s Status) NeedsAction() bool { return s == DeliveryAttempted || s == OnHold }

// fold lowercases and strips French accents and punctuation noise.
func fold(s string) string {
	r := strings.NewReplacer("é", "e", "è", "e", "ê", "e", "É", "e", "È", "e", "à", "a", "â", "a",
		"ç", "c", "ô", "o", "î", "i", "ï", "i", "û", "u", "ù", "u", "_", " ", "-", " ")
	s = strings.ToLower(r.Replace(strings.TrimSpace(s)))
	return strings.Join(strings.Fields(s), " ")
}

var exact = map[string]map[string]Status{
	FamilyYalidine: {
		"pas encore expedie": Created, "a verifier": Created, "en preparation": Created,
		"pas encore ramasse": PendingPickup, "pret a expedier": PendingPickup,
		"ramasse": PickedUp, "bloque": OnHold, "debloque": InTransit, "transfert": InTransit,
		"expedie": InTransit, "centre": AtHub, "en localisation": InTransit, "vers wilaya": InTransit,
		"recu a wilaya": AtHub, "en attente du client": AtHub, "pret pour livreur": AtHub,
		"sorti en livraison": OutForDelivery, "en attente": OnHold, "en alerte": OnHold,
		"tentative echouee": DeliveryAttempted, "livre": Delivered, "echec livraison": Returned,
		"retour vers centre": Returned, "retourne au centre": Returned, "retour transfert": Returned,
		"retour groupe": Returned, "retour a retirer": Returned, "retour vers vendeur": Returned,
		"retourne au vendeur": Returned, "echange echoue": Returned,
	},
	FamilyEcotrack: {
		"order information received by carrier": Created, "notification on order": Created,
		"en preparation": Created, "en preparation stock": Created, "prete a expedier": PendingPickup,
		"en ramassage": PendingPickup, "picked": PickedUp, "accepted by carrier": PickedUp,
		"vers hub": InTransit, "en hub": AtHub, "vers wilaya": InTransit,
		"dispatched to driver": OutForDelivery, "en livraison": OutForDelivery,
		"attempt delivery": DeliveryAttempted, "suspendu": OnHold,
		"livred": Delivered, "livre": Delivered, "encaissed": Delivered, "payed": Delivered,
		"livre non encaisse": Delivered, "encaisse non paye": Delivered, "paiements prets": Delivered,
		"paye et archive": Delivered, "return asked": Returned, "return in transit": Returned,
		"return received": Returned, "retour chez livreur": Returned, "retour transit entrepot": Returned,
		"retour en traitement": Returned, "retour recu": Returned, "retour archive": Returned,
		"annule": Cancelled,
	},
	FamilyNoest: {
		"upload": Created, "customer validation": PendingPickup, "validation collect colis": PickedUp,
		"validation reception admin": AtHub, "validation reception": AtHub, "fdr activated": OutForDelivery,
		"sent to redispatch": InTransit, "nouvel tentative asked by customer": DeliveryAttempted,
		"return asked by customer": Returned, "return asked by hub": Returned,
		"retour dispatched to partenaires": Returned, "return dispatched to partenaire": Returned,
		"colis retour transmit to partner": Returned, "livraison echoue recu": Returned,
		"return validated by partener": Returned, "return redispatched to livraison": InTransit,
		"return dispatched to warehouse": Returned, "colis suspendu": OnHold, "livre": Delivered,
		"livred": Delivered, "pickedup": PickedUp,
	},
	FamilyProcolis: {
		"en preparation": Created, "en traitement pret a expedie": PendingPickup, "dispatcher": InTransit,
		"au bureau": AtHub, "en livraison": OutForDelivery, "en livraison ( 1528 )": OutForDelivery,
		"a relance": DeliveryAttempted, "appel sans reponse 1": DeliveryAttempted,
		"appel sans reponse 2": DeliveryAttempted, "appel sans reponse 3": DeliveryAttempted,
		"reporte": OnHold, "livree": Delivered, "colis livree": Delivered, "livree [ encaisser ]": Delivered,
		"echange": Delivered, "annuler par le client": Returned, "retour de dispatche": Returned,
		"retour livreur": Returned, "retour navette": Returned, "retour stock": Returned,
		"sd annuler 3x": Returned, "sd annuler par le client": Returned,
		"sd appel sans reponse 1": DeliveryAttempted, "sd appel sans reponse 2": DeliveryAttempted,
		"sd appel sans reponse 3": DeliveryAttempted, "sd en attente du client": AtHub, "sd reporte": OnHold,
	},
	FamilyMaystro: {
		"4": Created, "5": PendingPickup, "6": PendingPickup, "8": AtHub, "9": InTransit, "10": Returned,
		"11": OnHold, "12": OnHold, "15": AtHub, "22": OutForDelivery, "31": OutForDelivery,
		"32": DeliveryAttempted, "41": Delivered, "42": OnHold, "50": Cancelled, "51": Returned,
		"52": Returned, "53": Lost,
	},
}

// Normalize maps a provider's raw status text (or code) to a Status.
func Normalize(family, raw string) Status {
	f := fold(raw)
	if f == "" {
		return Unknown
	}
	if m := exact[family]; m != nil {
		if s, ok := m[f]; ok {
			return s
		}
	}
	return guess(f)
}

// guess handles statuses no table lists yet (ZR's state names, new provider wording).
func guess(f string) Status {
	has := func(words ...string) bool {
		for _, w := range words {
			if strings.Contains(f, w) {
				return true
			}
		}
		return false
	}
	switch {
	case has("retour", "return", "returned", "echec", "failed"):
		return Returned
	case has("annul", "cancel", "abort"):
		return Cancelled
	case has("perdu", "lost"):
		return Lost
	case has("livre", "livree", "delivered", "encaiss", "paid", "payed"):
		if has("en livraison", "sorti", "out for") {
			return OutForDelivery
		}
		return Delivered
	case has("tentative", "attempt", "sans reponse", "no answer", "unreachable"):
		return DeliveryAttempted
	case has("en livraison", "sorti", "out for", "outfordelivery", "dispatched to driver", "with driver"):
		return OutForDelivery
	case has("suspend", "reporte", "postpone", "hold", "attente", "alert", "bloque"):
		return OnHold
	case has("hub", "centre", "center", "agence", "bureau", "desk", "wilaya"):
		return AtHub
	case has("transit", "expedie", "shipped", "vers", "transfer"):
		return InTransit
	case has("ramass", "picked", "pickup", "collect"):
		return PickedUp
	case has("pret", "ready", "confirm", "valid"):
		return PendingPickup
	case has("cree", "created", "prepar", "new", "upload", "pending"):
		return Created
	}
	return Unknown
}
