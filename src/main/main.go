package main

import (
	"../def"
	"../IO"
	"../ordermanager"
	"../fsm"
	//"net"
	//"../network/bcast"
	//"../network/localip"
	// "flag"
	"fmt"
	//"os"
	"time"
)

func main() {
		backup := ordermanager.AmIBackup()
		fmt.Println("backup: ", backup)
	  IO.Init("localhost:15657", def.NUMFLOORS)

		ordermanager.InitElevMap(backup)
		go ordermanager.SoftwareBackup()

		var motor_direction IO.MotorDirection


		msg_buttonEvent := make(chan def.MapMessage, 100)
		msg_fromHWFloor := make(chan def.MapMessage, 100)
		msg_fromHWButton := make(chan def.MapMessage, 100)
		msg_toHW := make(chan def.MapMessage, 100)
		msg_toNetwork := make(chan def.MapMessage, 100)
		msg_fromNetwork := make(chan def.MapMessage, 100)
		msg_fromFSM := make(chan def.MapMessage, 100)
		msg_deadElev := make(chan def.MapMessage, 100)

	  drv_buttons := make(chan IO.ButtonEvent)
	  drv_floors  := make(chan int)
		fsm_chn			:= make(chan bool, 1)
		elevator_map_chn := make(chan def.MapMessage)

		go IO.PollButtons(drv_buttons)
		go IO.PollFloorSensor(drv_floors)


		drv_obstr   := make(chan bool)
	  drv_stop    := make(chan bool)
	  go IO.PollObstructionSwitch(drv_obstr)
	  go IO.PollStopButton(drv_stop)

		motor_direction = IO.MD_Down

		go fsm.FSM(drv_buttons, drv_floors, fsm_chn, elevator_map_chn, motor_direction, msg_buttonEvent, msg_fromHWFloor, msg_fromHWButton, msg_fromFSM, msg_deadElev)

		transmitTicker := time.NewTicker(100 * time.Millisecond)

		currentMap := ordermanager.GetElevMap()
		var newMsg def.MapMessage
		transmitFlag := false
	    for {
					fmt.Println("Looping")
					currentMap = ordermanager.GetElevMap()
	        select {
	        case msg_button := <- drv_buttons:
							currentMap[def.LOCAL_ID].Buttons[msg_button.Floor][msg_button.Button] = 1
							sendMessage := def.MakeMapMessage(currentMap, nil)
							newMap, _ := ordermanager.UpdateElevMap(sendMessage.SendMap.(ordermanager.ElevatorMap))
							newMap[1].Dir = 0 // fordi newMap is declared and not used...
	            IO.SetButtonLamp(msg_button.Button, msg_button.Floor, true)
							//bcast_chn <- msg_button

	        case msg_floor := <- drv_floors:
	            if msg_floor == def.NUMFLOORS-1 {
	                motor_direction = IO.MD_Down
	            } else if msg_floor == 0 {
	                motor_direction = IO.MD_Up
	            }
							currentMap[def.LOCAL_ID].Dir = motor_direction
							currentMap[def.LOCAL_ID].Floor = msg_floor
						  sendMessage := def.MakeMapMessage(currentMap, nil)
						  newMap, _ := ordermanager.UpdateElevMap(sendMessage.SendMap.(ordermanager.ElevatorMap))
							newMap[1].Dir = 0 // fordi newMap is declared and not used...
	            IO.SetMotorDirection(motor_direction)


	        case <- drv_obstr:
	            fsm.Dust(msg_fromFSM)

	        case msg_stop := <- drv_stop:
	            fmt.Printf("%+v\n", msg_stop)
	            for floor := 0; floor < def.NUMFLOORS; floor++ {
	                for button := IO.ButtonType(0); button < def.NUMBUTTON_TYPES; button++ {
	                    IO.SetButtonLamp(button, floor, false)
									}
							}

					//case msg_recieve := <- recieve_chn:
						//	fmt.Printf("%+v\n", msg_recieve)

	        }
	    }

			for {
				select {
				case msg := <- msg_fromHWButton:
					msg_buttonEvent <- msg

				case msg := <- msg_fromNetwork:
					recievedMap := msg.SendMap.(ordermanager.ElevatorMap)
					currentMap, buttonPushes := ordermanager.GetNewEvent(recievedMap)

					newMsg = def.MakeMapMessage(currentMap, nil)
					msg_toHW <- newMsg

					for _, push := range buttonPushes {
						fsmEvent := def.NewEvent{def.BUTTON_PUSHED, []int{push[0], push[1]}}

						newMsg = def.MakeMapMessage(currentMap, fsmEvent)

						msg_buttonEvent <- newMsg
					}

				case msg := <- msg_fromFSM:
					recievedMap := msg.SendMap.(ordermanager.ElevatorMap)
					currentMap, changeMade := ordermanager.UpdateElevMap(recievedMap)

					newMsg = def.MakeMapMessage(currentMap, nil)
					msg_toHW <- newMsg

					if changeMade {
						transmitFlag = true
					}
				}

				select {
				case <- transmitTicker.C:
					if transmitFlag{
						if newMsg.SendMap != nil {
							msg_toNetwork <- newMsg
							transmitFlag = false
						}
					}
				}
			}
	}
