package main

import (
	"log"
	"os"
	"math/rand"
	"time"

	"github.com/andyleap/gioframework"
)

func main() {

	client, _ := gioframework.Connect("bot", os.Getenv("GENERALS_BOT_ID"), "Terminator")
	go client.Run()

	num_games_to_play := 1

	for i := 0; i < num_games_to_play; i++ {
		var game *gioframework.Game
		if os.Getenv("REAL_GAME") == "true" {
			game = client.Join1v1()
		} else {
			game_id := "bot_testing_game"
			game = client.JoinCustomGame(game_id)
			url := "http://bot.generals.io/games/" + game_id
			log.Printf("Joined custom game, go to: %v", url)
			game.SetForceStart(true)
		}

		started := false
		game.Start = func(playerIndex int, users []string) {
			log.Println("Game started with ", users)
			started = true
		}
		done := false
		game.Won = func() {
			log.Println("Won game!")
			done = true
		}
		game.Lost = func() {
			log.Println("Lost game...")
			done = true
		}
		for !started {
			time.Sleep(1 * time.Second)
		}

		time.Sleep(1 * time.Second)

		for !done {
			time.Sleep(100 * time.Millisecond)
			if game.QueueLength() > 0 {
				continue
			}

			if game.TurnCount < 20 {
				log.Println("Waiting for turn 20...")
				continue
			}


			from, to, score := GetTileToAttack(game)

			







			//mine := []int{}
			//for i, tile := range game.GameMap {
			//	if tile.Faction == game.PlayerIndex && tile.Armies > 1 {
			//		mine = append(mine, i)
			//	}
			//}
			//if len(mine) == 0 {
			//	continue
			//}
			//cell := rand.Intn(len(mine))
			//move := []int{}
			//for _, adjacent := range game.GetAdjacents(mine[cell]) {
			//	if game.Walkable(adjacent) {
			//		move = append(move, adjacent)
			//	}
			//}
			//if len(move) == 0 {
			//	continue
			//}
			//movecell := rand.Intn(len(move))
			game.Attack(from, to, false)

		}
	}
}

func GetTileToAttack(game *gioframework.Game) (int, int, float64) {

	best_from := -1
	best_to := -1
	best_score := 0.


	for from_idx, from_tile := range game.GameMap {
		if from_tile.Faction != game.PlayerIndex || from_tile.Armies < 2 {
			continue
		}
		//my_army_size := from_tile.Armies

		for to_idx, to_tile := range game.GameMap {
			if !game.Walkable(to_idx) {
				continue
			}
			// Note: I'm not dealing with impossible to reach tiles for now
			// No gathering for now...
			if to_tile.Faction == game.PlayerIndex {
				continue
			}
			score := 0.5

			if score > best_score {
				best_from = from_idx
				best_to = to_idx
				best_score = score
			}
		}
	}
	return best_from, best_to, best_score
}
