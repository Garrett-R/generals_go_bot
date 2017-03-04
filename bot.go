package main

import (
	"fmt"
	"io"
	"log"
	"math"
	"math/rand"
	"os"
	"path"
	"sort"
	"strconv"
	"time"

	"github.com/andyleap/gioframework"
	"github.com/xarg/gopathfinding"
)

const (
	TileEmpty       = -1
	TileMountain    = -2
	TileFog         = -3
	TileFogObstacle = -4
)

// If we allow too few future moves, then slow network means we could miss turns
// If we allow too many future moves, then bot is less adaptive to changing
// conditions
const MaxPlannedMoves = 6
const NumGamesToPlay = 1

func main() {
	client, _ := gioframework.Connect("bot", os.Getenv("GENERALS_BOT_ID"), os.Getenv("GENERALS_BOT_NAME"))
	go client.Run()

	for i := 0; i < NumGamesToPlay; i++ {
		setupLogging()

		log.Printf("---------- Game #%v/%v -----------", i+1, NumGamesToPlay)
		realGame := os.Getenv("REAL_GAME") == "true"

		var game *gioframework.Game
		if realGame {
			game = client.Join1v1()
			log.Println("Waiting for opponent...")
		} else {
			gameId := "bot_testing_game"
			game = client.JoinCustomGame(gameId)
			url := "http://bot.generals.io/games/" + gameId
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
			if game.TurnCount < 30 {
				//log.Println("Waiting for turn 30...")
				continue
			}

			logTurnData(game)

			from, to_target := GetBestMove(game)
			if from < 0 {
				continue
			}
			path := GetShortestPath(game, from, to_target)
			if len(path) == 0 {
				log.Printf("Registering impossible tile: %v", game.GetCoordString(to_target))
				game.ImpossibleTiles[to_target] = true
			}

			max_num_moves := min(len(path)-1, MaxPlannedMoves)
			for i := 0; i < max_num_moves; i++ {
				log.Printf("Move army: %v -> %v  (Armies: %v -> %v)",
					game.GetCoordString(path[i]), game.GetCoordString(path[i+1]),
					game.GameMap[path[i]].Armies, game.GameMap[path[i+1]].Armies)
				game.Attack(path[i], path[i+1], false)
			}
		}
		log.Printf("Replay available at: http://bot.generals.io/replays/%v", game.ReplayID)
	}
}

func setupLogging() {
	logDir := "log"
	_ = os.Mkdir(logDir, os.ModePerm)

	rand.Seed(time.Now().UTC().UnixNano())
	logFilename := path.Join(logDir, "log_"+strconv.Itoa(rand.Intn(10000)))
	logFile, err := os.OpenFile(logFilename, os.O_CREATE|os.O_APPEND|os.O_RDWR, 0666)
	check(err)

	mw := io.MultiWriter(os.Stdout, logFile)
	log.SetOutput(mw)
}

