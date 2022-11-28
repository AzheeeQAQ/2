package main

import (
	"flag"
	"fmt"
	"log"
	"net"
	"net/rpc"
	"os"
	"sync"

	"uk.ac.bris.cs/gameoflife/gol"
	"uk.ac.bris.cs/gameoflife/util"
)

// AWSEngine exported type to register RPC
type AWSEngine struct{}

var turn = 0

var world [][]byte

var mutex = new(sync.Mutex)

// StateCalculator the callee
func StateCalculator(req gol.Request, res *gol.Response) [][]byte {
	// initialize a blank gameBoard
	world = make([][]byte, req.P.ImageHeight)
	for i := range world {
		world[i] = make([]byte, req.P.ImageWidth)
	}
	world = req.InitialGameBoard

	newWorld := make([][]byte, len(world)+2)
	for i := range newWorld {
		if i == 0 {
			newWorld[i] = make([]byte, len(world[0]))
			copy(newWorld[i], req.UpperBound)
		} else if i == len(newWorld)-1 {
			newWorld[i] = make([]byte, len(world[0]))
			copy(newWorld[i], req.LowerBound)
		} else {
			newWorld[i] = make([]byte, len(world[0]))
			copy(newWorld[i], world[i-1])
		}
	}

	////set up client
	//client, _ = rpc.Dial("", "127.0.0.1:8040")
	//defer func(client *rpc.Client) {
	//	err := client.Close()
	//	if err != nil {
	//		log.Fatal("client closing with port 8040: ", err)
	//	}
	//}(client)

	if req.P.Threads == 1 {
		mutex.Lock()
		world = calculateNextState(newWorld)
		turn++
		mutex.Unlock()
	} else {
		mutex.Lock()

		numJobs := len(world)
		jobs := make(chan int, numJobs)
		results := make(chan []util.Cell, numJobs)

		for w := 1; w <= req.P.Threads; w++ {
			go worker(jobs, results, req.P, newWorld)
		}

		for j := 0; j < numJobs; j++ {
			jobs <- j
		}

		close(jobs)

		var parts []util.Cell
		for a := 1; a <= numJobs; a++ {
			part := <-results
			parts = append(parts, part...)
		}
		world = makeWorldFromCells(parts, world)
		//make world from alive cell
		turn++
		mutex.Unlock()
	}
	return world
}

// CalculateNextState the caller to the StateCalculator
func (a *AWSEngine) CalculateNextState(req gol.Request, res *gol.Response) (e error) {
	// calling StateCalculator
	res.CalculatedGameBoard = StateCalculator(req, res)
	fmt.Println("Calculating new GameBoard")
	return
}

func (a *AWSEngine) GetAliveNums(req gol.AliveReq, res *gol.AliveRes) (e error) {
	// calculate alive cells
	mutex.Lock()
	cell := len(calculateAliveCells(world))
	turn := turn
	res.Num = cell
	res.Turn = turn
	mutex.Unlock()
	return
}

// GetCompletedTurns get the number of alive cells for TurnComplete event
func (a *AWSEngine) GetCompletedTurns(req gol.AliveReq, res *gol.AliveRes) (e error) {
	mutex.Lock()
	res.Turn = turn
	mutex.Unlock()
	return
}

func (a *AWSEngine) ServerOff(req gol.AliveReq, res *gol.AliveRes) (e error) {
	go os.Exit(886)
	return
}

// space for helper function
func calculateNextState(world [][]byte) [][]byte {
	newWorld := make([][]byte, len(world)+2)
	// nlc stands for number of alive neighbors
	var nlc int
	for i := 0; i < len(newWorld); i++ {
		for j := 0; j < len(newWorld[i]); j++ {
			nlc = getNeighbors(i, j, newWorld)
			// assert if this cell is alive
			isAlive := world[i][j] == 255
			newWorld[i][j] = checkCellAlive(isAlive, nlc)
		}
	}
	return newWorld[1 : len(newWorld)-2]
}

