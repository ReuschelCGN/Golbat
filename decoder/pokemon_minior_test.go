package decoder

import (
	"context"
	"encoding/json"
	"reflect"
	"testing"
	"time"

	"golbat/db"
	"golbat/pogo"
	"golbat/webhooks"

	"github.com/guregu/null/v6"
)

// miniorTestPokemon is a wild Minior as a sighting leaves it: identity,
// location and despawn, no encounter stats.
func miniorTestPokemon() *Pokemon {
	p := &Pokemon{PokemonData: PokemonData{
		Id: 977401, PokemonId: int16(pogo.HoloPokemonId_MINIOR),
		Lat: 48.85, Lon: 8.2, SpawnId: null.ValueFrom(int64(1234)),
		ExpireTimestamp:         null.ValueFrom(uint32(time.Now().Unix() + 600)),
		ExpireTimestampVerified: true,
		Weather:                 null.ValueFrom(uint8(1)), Form: null.ValueFrom(uint16(2707)),
		Gender: null.ValueFrom(uint8(3)), Costume: null.ValueFrom(uint8(0)),
		IsStrong: null.ValueFrom(false), SeenType: SeenTypeFrom(SeenTypeCodeWild),
	}, newRecord: true}
	p.snapshotOldValues()
	return p
}

func miniorTestEncounter(species pogo.HoloPokemonId, form pogo.PokemonDisplayProto_Form) *pogo.PokemonProto {
	return &pogo.PokemonProto{
		PokemonId: species, Cp: 200, CpMultiplier: .5974,
		IndividualAttack: 10, IndividualDefense: 11, IndividualStamina: 12,
		Move1: 1, Move2: 2, HeightM: 1.5, WeightKg: 25, Size: 3,
		PokemonDisplay: &pogo.PokemonDisplayProto{Form: form, Shiny: true},
	}
}

func assertMiniorStatsUnset(t *testing.T, p *Pokemon) {
	t.Helper()
	if p.AtkIv.Valid || p.DefIv.Valid || p.StaIv.Valid || p.Iv.Valid ||
		p.Cp.Valid || p.Level.Valid || p.Move1.Valid || p.Move2.Valid ||
		p.Height.Valid || p.Weight.Valid || p.Size.Valid || p.Shiny.Valid || p.Pvp.Valid {
		t.Errorf("Minior was given encounter stats: %+v", p.PokemonData)
	}
	if len(p.scanHistory) != 0 {
		t.Error("Minior recorded encounter scan history")
	}
}

func TestMiniorEncounterSetsNoStats(t *testing.T) {
	for _, form := range []pogo.PokemonDisplayProto_Form{
		0, pogo.PokemonDisplayProto_MINIOR_METEOR_BLUE, pogo.PokemonDisplayProto_MINIOR_BLUE,
		pogo.PokemonDisplayProto_MINIOR_METEOR_YELLOW, pogo.PokemonDisplayProto_MINIOR_YELLOW,
	} {
		t.Run(form.String(), func(t *testing.T) {
			p := miniorTestPokemon()
			p.SetForm(null.IntFrom(int64(form)))
			p.addEncounterPokemon(context.Background(), db.DbDetails{}, miniorTestEncounter(pogo.HoloPokemonId_MINIOR, form), "tester", priorSighting{})
			assertMiniorStatsUnset(t, p)
			if p.PokemonId != int16(pogo.HoloPokemonId_MINIOR) || p.Form.ValueOrZero() != uint16(form) {
				t.Error("Minior identity/form changed")
			}
		})
	}
}

// A weather flip restores nothing, even from a scan history an older build
// recorded.
func TestMiniorWeatherFlipSetsNoStats(t *testing.T) {
	p := miniorTestPokemon()
	p.scanHistory = []*pokemonScan{{Pokemon: int32(pogo.HoloPokemonId_MINIOR), Weather: 1, Level: 20, Attack: 15, Defense: 15, Stamina: 15}}
	p.repopulateIv(int64(pogo.GameplayWeatherProto_NONE), false)
	if p.AtkIv.Valid || p.Level.Valid || p.Cp.Valid {
		t.Fatalf("weather flip set Minior stats: %+v", p.PokemonData)
	}
}

func TestMiniorLureAndTappableEncounters(t *testing.T) {
	lureTestSetup(t)
	ctx := context.Background()
	encounter := miniorTestEncounter(pogo.HoloPokemonId_MINIOR, pogo.PokemonDisplayProto_MINIOR_BLUE)
	p := miniorTestPokemon()
	p.updatePokemonFromDiskEncounterProto(ctx, db.DbDetails{}, &pogo.DiskEncounterOutProto{Pokemon: encounter}, "tester")
	assertMiniorStatsUnset(t, p)
	if p.SeenType.Code != SeenTypeCodeLureEncounter || p.PokemonId != int16(pogo.HoloPokemonId_MINIOR) {
		t.Fatal("lure encounter lost its identity/source")
	}
	p = miniorTestPokemon()
	p.updatePokemonFromTappableEncounterProto(ctx, db.DbDetails{},
		&pogo.ProcessTappableProto{LocationHintLat: 48.86, LocationHintLng: 8.21},
		&pogo.TappableEncounterProto{Pokemon: encounter}, "tester", time.Now().UnixMilli())
	assertMiniorStatsUnset(t, p)
	if p.Lat != 48.86 || p.Lon != 8.21 || p.PokemonId != int16(pogo.HoloPokemonId_MINIOR) {
		t.Fatal("tappable encounter lost its location/identity")
	}
}

