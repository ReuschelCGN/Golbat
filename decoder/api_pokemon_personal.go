package decoder

import (
	"strconv"

	"golbat/config"
	"golbat/derive/encounter"
	"golbat/derive/shiny"
	"golbat/derive/showcase"
	"golbat/pogo"
)

// Per-player readings of an encounter: whether it is shiny or offers a
// background for one player, and the height and weight that player is shown.
// They come from the player's draws (derive/shiny), whose positions depend on
// species rates the request carries, so Golbat holds no rate table.

const (
	pumpkabooPokemonId = int16(pogo.HoloPokemonId_PUMPKABOO)
	gourgeistPokemonId = int16(pogo.HoloPokemonId_GOURGEIST)
	miniorPokemonId    = int16(pogo.HoloPokemonId_MINIOR)
)

// ApiRollRate is a species' shiny and background rates, which decide both
// the player's readings and which draw each one is at. A species not listed
// takes default_shiny_odds and has no background on offer.
type ApiRollRate struct {
	Pokemon        int16  `json:"id" doc:"Pokedex id."`
	Form           *int16 `json:"form" required:"false" doc:"Form id; null applies to every form of the species without its own entry."`
	ShinyOdds      *int32 `json:"shiny_odds" required:"false" doc:"Shiny rate, 1 in this many; 0 means the species cannot be shiny, so the game takes no shiny draw. null takes default_shiny_odds."`
	BackgroundOdds int32  `json:"background_odds" required:"false" doc:"Background rate, 1 in this many; 0 (default) means no background is on offer and no background draw is taken."`
}

// ApiFloatMinMax is an inclusive range whose omitted bound is unbounded.
type ApiFloatMinMax struct {
	Min *float64 `json:"min" required:"false" doc:"Minimum value (inclusive); omitted is unbounded."`
	Max *float64 `json:"max" required:"false" doc:"Maximum value (inclusive); omitted is unbounded."`
}

func (r *ApiFloatMinMax) contains(v float64) bool {
	return (r.Min == nil || v >= *r.Min) && (r.Max == nil || v <= *r.Max)
}

// ApiPlayerSize is the height and weight one player is shown for an
// encounter, and that entry's Biggest Showcase score.
type ApiPlayerSize struct {
	Height        float32  `json:"height" doc:"Height (m) the player is shown"`
	Weight        float32  `json:"weight" doc:"Weight (kg) the player is shown"`
	ShowcaseScore *float64 `json:"showcase_score,omitempty" doc:"Biggest Showcase score for this height, weight and IVs; absent when the species has no scoring data. Full precision: round only for display."`
}

// rollRate is a species' resolved rates. shinyOdds is nil when neither the
// species' entry nor the request says, in which case the species is walked as
// shiny-capable (the clause shiny_odds contract) but not annotated as shiny.
type rollRate struct {
	shiny.Rates
	shinyOdds *int32
}

// rollRateTable resolves a species' rates: exact form, then the species-wide
// entry, then the request default.
type rollRateTable struct {
	entries     map[dnfFilterLookup]ApiRollRate
	defaultOdds *int32
}

func newRollRateTable(rates []ApiRollRate, defaultOdds *int32) rollRateTable {
	table := rollRateTable{defaultOdds: defaultOdds}
	if len(rates) > 0 {
		table.entries = make(map[dnfFilterLookup]ApiRollRate, len(rates))
	}
	for _, r := range rates {
		key := dnfFilterLookup{pokemon: r.Pokemon, form: -1}
		if r.Form != nil {
			key.form = *r.Form
		}
		table.entries[key] = r
	}
	return table
}

func (t rollRateTable) lookup(pokemon, form int16) rollRate {
	entry, ok := t.entries[dnfFilterLookup{pokemon: pokemon, form: form}]
	if !ok {
		entry, ok = t.entries[dnfFilterLookup{pokemon: pokemon, form: -1}]
	}
	odds := t.defaultOdds
	if ok && entry.ShinyOdds != nil {
		odds = entry.ShinyOdds
	}
	return rollRate{
		Rates: shiny.Rates{
			ShinyCapable:   odds == nil || *odds > 0,
			BackgroundOdds: entry.BackgroundOdds,
		},
		shinyOdds: odds,
	}
}

// personalScan is one request's player and rates. It remembers the last
// candidate's draws, so a candidate is hashed at most once however many
// clauses read it. Used by one goroutine (the scan walk, then annotation).
type personalScan struct {
	player     *shiny.Player
	rates      rollRateTable
	masterFile MasterFileData

	last      uint64
	lastValid bool
	draws     shiny.Draws
}

