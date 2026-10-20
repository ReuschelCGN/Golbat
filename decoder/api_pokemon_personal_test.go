package decoder

import (
	stdjson "encoding/json"
	"math"
	"strconv"
	"testing"
	"time"

	gojson "github.com/goccy/go-json"
	"github.com/guregu/null/v6"

	"golbat/config"
	"golbat/derive/encounter"
	"golbat/derive/javarandom"
	"golbat/derive/shiny"
)

func TestPersonalSizeVectors(t *testing.T) {
	// Real wild encounters from roundaboutluke (UnownHash/Golbat#423, #424):
	// the player's 32-bit draw at the height index and the next one, the raw
	// second Gaussian deviate, and the height (m) and weight (kg) the game
	// showed.
	for _, v := range []struct {
		class          int32
		draw, next     uint32
		noise          float64
		height, weight float64
		baseH, baseW   float64
		bounds         sizeBounds
	}{
		{3, 3890164944, 917775855, 1.516281, 0.8420, 20.456, 0.7, 12.5, sizeBounds{0.343, 0.35, 0.525, 0.875, 1.05, 1.225}},
		{4, 2539945545, 1892707713, 0.814014, 0.4194, 13.362, 0.3, 6.5, sizeBounds{0.147, 0.15, 0.225, 0.375, 0.45, 0.525}},
		{1, 4081782436, 1664957500, 1.13957, 0.6495, 21.367, 1.3, 54.5, sizeBounds{0.637, 0.65, 0.975, 1.625, 1.95, 2.015}},
		{1, 4165317312, 74602773, -0.534469, 0.1476, 0.543, 0.3, 3.1, sizeBounds{0.147, 0.15, 0.225, 0.375, 0.45, 0.525}},
		{5, 2455997842, 1299456597, -0.286937, 0.8072, 44.198, 0.5, 28, sizeBounds{0.245, 0.25, 0.375, 0.625, 0.75, 0.875}},
		{5, 2975473780, 191867137, -0.137099, 2.1657, 78.019, 1.4, 51, sizeBounds{0.686, 0.7, 1.05, 1.75, 2.1, 2.17}},
	} {
		// nextFloat reads the top 24 of the 32 bits.
		float := func(bits uint32) float64 { return float64(bits>>8) / (1 << 24) }
		e := encounterSize{class: v.class, noise: v.noise, MasterFileSize: MasterFileSize{Height: v.baseH, Weight: v.baseW, Bounds: v.bounds}}
		height, weight := e.at(float(v.draw), func() float64 { return float(v.next) })
		if math.Abs(height-v.height) > 5e-5 || math.Abs(weight-v.weight) > 5e-4 {
			t.Errorf("class %d draw %d: got %.4f m %.3f kg, want %.4f m %.3f kg", v.class, v.draw, height, weight, v.height, v.weight)
		}
	}
}

func TestPersonalSizeBands(t *testing.T) {
	bounds := sizeBounds{0.343, 0.35, 0.525, 0.875, 1.05, 1.225} // Bulbasaur
	base := MasterFileSize{Height: 0.7, Weight: 6.9, Bounds: bounds}
	bands := map[int32][2]float64{
		1: {bounds[0], bounds[1]}, 2: {bounds[1], bounds[2]}, 3: {bounds[2], bounds[3]},
		4: {bounds[3], bounds[4]}, 5: {bounds[4], bounds[5]},
	}
	for class, band := range bands {
		far := 0
		const n = 20000
		for seed := uint64(0); seed < n; seed++ {
			e := encounterSize{class: class, MasterFileSize: base}
			rng := javarandom.New(seed)
			height, weight := e.at(float64(rng.Float()), func() float64 { return float64(rng.Float()) })
			if height < band[0] || height > band[1] {
				t.Fatalf("class %d: height %v outside %v", class, height, band)
			}
			h := height / base.Height
			want := base.Weight * h * h
			if class == 5 {
				want = base.Weight * h
			}
			if math.Abs(weight-want) > 1e-9 {
				t.Fatalf("class %d: weight %v want %v without noise", class, weight, want)
			}
			if class == 1 && height < band[0]+0.2*(band[1]-band[0]) || class == 5 && height > band[0]+0.8*(band[1]-band[0]) {
				far++
			}
		}
		// XXS/XXL land in the far 20% of the band 5% of the time.
		if class == 1 || class == 5 {
			if rate := float64(far) / n; rate < 0.04 || rate > 0.06 {
				t.Errorf("class %d: far-band rate %v, want about 0.05", class, rate)
			}
		}
	}
}

