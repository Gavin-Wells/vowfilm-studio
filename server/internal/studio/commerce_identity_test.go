package studio

import "testing"

func TestCommerceUsesIdentityChainWhenMultipleUnits(t *testing.T) {
	p := &Project{
		Scene:          "commerce",
		Duration:       60,
		GenerationMode: commerceDirectModeFor(60),
		Shots: []Shot{
			{ID: "S01", GenerateUnit: true, GenerateGroup: "G01", Duration: 12},
			{ID: "S04", GenerateUnit: true, GenerateGroup: "G02", Duration: 12},
		},
	}
	if !usesGenerateIdentityChain(p) {
		t.Fatal("multi-unit commerce should chain identity references")
	}
	if commerceAnchorLeaderID(p) != "S01" {
		t.Fatal("anchor leader should be first generate unit")
	}
	single := &Project{
		Scene:          "commerce",
		Duration:       15,
		GenerationMode: commerceDirectModeFor(15),
		Shots:          []Shot{{ID: "S01", GenerateUnit: true, Duration: 15}},
	}
	if usesGenerateIdentityChain(single) {
		t.Fatal("single-unit commerce should not require identity chain")
	}
}
