package main

import (
	"../IO"
	"../def"
	"../fsm"
	"../ordermanager"
	"fmt"
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
	msg_fromHWButton := make(chan def.MapMessage, 100)
	msg_toHW := make(chan def.MapMessage, 100)
	msg_toNetwork := make(chan def.MapMessage, 100)
	msg_fromNetwork := make(chan def.MapMessage, 100)
	msg_fromFSM := make(chan def.MapMessage, 100)
	msg_deadElev := make(chan def.MapMessage, 100)

	drv_buttons := make(chan IO.ButtonEvent)
	drv_floors := make(chan int)
	fsm_chn := make(chan bool, 1)
	elevator_map_chn := make(chan def.MapMessage)

	go IO.PollButtons(drv_buttons)
	go IO.PollFloorSensor(drv_floors)

	motor_direction = IO.MD_Down

	go fsm.FSM(drv_buttons, drv_floors, fsm_chn, elevator_map_chn, motor_direction, msg_fromFSM, msg_deadElev)

	transmitTicker := time.NewTicker(100 * time.Millisecond)

	var newMsg def.MapMessage
	transmitFlag := false

	for {
		select {
		case msg := <-msg_fromHWButton:
			msg_buttonEvent <- msg

		case msg := <-msg_fromNetwork:
			recievedMap := msg.SendMap.(ordermanager.ElevatorMap)
			currentMap, buttonPushes := ordermanager.GetNewEvent(recievedMap)

			newMsg = def.MakeMapMessage(currentMap, nil)
			msg_toHW <- newMsg

			for _, push := range buttonPushes {
				fsmEvent := def.NewEvent{def.BUTTON_PUSHED, []int{push[0], push[1]}}

				newMsg = def.MakeMapMessage(currentMap, fsmEvent)

				msg_buttonEvent <- newMsg
			}

		case msg := <-msg_fromFSM:
			fmt.Println("case msg_fromFSM in main")
			recievedMap := msg.SendMap.(ordermanager.ElevatorMap)
			currentMap, changeMade := ordermanager.UpdateElevMap(recievedMap)

			newMsg = def.MakeMapMessage(currentMap, nil)
			msg_toHW <- newMsg

			if changeMade {
				transmitFlag = true
			}
		}

		select {
		case <-transmitTicker.C:
			if transmitFlag {
				if newMsg.SendMap != nil {
					msg_toNetwork <- newMsg
					transmitFlag = false
				}
			}
		}
	}
}