func TestRollRateTable(t *testing.T) {
	zero, five, odds := int32(0), int16(5), int32(64)
	table := newRollRateTable([]ApiRollRate{
		{Pokemon: 1, ShinyOdds: &zero},
		{Pokemon: 2, Form: &five, BackgroundOdds: 20},
		{Pokemon: 2, ShinyOdds: &odds},
	}, nil)
	if r := table.lookup(1, 0); r.ShinyCapable || r.shinyOdds == nil || *r.shinyOdds != 0 {
		t.Errorf("species 1: %+v, want not shiny-capable", r)
	}
	if r := table.lookup(2, 5); !r.ShinyCapable || r.BackgroundOdds != 20 || r.shinyOdds != nil {
		t.Errorf("species 2 form 5: %+v, want its own entry (default shiny odds, background 20)", r)
	}
	if r := table.lookup(2, 7); r.BackgroundOdds != 0 || r.shinyOdds == nil || *r.shinyOdds != 64 {
		t.Errorf("species 2 form 7: %+v, want the species-wide entry", r)
	}
	if r := table.lookup(3, 0); !r.ShinyCapable || r.shinyOdds != nil || r.BackgroundOdds != 0 {
		t.Errorf("unlisted species: %+v, want capable with unknown odds", r)
	}
	def := int32(512)
	if r := newRollRateTable(nil, &def).lookup(3, 0); r.shinyOdds == nil || *r.shinyOdds != 512 {
		t.Errorf("unlisted species with a default: %+v", r)
	}
}

// personalFixture caches wild pokemon whose stored IVs, level and size are
// their own derivation, so their size model applies.
type personalFixture struct {
	t    *testing.T
	lat  float64
	lon  float64
	next int
}

func (f *personalFixture) add(id uint64, species int16, edit func(p *Pokemon)) *Pokemon {
	d := encounter.For(id, false, 0)
	p := &Pokemon{PokemonData: PokemonData{
		Id:        Uint64Str(id),
		Lat:       f.lat + float64(f.next%20)*0.0001,
		Lon:       f.lon + float64(f.next/20)*0.0001,
		PokemonId: species,
		Form:      null.ValueFrom(uint16(0)),
		SeenType:  SeenTypeFrom(SeenTypeCodeWild),
		AtkIv:     null.ValueFrom(uint8(d.Attack)),
		DefIv:     null.ValueFrom(uint8(d.Defense)),
		StaIv:     null.ValueFrom(uint8(d.Stamina)),
		Level:     null.ValueFrom(uint8(d.Level)),
		Size:      null.ValueFrom(uint8(d.Size)),
	}}
	f.next++
	p.ExpireTimestamp = null.ValueFrom(uint32(time.Now().Unix() + 600))
	if edit != nil {
		edit(p)
	}
	pokemonRtreePreloadInsert(p)
	pokemonCache.Set(id, p, time.Minute)
	pLat, pLon := p.Lat, p.Lon
	f.t.Cleanup(func() {
		pokemonCache.Delete(id)
		pokemonLookupCache.Delete(id)
		pokemonTreeMutex.Lock()
		pokemonTree.Delete([2]float64{pLon, pLat}, [2]float64{pLon, pLat}, id)
		pokemonTreeMutex.Unlock()
	})
	return p
}

func (f *personalFixture) scan(filters []ApiPokemonDnfFilter, player string) map[uint64]bool {
	f.t.Helper()
	req := ApiPokemonScan2{
		Min:           ApiLatLon{Lat: f.lat - 0.01, Lon: f.lon - 0.01},
		Max:           ApiLatLon{Lat: f.lat + 0.01, Lon: f.lon + 0.01},
		DnfFilters:    filters,
		ShinyPlayerId: player,
	}
	keys, _, _, _ := internalGetPokemonInArea2(req)
	got := map[uint64]bool{}
	for _, k := range keys {
		got[k] = true
	}
	return got
}

