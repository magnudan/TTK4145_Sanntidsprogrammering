package fsm

import (
	"fmt"
	"time"

	"../IO"
	"../def"
	"../ordermanager"
)

var elevator_state def.ElevState = def.S_Dead
var motor_direction IO.MotorDirection
var initialized bool = false

func timer(timeout chan<- bool) {
	fmt.Println("Timer started")
	time.Sleep(5 * time.Second)
	timeout <- true
	fmt.Println("Timeout sent")
}

func Initialize(floor_detection <-chan int, fsm_chn chan<- bool, elevator_map_chn chan<- def.MapMessage, direction IO.MotorDirection) {
	currentMap := ordermanager.GetElevMap()

	timeout := make(chan bool, 1)
	elevator_state = def.S_Init
	currentMap[def.LOCAL_ID].State = elevator_state
	motor_direction = direction
	currentMap[def.LOCAL_ID].Dir = motor_direction
	sendMessage := def.MakeMapMessage(currentMap, nil)
	fmt.Println("updateElevMap Initialize")
	newMap, _ := ordermanager.UpdateElevMap(sendMessage.SendMap.(ordermanager.ElevatorMap))
	IO.SetMotorDirection(motor_direction)
	go timer(timeout)
	newMap[1].Dir = 0 // fordi newMap is declared and not used...

	select {
	case <-floor_detection:
		fmt.Println("Floor detected")
		motor_direction = IO.MD_Stop
		sendMessage = def.MakeMapMessage(nil, motor_direction)
		currentMap[def.LOCAL_ID].Dir = motor_direction
		sendMessage := def.MakeMapMessage(currentMap, nil)

		newMap, _ := ordermanager.UpdateElevMap(sendMessage.SendMap.(ordermanager.ElevatorMap))
		newMap[1].Dir = 0 // fordi newMap is declared and not used...
		IO.SetMotorDirection(motor_direction)
		elevator_state = def.S_Idle
		currentMap[def.LOCAL_ID].State = elevator_state
		sendMessage = def.MakeMapMessage(currentMap, nil)

		newMap, _ = ordermanager.UpdateElevMap(sendMessage.SendMap.(ordermanager.ElevatorMap))
		time.Sleep(2 * time.Second)
		fsm_chn <- true
	case <-timeout:
		fmt.Println("Timed out")
		Initialize(floor_detection, fsm_chn, elevator_map_chn, -motor_direction)
	}
}


func Dust(msg_fromFSM chan def.MapMessage) {
	fmt.Println("func: Dust")
	currentMap := ordermanager.GetElevMap()
	currentMap[def.LOCAL_ID].State = def.S_Dead
	message := def.MakeMapMessage(currentMap, nil)
	msg_fromFSM <- message
}


