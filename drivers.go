package dzcouriers

var drivers = map[string]driver{
	FamilyYalidine: yalidine{},
	FamilyEcotrack: ecotrack{},
	FamilyZR:       zr{},
	FamilyNoest:    noest{},
	FamilyProcolis: procolis{},
	FamilyMaystro:  maystro{},
}
