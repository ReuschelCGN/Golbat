package decoder

import (
	"golbat/geo"
)

type ApiPokemonScan2 struct {
	Min           ApiLatLon             `json:"min" doc:"Lower-left (minimum lat/lon) corner of the bounding box to scan."`
	Max           ApiLatLon             `json:"max" doc:"Upper-right (maximum lat/lon) corner of the bounding box to scan."`
	Limit         int                   `json:"limit" required:"false" doc:"Maximum number of results to return; 0 uses the server default."`
	DnfFilters    []ApiPokemonDnfFilter `json:"filters" required:"false" doc:"List of filter clauses OR'd together; a pokemon matches if it satisfies any one clause."`
	ShinyPlayerId string                `json:"shiny_player_id" required:"false" doc:"Raw player id the per-player conditions (shiny_odds, background_odds, player_height, player_weight) and annotate_player are rolled for. Without it those conditions never match and nothing is annotated."`
	// RollRates and DefaultShinyOdds place the player's draws: which draw a
	// background or height is depends on whether the species takes the
	// shiny and background draws at all.
	RollRates        []ApiRollRate `json:"roll_rates" required:"false" doc:"Shiny and background rates of the species that differ from the defaults (default_shiny_odds, no background). They decide which of the player's draws background_odds, player_height and player_weight read, and the annotate_player values."`
	DefaultShinyOdds *int32        `json:"default_shiny_odds" required:"false" doc:"Shiny rate, 1 in this many, of species not in roll_rates. null treats them as shiny-capable without annotating player_shiny for them."`
	AnnotatePlayer   bool          `json:"annotate_player" required:"false" doc:"Add the player's readings to each result: player_shiny, player_background and player_size."`
}

func (r ApiPokemonScan2) GetMin() geo.Location {
	return r.Min.Location()
}

func (r ApiPokemonScan2) GetMax() geo.Location {
	return r.Max.Location()
}

func (r ApiPokemonScan2) GetLimit() int {
	return r.Limit
}

type ApiPokemonDnfFilter struct {
	Pokemon []ApiPokemonDnfId    `json:"pokemon" required:"false" doc:"Pokemon/form ids this clause applies to; empty matches any pokemon. All other conditions in the clause are AND'd together."`
	Iv      *ApiPokemonDnfMinMax `json:"iv" required:"false" doc:"Inclusive IV percentage range; null means no IV constraint."`
	AtkIv   *ApiPokemonDnfMinMax `json:"atk_iv" required:"false" doc:"Inclusive attack IV range; null means no attack IV constraint."`
	DefIv   *ApiPokemonDnfMinMax `json:"def_iv" required:"false" doc:"Inclusive defense IV range; null means no defense IV constraint."`
	StaIv   *ApiPokemonDnfMinMax `json:"sta_iv" required:"false" doc:"Inclusive stamina IV range; null means no stamina IV constraint."`
	Level   *ApiPokemonDnfMinMax `json:"level" required:"false" doc:"Inclusive level range; null means no level constraint."`
	Cp      *ApiPokemonDnfMinMax `json:"cp" required:"false" doc:"Inclusive CP range; null means no CP constraint."`
	Gender  *ApiPokemonDnfMinMax `json:"gender" required:"false" doc:"Inclusive gender value range; null means no gender constraint."`
	Size    *ApiPokemonDnfMinMax `json:"size" required:"false" doc:"Inclusive size range; null means no size constraint."`
	Little  *ApiPokemonDnfMinMax `json:"pvp_little" required:"false" doc:"Inclusive Little League PVP rank range; null means no Little League constraint."`
	Great   *ApiPokemonDnfMinMax `json:"pvp_great" required:"false" doc:"Inclusive Great League PVP rank range; null means no Great League constraint."`
	Ultra   *ApiPokemonDnfMinMax `json:"pvp_ultra" required:"false" doc:"Inclusive Ultra League PVP rank range; null means no Ultra League constraint."`
	// ShinyOdds is evaluated last: it hashes the encounter id, the other
	// conditions are field compares.
	ShinyOdds *int32 `json:"shiny_odds" required:"false" doc:"Only pokemon shiny for shiny_player_id, rolled from the encounter id, when the species is shiny at 1 in this many; 0 never matches (no released shiny); null means no shiny constraint. A known Ditto is rolled by the Ditto rule (third draw, 1 in 64) whatever the odds."`
	// The conditions below read the player's later draws, whose positions
	// come from roll_rates; like shiny_odds they are evaluated last.
	BackgroundOdds *int32          `json:"background_odds" required:"false" doc:"Only pokemon offering shiny_player_id a background at 1 in this many; 0 never matches; null means no background constraint. Never matches a Ditto."`
	PlayerHeight   *ApiFloatMinMax `json:"player_height" required:"false" doc:"Inclusive range of the height (m) shiny_player_id is shown; null means no constraint. Never matches a pokemon without a size model (see player_size)."`
	PlayerWeight   *ApiFloatMinMax `json:"player_weight" required:"false" doc:"Inclusive range of the weight (kg) shiny_player_id is shown; null means no constraint. Never matches a pokemon without a size model (see player_size)."`
}

