package showcase

import (
	"math"
	"testing"
)

// Fixtures from UnownHash/Golbat#427: actual game dimensions (not predicted)
// against the server's score. For these, the weight deviation is baseW/8.
func TestScoreFixtures(t *testing.T) {
	for _, f := range []struct {
		name               string
		xxl                bool
		baseH, baseW, hMax float64
		height, weight     float32
		ivSum              int
		want               float64
	}{
		{"Misdreavus", true, .71, 1, 1.2425, 1.0916918516159058, 1.753897786140442, 18, 1017.8755153878249},
		{"Patrat", true, .5, 11.6, .775, .7692975997924805, 17.890289306640625, 32, 1120.5667727529465},
		{"Drifloon", false, .4, 1.2, .8, .5331827402114868, 2.131565570831299, 30, 673.0943441413433},
	} {
		m := Metadata{XXLUpperBound: f.hMax, BaseHeight: f.baseH, BaseWeight: f.baseW, WeightStdDev: f.baseW / 8}
		if got := Score(f.height, f.weight, f.ivSum, f.xxl, m); math.Abs(got-f.want) > 1e-10 {
			t.Errorf("%s: score %.13f, want %.13f", f.name, got, f.want)
		}
	}
}

func TestPatratDenominators(t *testing.T) {
	hMax, wMax := Metadata{XXLUpperBound: .775, BaseHeight: .5, BaseWeight: 11.6, WeightStdDev: 11.6 / 8}.Denominators()
	if hMax != 0.7749999761581421 || wMax != 23.779998779296875 {
		t.Fatalf("denominators %v %v, want 0.7749999761581421 23.779998779296875", hMax, wMax)
	}
}

func TestXXLBonusIsNotRounded(t *testing.T) {
	if b := float64(XXLBonus); b == 178 || math.Abs(b-178.0487804878049) > 1e-12 {
		t.Fatalf("XXLBonus = %v", b)
	}
}
