package main

import (
	"log"
	"math"
	"math/rand"
	"os"
	"time"
	"github.com/xarg/gopathfinding"
	"github.com/andyleap/gioframework"
	"fmt"
	"sort"
	"io"
	"path"
	"strconv"
)

const (
	TILE_EMPTY = -1
	TILE_MOUNTAIN = -2
	TILE_FOG = -3
	TILE_FOG_OBSTACLE = -4
)

// If we allow too few future moves, then slow network means we could miss turns
// If we allow too many future moves, then bot is less adaptive to changing
// conditions
const MAX_PLANNED_MOVES = 6
const NUM_GAMES_TO_PLAY = 1

func main() {
	client, _ := gioframework.Connect("bot", os.Getenv("GENERALS_BOT_ID"), os.Getenv("GENERALS_BOT_NAME"))
	go client.Run()

	for i := 0; i < NUM_GAMES_TO_PLAY; i++ {
		setupLogging()

		log.Printf("---------- Game #%v/%v -----------", i+1, NUM_GAMES_TO_PLAY)

		var game *gioframework.Game
		if os.Getenv("REAL_GAME") == "true" {
			game = client.Join1v1()
			log.Println("Waiting for opponent...")
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
			log.Printf("Replay available at: http://bot.generals.io/replays/%v", game.ReplayID)
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
			logTurnData(game)

			// Re-enable after debugging...
			if game.TurnCount < 30 {
				log.Println("Waiting for turn 30...")
				continue
			}

			from, to_target, score := GetTileToAttack(game)
			if from < 0 {
				continue
			}
			path := GetShortestPath(game, from, to_target)
			if len(path) == 0 {
				log.Printf("Registering impossible tile: %v", game.GetCoordString(to_target))
				game.ImpossibleTiles[to_target] = true
			}

			max_num_moves := min(len(path) - 1, MAX_PLANNED_MOVES)
			for i := 0; i < max_num_moves; i++ {
				log.Printf("Move army: %v -> %v (Score: %.2f) (Armies: %v -> %v)",
					game.GetCoordString(path[i]), game.GetCoordString(path[i+1]),
					score, game.GameMap[path[i]].Armies, game.GameMap[path[i+1]].Armies)
				game.Attack(path[i], path[i+1], false)
			}
		}
		log.Printf("Replay available at: http://bot.generals.io/replays/%v", game.ReplayID)
	}
}

func setupLogging() {
	log_dir := "log"
	_ = os.Mkdir(log_dir, os.ModePerm)

	rand.Seed(time.Now().UTC().UnixNano())
	log_file := path.Join(log_dir, "log_" + strconv.Itoa(rand.Intn(10000)))
	logFile, err := os.OpenFile(log_file, os.O_CREATE | os.O_APPEND | os.O_RDWR, 0666)
	check(err)

	mw := io.MultiWriter(os.Stdout, logFile)
	log.SetOutput(mw)
}

func logTurnData(g *gioframework.Game) {
	log.Println("------------------------------------------")
	log.Printf("Turn: %v (UI Turn: %v)", g.TurnCount, float64(g.TurnCount)/2.)

	for _, s := range g.Scores {
		var player_name string
		if s.Index == g.PlayerIndex {
			player_name = "Me"
		} else {
			player_name = fmt.Sprintf("Opponent %v", s.Index)
		}
		log.Printf("%10v: Tiles: %v, Army: %v", player_name, s.Tiles, s.Armies)

	}
	if g.TurnCount < 10 {
		log.Printf("My General at: %v", g.GetCoordString(g.Generals[g.PlayerIndex]))
	}
	log.Println("------------------------------------------")
}

func check(err error) {
	if err != nil {
		panic(err)
	}
}

func min(a, b int) int {
    if a < b {
        return a
    }
    return b
}

func Btoi(b bool) int {
    if b {
        return 1
    }
    return 0
}

func Btof(b bool) float64 {
    if b {
        return 1.
    }
    return 0.
}


func getHeuristicPathDistance(game *gioframework.Game, from, to int)  (float64) {
	/* Would have preferred to use A* to get actual path distance, but that's
     prohibitvely expensive. (I need to calculate this many times per turn)
      */
	base_distance := game.GetDistance(from, to)
	tiles_in_square := getTilesInSquare(game, from, to)
	num_obstacles := 0
	for _, tile := range tiles_in_square {
		num_obstacles += Btoi(game.Walkable(tile))
	}
	total_area := len(tiles_in_square)
	obstacle_ratio := num_obstacles / total_area
	// Not sure this is the best heuristic, but it's simple, so I'll use it for
    // now
	return float64(base_distance) * (1. + 2.0 * float64(obstacle_ratio))
}


