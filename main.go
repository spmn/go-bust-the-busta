package main

import (
	"fmt"
	"os"
	"path/filepath"
	"time"

	dem "github.com/markus-wa/demoinfocs-golang/v2/pkg/demoinfocs"
	common "github.com/markus-wa/demoinfocs-golang/v2/pkg/demoinfocs/common"
	events "github.com/markus-wa/demoinfocs-golang/v2/pkg/demoinfocs/events"
	st "github.com/markus-wa/demoinfocs-golang/v2/pkg/demoinfocs/sendtables"
)

const (
	obsModeNone      = iota // not in spectator mode
	obsModeDeathCam         // special mode for death cam animation
	obsModeFreezeCam        // zooms to a target, and freeze-frames on them
	obsModeFixed            // view from a fixed camera position
	obsModeInEye            // follow a player in first person view
	obsModeChase            // follow a player in third person view
	obsModeRoaming          // free roaming
)

func main() {
	if len(os.Args) < 2 {
		fmt.Printf("Usage: %s <path/to/demo>\n", filepath.Base(os.Args[0]))
		return
	}

	fmt.Printf("Analyze started: %s\n", os.Args[1])

	f, err := os.Open(os.Args[1])
	if err != nil {
		panic(err)
	}
	defer f.Close()

	p := dem.NewParser(f)
	defer p.Close()

	var roundStartTime time.Duration
	roundNo := p.GameState().TotalRoundsPlayed() + 1
	inRound := false
	bustas := make(map[*common.Player]bool)

	p.RegisterEventHandler(func(e events.RoundFreezetimeEnd) {
		roundNo = p.GameState().TotalRoundsPlayed() + 1
		inRound = true
		roundStartTime = p.CurrentTime()
		bustas = make(map[*common.Player]bool)

		// fmt.Printf("Round %d started\n", roundNo)
		for _, player := range p.GameState().Participants().Connected() {
			coachingTeam, ok := player.Entity.PropertyValue("m_iCoachingTeam")
			if !ok || coachingTeam.IntVal == 0 {
				continue
			}

			obsMode, ok := player.Entity.PropertyValue("m_iObserverMode")
			if !ok {
				continue
			}

			if obsMode.IntVal == obsModeFixed {
				bustas[player] = true
			}
		}
	})

	p.RegisterEventHandler(func(e events.RoundEnd) {
		inRound = false

		// timeSinceRoundStart := p.CurrentTime() - roundStartTime
		// fmt.Printf("Round %d ended [%fs]\n", roundNo, timeSinceRoundStart.Seconds())

		for busta := range bustas {
			coachingTeam, ok := busta.Entity.PropertyValue("m_iCoachingTeam")
			if !ok || coachingTeam.IntVal == 0 {
				continue
			}

			fmt.Printf("Round: %d, Busta: [%s]%s (%d)\n", roundNo, getTeamTag(common.Team(coachingTeam.IntVal)), busta.Name, busta.SteamID64)
		}
		bustas = make(map[*common.Player]bool)
	})

	p.RegisterEventHandler(func(e events.PlayerDisconnected) {
		timeSinceRoundStart := p.CurrentTime() - roundStartTime

		_, ok := bustas[e.Player]
		if !ok {
			return
		}
		delete(bustas, e.Player)

		if !inRound || timeSinceRoundStart.Milliseconds() < 10000 {
			return
		}

		coachingTeam, ok := e.Player.Entity.PropertyValue("m_iCoachingTeam")
		if !ok || coachingTeam.IntVal == 0 {
			return
		}

		fmt.Printf("Round: %d, Busta: [%s]%s (%d)\n", roundNo, getTeamTag(common.Team(coachingTeam.IntVal)), e.Player.Name, e.Player.SteamID64)
	})

	p.RegisterEventHandler(func(events.DataTablesParsed) {
		p.ServerClasses().FindByName("CCSPlayer").OnEntityCreated(func(entity st.Entity) {
			entity.Property("m_iObserverMode").OnUpdate(func(val st.PropertyValue) {
				obs := p.GameState().Participants().ByEntityID()[entity.ID()]
				newObsMode := val.IntVal
				coachingTeam := common.TeamUnassigned
				timeSinceRoundStart := p.CurrentTime() - roundStartTime

				if obs.Team != common.TeamSpectators {
					return
				}

				prop, ok := obs.Entity.PropertyValue("m_iCoachingTeam")
				if ok {
					coachingTeam = common.Team(prop.IntVal)
				}

				// fmt.Printf("%s -> %d [%fs]\n", obs.Name, newObsMode, timeSinceRoundStart.Seconds())

				if newObsMode == obsModeInEye {
					if !inRound || timeSinceRoundStart.Milliseconds() < 500 {
						delete(bustas, obs)
					}
				} else if newObsMode == obsModeFixed {
					allTeamDead := true

					for _, busta := range p.GameState().Participants().TeamMembers(coachingTeam) {
						if busta != nil && busta.IsAlive() {
							allTeamDead = false
							break
						}
					}

					if !allTeamDead || (coachingTeam != common.TeamTerrorists && coachingTeam != common.TeamCounterTerrorists) {
						bustas[obs] = true
					}
				}
			})
		})
	})

	err = p.ParseToEnd()
	if err != nil {
		panic(err)
	}

	fmt.Printf("Analyze ended: %s\n", os.Args[1])
}

func getTeamTag(team common.Team) string {
	switch team {
	case common.TeamUnassigned:
		return "UNASSIGNED"
	case common.TeamSpectators:
		return "SPEC"
	case common.TeamTerrorists:
		return "T"
	case common.TeamCounterTerrorists:
		return "CT"
	}

	return "UNKNOWN"
}
