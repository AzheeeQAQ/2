package gol

// BrokerMainHandler Handler using exported method to call server
var BrokerMainHandler = "Broker.CalculateNextState"

var GetAliveNumsFromBroker = "Broker.GetAliveNums"

var GetCompletedTurns = "Broker.GetCompletedTurns"

var GetCurrentBoard = "Broker.GetCurrentBoard"

var LockMutex = "Broker.LockMutex"

var UnlockMutex = "Broker.UnlockMutex"

var ControllerOut = "Broker.ControllerOut"

var PowerOff = "Broker.PowerOff"

var ServerOff = "AWSEngine.ServerOff"

var WorkerMainHandler = "AWSEngine.CalculateNextState"

var DifferentCells = "LocalController.TellTheDifference"

var TurnCompletedHandler = "LocalController.TurnCompletedHandler"

type Response struct {
	CalculatedGameBoard [][]byte
}

type Request struct {
	InitialGameBoard [][]byte
	P                Params
	UpperBound		 []byte
	LowerBound		 []byte
}

type AliveReq struct {
	OldBoard		[][]byte
	NewBoard		[][]byte
	CompletedTurns	int
}

type AliveRes struct {
	Turn            int
	Num  			int
	CurrentBoard    [][]byte
}