func FSM(drv_buttons chan IO.ButtonEvent, drv_floors chan int, fsm_chn chan bool, elevator_map_chn chan def.MapMessage, direction IO.MotorDirection, msg_fromFSM chan def.MapMessage, msg_deadElev chan def.MapMessage) {

	if initialized == false {
		Initialize(drv_floors, fsm_chn, elevator_map_chn, IO.MD_Up)
		<-fsm_chn
		initialized = true
		fmt.Println("Elevator is initialized!")
	}

	doorTimer := time.NewTimer(def.DOOR_TIMEOUT_TIME * time.Second)
	doorTimer.Stop()
	idleTimer := time.NewTimer(def.IDLE_TIMEOUT_TIME * time.Second)

	for {
		select {

		case arrivalFloor := <-drv_floors: //elevator arrives at new floor
			fmt.Println("case msg from drv_floors FSM")
      currentMap := ordermanager.GetElevMap()

      FloorArrival(msg_fromFSM, arrivalFloor, doorTimer)
			idleTimer.Reset(def.IDLE_TIMEOUT_TIME * time.Second)

      //for looping purposes only
      if arrivalFloor == def.NUMFLOORS-1 {
				motor_direction = IO.MD_Down
			} else if arrivalFloor == 0 {
				motor_direction = IO.MD_Up
			}
      IO.SetMotorDirection(motor_direction)

			currentMap = ordermanager.GetElevMap()
      //update currentMap
			if motor_direction != IO.MD_Stop && currentMap[def.LOCAL_ID].State != def.S_Dead {
				currentMap[def.LOCAL_ID].State = def.S_Moving
        elevator_state = def.S_Moving
			}
      currentMap[def.LOCAL_ID].Dir = motor_direction
			currentMap[def.LOCAL_ID].Floor = arrivalFloor
      currentMap[def.LOCAL_ID].State = elevator_state

      //send map to main
			SendMapMessage(msg_fromFSM, currentMap, nil)


		case msg := <-msg_deadElev: //elevator is dead
      fmt.Println("case message from msg_deadElev in FSM")

			switch msg.SendEvent.(def.NewEvent).EventType {
			case def.ELEVATOR_DEAD:
				DeadElevator(msg_fromFSM, msg.SendEvent.(def.NewEvent).Type.(int))
				idleTimer.Reset(def.IDLE_TIMEOUT_TIME * time.Second)
			}

		case msg_button := <-drv_buttons: //detects new buttons pushed
			fmt.Println("case msg from drv_buttons in FSM")
      currentMap := ordermanager.GetElevMap()

      IO.SetButtonLamp(msg_button.Button, msg_button.Floor, true)
      ButtonPushed(msg_fromFSM, msg_button.Floor, int(msg_button.Button), doorTimer)
			idleTimer.Reset(def.IDLE_TIMEOUT_TIME * time.Second)

      //update currentMap
			SendMapMessage(msg_fromFSM, currentMap, nil)


		default:

		}
	}
}


func ChooseDirection(currentMap ordermanager.ElevatorMap) IO.MotorDirection {
	currentFloor := currentMap[def.LOCAL_ID].Floor

	if currentMap[def.LOCAL_ID].Orders[currentFloor][IO.BT_Cab] == 2 {
		return IO.MD_Stop
	}

	switch motor_direction {
	case IO.MD_Up:
		if currentMap[def.LOCAL_ID].Floor != def.NUMFLOORS {
			if currentMap[def.LOCAL_ID].Orders[currentFloor][IO.BT_HallUp] == 2 {
				return IO.MD_Stop
			}

			for floor := currentMap[def.LOCAL_ID].Floor + 1; floor < def.NUMFLOORS; floor++ {
				if IsOrderOnFloor(currentMap, floor) || ordermanager.IsClosestElevator(currentMap, floor) {
					return IO.MD_Up
				}
			}
		}

		if currentMap[def.LOCAL_ID].Floor != 0 {
			if currentMap[def.LOCAL_ID].Orders[currentFloor][IO.BT_HallDown] == 2 {
				return IO.MD_Stop
			}

			for floor := currentMap[def.LOCAL_ID].Floor - 1; floor > -1; floor-- {
				if IsOrderOnFloor(currentMap, floor) || ordermanager.IsClosestElevator(currentMap, floor) {
					return IO.MD_Down
				}
			}
		}

		return IO.MD_Stop

	case IO.MD_Down:
		if currentMap[def.LOCAL_ID].Floor != 0 {
			if currentMap[def.LOCAL_ID].Orders[currentFloor][IO.BT_HallDown] == 2 {
				return IO.MD_Stop
			}

			for floor := currentMap[def.LOCAL_ID].Floor - 1; floor > -1; floor-- {
				if IsOrderOnFloor(currentMap, floor) || ordermanager.IsClosestElevator(currentMap, floor) {
					return IO.MD_Down
				}
			}
		}

		if currentMap[def.LOCAL_ID].Floor != def.NUMFLOORS {
			if currentMap[def.LOCAL_ID].Orders[currentFloor][IO.BT_HallUp] == ordermanager.ORDER_ACCEPTED {
				return IO.MD_Stop
			}

			for floor := currentMap[def.LOCAL_ID].Floor + 1; floor < def.NUMFLOORS; floor++ {
				if IsOrderOnFloor(currentMap, floor) || ordermanager.IsClosestElevator(currentMap, floor) {
					return IO.MD_Up
				}
			}
		}

		return IO.MD_Stop

	case IO.MD_Stop:
		if currentMap[def.LOCAL_ID].Floor != 0 {
			for floor := currentMap[def.LOCAL_ID].Floor - 1; floor > -1; floor-- {
				if IsOrderOnFloor(currentMap, floor) || ordermanager.IsClosestElevator(currentMap, floor) {
					return IO.MD_Down
				}
			}
		}

		if currentMap[def.LOCAL_ID].Floor != def.NUMFLOORS {
			for floor := currentMap[def.LOCAL_ID].Floor + 1; floor < def.NUMFLOORS; floor++ {
				if IsOrderOnFloor(currentMap, floor) || ordermanager.IsClosestElevator(currentMap, floor) {
					return IO.MD_Up
				}
			}
		}

		return IO.MD_Stop

	default:
		return IO.MD_Stop
	}
}