func assertSameIds(t *testing.T, name string, got, want map[uint64]bool) {
	t.Helper()
	if len(want) == 0 {
		t.Fatalf("%s: test data matches nothing, pick other ids", name)
	}
	if len(got) != len(want) {
		t.Errorf("%s: got %d pokemon, want %d", name, len(got), len(want))
	}
	for id := range got {
		if !want[id] {
			t.Errorf("%s: pokemon %d returned but should not match", name, id)
		}
	}
}

const personalPlayer = "test-player"

func newPersonalFixture(t *testing.T) *personalFixture {
	withScanLimits(t)
	withMasterFileData(t, MasterFileData{
		Initialized: true,
		Pokemon: map[int]MasterFilePokemon{
			// Magikarp
			129: {Size: MasterFileSize{Height: 0.9, Weight: 10, Bounds: sizeBounds{0.441, 0.45, 0.675, 1.125, 1.35, 1.8}}},
		},
	})
	pokemonTreeSnapshot.Store(nil)
	t.Cleanup(func() { pokemonTreeSnapshot.Store(nil) })
	return &personalFixture{t: t, lat: 61.25, lon: 81.75}
}

func TestPokemonScan2PersonalConditions(t *testing.T) {
	f := newPersonalFixture(t)
	const base = uint64(960001)
	const count = 300
	for i := 0; i < count; i++ {
		f.add(base+uint64(i), 129, nil)
	}
	// Not roll-eligible: a lure sighting and an undetected disguise.
	lure := f.add(base+1000, 129, func(p *Pokemon) { p.SeenType = SeenTypeFrom(SeenTypeCodeLureWild) })
	disguised := f.add(base+1001, 129, func(p *Pokemon) { p.DisplayPokemonId = null.ValueFrom(uint16(16)) })

	player := shiny.NewPlayer(personalPlayer)
	draws := func(id uint64) *shiny.Draws { d := player.Draws(id); return &d }
	rates := shiny.Rates{ShinyCapable: true}

	// shiny_odds now skips sightings that take no player roll.
	odds := int32(2)
	got := f.scan([]ApiPokemonDnfFilter{{ShinyOdds: &odds}}, personalPlayer)
	want := map[uint64]bool{}
	for i := 0; i < count; i++ {
		if id := base + uint64(i); draws(id).IsShiny(odds) {
			want[id] = true
		}
	}
	assertSameIds(t, "shiny_odds", got, want)
	if got[uint64(lure.Id)] || got[uint64(disguised.Id)] {
		t.Error("an ineligible sighting matched shiny_odds")
	}

	// background_odds reads the second draw for a shiny-capable species.
	bg := int32(3)
	got = f.scan([]ApiPokemonDnfFilter{{BackgroundOdds: &bg}}, personalPlayer)
	want = map[uint64]bool{}
	for i := 0; i < count; i++ {
		if id := base + uint64(i); draws(id).Background(true, bg) {
			want[id] = true
		}
	}
	assertSameIds(t, "background_odds", got, want)

	// player_weight: the Fisher medal's heavy Magikarp.
	minWeight := 13.13
	got = f.scan([]ApiPokemonDnfFilter{{Pokemon: []ApiPokemonDnfId{{Pokemon: 129}}, PlayerWeight: &ApiFloatMinMax{Min: &minWeight}}}, personalPlayer)
	ps := newPersonalScan(personalPlayer, nil, nil)
	want = map[uint64]bool{}
	heights := map[uint64]float32{}
	for i := 0; i < count; i++ {
		id := base + uint64(i)
		d := encounter.For(id, false, 0)
		shared, ok := ps.sharedSize(id, 129, 0, false, d.Attack, d.Defense, d.Stamina, d.Level, d.Size)
		if !ok {
			t.Fatalf("pokemon %d has no size model", id)
		}
		h, w, _ := shared.personal(draws(id), rates)
		heights[id] = h
		if float64(w) >= minWeight {
			want[id] = true
		}
	}
	assertSameIds(t, "player_weight", got, want)

	// player_height with both bounds.
	lo, hi := 0.7, 0.8
	got = f.scan([]ApiPokemonDnfFilter{{PlayerHeight: &ApiFloatMinMax{Min: &lo, Max: &hi}}}, personalPlayer)
	want = map[uint64]bool{}
	for id, h := range heights {
		if float64(h) >= lo && float64(h) <= hi {
			want[id] = true
		}
	}
	assertSameIds(t, "player_height", got, want)

	// Without a player id no per-player condition matches.
	if got := f.scan([]ApiPokemonDnfFilter{{PlayerHeight: &ApiFloatMinMax{Min: &lo}}}, ""); len(got) != 0 {
		t.Errorf("without shiny_player_id got %d pokemon, want 0", len(got))
	}
}

