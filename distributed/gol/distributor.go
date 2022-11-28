package gol

import (
	//"flag"
	"fmt"
	"log"
	"net"
	"os"
	//"net"
	_ "net"
	"net/rpc"
	"time"
	"uk.ac.bris.cs/gameoflife/util"
)

type distributorChannels struct {
	events     chan<- Event
	ioCommand  chan<- ioCommand
	ioIdle     <-chan bool
	ioFilename chan<- string
	ioOutput   chan<- uint8
	ioInput    <-chan uint8
	keyPresses <-chan rune
}

type LocalController struct {
	c distributorChannels
}

//var mutex = new(sync.Mutex)

// distributor divides the work between workers and interacts with other goroutines.
func distributor(p Params, c distributorChannels) {

	// TODO: Create a 2D slice to store the world.
	filename := fmt.Sprintf("%dx%d", p.ImageWidth, p.ImageHeight)

	c.ioFilename <- filename
	c.ioCommand <- ioInput

	// set up a server at port 8060
	nextTest := make(chan bool)
	go asServer(c, nextTest)

	// client set up at port 8030
	client, err := rpc.Dial("tcp", "127.0.0.1:8030")
	if err != nil {
		log.Fatal("dialing: ", err)
	}
	//defer func(client *rpc.Client) {
	//	err := client.Close()
	//	if err != nil {
	//		log.Fatal("client closing: ", err)
	//	}
	//}(client)

	// a copy of current game board
	world := make([][]byte, p.ImageHeight)
	for i := range world {
		world[i] = make([]byte, p.ImageWidth)
	}

	// TODO detect the keyPresses event continuously
	go keyPressesHandler(client, p, c)

	var val byte
	for j := 0; j < p.ImageHeight; j++ {
		for i := 0; i < p.ImageWidth; i++ {
			val = <-c.ioInput
			world[j][i] = val
			if val == 255 {
				c.events <- CellFlipped{CompletedTurns: 0, Cell: util.Cell{X: i, Y: j}}
			}
		}
	}

	turn := 0

	// TODO: Execute all turns of the Game of Life.
	// ticker
	done := make(chan bool)
	go ticker(client, done, c)

	// make the call and get the CalculatedGameBoard back
	fmt.Println("making the call")
	CalculatedGameBoard := makeCall(client, world, p)
	// connection shunt down here with the finish of each call
	done <- true

	cells := calculateAliveCells(CalculatedGameBoard)
	c.ioCommand <- ioOutput
	pgmFilname := fmt.Sprintf("%vx%vx%v", p.ImageWidth, p.ImageHeight, p.Turns)
	c.ioFilename <- pgmFilname

	for y := 0; y < p.ImageHeight; y++ {
		for x := 0; x < p.ImageWidth; x++ {
			val := CalculatedGameBoard[y][x]
			c.ioOutput <- val
		}
	}

	// TODO: Report the final state using FinalTurnCompleteEvent.
	completeEvent := FinalTurnComplete{p.Turns, cells}
	c.events <- completeEvent

	imageOutputEvent := ImageOutputComplete{CompletedTurns: p.Turns, Filename: pgmFilname}
	c.events <- imageOutputEvent

	// Make sure that the Io has finished any output before exiting.
	c.ioCommand <- ioCheckIdle
	<-c.ioIdle

	c.events <- StateChange{turn, Quitting}

	// Close the channel to stop the SDL goroutine gracefully. Removing may cause deadlock.
	close(c.events)
	nextTest <- true
}

func keyPressesHandler(broker *rpc.Client, p Params, c distributorChannels) {
	for {
		key := <-c.keyPresses

		req := new(AliveReq)
		res := new(AliveRes)
		// TODO err handling
		err := broker.Call(GetCompletedTurns, req, res)
		if err != nil {
			log.Fatal("err while calling broker getCurrentBoard from keyPressHandler : ", err)
		}
		turn := res.Turn

		switch key {
		case 'p':
			// print current turn
			fmt.Println(turn)
			lockMutex(broker)
			for 'p' != <-c.keyPresses {
				time.Sleep(500 * time.Millisecond)
			}
			unlockMutex(broker)
			fmt.Println("Continuing")
		case 'q':
			// TODO LocalController shut down with no error generating in server
			controllerOut(broker)
			fmt.Println("controllerOut")
			os.Exit(001)
		case 'k':
			// TODO Worker shut down cleanly with last turn Image
			// first and foremost, generate a PGM file, which is same as s
			c.ioCommand <- ioOutput
			pgmFilename := fmt.Sprintf("%vx%vx%v", p.ImageWidth, p.ImageHeight, turn)
			c.ioFilename <- pgmFilename

			duplicateWorld := getCurrentBoard(broker)
			for y := 0; y < p.ImageHeight; y++ {
				for x := 0; x < p.ImageWidth; x++ {
					val := duplicateWorld[y][x]
					c.ioOutput <- val
				}
			}
			imageOutputEvent := ImageOutputComplete{CompletedTurns: turn, Filename: pgmFilename}
			c.events <- imageOutputEvent

			go powerOff(broker)
			os.Exit(001)
		case 's':
			// If s is pressed, generate a PGM file with the current state of the board.
			c.ioCommand <- ioOutput
			pgmFilename := fmt.Sprintf("%vx%vx%v", p.ImageWidth, p.ImageHeight, turn)
			c.ioFilename <- pgmFilename

			duplicateWorld := getCurrentBoard(broker)
			for y := 0; y < p.ImageHeight; y++ {
				for x := 0; x < p.ImageWidth; x++ {
					val := duplicateWorld[y][x]
					c.ioOutput <- val
				}
			}
			imageOutputEvent := ImageOutputComplete{CompletedTurns: turn, Filename: pgmFilename}
			c.events <- imageOutputEvent
		}
	}
}