func newPersonalScan(playerID string, rates []ApiRollRate, defaultShinyOdds *int32) *personalScan {
	return &personalScan{
		player:     shiny.NewPlayer(playerID),
		rates:      newRollRateTable(rates, defaultShinyOdds),
		masterFile: getMasterFileData(),
	}
}

func (s *personalScan) drawsFor(id uint64) *shiny.Draws {
	if !s.lastValid || s.last != id {
		s.draws = s.player.Draws(id)
		s.last, s.lastValid = id, true
	}
	return &s.draws
}

// matchClause applies a clause's per-player conditions. They are checked
// after its field compares, so only candidates that got that far are hashed.
// A pokemon that takes no player rolls (RollEligible 0) matches none of them.
func (s *personalScan) matchClause(id uint64, lookup *PokemonLookup, filter *ApiPokemonDnfFilter) bool {
	if filter.ShinyOdds == nil && filter.BackgroundOdds == nil && filter.PlayerHeight == nil && filter.PlayerWeight == nil {
		return true
	}
	if s.player == nil || lookup.RollEligible == 0 {
		return false
	}
	draws := s.drawsFor(id)
	if lookup.PokemonId == dittoPokemonId {
		// A known Ditto: the Ditto shiny rule, whatever the clause's odds,
		// and never a background or a modelled size.
		return filter.BackgroundOdds == nil && filter.PlayerHeight == nil && filter.PlayerWeight == nil &&
			draws.DittoShiny()
	}
	rates := s.rates.lookup(lookup.PokemonId, lookup.Form)
	if filter.ShinyOdds != nil && !(rates.ShinyCapable && draws.IsShiny(*filter.ShinyOdds)) {
		return false
	}
	if filter.BackgroundOdds != nil && !draws.Background(rates.ShinyCapable, *filter.BackgroundOdds) {
		return false
	}
	if filter.PlayerHeight != nil || filter.PlayerWeight != nil {
		shared, ok := s.sharedSizeFromLookup(id, lookup)
		if !ok {
			return false
		}
		height, weight, ok := shared.personal(draws, rates.Rates)
		if !ok ||
			filter.PlayerHeight != nil && !filter.PlayerHeight.contains(float64(height)) ||
			filter.PlayerWeight != nil && !filter.PlayerWeight.contains(float64(weight)) {
			return false
		}
	}
	return true
}

func (s *personalScan) sharedSizeFromLookup(id uint64, lookup *PokemonLookup) (encounterSize, bool) {
	return s.sharedSize(id, lookup.PokemonId, lookup.Form, lookup.Weather > 0, int32(lookup.Atk), int32(lookup.Def), int32(lookup.Sta), int32(lookup.Level), int32(lookup.Size))
}

// annotate adds the player's readings to an already-built result, so it
// needs no record lock. A result that takes no player rolls gets none; a
// known Ditto gets only PlayerShiny, since its size draws follow Ditto's own,
// which are not modelled.
func (s *personalScan) annotate(r *ApiPokemonResultV2) {
	if s.player == nil {
		return
	}
	id, err := strconv.ParseUint(r.Id, 10, 64)
	if err != nil {
		return
	}
	pokemon := Pokemon{}
	pokemon.PokemonId = r.PokemonId
	pokemon.IsDitto = r.IsDitto
	if r.Costume != nil {
		pokemon.Costume.SetValid(*r.Costume)
	}
	if r.DisplayPokemonId != nil {
		pokemon.DisplayPokemonId.SetValid(*r.DisplayPokemonId)
	}
	if r.SeenType != nil {
		pokemon.SeenType, _ = ParseSeenType(*r.SeenType)
	}
	if rollEligible(&pokemon) == 0 {
		return
	}
	draws := s.drawsFor(id)
	if isKnownDitto(&pokemon) {
		r.PlayerShiny = draws.DittoShiny()
		return
	}
	var form int16
	if r.Form != nil {
		form = int16(*r.Form)
	}
	rates := s.rates.lookup(r.PokemonId, form)
	if rates.shinyOdds != nil {
		r.PlayerShiny = draws.IsShiny(*rates.shinyOdds)
	}
	r.PlayerBackground = draws.Background(rates.ShinyCapable, rates.BackgroundOdds)

	shared, ok := s.sharedSize(id, r.PokemonId, form, r.Weather != nil && *r.Weather != 0,
		int32OrMinus(r.AtkIv), int32OrMinus(r.DefIv), int32OrMinus(r.StaIv), int32OrMinus(r.Level), int32OrMinus(r.Size))
	if !ok {
		return
	}
	height, weight, ok := shared.personal(draws, rates.Rates)
	if !ok {
		return
	}
	size := &ApiPlayerSize{Height: height, Weight: weight}
	if meta, ok := shared.showcaseMetadata(); ok {
		score := showcase.Score(height, weight, int(shared.ivSum), shared.class == encounter.SizeXXL, meta)
		size.ShowcaseScore = &score
	}
	r.PlayerSize = size
}

