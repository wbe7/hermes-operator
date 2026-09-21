package main

import (
	"encoding/json"
	"fmt"
	api "github.com/wbe7/hermes-operator/api/v1alpha1"
	"github.com/wbe7/hermes-operator/internal/network"
	"github.com/wbe7/hermes-operator/internal/settings"
	"os"
)

func main() {
	if len(os.Args) != 4 {
		panic("usage: inputCR networkConfig output")
	}
	b, e := os.ReadFile(os.Args[1])
	must(e)
	var h api.Hermes
	must(json.Unmarshal(b, &h))
	b, e = os.ReadFile(os.Args[2])
	must(e)
	cfg, e := settings.DecodeNetworkConfig(b)
	must(e)
	p, e := network.Build(&h, cfg)
	must(e)
	p.APIVersion = "networking.k8s.io/v1"
	p.Kind = "NetworkPolicy"
	b, e = json.MarshalIndent(p, "", "  ")
	must(e)
	must(os.WriteFile(os.Args[3], b, 0600))
	fmt.Println("policy compiled")
}
func must(e error) {
	if e != nil {
		panic(e)
	}
}