func logTurnData(g *gioframework.Game) {
	log.Println("------------------------------------------")
	log.Printf("Turn: %v (UI Turn: %v)", g.TurnCount, float64(g.TurnCount)/2.)

	for _, s := range g.Scores {
		var playerName string
		if s.Index == g.PlayerIndex {
			playerName = "Me"
		} else {
			playerName = fmt.Sprintf("Opponent %v", s.Index)
		}
		log.Printf("%10v: Tiles: %v, Army: %v", playerName, s.Tiles, s.Armies)

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

func getHeuristicPathDistance(game *gioframework.Game, from, to int) float64 {
	/* Would have preferred to use A* to get actual path distance, but that's
	   prohibitvely expensive. (I need to calculate this many times per turn)
	*/
	baseDistance := game.GetDistance(from, to)
	tilesInSquares := getTilesInSquare(game, from, to)
	numObstacles := 0
	for _, tile := range tilesInSquares {
		numObstacles += Btoi(game.Walkable(tile))
	}
	total_area := len(tilesInSquares)
	obstacleRatio := numObstacles / total_area
	// Not sure this is the best heuristic, but it's simple, so I'll use it for
	// now
	return float64(baseDistance) * (1. + 2.0*float64(obstacleRatio))
}

func getTilesInSquare(game *gioframework.Game, i, j int) []int {
	// Gets index of all tiles in a square defined by two diagonally opposed
	// corners
	rowI := game.GetRow(i)
	colI := game.GetCol(i)
	rowJ := game.GetRow(j)
	colJ := game.GetCol(j)

	rowLimits := []int{rowI, rowJ}
	colLimits := []int{colI, colJ}
	sort.Ints(rowLimits)
	sort.Ints(colLimits)
	var tiles []int
	for row := rowLimits[0]; row < rowLimits[1]+1; row++ {
		for col := colLimits[0]; col < colLimits[1]+1; col++ {
			tiles = append(tiles, game.GetIndex(row, col))
		}
	}
	return tiles
}

func GetShortestPath(game *gioframework.Game, from, to int) []int {
	map_data := *pathfinding.NewMapData(game.Height, game.Width)
	for row := 0; row < game.Height; row++ {
		for col := 0; col < game.Width; col++ {
			i := game.GetIndex(row, col)
			tile := game.GameMap[i]
			// We don't want to accidentally attack cities on route to
			// somewhere else.  Note: if it is the final destination, it'll be
			// changed
			not_my_city := tile.Type == gioframework.City && tile.Faction != game.PlayerIndex
			map_data[row][col] = Btoi(!game.Walkable(i) || not_my_city)
		}
	}
	map_data[game.GetRow(from)][game.GetCol(from)] = pathfinding.START
	map_data[game.GetRow(to)][game.GetCol(to)] = pathfinding.STOP

	graph := pathfinding.NewGraph(&map_data)
	nodesPath := pathfinding.Astar(graph)
	path := []int{}
	for _, node := range nodesPath {
		path = append(path, game.GetIndex(node.X, node.Y))
	}
	return path
}

func GetBestMove(game *gioframework.Game) (int, int) {

	bestFrom := -1
	bestTo := -1
	bestTotalScore := 0.
	var bestScores map[string]float64

	myGeneral := game.Generals[game.PlayerIndex]

	/// First check for attacking new empty or enemy tiles
	for from, fromTile := range game.GameMap {
		if fromTile.Faction != game.PlayerIndex || fromTile.Armies < 2 {
			continue
		}

		for to, toTile := range game.GameMap {
			if toTile.Faction < -1 || toTile.Faction == game.PlayerIndex {
				continue
			}
			if game.ImpossibleTiles[to] {
				continue
			}

			isEmpty := toTile.Faction == TileEmpty
			isEnemy := toTile.Faction != game.PlayerIndex && toTile.Faction >= 0
			isGeneral := toTile.Type == gioframework.General
			isCity := toTile.Type == gioframework.City
			outnumber := float64(fromTile.Armies - toTile.Armies)
			dist := getHeuristicPathDistance(game, from, to)
			distFromGen := getHeuristicPathDistance(game, myGeneral, to)
			center := game.GetIndex(game.Width/2, game.Height/2)
			distFromCenter := getHeuristicPathDistance(game, center, to)
			centerness := 1. - distFromCenter/float64(game.Width/2)

			scores := make(map[string]float64)

			scores["outnumber score"] = Truncate(outnumber/200, 0., 0.3) * Btof(isEnemy)
			scores["outnumbered penalty"] = -0.2 * Btof(outnumber < 2)
			scores["general threat score"] = (0.2 * math.Pow(distFromGen, -0.7)) * Btof(isEnemy)
			scores["dist penalty"] = Truncate(-0.5*dist/30, -0.3, 0)
			scores["dist gt army penalty"] = -0.2 * Btof(fromTile.Armies < int(dist))
			scores["is enemy score"] = 0.05 * Btof(isEnemy)
			scores["close city score"] = 0.15 * Btof(isCity) * math.Pow(distFromGen, -0.5)
			scores["enemy gen score"] = 0.15 * Btof(isGeneral) * Btof(isEnemy)
			scores["empty score"] = 0.08 * Btof(isEmpty)
			// Generally a good strategy to take the center of the board
			scores["centerness score"] = 0.03 * centerness

			totalScore := 0.
			for _, score := range scores {
				totalScore += score
			}

			if totalScore > bestTotalScore {
				bestScores = scores
				bestTotalScore = totalScore
				bestFrom = from
				bestTo = to
			}

		}
	}

	logSortedScores(bestScores)

	log.Printf("Attack score: %.2f", bestTotalScore)
	log.Printf("From:%v To:%v", game.GetCoordString(bestFrom), game.GetCoordString(bestTo))
	log.Println("--------")

	/////////////// Then check for consolidation  //////////////////////////////
	//consolScore := getConsolidationScore(game)
	// It's a good idea to consolidate armies right after the armies regenerate.
	// armyCycle shows the amount of the cycle that's passed.  [0, 1]
	armyCycle := float64(game.TurnCount % 50) / 50
	consolScore := 0.35 * math.Pow(1-armyCycle, 6)
	log.Printf("Consolidation score:%.2f", consolScore)

	tiles := getTilesSortedOnArmies(game)
	if len(tiles) > 10 && consolScore > bestTotalScore {
		largestTile := tiles[0]
		for _, tile := range tiles[:5] {
			log.Printf("Army ranked: %v", game.GameMap[tile].Armies)
		}
		highestAvArmy := 0.
		for _, from := range tiles[1:] {
			if game.GameMap[from].Armies < 2 {
				continue
			}
			// Warning: this path could cut through enemy territory!  Keep an
			// eye for this
			path_ := GetShortestPath(game, from, largestTile)
			armies := 0
			for _, i := range path_[:len(path_)-1] {
				armies += game.GameMap[i].Armies
			}
			av_army := float64(armies) / float64(len(path_)-1)
			if av_army > highestAvArmy {
				highestAvArmy = av_army
				bestFrom = from
			}
		}
		if highestAvArmy > 0 {
			bestTo = largestTile
			log.Printf("Consolidating, with average army: %v", highestAvArmy)
		}
	}

	return bestFrom, bestTo
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

func getConsolidationScore(game *gioframework.Game) float64 {
	gini := getArmyGiniCoefficient(game)
	totalArmy := float64(game.Scores[game.PlayerIndex].Armies)
	log.Printf("Gini coefficient: %.2f", gini)
	log.Printf("Total army: %v", totalArmy)
	return (0.65 - gini) * Truncate(totalArmy/500., 0.5, 2.)
}

func getArmyGiniCoefficient(game *gioframework.Game) float64 {
	movableArmies := []int{}
	for i := 0; i < game.Height*game.Width; i++ {
		tile := game.GameMap[i]
		if tile.Faction == game.PlayerIndex {
			movableArmies = append(movableArmies, tile.Armies-1)
		}
	}
	//log.Printf("movableArmies: %v", movableArmies)
	return giniCoefficient(movableArmies)
}

// giniCoefficient is calculated as described here:
// https://en.wikipedia.org/wiki/Gini_coefficient#Alternate_expressions
func giniCoefficient(nums []int) float64 {
	sort.Ints(nums)
	n := len(nums)
	denom := 0
	for _, num := range nums {
		denom += num
	}
	denom *= n
	numer := 0
	for i, num := range nums {
		numer += (i + 1) * num
	}
	numer *= 2
	return float64(numer)/float64(denom) - float64(n+1)/float64(n)
}

func getTilesSortedOnArmies(game *gioframework.Game) []int {
	tileToArmy := make(map[int]int)
	for i := 0; i < game.Height*game.Width; i++ {
		tile := game.GameMap[i]
		if tile.Faction == game.PlayerIndex {
			tileToArmy[i] = tile.Armies
		}
	}
	largestArmyTiles := sortKeysByValues(tileToArmy, true)
	return largestArmyTiles
}

func sortKeysByValues(m map[int]int, reversed bool) []int {
	n := map[int][]int{}
	var a []int
	for k, v := range m {
		n[v] = append(n[v], k)
	}
	for v := range n {
		a = append(a, v)
	}
	keys := []int{}
	if reversed {
		sort.Sort(sort.Reverse(sort.IntSlice(a)))
	} else {
		sort.Sort(sort.IntSlice(a))
	}

	for _, v := range a {
		for _, k := range n[v] {
			keys = append(keys, k)
		}
	}
	return keys
}
