package shiny

import (
	"testing"
)

// Sylvie's vectors from UnownHash/Golbat#423: the first two draws of the
// player's generator, the shiny and background rolls of a shiny-capable
// species. Exact, so a failure is a port bug.
var playerVectors = []struct {
	playerID          string
	encounterID       uint64
	shiny, background float64
}{
	{"0123456789abcdef", 1, 0.22137850522994995, 0.1431974172592163},
	{"Player-ID:ABC/123", 1, 0.3684830665588379, 0.9563266038894653},
	{" Trainer-é ", 1, 0.03772324323654175, 0.7365694642066956},
}

func TestPlayerVectors(t *testing.T) {
	for _, v := range playerVectors {
		d := NewPlayer(v.playerID).Draws(v.encounterID)
		if got := d.float(0); got != v.shiny {
			t.Errorf("%q: shiny draw %v, want %v", v.playerID, got, v.shiny)
		}
		if got := d.float(1); got != v.background {
			t.Errorf("%q: background draw %v, want %v", v.playerID, got, v.background)
		}
		// Roll is the same first draw.
		if got := float64(Roll(v.encounterID, v.playerID)); got != v.shiny {
			t.Errorf("%q: Roll %v, want %v", v.playerID, got, v.shiny)
		}
	}
}

func TestDrawsIsShinyMatchesIsShiny(t *testing.T) {
	p := NewPlayer("abc123")
	for id := uint64(0); id < 5000; id++ {
		d := p.Draws(id)
		for _, odds := range []int32{0, 2, 4, 64} {
			if d.IsShiny(odds) != IsShiny(id, "abc123", odds) {
				t.Fatalf("encounter %d odds %d: Draws.IsShiny disagrees with IsShiny", id, odds)
			}
		}
	}
}

// The background draw is the second for a shiny-capable species and the
// first otherwise; the vectors' thresholds tell them apart.
func TestBackgroundDrawPosition(t *testing.T) {
	d := NewPlayer("0123456789abcdef").Draws(1) // shiny 0.221, background 0.143
	// 1 in 6 (0.1667): the second draw is below, the first is not.
	if !d.Background(true, 6) {
		t.Error("capable: the second draw (0.143) should be below 1/6")
	}
	if d.Background(false, 6) {
		t.Error("not capable: the first draw (0.221) should not be below 1/6")
	}
	if d.Background(true, 0) {
		t.Error("odds 0 offers no background")
	}
}

// Height continues after exactly the draws the rates call for: none, the
// shiny draw, the background draw, and the two card draws after a hit.
func TestHeightPosition(t *testing.T) {
	p := NewPlayer("0123456789abcdef")
	ref := p.Draws(1)
	for _, c := range []struct {
		name  string
		rates Rates
		index int
	}{
		{"no draws", Rates{}, 0},
		{"shiny only", Rates{ShinyCapable: true}, 1},
		{"background missed", Rates{ShinyCapable: true, BackgroundOdds: 10}, 2}, // 0.143 >= 0.1
		{"background hit", Rates{ShinyCapable: true, BackgroundOdds: 2}, 4},     // 0.143 < 0.5
		{"background only, hit", Rates{BackgroundOdds: 2}, 3},                   // 0.221 < 0.5
	} {
		d := p.Draws(1)
		u, selector, ok := d.Height(c.rates)
		if !ok || u != ref.float(c.index) || selector() != ref.float(c.index+1) {
			t.Errorf("%s: height draws are not draws #%d and #%d", c.name, c.index, c.index+1)
		}
	}
}

// Readings share one hash and agree whatever order they are made in.
func TestDrawsOrderIndependent(t *testing.T) {
	p := NewPlayer("abc123")
	rates := Rates{ShinyCapable: true, BackgroundOdds: 3}
	for id := uint64(0); id < 2000; id++ {
		a := p.Draws(id)
		ua, _, _ := a.Height(rates)
		sa, ba := a.IsShiny(4), a.Background(true, 3)
		b := p.Draws(id)
		sb, bb := b.IsShiny(4), b.Background(true, 3)
		ub, _, _ := b.Height(rates)
		if ua != ub || sa != sb || ba != bb {
			t.Fatalf("encounter %d: readings depend on order", id)
		}
	}
}

func TestDittoShinyIsTheThirdDraw(t *testing.T) {
	p := NewPlayer("abc123")
	hits := 0
	for id := uint64(0); id < 64000; id++ {
		d := p.Draws(id)
		want := d.float(2) < 1.0/64
		if d.DittoShiny() != want {
			t.Fatalf("encounter %d: DittoShiny disagrees with the third draw", id)
		}
		if want {
			hits++
		}
	}
	if hits < 850 || hits > 1150 {
		t.Errorf("%d Ditto shinies in 64000, want about 1000", hits)
	}
}

func TestNoPlayerNeverMatches(t *testing.T) {
	p := NewPlayer("")
	if p != nil {
		t.Fatal("an empty player id should give a nil Player")
	}
	d := p.Draws(1)
	if _, _, ok := d.Height(Rates{}); ok || d.IsShiny(1) || d.DittoShiny() || d.Background(true, 1) {
		t.Fatal("Draws without a player matched")
	}
}