func worker(jobs chan int, out chan<- []util.Cell, p gol.Params, world [][]uint8) {
	for rowNum := range jobs {
		var cells []util.Cell
		if rowNum == 0 || rowNum == len(world)-1 {
			out <- cells
		} else {
			world := smallerWorld(world, rowNum, p)
			//the line that we work on, initialize it into a 2D slice
			theLine := make([][]byte, 1)
			theLine[0] = make([]byte, p.ImageWidth)

			for index, cell := range world[1] {
				isAlive := cell == 255
				nlc := getNeighbors(1, index, world)
				theLine[0][index] = checkCellAlive(isAlive, nlc)
			}
			cells = calculateAliveCells(theLine)
			for i, cell := range cells {
				cells[i] = util.Cell{X: cell.X, Y: rowNum - 1}
			}
			out <- cells
		}
	}
}

func smallerWorld(world [][]byte, line int, p gol.Params) [][]byte {
	smallerWorld := make([][]byte, 3)
	for i := range smallerWorld {
		smallerWorld[i] = make([]byte, p.ImageWidth)
	}

	if line == 0 {
		copy(smallerWorld[0], world[len(world)-1])
		copy(smallerWorld[1], world[line])
		copy(smallerWorld[2], world[line+1])
	} else if line == len(world)-1 {
		copy(smallerWorld[0], world[line-1])
		copy(smallerWorld[1], world[line])
		copy(smallerWorld[2], world[0])
	} else {
		copy(smallerWorld[0], world[line-1])
		copy(smallerWorld[1], world[line])
		copy(smallerWorld[2], world[line+1])
	}

	return smallerWorld
}

// getNeighbors return number of alive neighbors
func getNeighbors(i int, j int, world [][]byte) int {
	sum := 0
	w := len(world)
	h := len(world[0])

	for di := -1; di <= 1; di++ {
		for dj := -1; dj <= 1; dj++ {
			if di == 0 && dj == 0 {
				continue
			}
			i1 := i + di
			j1 := j + dj
			if i1 < 0 {
				i1 = w - 1
			}
			if i1 >= w {
				i1 = 0
			}
			if j1 < 0 {
				j1 = h - 1
			}
			if j1 >= h {
				j1 = 0
			}
			if world[i1][j1] == 255 {
				sum += 1
			}
		}
	}
	return sum
}

func calculateAliveCells(world [][]byte) []util.Cell {
	var cells []util.Cell

	for i, row := range world {
		for j, _ := range row {
			if world[i][j] == 255 {
				theCell := util.Cell{X: j, Y: i}
				cells = append(cells, theCell)
			}
		}
	}
	return cells[0:]
}

// checkCellAlive check if a cell is alive in next state
func checkCellAlive(isAlive bool, nlc int) uint8 {
	if isAlive {
		if nlc < 2 || nlc > 3 {
			return 0
		} else {
			return 255
		}
	} else {
		if nlc == 3 {
			return 255
		} else {
			return 0
		}
	}
}

func makeWorldFromCells(cells []util.Cell, oldWorld [][]uint8) [][]uint8 {
	newWorld := make([][]byte, len(oldWorld))
	for i := range newWorld {
		newWorld[i] = make([]byte, len(oldWorld[0]))
	}

	for _, cell := range cells {
		newWorld[cell.Y][cell.X] = 255
	}
	return newWorld[0:]
}

// divider determines the breakpoints that will evenly divide world into smaller worlds
func divider(num, h int) []int {
	h = h - 1
	part := h / (num - 1)
	var p1 int

	breakPoints := make([]int, num)
	for i := range breakPoints {
		if i == num-1 {
			breakPoints[i] = h
		} else {
			breakPoints[i] = p1
			p1 = p1 + part
		}
	}
	return breakPoints
}

func main() {
	pAddr := flag.String("port", "8050", "Port to listen to")
	flag.Parse()
	// register with RPC service
	err := rpc.Register(&AWSEngine{})
	if err != nil {
		log.Fatal("server register : ", err)
		return
	}
	listener, _ := net.Listen("tcp", ":"+*pAddr)
	defer func(listener net.Listener) {
		err := listener.Close()
		if err != nil {
			log.Fatal("server listener closing: ", err)
			return
		}
	}(listener)

	rpc.Accept(listener)
}
