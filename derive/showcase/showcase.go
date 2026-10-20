// Package showcase scores a pokemon for a Biggest Showcase from its height,
// weight and IVs, as the server scores an entry (UnownHash/Golbat#427,
// checked against Pokémon GO 0.433.1 for the standard Biggest/MAX settings).
package showcase

// XXLBonus is the bonus an XXL entry gets: (weightCoefficient * 35/41 +
// ivCoefficient) * xxlAdjustmentFactor with the standard 150, 50 and 1, which
// is exactly 7300/41. It must not be rounded to 178.
const XXLBonus = 7300.0 / 41.0

const (
	heightCoefficient = 800
	weightCoefficient = 150
	ivCoefficient     = 50
)

// Metadata is the species/form data a score is normalized by: the XXL upper
// height bound (m), the average height (m) and weight (kg), and the weight
// standard deviation (kg).
type Metadata struct {
	XXLUpperBound, BaseHeight, BaseWeight, WeightStdDev float64
}

// Denominators are the largest height and weight a score is normalized by.
// Each step is its own float32 operation, as the client computes them: a
// single final cast of a simplified expression gives a different Wmax.
func (m Metadata) Denominators() (hMax, wMax float64) {
	h := float32(m.XXLUpperBound)
	meanH := float32(m.BaseHeight)
	meanW := float32(m.BaseWeight)
	sigma := float32(m.WeightStdDev)

	ratio := h / meanH
	scaled := meanW * ratio
	spread := 4 * sigma
	w := scaled + spread
	return float64(h), float64(w)
}

// Score is the Biggest Showcase score for the height and weight the player
// is shown (float32, as the game reports them) and the IV sum (0-45). The
// result is float64, as the server's score is: round only for display.
func Score(height, weight float32, ivSum int, xxl bool, m Metadata) float64 {
	hMax, wMax := m.Denominators()
	score := heightCoefficient*float64(height)/hMax +
		weightCoefficient*float64(weight)/wMax +
		ivCoefficient*float64(ivSum)/45
	if xxl {
		score += XXLBonus
	}
	return score
}
