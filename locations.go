package dzcouriers

// wilayas holds the Latin names providers match on (Yalidine spelling), codes 1..69
// (59..69 are the wilayas created in 2025). Providers that have not added the new
// ones yet reject them; pass Shipment.Wilaya to override the name.
var wilayas = [...]string{"",
	"Adrar", "Chlef", "Laghouat", "Oum El Bouaghi", "Batna", "Béjaïa", "Biskra", "Béchar", "Blida", "Bouira",
	"Tamanrasset", "Tébessa", "Tlemcen", "Tiaret", "Tizi Ouzou", "Alger", "Djelfa", "Jijel", "Sétif", "Saïda",
	"Skikda", "Sidi Bel Abbès", "Annaba", "Guelma", "Constantine", "Médéa", "Mostaganem", "M'Sila", "Mascara", "Ouargla",
	"Oran", "El Bayadh", "Illizi", "Bordj Bou Arreridj", "Boumerdès", "El Tarf", "Tindouf", "Tissemsilt", "El Oued", "Khenchela",
	"Souk Ahras", "Tipaza", "Mila", "Aïn Defla", "Naâma", "Aïn Témouchent", "Ghardaïa", "Relizane", "Timimoun", "Bordj Badji Mokhtar",
	"Ouled Djellal", "Béni Abbès", "In Salah", "In Guezzam", "Touggourt", "Djanet", "El M'Ghair", "El Meniaa",
	"Aflou", "Barika", "El Kantara", "Bir El Ater", "El Aricha", "Ksar Chellala", "Aïn Oussara", "Messaad", "Ksar El Boukhari", "Bou Saâda",
	"El Abiodh Sidi Cheikh",
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