func OrderAbove(currentMap ordermanager.ElevatorMap) bool {
	for floor := currentMap[def.LOCAL_ID].Floor + 1; floor < def.NUMFLOORS; floor++ {
		if IsOrderOnFloor(currentMap, floor) {
			return true
		}
	}
	return false
}


func OrderBelow(currentMap ordermanager.ElevatorMap) bool {
	for floor := 0; floor < currentMap[def.LOCAL_ID].Floor; floor++ {
		if IsOrderOnFloor(currentMap, floor) {
			return true
		}
	}
	return false
}


func DeleteOrdersOnFloor(currentMap ordermanager.ElevatorMap, currentFloor int) ordermanager.ElevatorMap {
  //also turns off light for the orders deleted
	for elev := 0; elev < def.NUMELEVATORS; elev++ {
		currentMap[elev].Orders[currentFloor][IO.BT_HallUp] = ordermanager.NO_ORDER
		currentMap[elev].Orders[currentFloor][IO.BT_HallDown] = ordermanager.NO_ORDER
	}

	currentMap[def.LOCAL_ID].Orders[currentFloor][IO.BT_Cab] = ordermanager.NO_ORDER

	SetButtonLights(currentMap)
	return currentMap
}


func IsOrderOnFloor(currentMap ordermanager.ElevatorMap, currentFloor int) bool {
	for button := 0; button < def.NUMBUTTON_TYPES; button++ {
		if currentMap[def.LOCAL_ID].Orders[currentFloor][button] == ordermanager.ORDER_ACCEPTED {
			return true
		}
	}
	return false
}


func FloorArrival(msg_fromFSM chan def.MapMessage, arrivalFloor int, doorTimer *time.Timer) {
	currentMap := ordermanager.GetElevMap()
	currentMap[def.LOCAL_ID].Floor = arrivalFloor
  IO.SetFloorIndicator(arrivalFloor)

  //check if there is an order on floor, if so delete orders (also do lights)
	switch elevator_state {
	case def.S_Idle:
    //if order on floor, delete orders and set door open
    if IsOrderOnFloor(currentMap, arrivalFloor) {
      //direction i stop and door is set open
      motor_direction = IO.MD_Stop
      elevator_state = def.S_DoorOpen

      IO.SetMotorDirection(motor_direction)
      IO.SetDoorOpenLamp(true)

      currentMap = DeleteOrdersOnFloor(currentMap, arrivalFloor)
			fmt.Println("case def.S_Idle in FloorArrival")
			fmt.Println(currentMap)
      currentMap[def.LOCAL_ID].Dir = motor_direction
      currentMap[def.LOCAL_ID].State = elevator_state
			SendMapMessage(msg_fromFSM, currentMap, nil)


      //wait for door timer, then set state to idle
      doorTimer.Reset(def.DOOR_TIMEOUT_TIME * time.Second)
      <-doorTimer.C
			DoorTimeout(msg_fromFSM)
    }

    motor_direction = ChooseDirection(currentMap)
    if motor_direction != IO.MD_Stop {
      elevator_state = def.S_Moving
    } else {
      elevator_state = def.S_Idle
    }
    IO.SetMotorDirection(motor_direction)

    currentMap[def.LOCAL_ID].Dir = motor_direction
    currentMap[def.LOCAL_ID].State = elevator_state
		SendMapMessage(msg_fromFSM, currentMap, nil)


	case def.S_Moving:
		if PossibleStop(currentMap) || IsOrderOnFloor(currentMap, arrivalFloor) {
      motor_direction = IO.MD_Stop
      elevator_state = def.S_DoorOpen

      IO.SetMotorDirection(motor_direction)
			IO.SetDoorOpenLamp(true)
      doorTimer.Reset(def.DOOR_TIMEOUT_TIME * time.Second)

			currentMap := DeleteOrdersOnFloor(currentMap, arrivalFloor)
			fmt.Println("case def.S_Moving in FloorArrival")
			fmt.Println(currentMap[def.LOCAL_ID].Buttons)
      currentMap[def.LOCAL_ID].Dir = motor_direction
      currentMap[def.LOCAL_ID].State = elevator_state
      SendMapMessage(msg_fromFSM, currentMap, nil)

      <-doorTimer.C
			DoorTimeout(msg_fromFSM)
    }

    case def.S_DoorOpen:
      DoorTimeout(msg_fromFSM)
	}
}