func getTilesInSquare(game *gioframework.Game, i, j int) ([]int) {
	// Gets index of all tiles in a square defined by two diagonally opposed
	// corners
	row_i := game.GetRow(i)
	col_i := game.GetCol(i)
	row_j := game.GetRow(j)
	col_j := game.GetCol(j)

	row_limits := []int{row_i, row_j}
	col_limits := []int{col_i, col_j}
	sort.Ints(row_limits)
	sort.Ints(col_limits)
	var tiles []int
	for row := row_limits[0]; row < row_limits[1] + 1; row++ {
		for col := col_limits[0]; col < col_limits[1] + 1; col++ {
			tiles = append(tiles, game.GetIndex(row, col))
		}
	}
	return tiles
}

func GetShortestPath(game *gioframework.Game, from, to int) []int {
	map_data := *pathfinding.NewMapData(game.Height, game.Width)
	for row := 0; row <  game.Height; row++ {
		for col := 0; col < game.Width; col++ {
			i := game.GetIndex(row, col)
			tile := game.GameMap[i]
			// We don't want to accidentally attack cities on route to
			// somewhere else.  Note: if it is the final destination, it'll be
			// changed
			not_my_city := tile.Type == gioframework.City && tile.Faction != game.PlayerIndex
			map_data[row][col] = Btoi(!game.Walkable(i) || not_my_city || game.ImpossibleTiles[i])
		}
	}
	map_data[game.GetRow(from)][game.GetCol(from)] = pathfinding.START
	map_data[game.GetRow(to)][game.GetCol(to)] = pathfinding.STOP

	graph := pathfinding.NewGraph(&map_data)
	nodes_path := pathfinding.Astar(graph)
	path := []int{}
	for _, node := range nodes_path {
		path = append(path, game.GetIndex(node.X, node.Y))
	}
	return path
}

func GetTileToAttack(game *gioframework.Game) (int, int, float64) {

	best_from := -1
	best_to := -1
	best_total_score := 0.
	var best_scores map[string]float64

	my_general := game.Generals[game.PlayerIndex]


	for from, from_tile := range game.GameMap {
		if from_tile.Faction != game.PlayerIndex || from_tile.Armies < 2 {
			continue
		}
		//my_army_size := from_tile.Armies

		for to, to_tile := range game.GameMap {
			if to_tile.Faction < -1 {
				continue
			}
			// Note: I'm not dealing with impossible to reach tiles for now
			// No gathering for now...
			if to_tile.Faction == game.PlayerIndex {
				continue
			}

			is_empty := to_tile.Faction == TILE_EMPTY
			is_enemy := to_tile.Faction != game.PlayerIndex && to_tile.Faction >= 0
			is_general := to_tile.Type == gioframework.General
			is_city := to_tile.Type == gioframework.City
			outnumber := float64(from_tile.Armies - to_tile.Armies)
			// Should I translate my heuristic distance from my JS code?
			dist := getHeuristicPathDistance(game, from, to)
			dist_from_gen := getHeuristicPathDistance(game, my_general, to)
			center := game.GetIndex(game.Width / 2, game.Height / 2)
			dist_from_center := getHeuristicPathDistance(game, center, to)
			centerness := 1. - dist_from_center / float64(game.Width / 2)

			scores := make(map[string]float64)

			scores["outnumber_score"] = Truncate(outnumber / 300, 0., 0.2)
			scores["outnumbered_penalty"] = -0.1 * Btof(outnumber < 2)
			scores["general_threat_score"] = (0.2 * math.Pow(dist_from_gen, -0.7)) * Btof(is_enemy)
			scores["dist_penalty"] = Truncate(-0.2 * dist / 30, -0.2, 0)
			scores["dist_gt_army_penalty"] = -0.1 * Btof(from_tile.Armies < int(dist))
			scores["is_enemy_score"] = 0.05 * Btof(is_enemy)
			scores["close_city_score"] = 0.1 * Btof(is_city) * math.Pow(dist_from_gen, -0.5)
			scores["enemy_gen_score"] = 0.1 * Btof(is_general) * Btof(is_enemy)
			scores["empty_score"] = 0.05 * Btof(is_empty)
			// Generally a good strategy to take the center of the board
			scores["centerness_score"] = 0.05 * centerness

			total_score := 0.
			for _, score := range scores {
				total_score += score
			}

			if total_score > best_total_score {
				best_scores = scores
				best_total_score = total_score
				best_from = from
				best_to = to
			}

		}
	}
	logSortedScores(best_scores)

	log.Printf("Total score: %.2f", best_total_score)
	log.Printf("From:%v To:%v", game.GetCoordString(best_from), game.GetCoordString(best_to))
	return best_from, best_to, best_total_score
}

func logSortedScores(scores map[string]float64) {
	keys := make([]string, len(scores))

	i := 0
	for k := range scores {
		keys[i] = k
		i++
	}
	sort.Strings(keys)
	for _, k := range keys {
		log.Printf("%20v: %.3f\n", k, scores[k])
	}
}

func Truncate(val, min, max float64) float64 {
    return math.Min(math.Max(val, min), max)
}
