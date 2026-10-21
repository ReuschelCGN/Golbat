package shiny

import (
	"crypto/sha1"
	"encoding/binary"

	"golbat/derive/javarandom"
)

// drawScale is the range of one draw: nextFloat's 24 bits.
const drawScale = 1 << 24

// dittoOdds is a known Ditto's shiny rate, 1 in 64, rolled on the third draw
// whatever the disguise's rates (see Draws.DittoShiny).
const dittoOdds = 64

// maxDraws is the most draws any reading takes: shiny, background, two
// location-card draws, the height and the XXS/XXL band selector.
const maxDraws = 6

// Rates are what the game needs to know about a species to walk a player's
// draws for one of its encounters: whether the shiny draw is taken at all,
// and the background rate, 1 in BackgroundOdds (0 = no background on offer,
// and no background draw).
type Rates struct {
	ShinyCapable   bool
	BackgroundOdds int32
}

// Player rolls encounters for one raw player id. It reuses one hash input
// buffer, so a Player is not safe for concurrent use; build one per request.
type Player struct {
	input []byte
}

// NewPlayer prepares the hash input for playerID. An empty id gives a nil
// Player, whose Draws never match anything.
func NewPlayer(playerID string) *Player {
	if playerID == "" {
		return nil
	}
	input := make([]byte, 8+len(playerID))
	copy(input[8:], playerID)
	return &Player{input: input}
}

// Draws hashes the encounter for the player (one SHA-1 block for ids up to 47
// bytes) and returns its draws, read lazily.
func (p *Player) Draws(encounterID uint64) Draws {
	if p == nil {
		return Draws{}
	}
	binary.LittleEndian.PutUint64(p.input[:8], encounterID)
	sum := sha1.Sum(p.input)
	return Draws{rng: javarandom.New(binary.LittleEndian.Uint64(sum[:8]))}
}

// Draws are one player's nextFloat draws for one encounter, kept as their
// 24-bit integers. The game reads them in this order (Sylvie's model on
// UnownHash/Golbat#423, extended by #424), taking each only when it applies:
//
//  1. the shiny draw, when the species can be shiny;
//  2. the background draw, when a background is on offer;
//  3. two location-card draws, when the background hit;
//  4. the height draw, then for XXS and XXL the band selector.
//
// So which index a reading is at depends on the species' Rates. The zero
// Draws (no player) never matches.
type Draws struct {
	rng *javarandom.Random
	d   [maxDraws]uint32
	n   int
}

// at is draw i, drawing up to it on first use.
func (d *Draws) at(i int) uint32 {
	for d.n <= i {
		d.d[d.n] = d.rng.Next(24)
		d.n++
	}
	return d.d[i]
}

// IsShiny reports whether a shiny-capable species is shiny at 1 in odds: the
// first draw, as Roll. odds of 0 or less never are.
func (d *Draws) IsShiny(odds int32) bool {
	return d.rng != nil && below(d.at(0), odds)
}

// Background reports whether the encounter offers the player a background at
// 1 in odds: the draw after the shiny draw, so the first when the species
// cannot be shiny.
func (d *Draws) Background(shinyCapable bool, odds int32) bool {
	if d.rng == nil || odds <= 0 {
		return false
	}
	return below(d.at(backgroundIndex(shinyCapable)), odds)
}

// DittoShiny reports whether a known Ditto is shiny for the player: the third
// draw below 1 in 64. The game takes all three draws for a Ditto whatever the
// disguise's rates; the first two are the disguise's shiny and background
// rolls, which never apply (a disguise is never shiny). Validated on 124
// Dittos in UnownHash/Golbat#423.
func (d *Draws) DittoShiny() bool {
	return d.rng != nil && below(d.at(2), dittoOdds)
}

// Height returns the height draw and a reader for the band selector after
// it, as nextFloat values, for a species with these rates. ok is false
// without a player.
func (d *Draws) Height(rates Rates) (u float64, selector func() float64, ok bool) {
	if d.rng == nil {
		return 0, nil, false
	}
	i := backgroundIndex(rates.ShinyCapable)
	if rates.BackgroundOdds > 0 {
		hit := below(d.at(i), rates.BackgroundOdds)
		i++
		if hit {
			i += 2
		}
	}
	return d.float(i), func() float64 { return d.float(i + 1) }, true
}

func (d *Draws) float(i int) float64 {
	return float64(d.at(i)) / drawScale
}

func backgroundIndex(shinyCapable bool) int {
	if shinyCapable {
		return 1
	}
	return 0
}

// below is draw/2^24 < 1/odds, the comparison IsShiny makes on Roll's float.
// odds of 0 or less never are.
func below(draw uint32, odds int32) bool {
	return odds > 0 && float64(draw)/drawScale < 1/float64(odds)
}
