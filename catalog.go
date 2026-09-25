package dzcouriers

import "sort"

// Driver families: providers in one family share an API.
const (
	FamilyYalidine = "yalidine"
	FamilyEcotrack = "ecotrack"
	FamilyZR       = "zr"
	FamilyNoest    = "noest"
	FamilyProcolis = "procolis"
	FamilyMaystro  = "maystro"
)

// Capabilities of a family.
type Capabilities struct {
	Create, Track, Label, Desks, Cancel, Webhook bool
}

// Provider is one delivery company.
type Provider struct {
	Key      string
	Name     string
	Family   string
	BaseURL  string
	AltURLs  []string // other base URLs seen in the wild; dzcheck ping tries them
	Fields   []string // required credential keys
	Optional []string // optional credential keys
	Caps     Capabilities
}

var familyCaps = map[string]Capabilities{
	FamilyYalidine: {Create: true, Track: true, Label: true, Desks: true, Cancel: true, Webhook: true},
	FamilyEcotrack: {Create: true, Track: true, Label: true, Desks: true, Cancel: true},
	FamilyZR:       {Create: true, Track: true, Label: true, Desks: true, Cancel: true, Webhook: true},
	FamilyNoest:    {Create: true, Track: true, Label: true, Desks: true, Cancel: true},
	FamilyProcolis: {Create: true, Track: true},
	FamilyMaystro:  {Create: true, Track: true, Label: true, Cancel: true},
}

var familyFields = map[string][2][]string{
	FamilyYalidine: {{"api_id", "api_token"}, {"from_wilaya"}},
	FamilyEcotrack: {{"api_token"}, nil},
	FamilyZR:       {{"api_key", "tenant_id"}, nil},
	FamilyNoest:    {{"api_token", "user_guid"}, nil},
	FamilyProcolis: {{"key", "token"}, nil},
	FamilyMaystro:  {{"api_token"}, {"store_id"}},
}

var catalog = map[string]Provider{}

func add(key, name, family, base string, alt ...string) {
	f := familyFields[family]
	catalog[key] = Provider{Key: key, Name: name, Family: family, BaseURL: base, AltURLs: alt,
		Fields: f[0], Optional: f[1], Caps: familyCaps[family]}
}

// ecotrack tenants: key, name, and the subdomain when it is not the key. Sources: CodFlow's
// tenant list (dzship keys) and Vargo's config; where they disagree both URLs are kept.
var ecotrackTenants = [][3]string{
	{"e48hrlivraison", "48Hr Livraison", "48hr"}, {"abdelivery", "AB Delivery", ""},
	{"alania", "Alania Express", ""}, {"allolivraison", "Allo Livraison", ""},
	{"amana", "Amana Speed", ""}, {"andersondelivery", "Anderson Delivery", ""},
	{"aranex", "Aranex", ""}, {"areex", "Areex", ""}, {"assildelivery", "Assil Delivery", ""},
	{"atlasexpress", "Atlas Express", "atlaexpress"}, {"baconsult", "BA Consult", "bacexpress"},
	{"bfkexpress", "BFK Express", ""}, {"boogi", "Boogi Technologie", ""},
	{"championlogistics", "Champion Logistics", "champion"}, {"chronorex", "Chronorex", ""},
	{"cirtaexpress", "Cirta Express", ""}, {"colex", "Colex", ""}, {"colireli", "Colireli", ""},
	{"colizone", "Colizone", ""}, {"conexlog", "Conexlog", "https://app.conexlog-dz.com"},
	{"coyoteexpress", "Coyote Express", "coyoteexpressdz"}, {"delivromail", "Delivromail", ""},
	{"dhd", "DHD Livraison", "https://platform.dhd-dz.com"}, {"distazero", "Distazero", ""},
	{"ecorapideexpress", "Eco Rapide Express", "ecorapide-express"},
	{"elguidedelivery", "El Guide Delivery", ""}, {"expediachrono", "Expedia Chrono", ""},
	{"fasthorse", "Fast Horse Express", ""}, {"fretdirect", "FRET.Direct", "fret"},
	{"fzdelivery", "FZ Delivery", ""}, {"golivri", "GOLIVRI", ""}, {"gsecommerce", "GS Ecommerce", ""},
	{"hhdexpress", "HHD Express", ""}, {"imir", "Imir Logistics", ""}, {"jaguar", "Jaguar Livraison", ""},
	{"joexpress", "Jo Express", ""}, {"lihlihexpress", "LIH LIH Express", ""}, {"lynx", "Lynx Express", ""},
	{"majorex", "Majorex", ""}, {"marsexpress", "Mars Express", ""}, {"mazaya", "Mazaya Logistics", ""},
	{"medexpress", "Med Express", ""}, {"monohub", "Mono Hub", ""}, {"msmgo", "MSM Go", ""},
	{"navexdelivery", "Navex Delivery", ""}, {"negmarexpress", "Negmar Express", ""},
	{"oksbox", "OKS Box", ""}, {"omexpress", "OM Express", ""}, {"ontimeexpress", "On Time Express", "ontime"},
	{"oneexpress", "One Express", ""}, {"ovred", "Ovred", ""}, {"packers", "Packers", ""}, {"pdex", "PDEX", ""},
	{"prest", "Prest", ""}, {"quickdeliverydz", "Quick Delivery DZ", "quickdelivery"},
	{"rblivraison", "RB Livraison", ""}, {"redex", "Red Ex", ""}, {"rexlivraison", "Rex Livraison", "rex"},
	{"rihalexpress", "Rihal Express", ""}, {"rj360express", "RJ 360 Express", ""},
	{"rmexpress", "RM Express", ""}, {"rocketdelivery", "Rocket Delivery", "rocket"},
	{"royaumedelivery", "Royaume Delivery", ""}, {"rsexpress", "RS Express", ""},
	{"rutaexpress", "Ruta Express", ""}, {"salvadelivery", "Salva Delivery", ""}, {"samex", "Samex", ""},
	{"sbl", "SBL Express", ""}, {"siexpress", "SI Express", ""}, {"speeddelivery", "Speed Delivery", ""},
	{"speedmail", "Speed Mail", ""}, {"sultancolisexpress", "Sultan Colis Express", ""},
	{"swiftexpress", "Swift Express", "swift"}, {"tawsilstar", "Tawsil Star", "tawsil"},
	{"tslexpress", "TSL Express", "tsl"}, {"ultraexpress", "Ultra Express", ""},
	{"univerdelivery", "Univer Delivery", ""}, {"vitrans", "Vitrans", ""},
	{"wassimexpress", "Wassim Express", ""}, {"weeweedelivery", "Wee Wee Delivery", ""},
	{"windelivery", "Win Delivery", ""}, {"worldexpress", "WorldExpress", "world-express"}, // worldexpress.ecotrack.dz does not resolve,
	{"zinyatec", "Zinya Tec", ""},
}

