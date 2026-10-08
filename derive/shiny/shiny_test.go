package shiny

import (
	"strings"
	"testing"
)

// Computed from the reference implementation and exact, so a failure here is
// a port bug.
func TestKnownRolls(t *testing.T) {
	cases := []struct {
		encounterID uint64
		playerID    string
		want        float64
	}{
		{1, "abc123", 0.9905156493186951},
		{18446744073709551615, "9f1c2e", 0.3631664514541626},
		{12345678901234567890, "PlayerXYZ", 0.9953028559684753},
		{9007199254740993, "p", 0.7121872305870056},
		{4242, "äöü", 0.717978835105896},
	}
	for _, c := range cases {
		if got := float64(Roll(c.encounterID, c.playerID)); got != c.want {
			t.Errorf("Roll(%d, %q) = %v, want %v", c.encounterID, c.playerID, got, c.want)
		}
	}
}

// Player ids longer than the stack buffer take the allocating path and must
// hash identically.
func TestLongPlayerID(t *testing.T) {
	long := strings.Repeat("x", maxStackPlayerID+10)
	if Roll(42, long) != Roll(42, long) {
		t.Fatal("long player id roll is not deterministic")
	}
	edge := strings.Repeat("y", maxStackPlayerID)
	if Roll(42, edge) == Roll(42, edge+"y") {
		t.Fatal("the byte past the stack buffer was not hashed")
	}
}

func TestIsShiny(t *testing.T) {
	// Roll(4242, "äöü") ≈ 0.718, shiny only at odds where 1/odds exceeds it.
	if !IsShiny(4242, "äöü", 1) {
		t.Error("odds of 1 should always be shiny")
	}
	if IsShiny(4242, "äöü", 2) {
		t.Error("0.718 is not below 1/2")
	}
	if IsShiny(4242, "äöü", 0) || IsShiny(4242, "äöü", -5) {
		t.Error("odds of 0 or less should never be shiny")
	}
	if IsShiny(4242, "", 1) {
		t.Error("an empty player id should never be shiny")
	}
}

// Across many encounters the shiny share tracks the odds.
func TestShinyShare(t *testing.T) {
	const n = 200000
	shiny := 0
	for id := uint64(1); id <= n; id++ {
		if IsShiny(id, "player", 64) {
			shiny++
		}
	}
	got := float64(shiny) / n
	if got < 1.0/64*0.85 || got > 1.0/64*1.15 {
		t.Fatalf("shiny share %v, want about %v", got, 1.0/64)
	}
}
