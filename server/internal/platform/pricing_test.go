package platform

import "testing"

func TestPriceRoundingMinimumAndBounds(t *testing.T) {
	cases := []struct {
		name       string
		rule       Rule
		q, f, want int64
	}{{"ceiling", Rule{Base: 1, Rate: 1}, 1, 3333, 1}, {"minimum", Rule{Rate: 1, Minimum: 500}, 2, 10000, 500}, {"quantity", Rule{Base: 1000, Rate: 200}, 30, 12500, 8750}, {"free", Rule{}, 1, 10000, 0}}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			v, e := Price(c.rule, c.q, c.f)
			if e != nil || v != c.want {
				t.Fatalf("got %d, %v; want %d", v, e, c.want)
			}
		})
	}
	for _, c := range []struct {
		r    Rule
		q, f int64
	}{{Rule{Rate: -1}, 1, 10000}, {Rule{Rate: 100000001}, 1, 10000}, {Rule{}, 0, 10000}, {Rule{}, 1, 100001}, {Rule{Rate: 100000000}, 100000, 100000}} {
		if _, e := Price(c.r, c.q, c.f); e == nil {
			t.Fatal("invalid or overflowing price accepted")
		}
	}
}
func TestPricingVersionShape(t *testing.T) {
	p := DefaultPricing()
	if err := ValidatePricing(p); err != nil {
		t.Fatal(err)
	}
	p.Rules[1].Action = p.Rules[0].Action
	if ValidatePricing(p) == nil {
		t.Fatal("duplicate action accepted")
	}
}