// A record whose stored IVs are not its derivation (here: an encounter that
// disagrees, as a strong or size-event pokemon would) has no size model.
func TestPokemonScan2PlayerSizeNeedsMatchingDerivation(t *testing.T) {
	f := newPersonalFixture(t)
	const id = uint64(961001)
	f.add(id, 129, func(p *Pokemon) {
		p.AtkIv = null.ValueFrom(uint8((p.AtkIv.ValueOrZero() + 1) % 16))
	})
	any := 0.0
	if got := f.scan([]ApiPokemonDnfFilter{{PlayerHeight: &ApiFloatMinMax{Min: &any}}}, personalPlayer); len(got) != 0 {
		t.Error("a record whose IVs are not its derivation matched player_height")
	}
}

func TestPokemonScan2DittoShinyRule(t *testing.T) {
	f := newPersonalFixture(t)
	const base = uint64(962001)
	const count = 1500
	for i := 0; i < count; i++ {
		f.add(base+uint64(i), dittoPokemonId, func(p *Pokemon) {
			p.IsDitto = true
			p.DisplayPokemonId = null.ValueFrom(uint16(16))
		})
	}
	player := shiny.NewPlayer(personalPlayer)
	want := map[uint64]bool{}
	for i := 0; i < count; i++ {
		id := base + uint64(i)
		if d := player.Draws(id); d.DittoShiny() {
			want[id] = true
		}
	}
	// The clause's odds do not matter for a known Ditto.
	odds := int32(1)
	assertSameIds(t, "ditto shiny", f.scan([]ApiPokemonDnfFilter{{ShinyOdds: &odds}}, personalPlayer), want)
	bg := int32(1)
	if got := f.scan([]ApiPokemonDnfFilter{{BackgroundOdds: &bg}}, personalPlayer); len(got) != 0 {
		t.Errorf("background_odds matched %d Dittos, want 0", len(got))
	}
}

