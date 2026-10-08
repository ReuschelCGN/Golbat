// Package shiny answers whether a wild encounter is shiny for one player,
// given its encounter id and that player's raw player id.
package shiny

import (
	"crypto/sha1"
	"encoding/binary"

	"golbat/derive/javarandom"
)

// maxStackPlayerID is the longest player id hashed without a heap allocation.
const maxStackPlayerID = 64

// Roll is the encounter's shiny roll for one player: the first
// java.util.Random nextFloat, seeded by the low 8 bytes (little endian) of
// SHA-1(encounter id as little endian uint64 || player id as UTF-8). The
// encounter is shiny for that player when Roll is below the species' rate.
// Pure: same inputs, same output.
func Roll(encounterID uint64, playerID string) float32 {
	var sum [sha1.Size]byte
	if len(playerID) <= maxStackPlayerID {
		var in [8 + maxStackPlayerID]byte
		binary.LittleEndian.PutUint64(in[:8], encounterID)
		n := copy(in[8:], playerID)
		sum = sha1.Sum(in[:8+n])
	} else {
		in := make([]byte, 8+len(playerID))
		binary.LittleEndian.PutUint64(in[:8], encounterID)
		copy(in[8:], playerID)
		sum = sha1.Sum(in)
	}
	return javarandom.New(binary.LittleEndian.Uint64(sum[:8])).Float()
}

// IsShiny reports whether the encounter is shiny for the player when the
// species is shiny at 1 in odds. Odds of 0 or less never are, which is how a
// species without a released shiny is expressed. An empty player id never is.
func IsShiny(encounterID uint64, playerID string, odds int32) bool {
	if odds <= 0 || playerID == "" {
		return false
	}
	return float64(Roll(encounterID, playerID)) < 1/float64(odds)
}