func controllerOut(client *rpc.Client) {
	req := new(AliveReq)
	res := new(AliveRes)
	err := client.Call(ControllerOut, req, res)
	if err != nil {
		log.Fatal("err while controller out : ", err)
	}
}

func powerOff(client *rpc.Client) {
	req := new(AliveReq)
	res := new(AliveRes)
	err := client.Call(PowerOff, req, res)
	if err != nil {
		log.Fatal("err while power off : ", err)
	}
}

func lockMutex(client *rpc.Client) {
	req := new(AliveReq)
	res := new(AliveRes)
	err := client.Call(LockMutex, req, res)
	if err != nil {
		log.Fatal("err while locking global mutex : ", err)
	}
}

func unlockMutex(client *rpc.Client) {
	req := new(AliveReq)
	res := new(AliveRes)
	err := client.Call(UnlockMutex, req, res)
	if err != nil {
		log.Fatal("err while unlocking global mutex : ", err)
	}
}

func getCurrentBoard(broker *rpc.Client) [][]byte {
	reqBoard := new(AliveReq)
	resBoard := new(AliveRes)
	e := broker.Call(GetCurrentBoard, reqBoard, resBoard)
	// TODO err handling
	if e != nil {
		log.Fatal("err while calling broker getCurrentBoard from keyPressHandler : ", e)
	}
	return resBoard.CurrentBoard
}

func asServer(c distributorChannels, done chan bool) {
	// server set up
	err := rpc.Register(&LocalController{c: c})
	if err != nil {
		log.Fatal("LocalController register : ", err)
		return
	}
	listener, err := net.Listen("tcp", ":8060")
	if err != nil {
		log.Fatal("LocalController listening problem: ", err)
	}
	defer func(listener net.Listener) {
		err := listener.Close()
		if err != nil {
			log.Fatal("LocalController listener closing: ", err)
			return
		}
	}(listener)
	rpc.Accept(listener)

	//if <- done {
	//	listener.Close()
	//}
}

func (l *LocalController) TurnCompletedHandler(req AliveReq, res *AliveRes) (e error) {
	l.c.events <- TurnComplete{CompletedTurns: req.CompletedTurns}
	return
}

// TellTheDifference pass the CellFlipped event to main channel
// server call the LocalController every turn to report CellFlipped event
func (l *LocalController) TellTheDifference(req AliveReq, res *AliveRes) (e error) {
	//fmt.Println("TellTheDifference get called")
	for y := 0; y < len(req.OldBoard); y++ {
		for x := 0; x < len(req.OldBoard); x++ {
			newVal := req.NewBoard[y][x]
			oldVal := req.OldBoard[y][x]
			if oldVal != newVal {
				currentTurn := req.CompletedTurns
				l.c.events <- CellFlipped{CompletedTurns: currentTurn, Cell: util.Cell{X: x, Y: y}}
			}
		}
	}
	return
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

// return with CalculatedGameBoard
func makeCall(client *rpc.Client, initialBoard [][]byte, p Params) [][]byte {
	req := Request{InitialGameBoard: initialBoard, P: p}
	res := new(Response)
	err := client.Call(BrokerMainHandler, req, res)
	if err != nil {
		log.Fatal("err while making call from distributor: ", err)
	}
	// TODO if defer close here would  err while making call to Pass FlippedCells:
	//  read ud 127.0.0.1:50026->127.0.0.1:8040: read: connection reset by peer
	//defer client.Close()
	return res.CalculatedGameBoard
}

// ticker
func ticker(client *rpc.Client, done chan bool, c distributorChannels) {
	ticker := time.NewTicker(2 * time.Second)
	go func() {
		for {
			select {
			case <-done:
				return
			case <-ticker.C:
				// make an RPC call to get live cells
				req := new(AliveReq)
				res := new(AliveRes)
				err := client.Call(GetAliveNumsFromBroker, req, res)
				if err != nil {
					log.Fatal("err while getting alive number from ticker: ", err)
				}
				defer client.Close()
				c.events <- AliveCellsCount{CompletedTurns: res.Turn, CellsCount: res.Num}
			}
		}
	}()
}
