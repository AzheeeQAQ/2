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

// Broker exported type to register RPC
type Broker struct {}

var controllerOut = false

var turn = 0

var world [][]byte

var mutex = new(sync.Mutex)

var LocalController *rpc.Client

var distributedWorkers []*rpc.Client

var numOfServers = 3

// StateCalculator the callee
func StateCalculator(req gol.Request, res *gol.Response) [][]byte {
	ip := [3]string{"127.0.0.1", "127.0.0.1","127.0.0.1"}
	// initialize a blank gameBoard
	world = make([][]byte, req.P.ImageHeight)
	for i := range world {
		world[i] = make([]byte, req.P.ImageWidth)
	}
	world = req.InitialGameBoard

	// call LocalController at port 8060
	LocalController, _ = rpc.Dial("tcp", "127.0.0.1:8060")
	defer func(client *rpc.Client) {
		err := client.Close()
		if err != nil {
			log.Fatal("client closing with port 8060: ", err)
		}
	}(LocalController)

	// TODO using subscribers
	// call worker with port 8050
	//port 8050+i
	for i:=0;i<numOfServers; i++{
		port := 8050+i
		address := fmt.Sprintf("%s:%d",ip[i], port)
		fmt.Println(address)
		distributedWorker, _ := rpc.Dial("tcp", address)
		distributedWorkers = append(distributedWorkers, distributedWorker)
		//defer func(client *rpc.Client) {
		//	err := client.Close()
		//	if err != nil {
		//		log.Fatal("cannot Dial worker on 8050: ", err)
		//	}
		//}(distributedWorker)
	}

	for turn < req.P.Turns {
		// RPC call to distributed worker
		mutex.Lock()
		oldWorld := make([][]byte, req.P.ImageHeight)
		for i := range world {
			oldWorld[i] = make([]byte, req.P.ImageWidth)
			copy(oldWorld[i], world[i])
		}

		breakpoints := divider(numOfServers,  req.P.ImageHeight)
		worlds := sliceWorld(breakpoints, oldWorld)
		chans := make([]chan [][]byte, numOfServers)
		for i := range chans {
			chans[i] = make(chan [][]byte)
		}

		for index := range worlds{
			if index==0{
				upper := oldWorld[len(oldWorld)-1]
				lower := oldWorld[breakpoints[index+1]]
				go callWorker(distributedWorkers[index], worlds[index], chans[index], req.P, upper, lower)
			}else if index==len(worlds)-1 {
				upper := oldWorld[breakpoints[index]]
				lower := oldWorld[0]
				go callWorker(distributedWorkers[index], worlds[index], chans[index], req.P, upper, lower)
			} else {
				upper := oldWorld[breakpoints[index]-1]
				lower := oldWorld[breakpoints[index+1]-1]
				go callWorker(distributedWorkers[index], worlds[index], chans[index], req.P, upper, lower)
			}
		}

		var newWorld [][]byte
		for _,channel := range chans{
			data := <-channel
			newWorld = append(newWorld, data...)
		}

		// get gameBoard of each turn in server
		world = newWorld
		turn++
		mutex.Unlock()
		// make the call to pass the CellFlipped & turnCompleted event
		if controllerOut == false {
			fmt.Println(controllerOut)
			sendDifference(oldWorld, world, &turn)
			sendTurnCompleted(&turn)
		} else {
			controllerOut = false
			break
		}
	}
	turn = 0
	return world
}


func sendTurnCompleted(turn *int) {
	req := gol.AliveReq{OldBoard: nil, NewBoard: nil, CompletedTurns: *turn}
	res := new(gol.AliveRes)
	err := LocalController.Call(gol.TurnCompletedHandler, req, res)
	if err != nil {
		//log.Fatal("err while making call to Pass Turn: ", err)
		//fmt.Println(err)
	}
	return
}


func sendDifference(oldBoard [][]byte, newBoard [][]byte, turn *int) {
	//mutex.Lock()
	req := gol.AliveReq{OldBoard: oldBoard, NewBoard: newBoard, CompletedTurns: *turn}
	//mutex.Unlock()
	res := new(gol.AliveRes)
	// TODO fix the fault tolerance
	err := LocalController.Call(gol.DifferentCells, req, res)
	if err != nil {
		//log.Fatal("err while making call to Pass FlippedCells: ", err)
		//fmt.Println(err)
		return
	}
}


