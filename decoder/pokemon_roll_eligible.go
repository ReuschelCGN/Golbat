package decoder

import (
	"math"

	"golbat/pogo"
)

const dittoPokemonId = int16(pogo.HoloPokemonId_DITTO)

// rollEligible packs whether a player's per-encounter rolls (shiny,
// background, height) describe this pokemon, and its costume, into
// PokemonLookup.RollEligible. 0 means never: anything but a wild or
// encountered sighting (a lure, nearby or tappable sighting has its own
// encounter id), a disguise not known as a Ditto, an undisguised species 132
// that Golbat has not identified, or costume 255. Otherwise costume+1. A
// known Ditto is 1: its rule ignores costume, and an eligible lookup with
// PokemonId 132 is always a known Ditto (see isKnownDitto). Rules from
// UnownHash/Golbat#423 (personalLookup).
func rollEligible(pokemon *Pokemon) uint8 {
	if pokemon.SeenType != SeenTypeFrom(SeenTypeCodeWild) && pokemon.SeenType != SeenTypeFrom(SeenTypeCodeEncounter) {
		return 0
	}
	if isKnownDitto(pokemon) {
		return 1
	}
	costume := pokemon.Costume.ValueOrZero()
	if pokemon.IsDitto || pokemon.PokemonId == dittoPokemonId || costume == math.MaxUint8 ||
		pokemon.DisplayPokemonId.ValueOrZero() != 0 && int16(pokemon.DisplayPokemonId.ValueOrZero()) != pokemon.PokemonId {
		return 0
	}
	return uint8(costume) + 1
}

// isKnownDitto reports whether Golbat has identified the record as a Ditto:
// setDittoAttributes sets IsDitto and species 132 together, whether from
// detectDitto's scan evidence or the encounter-id roll, and
// resetDittoAttributes clears both. Until then a disguised Ditto looks like
// its disguise and takes that species' rolls; nothing can tell it apart.
func isKnownDitto(pokemon *Pokemon) bool {
	return pokemon.IsDitto && pokemon.PokemonId == dittoPokemonId
}
