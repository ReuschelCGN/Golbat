package decoder

import (
	"testing"
	"time"

	"github.com/guregu/null/v6"

	"golbat/derive/shiny"
)

// The shiny_odds clause condition keeps exactly the pokemon shiny for
// shiny_player_id, per species odds, and never matches without a player id.
func TestPokemonScan2ShinyOdds(t *testing.T) {
	withScanLimits(t)
	const lat, lon = 56.25, 78.75
	const count = 400
	const player = "test-player"

	ids := make([]uint64, 0, count)
	for i := 0; i < count; i++ {
		id := uint64(950001 + i)
		species := int16(25)
		if i%5 == 0 {
			species = 1
		}
		p := &Pokemon{PokemonData: PokemonData{
			Id:        Uint64Str(id),
			Lat:       lat + float64(i%20)*0.0001,
			Lon:       lon + float64(i/20)*0.0001,
			PokemonId: species,
			Form:      null.ValueFrom(uint16(0)),
		}}
		p.ExpireTimestamp = null.ValueFrom(uint32(time.Now().Unix() + 600))
		pokemonRtreePreloadInsert(p)
		pokemonCache.Set(id, p, time.Minute)
		ids = append(ids, id)
		pLat, pLon := p.Lat, p.Lon
		t.Cleanup(func() {
			pokemonCache.Delete(id)
			pokemonLookupCache.Delete(id)
			pokemonTreeMutex.Lock()
			pokemonTree.Delete([2]float64{pLon, pLat}, [2]float64{pLon, pLat}, id)
			pokemonTreeMutex.Unlock()
		})
	}
	pokemonTreeSnapshot.Store(nil)
	t.Cleanup(func() { pokemonTreeSnapshot.Store(nil) })

	never, odds := int32(0), int32(4)
	req := ApiPokemonScan2{
		Min: ApiLatLon{Lat: lat - 0.01, Lon: lon - 0.01},
		Max: ApiLatLon{Lat: lat + 0.01, Lon: lon + 0.01},
		DnfFilters: []ApiPokemonDnfFilter{
			// species 1 has no released shiny
			{Pokemon: []ApiPokemonDnfId{{Pokemon: 1}}, ShinyOdds: &never},
			// every other species at 1 in 4
			{ShinyOdds: &odds},
		},
		ShinyPlayerId: player,
	}

	want := map[uint64]bool{}
	for i, id := range ids {
		if i%5 != 0 && shiny.IsShiny(id, player, odds) {
			want[id] = true
		}
	}
	if len(want) == 0 {
		t.Fatal("test data has no shinies, pick other ids")
	}

	keys, _, _, _ := internalGetPokemonInArea2(req)
	if len(keys) != len(want) {
		t.Fatalf("got %d pokemon, want %d", len(keys), len(want))
	}
	for _, id := range keys {
		if !want[id] {
			t.Errorf("pokemon %d returned but is not shiny for %q", id, player)
		}
	}

	req.ShinyPlayerId = ""
	if keys, _, _, _ := internalGetPokemonInArea2(req); len(keys) != 0 {
		t.Errorf("without shiny_player_id got %d pokemon, want 0", len(keys))
	}
}
