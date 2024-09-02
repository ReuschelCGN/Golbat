package decoder

import (
	"context"
	"golbat/db"
	"golbat/pogo"
	"time"

	"github.com/jellydator/ttlcache/v3"
	"gopkg.in/guregu/null.v4"
)

type Station struct {
	Id                string
	Name              string
	Lat               float64
	Lon               float64
	StartTime         int64
	EndTime           int64
	CooldownComplete  int64
	IsBattleAvailable bool
	IsInactive        bool

	BattleLevel            null.Int
	BattlePokemonId        null.Int
	BattlePokemonForm      null.Int
	BattlePokemonCostume   null.Int
	BattlePokemonGender    null.Int
	BattlePokemonAlignment null.Int
	Updated                int64
	//TODO
}

func getStationRecord(ctx context.Context, db db.DbDetails, stationId string) (*Station, error) {
	inMemoryStation := stationCache.Get(stationId)
	if inMemoryStation != nil {
		station := inMemoryStation.Value()
		return &station, nil
	}
	station := Station{}
	//TODO
	return &station, nil
}

func saveStationRecord(ctx context.Context, db db.DbDetails, station *Station) {
	oldStation, _ := getStationRecord(ctx, db, station.Id)
	now := time.Now().Unix()
	if oldStation != nil && !hasChangesStation(oldStation, station) {
		if oldStation.Updated > now-900 {
			// if a gym is unchanged, but we did see it again after 15 minutes, then save again
			return
		}
	}

	station.Updated = now

	//TODO db magic

	stationCache.Set(station.Id, *station, ttlcache.DefaultTTL)
	createStationWebhooks(oldStation, station)

}

// hasChangesStation compares two Station structs
// Float tolerance: Lat, Lon
func hasChangesStation(old *Station, new *Station) bool {
	return old.Id != new.Id ||
		old.Name != new.Name ||
		old.StartTime != new.StartTime ||
		old.EndTime != new.EndTime ||
		old.CooldownComplete != new.CooldownComplete ||
		!floatAlmostEqual(old.Lat, new.Lat, floatTolerance) ||
		!floatAlmostEqual(old.Lon, new.Lon, floatTolerance)
}

func (station *Station) updateFromStationProto(stationProto *pogo.StationProto) *Station {
	station.Id = stationProto.Id
	station.Name = stationProto.Name
	station.Lat = stationProto.Lat
	station.Lon = stationProto.Lng
	station.StartTime = stationProto.StartTimeMs
	station.EndTime = stationProto.EndTimeMs
	station.CooldownComplete = stationProto.CooldownCompleteMs
	station.IsBattleAvailable = stationProto.IsBreadBattleAvailable
	if battleDetails := stationProto.BattleDetails; battleDetails != nil {
		station.BattleLevel = null.IntFrom(int64(battleDetails.BattleLevel))
		if pokemon := battleDetails.BattlePokemon; pokemon != nil {
			station.BattlePokemonId = null.IntFrom(int64(pokemon.PokemonId))
			station.BattlePokemonForm = null.IntFrom(int64(pokemon.PokemonDisplay.Form))
			station.BattlePokemonCostume = null.IntFrom(int64(pokemon.PokemonDisplay.Costume))
			station.BattlePokemonGender = null.IntFrom(int64(pokemon.PokemonDisplay.Gender))
			station.BattlePokemonAlignment = null.IntFrom(int64(pokemon.PokemonDisplay.Alignment))
		}
	}
	return station
}

func createStationWebhooks(oldStation *Station, station *Station) {
	//TODO we need to define webhooks, are they needed for stations, or only for battles?
}