func DeadElevator(msg_fromFSM chan def.MapMessage, deadElevID int) {
	currentMap := ordermanager.GetElevMap()
	currentMap[deadElevID].State = def.S_Dead
	for button := 0; button < def.NUMBUTTON_TYPES; button++ {
		for floor := 0; floor < def.NUMFLOORS; floor++ {
			currentMap[deadElevID].Orders[floor][button] = ordermanager.ORDER_IMPOSSIBLE
		}
	}

	SendMapMessage(msg_fromFSM, currentMap, nil)

	//dette lurer jeg veldig på om vi trenger, skjønner ikke helt hvorfor det er her
	switch elevator_state {
	case def.S_Idle:
		motor_direction = ChooseDirection(currentMap)
		IO.SetMotorDirection(motor_direction)
		currentMap[def.LOCAL_ID].Dir = motor_direction

		if motor_direction != IO.MD_Stop {
			elevator_state = def.S_Moving
		}
	}
	currentMap[def.LOCAL_ID].State = elevator_state
	SendMapMessage(msg_fromFSM, currentMap, nil)
}


func ButtonPushed(msg_fromFSM chan def.MapMessage, floor int, button int, doorTimer *time.Timer) {
	currentMap := ordermanager.GetElevMap()
	SetButtonLights(currentMap)

	switch elevator_state {
	case def.S_Idle:
		if currentMap[def.LOCAL_ID].Floor == floor {
			currentMap = DeleteOrdersOnFloor(currentMap, floor)

			IO.SetDoorOpenLamp(true)
			doorTimer.Reset(def.DOOR_TIMEOUT_TIME * time.Second)

			elevator_state = def.S_DoorOpen
			currentMap[def.LOCAL_ID].State = elevator_state
			SendMapMessage(msg_fromFSM, currentMap, nil)

			<-doorTimer.C
			DoorTimeout(msg_fromFSM)

		} else {
			motor_direction = ChooseDirection(currentMap)
			IO.SetMotorDirection(motor_direction)

			if motor_direction != IO.MD_Stop {
				elevator_state = def.S_Moving
			}

			currentMap[def.LOCAL_ID].State = def.S_DoorOpen
			currentMap[def.LOCAL_ID].Dir = motor_direction
			SendMapMessage(msg_fromFSM, currentMap, nil)
		}

	case def.S_DoorOpen:
		if currentMap[def.LOCAL_ID].Floor == floor {
			currentMap = DeleteOrdersOnFloor(currentMap, floor)
			doorTimer.Reset(def.DOOR_TIMEOUT_TIME * time.Second)

			SendMapMessage(msg_fromFSM, currentMap, nil)
		} else {
			if currentMap[def.LOCAL_ID].Orders[floor][button] == ordermanager.ORDER_ACCEPTED {
				DoorTimeout(msg_fromFSM)
			}
				SetButtonLights(currentMap)
				SendMapMessage(msg_fromFSM, currentMap, nil)
			}

	case def.S_Moving:
		if currentMap[def.LOCAL_ID].Buttons[floor][button] != 1 {
			currentMap[def.LOCAL_ID].Buttons[floor][button] = 1

			if ordermanager.IsClosestElevator(currentMap, floor) {
				currentMap[def.LOCAL_ID].Orders[floor][button] = ordermanager.ORDER_ACCEPTED
			}

			SendMapMessage(msg_fromFSM, currentMap, nil)
		}

	default:

	}
}


