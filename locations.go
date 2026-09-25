package dzcouriers

// wilayas holds the Latin names providers match on (Yalidine spelling), codes 1..58.
// Codes 59..69 are not included yet: pass Shipment.Wilaya explicitly for them.
var wilayas = [...]string{"",
	"Adrar", "Chlef", "Laghouat", "Oum El Bouaghi", "Batna", "Béjaïa", "Biskra", "Béchar", "Blida", "Bouira",
	"Tamanrasset", "Tébessa", "Tlemcen", "Tiaret", "Tizi Ouzou", "Alger", "Djelfa", "Jijel", "Sétif", "Saïda",
	"Skikda", "Sidi Bel Abbès", "Annaba", "Guelma", "Constantine", "Médéa", "Mostaganem", "M'Sila", "Mascara", "Ouargla",
	"Oran", "El Bayadh", "Illizi", "Bordj Bou Arreridj", "Boumerdès", "El Tarf", "Tindouf", "Tissemsilt", "El Oued", "Khenchela",
	"Souk Ahras", "Tipaza", "Mila", "Aïn Defla", "Naâma", "Aïn Témouchent", "Ghardaïa", "Relizane", "Timimoun", "Bordj Badji Mokhtar",
	"Ouled Djellal", "Béni Abbès", "In Salah", "In Guezzam", "Touggourt", "Djanet", "El M'Ghair", "El Meniaa",
}

// WilayaName returns the provider-facing name for a wilaya code, or "".
func WilayaName(code int) string {
	if code < 1 || code >= len(wilayas) {
		return ""
	}
	return wilayas[code]
}

// WilayaCode finds a wilaya by name, ignoring case and accents. It returns 0 when unknown.
func WilayaCode(name string) int {
	f := fold(name)
	for i := 1; i < len(wilayas); i++ {
		if fold(wilayas[i]) == f {
			return i
		}
	}
	return 0
}
