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

	"github.com/go-errors/errors"
	"github.com/andyleap/gioframework"
	"github.com/xarg/gopathfinding"
	"os/signal"
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
const MaxPlannedMoves = 8
const NumGamesToPlay = 100

func main() {
	client, _ := gioframework.Connect("bot", os.Getenv("GENERALS_BOT_ID"), os.Getenv("GENERALS_BOT_NAME"))
	go client.Run()

	abort := false
	ch := make(chan os.Signal)
	signal.Notify(ch, os.Interrupt)
	go func() {
		<-ch
		log.Println("abort set to true")
		abort = true
		<-ch
		log.Println("ok leaving this function")
		os.Exit(2)
	}()
	// Hack to help with race condition for setting name
	time.Sleep(time.Second)

	for i := 0; i < NumGamesToPlay; i++ {
		if abort {
			break
		}
		setupLogging()

		log.Printf("---------- Game #%v/%v -----------", i+1, NumGamesToPlay)
		realGame := os.Getenv("REAL_GAME") == "true"

		var game *gioframework.Game
		if realGame {
			game = client.Join1v1()
			log.Println("Waiting for opponent...")
		} else {
			gameId := "bot_game"
			game = client.JoinCustomGame(gameId)
			teamVar := os.Getenv("TEAM")
			if teamVar != "" {
				team, _ := strconv.Atoi(teamVar)
				game.SetTeam(team, gameId)
			}
			url := "http://bot.generals.io/games/" + gameId
			log.Printf("Joined custom game, go to: %v", url)
			game.SetForceStart(true)
		}

		started := false
		game.Start = func(playerIndex int, users []string) {
			log.Println("Game started with ", users)
			log.Printf("Replay available at: http://bot.generals.io/replays/%v", game.ReplayID)
			for i, user := range users {
				if i == playerIndex {
					continue
				}
				game.SendChat(fmt.Sprintf("%v, prepare to be destroyed!", user))
			}
			started = true
		}
		done := false
		game.Won = func() {
			log.Println("===========================   Won game!  ============================")
			done = true
		}
		game.Lost = func() {
			log.Println("============================   Lost game...  ========================")
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
				continue
			}

			logTurnData(game)

			from, toTarget := GetBestMove(game)
			if from < 0 {
				continue
			}
			path, err := GetShortestPath(game, from, toTarget)
			if err != nil {
				log.Println(err)
				continue
			}
			if len(path) == 0 {
				log.Printf("Registering impossible tile: %v", game.GetCoordString(toTarget))
				game.ImpossibleTiles[toTarget] = true
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
		time.Sleep(10*time.Second)
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
	log.Printf("Turn: %v (UI%v)", g.TurnCount, float64(g.TurnCount)/2.)

	var msgs []string
	for i, s := range g.Scores {
		msg := fmt.Sprintf("%10v: Tiles: %v, Army: %v", g.Usernames[i], s.Tiles, s.Armies)
		msgs = append(msgs, msg)
	}
	sort.Strings(msgs)
	for _, msg := range msgs {
		log.Println(msg)
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
	tilesInSquare := getTilesInSquare(game, from, to)
	numObstacles := 0.
	for _, tile := range tilesInSquare {
		numObstacles += Btof(!game.Walkable(tile))
	}

	total_area := len(tilesInSquare)
	obstacleRatio := numObstacles / float64(total_area)
	// Not sure this is the best heuristic, but it's simple, so I'll use it for
	// now
	hDist := float64(baseDistance) * (1. + 2.0*obstacleRatio)
	//log.Printf("hDist from %v to %v is: %v", game.GetCoordString(from), game.GetCoordString(to), hDist)
	//log.Println("tilesInSquare: ")
	//for _, i := range tilesInSquare {
	//	log.Println(game.GetCoordString(i))
	//}
	//log.Println("baseDistance:", baseDistance)
	//log.Println("obstacleRatio:", obstacleRatio)
	return hDist
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


type AstarError struct {
	From, To string
}

func (e AstarError) Error() string {
	return fmt.Sprintf("Astar error with from:%v to:%v", e.From, e.To)
}

func GetShortestPath(game *gioframework.Game, from, to int) (path []int, err error) {
	// pathfinding.Astar has no error handling, so we catch its panics
	defer func() {
		if r := recover(); r != nil {
			log.Printf("ERROR, GetShortestPath recovered from panic: %v\n", r)
			path = []int{}
			err = AstarError{
				game.GetCoordString(from),
				game.GetCoordString(to),
			}
		}
	}()
	// TODO: if from and to are the same, just erturn an err.

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
	path = []int{}
	for _, node := range nodesPath {
		path = append(path, game.GetIndex(node.X, node.Y))
	}
	return path, nil
}

func GetBestMove(game *gioframework.Game) (bestFrom int, bestTo int) {
	defer func() {
		if r := recover(); r != nil {
			log.Printf("ERROR, GetBestMove recovered from panic: %v", r)
			fmt.Println(errors.Wrap(r, 2).ErrorStack())
			bestFrom = -1
			bestTo = -1
		}
	}()

	bestFrom = -1
	bestTo = -1
	bestTotalScore := -10.
	var bestScores map[string]float64

	myGeneral := game.Generals[game.PlayerIndex]
	enemyCOM := getEnemyCenterOfMass(game)

	/// First check for attacking new empty or enemy tiles
	for from, fromTile := range game.GameMap {
		if fromTile.Faction != game.PlayerIndex || fromTile.Armies < 2 {
			continue
		}

		for to, toTile := range game.GameMap {
			if toTile.Faction < TileEmpty || toTile.Faction == game.PlayerIndex {
				continue
			}
			if game.ImpossibleTiles[to] {
				continue
			}

			isEmpty := toTile.Faction == TileEmpty
			isEnemy := IsEnemy(game, toTile)
			isGeneral := toTile.Type == gioframework.General
			isCity := toTile.Type == gioframework.City
			outnumber := float64(fromTile.Armies - toTile.Armies)
			dist := getHeuristicPathDistance(game, from, to)
			distFromGen := getHeuristicPathDistance(game, myGeneral, to)
			center := game.GetIndex(game.Width/2, game.Height/2)
			distCenter := getHeuristicPathDistance(game, center, to)
			centerness := 1. - distCenter/float64(game.Width)
			// This is the vector pointing towards the enemy
			enemyVector := [2]int{
				game.GetRow(enemyCOM) - game.GetRow(myGeneral),
				game.GetCol(enemyCOM) - game.GetCol(myGeneral),
			}
			// The vector showing the proposed move
			moveVector := [2]int{
				game.GetRow(to) - game.GetRow(from),
				game.GetCol(to) - game.GetCol(from),
			}
			neighbors := game.GetNeighborhood(to, false)
			numAlliedNeighbors := 0
			for _, neighbor := range neighbors {
				if !IsEnemy(game, game.GameMap[neighbor]) {
					numAlliedNeighbors += 1
				}
			}

			if isCity && outnumber < 2 && !isEnemy {
				// Never attack a neutral city and lose
				continue
			}

			scores := make(map[string]float64)

			scores["outnumber score"] = Truncate(outnumber/200, 0., 0.25) * Btof(isEnemy)
			scores["outnumbered penalty"] = -0.2 * Btof(outnumber < 2)
			scores["general threat score"] = (0.25 * math.Pow(distFromGen, -1.0)) *
				Truncate(float64(toTile.Armies)/10, 0., 1.0) * Btof(isEnemy)
			scores["dist penalty"] = Truncate(-0.5*dist/30, -0.3, 0)
			scores["dist gt army penalty"] = -0.2 * Btof(fromTile.Armies < int(dist))
			scores["is enemy score"] = 0.05 * Btof(isEnemy)
			scores["close city score"] = 0.35 * Btof(isCity && outnumber >= 2) *
				math.Pow(distFromGen, -0.5)
			scores["enemy city score"] = 0.2 * Btof(isCity && isEnemy)
			scores["enemy gen score"] = 0.15 * Btof(isGeneral) * Btof(isEnemy)
			scores["empty score"] = 0.08 * Btof(isEmpty)
			// Generally a good strategy to take the center of the board
			scores["centerness score"] = 0.02 * centerness
			// You should move towards enemy's main base, not random little
			// patches of enemy.  This prevents the bot from "cleaning up"
			// irrelevant squares.  This could be improved by making the vectors
			// normalized and having the score gradually increase as you point
			// towards the enemy
			scores["towards enemy score"] = 0.03 * Btof(dotProduct(enemyVector, moveVector) > 1)
			// Instead of attacking all the tiles on the enemy's border it is
			// typically better to make a deep drive into enemy land
			scores["deep drive score"] = 0.04 * Btof(numAlliedNeighbors < 2)

			totalScore := 0.
			for _, score := range scores {
				totalScore += score
			}
			//log.Printf("============Considering move %v->%v, got score: %v\n", from, to, totalScore)
			//logSortedScores(scores)

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
	consolScore := 0.6 * math.Pow(1-armyCycle, 6)  - 0.2
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
			path_, _ := GetShortestPath(game, from, largestTile)  // TODO: handle err  //TODO: this throws an error it seems!  largestTile == from ??!!
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
		bestTo = largestTile
		// We want to move towards the enemy.  Reverse if that's not the
		// case
		if enemyCOM >= 0 {
			fromDistEnemy := getHeuristicPathDistance(game, enemyCOM, bestFrom)
			toDistEnemy := getHeuristicPathDistance(game, enemyCOM, bestTo)
			if fromDistEnemy < toDistEnemy {
				log.Println("Switching direction of consolidation")
				bestFrom, bestTo = bestTo, bestFrom
			}
		}

		log.Printf("Consolidating, with average army: %v", highestAvArmy)
	}

	// Trash talk
	toTile := game.GameMap[bestTo]
	if toTile.Type == gioframework.City && IsEnemy(game, toTile) {
		game.SendChat("Sorry, I'm gonna need that citadel")
	}

	return bestFrom, bestTo
}

func IsEnemy(game *gioframework.Game, tile gioframework.Cell) bool {
	if len(game.Teams) == 0 {
		// This means we're playing 1v1 or FFA
		return tile.Faction != game.PlayerIndex && tile.Faction >= 0
	} else {
		myTeam := game.Teams[game.PlayerIndex]
		return tile.Faction >= 0 && game.Teams[tile.Faction] != myTeam
	}
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

func Sum(x []int) int {  // TODO make this an interface for fun
	sum := 0
	for _, i := range x {
		sum += i
	}
	return sum
}

func dotProduct(x [2]int, y [2]int) float64 {
	return float64(x[0]) * float64(y[0]) + float64(x[1]) * float64(y[1])
}


// getEnemyCenterOfMass find the central point of the visible enemy terrain,
// weighted by armies, and rounded to the closes tile.
func getEnemyCenterOfMass(game *gioframework.Game) int {
	rows := []int{}
	cols := []int{}
	armies := 0
	for i, tile := range game.GameMap {
		if IsEnemy(game, tile) {
			army := tile.Armies
			rows = append(rows, army*game.GetRow(i))
			cols = append(cols, army*game.GetCol(i))
			armies += army
		}
	}
	var COM int
	if armies == 0 {
		COM = -1
		log.Println("COM is: -1")
		return COM
	}
	avRow := float64(Sum(rows))/float64(armies)
	avCol := float64(Sum(cols))/float64(armies)
	COM = game.GetIndex(int(avRow), int(avCol))
	log.Printf("COM is: %v\n", game.GetCoordString(COM))
	return COM
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
	for _, tile := range game.GameMap {
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