func init() {
	add("yalidine", "Yalidine", FamilyYalidine, "https://api.yalidine.app")
	add("guepex", "Guepex", FamilyYalidine, "https://api.guepex.app")
	add("yalitec", "Yalitec", FamilyYalidine, "https://api.yalitec.me")
	add("economiqua", "Economiqua", FamilyYalidine, "https://api.economiqua.app")
	add("easyandspeed", "Easy & Speed", FamilyYalidine, "https://api.easyandspeed.app")
	add("wecan", "WeCan Services", FamilyYalidine, "https://api.wecanservices.me")

	for _, t := range ecotrackTenants {
		key, name, sub := t[0], t[1], t[2]
		pattern := "https://" + key + ".ecotrack.dz"
		switch {
		case sub == "":
			add(key, name, FamilyEcotrack, pattern)
		case len(sub) > 8 && sub[:8] == "https://":
			add(key, name, FamilyEcotrack, sub, pattern)
		default:
			add(key, name, FamilyEcotrack, "https://"+sub+".ecotrack.dz", pattern)
		}
	}

	add("zr", "ZR Express", FamilyZR, "https://api.zrexpress.app")
	add("noest", "NOEST Express", FamilyNoest, "https://app.noest-dz.com")

	add("zrlegacy", "ZR Express (Procolis)", FamilyProcolis, "https://procolis.com/api_v1")
	add("abex", "ABEX", FamilyProcolis, "https://procolis.com/api_v1")
	add("colilog", "Colilog Express", FamilyProcolis, "https://procolis.com/api_v1")
	add("flashdelivery", "Flash Delivery", FamilyProcolis, "https://procolis.com/api_v1")
	add("leopard", "Leopard Express", FamilyProcolis, "https://procolis.com/api_v1")

	add("maystro", "Maystro Delivery", FamilyMaystro, "https://b.maystro-delivery.com/api")
}

// Lookup returns a provider by key.
func Lookup(key string) (Provider, bool) {
	p, ok := catalog[key]
	return p, ok
}

// Providers lists every provider sorted by name.
func Providers() []Provider {
	out := make([]Provider, 0, len(catalog))
	for _, p := range catalog {
		out = append(out, p)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Name < out[j].Name })
	return out
}