func int32OrMinus(v *uint8) int32 {
	if v == nil {
		return -1
	}
	return int32(*v)
}

// encounterSize is the player-independent part of a size: the class and
// weight noise from the encounter's own generator, and the masterfile data.
type encounterSize struct {
	class int32
	noise float64
	ivSum int32
	MasterFileSize
}

// sharedSize derives the class and weight noise the way the encounter's IVs
// are derived, checking the derivation against the IVs, level and size class
// already known (a size event, a strong or a mis-boosted record fails it).
// ok is false without masterfile size data, for species without a size
// model, or when the derivation does not describe this pokemon. A missing
// value is -1, which never matches.
func (s *personalScan) sharedSize(id uint64, pokemonId, form int16, boosted bool, atk, def, sta, level, sizeClass int32) (encounterSize, bool) {
	// Pumpkaboo and Gourgeist have no size model: their form is the size.
	// Minior's encounter values are per player (derivableSpecies).
	if pokemonId == pumpkabooPokemonId || pokemonId == gourgeistPokemonId || pokemonId == miniorPokemonId ||
		atk < 0 || def < 0 || sta < 0 || level < 0 {
		return encounterSize{}, false
	}
	size, ok := sizeFor(s.masterFile, pokemonId, form)
	if !ok {
		return encounterSize{}, false
	}
	draw := encounter.For(id, boosted, int32(form))
	if draw.Attack != atk || draw.Defense != def || draw.Stamina != sta || draw.Level != level ||
		sizeClass > 0 && draw.Size != sizeClass {
		return encounterSize{}, false
	}
	return encounterSize{
		class:          draw.Size,
		noise:          draw.WeightNoise,
		ivSum:          draw.Attack + draw.Defense + draw.Stamina,
		MasterFileSize: size,
	}, true
}

// personal continues the player's draws at the height draw (after the shiny,
// background and card draws the rates call for) and returns the height and
// weight that player is shown, as the game reports them.
func (e encounterSize) personal(draws *shiny.Draws, rates shiny.Rates) (height, weight float32, ok bool) {
	u, selector, ok := draws.Height(rates)
	if !ok {
		return 0, 0, false
	}
	h, w := e.at(u, selector)
	return float32(h), float32(w), true
}

// at places the height at u inside the class's bounds. XXS and XXL read one
// more draw, the selector: below 0.05 it picks the far 20% of the band (away
// from M), otherwise the near 80%. Model and vectors: roundaboutluke on
// UnownHash/Golbat#423, ported from #424.
func (e encounterSize) at(u float64, selector func() float64) (height, weight float64) {
	b := e.Bounds
	var lo, hi float64
	switch e.class {
	case encounter.SizeXXS:
		lo, hi = b[0], b[1]
		if split := b[1] - 0.8*(b[1]-b[0]); selector() < 0.05 {
			hi = split
		} else {
			lo = split
		}
	case encounter.SizeXS:
		lo, hi = b[1], b[2]
	case encounter.SizeXL:
		lo, hi = b[3], b[4]
	case encounter.SizeXXL:
		lo, hi = b[4], b[5]
		if split := b[4] + 0.8*(b[5]-b[4]); selector() < 0.05 {
			lo = split
		} else {
			hi = split
		}
	default:
		lo, hi = b[2], b[3]
	}
	height = lo + (hi-lo)*u
	h := height / e.Height
	ratio := h * h
	if e.class == encounter.SizeXXL {
		ratio = h
	}
	if withNoise := ratio + 0.125*max(-4, min(4, e.noise)); withNoise > 0 {
		ratio = withNoise
	}
	return height, e.Weight * ratio
}

// showcaseMetadata is the species' Biggest Showcase normalization data. The
// weight deviation is the masterfile's when it has one, else baseWeight/8
// with showcase_stddev_fallback on; otherwise there is no score.
func (e encounterSize) showcaseMetadata() (showcase.Metadata, bool) {
	var stdDev float64
	switch {
	case e.WeightStdDev != nil:
		stdDev = *e.WeightStdDev
	case config.Config.ShowcaseStdDevFallback:
		stdDev = e.Weight / 8
	default:
		return showcase.Metadata{}, false
	}
	if stdDev < 0 {
		return showcase.Metadata{}, false
	}
	return showcase.Metadata{
		XXLUpperBound: e.Bounds[5],
		BaseHeight:    e.Height,
		BaseWeight:    e.Weight,
		WeightStdDev:  stdDev,
	}, true
}
