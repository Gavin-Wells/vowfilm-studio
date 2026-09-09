package platform

import "errors"

const MaxMoney int64 = 1000000000000

var Actions = []string{"plan", "review", "generate", "shot", "render"}
var Scenes = []string{"wedding", "family", "anniversary", "commerce"}

func DefaultPricing() Pricing {
	return Pricing{Version: 1, Rules: []Rule{{"plan", "call", 0, 1000, 0}, {"review", "call", 0, 1000, 0}, {"generate", "task", 0, 10000, 0}, {"shot", "second", 0, 200, 0}, {"render", "task", 0, 1000, 0}}, SceneFactors: map[string]int64{"wedding": 10000, "family": 10000, "anniversary": 10000, "commerce": 10000}}
}
func ValidatePricing(p Pricing) error {
	if len(p.Rules) != len(Actions) || len(p.SceneFactors) != len(Scenes) {
		return errors.New("价格表必须覆盖五种动作和四种场景")
	}
	seen := map[string]bool{}
	for _, r := range p.Rules {
		valid := false
		for _, a := range Actions {
			if a == r.Action {
				valid = true
			}
		}
		if !valid || seen[r.Action] {
			return errors.New("计费动作无效或重复")
		}
		seen[r.Action] = true
		switch r.Unit {
		case "call", "task", "second", "shot":
		default:
			return errors.New("计费单位无效")
		}
		if r.Base < 0 || r.Rate < 0 || r.Minimum < 0 || r.Base > 100000000 || r.Rate > 100000000 || r.Minimum > 100000000 {
			return errors.New("价格范围为 0–100000 积分")
		}
	}
	for _, s := range Scenes {
		f, ok := p.SceneFactors[s]
		if !ok || f < 0 || f > 100000 {
			return errors.New("场景系数范围为 0–10 倍")
		}
	}
	return nil
}

// Price uses bounded integer arithmetic; never binary floating point for balances.
func Price(r Rule, quantity, factor int64) (int64, error) {
	if quantity < 1 || quantity > 100000 || factor < 0 || factor > 100000 || r.Base < 0 || r.Rate < 0 || r.Minimum < 0 || r.Base > 100000000 || r.Rate > 100000000 || r.Minimum > 100000000 {
		return 0, errors.New("报价参数超出范围")
	}
	raw := r.Base + r.Rate*quantity
	amount := (raw*factor + 9999) / 10000
	if amount < r.Minimum {
		amount = r.Minimum
	}
	if amount > MaxMoney {
		return 0, errors.New("报价金额超出上限")
	}
	return amount, nil
}
func RuleFor(p Pricing, action string) (Rule, error) {
	for _, r := range p.Rules {
		if r.Action == action {
			return r, nil
		}
	}
	return Rule{}, errors.New("不支持的计费动作")
}
