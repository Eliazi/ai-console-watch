package usage

import "regexp"

// Price is USD per 1M tokens (Anthropic list).
type Price struct {
	In, Out, CacheWrite, CacheRead float64
}

type priceRule struct {
	re    *regexp.Regexp
	price Price
}

var priceRules = []priceRule{
	{regexp.MustCompile(`(?i)opus`), Price{In: 15, Out: 75, CacheWrite: 18.75, CacheRead: 1.5}},
	{regexp.MustCompile(`(?i)sonnet-4|sonnet4|sonnet-5|sonnet5`), Price{In: 3, Out: 15, CacheWrite: 3.75, CacheRead: 0.3}},
	{regexp.MustCompile(`(?i)haiku-4|haiku4`), Price{In: 1, Out: 5, CacheWrite: 1.25, CacheRead: 0.1}},
	{regexp.MustCompile(`(?i)3-7-sonnet|3\.7`), Price{In: 3, Out: 15, CacheWrite: 3.75, CacheRead: 0.3}},
	{regexp.MustCompile(`(?i)3-5-haiku`), Price{In: 0.8, Out: 4, CacheWrite: 1, CacheRead: 0.08}},
}

var defaultPrice = Price{In: 3, Out: 15, CacheWrite: 3.75, CacheRead: 0.3}

func PriceFor(model string) Price {
	for _, r := range priceRules {
		if r.re.MatchString(model) {
			return r.price
		}
	}
	return defaultPrice
}

func (p Price) CostUSD(input, output, cacheWrite, cacheRead int64) float64 {
	return (float64(input)*p.In +
		float64(output)*p.Out +
		float64(cacheWrite)*p.CacheWrite +
		float64(cacheRead)*p.CacheRead) / 1e6
}