// return with CalculatedGameBoard
func divider(num, h int) []int {
	h = h - 1
	part := h / (num)
	var p1 int

	breakPoints := make([]int, num+1)
	for i := range breakPoints {
		if i == num {
			breakPoints[i] = h
		} else {
			breakPoints[i] = p1
			p1 = p1 + part
		}
	}
	return breakPoints
}


// return with CalculatedGameBoard
func callWorker(client *rpc.Client, worldSlice [][]byte, out chan [][]byte, p gol.Params, upper []byte, lower []byte){
	req := gol.Request{InitialGameBoard: worldSlice, P: p, UpperBound: upper, LowerBound: lower}
	res := new(gol.Response)
	err := client.Call(gol.WorkerMainHandler, req, res)
	if err != nil {
		log.Fatal("err while making call from distributor: ", err)
	}
	// do not defer close the client here
	out <- res.CalculatedGameBoard
}


func sliceWorld(breakPoints []int, bigWorld [][]uint8) [][][]uint8{
	worlds := make([][][]uint8, len(breakPoints)-1)

	for i,_ := range worlds{
		var lenOfWorld int
		if i==len(worlds)-1{
			lenOfWorld = (breakPoints[i+1])-breakPoints[i]+1
		}else {
			lenOfWorld = (breakPoints[i+1]-1)-breakPoints[i]+1
		}
		worlds[i] = make([][]uint8, lenOfWorld)
		for j,_ := range worlds[i]{
			worlds[i][j] = make([]uint8, len(bigWorld[j]))
		}
	}

	//count := 0 // line number for old world
	lineForWorld := 0 //line number for smaller world
	lineForBigWorld := 0 // line number for old world
	index := 0 // index of smaller world

	for lineForBigWorld<len(bigWorld)-1{
		if lineForBigWorld==breakPoints[index+1]{
			index += 1
			lineForWorld = 0
		}
		data := bigWorld[lineForBigWorld]
		copy(worlds[index][lineForWorld], data)
		lineForWorld += 1
		lineForBigWorld += 1
	}

	return worlds
}


func (b *Broker) ControllerOut(req gol.AliveReq, res *gol.AliveRes) (e error) {
	mutex.Lock()
	turn = 0
	controllerOut = true
	mutex.Unlock()
	return
}


func (b* Broker) PowerOff(req gol.AliveReq, res *gol.AliveRes) (e error) {
	mutex.Lock()
	for _,distributorWorker := range distributedWorkers {
		err := distributorWorker.Call(gol.ServerOff, req, res)
		if err != nil {
			fmt.Println("err while calling server offline")
		}
	}
	mutex.Unlock()
	go os.Exit(886)
	return
}


func (b *Broker) LockMutex(req gol.AliveReq, res *gol.AliveRes) (e error) {
	mutex.Lock()
	return
}


func (b *Broker) UnlockMutex(req gol.AliveReq, res *gol.AliveRes) (e error) {
	mutex.Unlock()
	return
}


// CalculateNextState the caller to the StateCalculator
func (b *Broker) CalculateNextState(req gol.Request, res *gol.Response) (e error) {
	// calling StateCalculator
	res.CalculatedGameBoard = StateCalculator(req, res)
	fmt.Println("Calculating new GameBoard")
	return
}


func (b *Broker) GetAliveNums(req gol.AliveReq, res *gol.AliveRes) (e error) {
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
func (b *Broker) GetCompletedTurns(req gol.AliveReq, res *gol.AliveRes) (e error) {
	mutex.Lock()
	res.Turn = turn
	mutex.Unlock()
	return
}


func (b *Broker) GetCurrentBoard(req gol.AliveReq, res *gol.AliveRes) (e error)  {
	mutex.Lock()
	res.CurrentBoard = world
	mutex.Unlock()
	return
}


func calculateAliveCells(world [][]byte) []util.Cell {
	var cells []util.Cell

	for i, row := range world{
		for j, _ := range row{
			if world[i][j] == 255{
				theCell := util.Cell{X:j, Y:i}
				cells = append(cells, theCell)
			}
		}
	}
	return cells[0:]
}


func main() {
	brokerAddr := flag.String("port", "8030", "broker port to listen to")
	flag.Parse()
	// register with RPC service
	err := rpc.Register(&Broker{})
	if err != nil {
		log.Fatal("broker register : ", err)
		return
	}
	listener,_ := net.Listen("tcp", ":"+*brokerAddr)
	defer func(listener net.Listener) {
		err := listener.Close()
		if err != nil {
			log.Fatal("broker listener closing: ", err)
			return
		}
	}(listener)

	rpc.Accept(listener)
}