// An encountered Minior is still published, as a spawn without IVs.
func TestMiniorEncounterPublishesANoIvSpawn(t *testing.T) {
	sink := lureTestSetup(t)
	p := miniorTestPokemon()
	p.addEncounterPokemon(context.Background(), db.DbDetails{}, miniorTestEncounter(pogo.HoloPokemonId_MINIOR, 0), "tester", priorSighting{})
	savePokemonRecordAsAtTime(context.Background(), db.DbDetails{}, p, false, false, true, time.Now().Unix(), "tester")
	msgs := sink.drain()
	if len(msgs) != 1 || msgs[0].whType != webhooks.PokemonNoIV {
		t.Fatalf("webhooks = %+v, want one no-IV spawn webhook", msgs)
	}
	raw, err := json.Marshal(msgs[0].message)
	if err != nil {
		t.Fatal(err)
	}
	var payload map[string]any
	if err := json.Unmarshal(raw, &payload); err != nil {
		t.Fatal(err)
	}
	if payload["pokemon_id"] != float64(pogo.HoloPokemonId_MINIOR) || payload["latitude"] != p.Lat || payload["longitude"] != p.Lon {
		t.Error("spawn missing from webhook")
	}
	for _, key := range []string{"individual_attack", "cp", "pokemon_level", "shiny"} {
		if payload[key] != nil {
			t.Errorf("webhook %s = %v, want null", key, payload[key])
		}
	}
}

func TestOtherSpeciesKeepsEncounterStats(t *testing.T) {
	p := &Pokemon{PokemonData: PokemonData{PokemonId: 25, Weather: null.ValueFrom(uint8(1))}, newRecord: true}
	p.addEncounterPokemon(context.Background(), db.DbDetails{}, miniorTestEncounter(pogo.HoloPokemonId_PIKACHU, 0), "tester", priorSighting{})
	if !p.AtkIv.Valid || p.AtkIv.ValueOrZero() != 10 || !p.DefIv.Valid || !p.StaIv.Valid ||
		!p.Iv.Valid || !p.Cp.Valid || !p.Level.Valid || !p.Move1.Valid || !p.Move2.Valid ||
		!p.Height.Valid || !p.Weight.Valid || !p.Size.Valid || !p.Shiny.Valid || len(p.scanHistory) == 0 {
		t.Fatalf("ordinary encounter lost stats: %+v", p.PokemonData)
	}
	before := p.PokemonData
	p.ClearDirty()
	p.repopulateIv(int64(pogo.GameplayWeatherProto_CLEAR), false)
	if !reflect.DeepEqual(before, p.PokemonData) {
		t.Fatal("ordinary unchanged-weather encounter data changed")
	}
}

// A wild Minior sighting is never derived: its encounter values are rolled
// per player, so the encounter id's stream describes nobody's Minior.
func TestMiniorSightingIsNotDerived(t *testing.T) {
	p := &Pokemon{}
	p.Id = Uint64Str(8429844873227918432)
	p.PokemonId = int16(pogo.HoloPokemonId_MINIOR)
	p.SetWeather(null.IntFrom(int64(pogo.GameplayWeatherProto_NONE)))
	p.SetSeenType(SeenTypeCodeWild)

	p.deriveIvIfAbsent()
	p.deriveSizeAndMovesIfAbsent()

	if p.AtkIv.Valid || p.Level.Valid || p.Size.Valid || p.Move1.Valid {
		t.Fatalf("Minior was derived: %+v", p.PokemonData)
	}
}

// A record that changes species into a Minior drops the IVs and level the
// previous species had: neither derivation nor a weather flip replaces them
// for Minior, so they would be published as the Minior's.
func TestSpeciesChangeToMiniorDropsStats(t *testing.T) {
	p := &Pokemon{PokemonData: PokemonData{
		Id: 977402, PokemonId: 25, Form: null.ValueFrom(uint16(0)),
		Gender: null.ValueFrom(uint8(1)), Costume: null.ValueFrom(uint8(0)),
		IsStrong: null.ValueFrom(false), SeenType: SeenTypeFrom(SeenTypeCodeWild),
		Weather: null.ValueFrom(uint8(0)),
		AtkIv:   null.ValueFrom(uint8(15)), DefIv: null.ValueFrom(uint8(15)),
		StaIv: null.ValueFrom(uint8(15)), Iv: null.ValueFrom(float32(100)),
		Level: null.ValueFrom(uint8(30)), Cp: null.ValueFrom(uint16(500)),
	}}
	p.snapshotOldValues()
	p.setPokemonDisplay(int16(pogo.HoloPokemonId_MINIOR),
		&pogo.PokemonDisplayProto{Form: pogo.PokemonDisplayProto_MINIOR_BLUE, Gender: 3}, SeenTypeCodeWild)
	assertMiniorStatsUnset(t, p)
	if p.PokemonId != int16(pogo.HoloPokemonId_MINIOR) {
		t.Fatalf("species is %d, want Minior", p.PokemonId)
	}
}