func TestPokemonScan2AnnotatePlayer(t *testing.T) {
	f := newPersonalFixture(t)
	prev := config.Config.ShowcaseStdDevFallback
	config.Config.ShowcaseStdDevFallback = true
	t.Cleanup(func() { config.Config.ShowcaseStdDevFallback = prev })

	const base = uint64(963001)
	const count = 200
	for i := 0; i < count; i++ {
		f.add(base+uint64(i), 129, nil)
	}
	f.add(base+500, 25, nil) // no masterfile size data
	f.add(base+501, 129, func(p *Pokemon) { p.SeenType = SeenTypeFrom(SeenTypeCodeNearbyStop) })

	shinyOdds := int32(4)
	req := ApiPokemonScan2{
		Min:            ApiLatLon{Lat: f.lat - 0.01, Lon: f.lon - 0.01},
		Max:            ApiLatLon{Lat: f.lat + 0.01, Lon: f.lon + 0.01},
		DnfFilters:     []ApiPokemonDnfFilter{{}},
		ShinyPlayerId:  personalPlayer,
		RollRates:      []ApiRollRate{{Pokemon: 129, ShinyOdds: &shinyOdds, BackgroundOdds: 3}},
		AnnotatePlayer: true,
	}
	results := GetPokemonInArea2Clean(req)
	if len(results) != count+2 {
		t.Fatalf("got %d results, want %d", len(results), count+2)
	}
	player := shiny.NewPlayer(personalPlayer)
	rates := shiny.Rates{ShinyCapable: true, BackgroundOdds: 3}
	shinies, backgrounds, scored := 0, 0, 0
	for _, r := range results {
		id, err := strconv.ParseUint(r.Id, 10, 64)
		if err != nil {
			t.Fatal(err)
		}
		switch id {
		case base + 500:
			if r.PlayerSize != nil || r.PlayerShiny {
				t.Errorf("species 25 without size data or a known rate: %+v", r)
			}
			continue
		case base + 501:
			if r.PlayerSize != nil || r.PlayerShiny || r.PlayerBackground {
				t.Errorf("a nearby sighting was annotated: %+v", r)
			}
			continue
		}
		d := player.Draws(id)
		if r.PlayerShiny != d.IsShiny(shinyOdds) || r.PlayerBackground != d.Background(true, 3) {
			t.Errorf("pokemon %d: shiny/background annotation disagrees with the draws", id)
		}
		if r.PlayerShiny {
			shinies++
		}
		if r.PlayerBackground {
			backgrounds++
		}
		if r.PlayerSize == nil {
			t.Fatalf("pokemon %d: no player_size", id)
		}
		// The height reads past the background draw, and its card draws on a
		// hit: the annotation agrees with the clause path.
		ps := newPersonalScan(personalPlayer, nil, nil)
		ev := encounter.For(id, false, 0)
		shared, _ := ps.sharedSize(id, 129, 0, false, ev.Attack, ev.Defense, ev.Stamina, ev.Level, ev.Size)
		h, w, _ := shared.personal(&d, rates)
		if r.PlayerSize.Height != h || r.PlayerSize.Weight != w {
			t.Errorf("pokemon %d: player_size %v/%v, want %v/%v", id, r.PlayerSize.Height, r.PlayerSize.Weight, h, w)
		}
		if r.PlayerSize.ShowcaseScore == nil || *r.PlayerSize.ShowcaseScore <= 0 {
			t.Errorf("pokemon %d: no showcase score", id)
		} else {
			scored++
		}
	}
	if shinies == 0 || backgrounds == 0 || scored == 0 {
		t.Errorf("annotations never fired: %d shiny, %d background, %d scored", shinies, backgrounds, scored)
	}

	// Without the fallback, and without masterfile weightStdDev, no score.
	config.Config.ShowcaseStdDevFallback = false
	for _, r := range GetPokemonInArea2Clean(req) {
		if r.PlayerSize != nil && r.PlayerSize.ShowcaseScore != nil {
			t.Fatal("a showcase score without weightStdDev or the fallback")
		}
	}

	// annotate_player off leaves results bare.
	req.AnnotatePlayer = false
	for _, r := range GetPokemonInArea2Clean(req) {
		if r.PlayerSize != nil || r.PlayerShiny || r.PlayerBackground {
			t.Fatal("annotated without annotate_player")
		}
	}
}

// ApiPokemonResultV2 serializes flat: the shared fields at the top level, the
// player fields beside them and omitted when unset, under both codecs huma
// may use.
func TestApiPokemonResultV2JSONIsFlat(t *testing.T) {
	score := 812.5
	annotated := ApiPokemonResultV2{
		ApiPokemonResult: ApiPokemonResult{Id: "42", PokemonId: 129},
		PlayerShiny:      true,
		PlayerSize:       &ApiPlayerSize{Height: 1.5, Weight: 14, ShowcaseScore: &score},
	}
	for name, marshal := range map[string]func(any) ([]byte, error){
		"encoding/json": stdjson.Marshal,
		"go-json":       gojson.Marshal,
	} {
		for _, c := range []struct {
			r          ApiPokemonResultV2
			wantPlayer bool
		}{{annotated, true}, {ApiPokemonResultV2{ApiPokemonResult: annotated.ApiPokemonResult}, false}} {
			raw, err := marshal(c.r)
			if err != nil {
				t.Fatal(err)
			}
			var got map[string]any
			if err := stdjson.Unmarshal(raw, &got); err != nil {
				t.Fatal(err)
			}
			if got["id"] != "42" || got["pokemon_id"] != float64(129) {
				t.Errorf("%s: shared fields not at the top level: %s", name, raw)
			}
			_, hasShiny := got["player_shiny"]
			_, hasSize := got["player_size"]
			_, hasBackground := got["player_background"]
			if hasShiny != c.wantPlayer || hasSize != c.wantPlayer || hasBackground {
				t.Errorf("%s: player fields wrong (want present=%v): %s", name, c.wantPlayer, raw)
			}
		}
	}
}