func (r ApiPokemonScan2) personalScan() *personalScan {
	return newPersonalScan(r.ShinyPlayerId, r.RollRates, r.DefaultShinyOdds)
}

func internalGetPokemonInArea2(retrieveParameters ApiPokemonScan2) ([]uint64, int, int, int) {
	return internalGetPokemonInArea2With(retrieveParameters, retrieveParameters.personalScan())
}

func internalGetPokemonInArea2With(retrieveParameters ApiPokemonScan2, personal *personalScan) ([]uint64, int, int, int) {
	dnfFilters := make(map[dnfFilterLookup][]ApiPokemonDnfFilter)

	for _, filter := range retrieveParameters.DnfFilters {
		if len(filter.Pokemon) > 0 {
			for _, keyString := range filter.Pokemon {
				pokemonId := keyString.Pokemon
				if pokemonId == 0 {
					pokemonId = -1
				}
				var formId int16 = -1
				if keyString.Form != nil {
					formId = *keyString.Form
				}
				key := dnfFilterLookup{
					pokemon: pokemonId,
					form:    formId,
				}
				dnfFilters[key] = append(dnfFilters[key], filter)
			}
		} else {
			key := dnfFilterLookup{
				pokemon: -1,
				form:    -1,
			}
			dnfFilters[key] = append(dnfFilters[key], filter)
		}
	}

	isPokemonDnfMatch := func(pokemonId uint64, pokemonLookup *PokemonLookup, pvpLookup *PokemonPvpLookup, filter *ApiPokemonDnfFilter) bool {
		if filter.Iv != nil && (int16(pokemonLookup.Iv) < filter.Iv.Min || int16(pokemonLookup.Iv) > filter.Iv.Max) ||
			filter.StaIv != nil && (int16(pokemonLookup.Sta) < filter.StaIv.Min || int16(pokemonLookup.Sta) > filter.StaIv.Max) ||
			filter.AtkIv != nil && (int16(pokemonLookup.Atk) < filter.AtkIv.Min || int16(pokemonLookup.Atk) > filter.AtkIv.Max) ||
			filter.DefIv != nil && (int16(pokemonLookup.Def) < filter.DefIv.Min || int16(pokemonLookup.Def) > filter.DefIv.Max) ||
			filter.Level != nil && (int16(pokemonLookup.Level) < filter.Level.Min || int16(pokemonLookup.Level) > filter.Level.Max) ||
			filter.Cp != nil && (pokemonLookup.Cp < filter.Cp.Min || pokemonLookup.Cp > filter.Cp.Max) ||
			filter.Gender != nil && (int16(pokemonLookup.Gender) < filter.Gender.Min || int16(pokemonLookup.Gender) > filter.Gender.Max) ||
			filter.Size != nil && (int16(pokemonLookup.Size) < filter.Size.Min || int16(pokemonLookup.Size) > filter.Size.Max) {
			return false
		}

		if filter.Little != nil && (pvpLookup == nil || pvpLookup.Little < filter.Little.Min || pvpLookup.Little > filter.Little.Max) ||
			filter.Great != nil && (pvpLookup == nil || pvpLookup.Great < filter.Great.Min || pvpLookup.Great > filter.Great.Max) ||
			filter.Ultra != nil && (pvpLookup == nil || pvpLookup.Ultra < filter.Ultra.Min || pvpLookup.Ultra > filter.Ultra.Max) {
			return false
		}

		return personal.matchClause(pokemonId, pokemonLookup, filter)
	}

	return internalGetPokemonInArea[ApiPokemonDnfFilter](retrieveParameters, dnfFilters, isPokemonDnfMatch)
}