func DoorTimeout(msg_fromFSM chan def.MapMessage) {
	switch elevator_state {
	case def.S_DoorOpen:
		currentMap := ordermanager.GetElevMap()
		IO.SetDoorOpenLamp(false)

		motor_direction = ChooseDirection(currentMap)
		IO.SetMotorDirection(motor_direction)

		if motor_direction == IO.MD_Stop {
			elevator_state = def.S_Idle
		} else {
			elevator_state = def.S_Moving
		}

		currentMap[def.LOCAL_ID].Dir = motor_direction
		currentMap[def.LOCAL_ID].State = elevator_state
		SendMapMessage(msg_fromFSM, currentMap, nil)
	}
}


func PossibleStop(currentMap ordermanager.ElevatorMap) bool { //run at every new floor to check if elev should stop
	floor := currentMap[def.LOCAL_ID].Floor

	if IsOrderOnFloor(currentMap, floor) {
		return true
	}

	switch motor_direction {
	case IO.MD_Up:
		if currentMap[def.LOCAL_ID].Orders[floor][IO.BT_Cab] == ordermanager.ORDER_ACCEPTED || currentMap[def.LOCAL_ID].Orders[floor][IO.BT_HallUp] == ordermanager.ORDER_ACCEPTED {
			return true
		} else if currentMap[def.LOCAL_ID].Orders[floor][IO.BT_HallDown] == ordermanager.ORDER_ACCEPTED && OrderBelow(currentMap) {
			return true
		} else if floor == def.NUMFLOORS {
			return true
		} else if OrderBelow(currentMap) {
			return true
		}

	case IO.MD_Down:
		if currentMap[def.LOCAL_ID].Orders[floor][IO.BT_Cab] == ordermanager.ORDER_ACCEPTED || currentMap[def.LOCAL_ID].Orders[floor][IO.BT_HallDown] == ordermanager.ORDER_ACCEPTED {
			return true
		} else if currentMap[def.LOCAL_ID].Orders[floor][IO.BT_HallUp] == ordermanager.ORDER_ACCEPTED && OrderAbove(currentMap) {
			return true
		} else if floor == 0 {
			return true
		} else if OrderAbove(currentMap) {
			return true
		}

	default:
		return false
	}
	return false
}


func SetButtonLights(currentMap ordermanager.ElevatorMap) {
	currentFloor := currentMap[def.LOCAL_ID].Floor
	for elev := 0; elev < def.NUMELEVATORS; elev++ {
		for floor := 0; floor < def.NUMFLOORS; floor++ {
			if currentMap[elev].Orders[floor][IO.BT_HallUp] == ordermanager.ORDER_ACCEPTED {
				IO.SetButtonLamp(IO.BT_HallUp, floor, true)
			} else {
				IO.SetButtonLamp(IO.BT_HallUp, floor, false)
			}

			if currentMap[elev].Orders[floor][IO.BT_HallDown] == ordermanager.ORDER_ACCEPTED {
				IO.SetButtonLamp(IO.BT_HallDown, floor, true)
			} else {
				IO.SetButtonLamp(IO.BT_HallDown, floor, false)
			}
		}
	}

	if currentMap[def.LOCAL_ID].Orders[currentFloor][IO.BT_Cab] == ordermanager.ORDER_ACCEPTED {
		IO.SetButtonLamp(IO.BT_Cab, currentFloor, true)
	} else {
		IO.SetButtonLamp(IO.BT_Cab, currentFloor, false)
	}
}


func SendMapMessage(msg_fromFSM chan def.MapMessage, newMap interface{}, newEvent interface{}) {
	sendMsg := def.MakeMapMessage(newMap, newEvent)
	msg_fromFSM <- sendMsg
}
