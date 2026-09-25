// Command wasmcheck exists so CI can prove the core package builds under TinyGo/WASI.
package main

import "github.com/ZaouiAmine/dzcouriers"

func main() {
	_ = dzcouriers.Normalize(dzcouriers.FamilyYalidine, "Livré")
	_ = len(dzcouriers.Providers())
}
