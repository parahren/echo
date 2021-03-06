/*
 * Copyright 2015, Rowe Technology Inc.
 * All rights reserved.
 * http://www.rowetechinc.com
 * https://github.com/rowetechinc
 *
 * Redistribution and use in source and binary forms, with or without modification, are
 * permitted provided that the following conditions are met:
 *
 *  1. Redistributions of source code must retain the above copyright notice, this list of
 *      conditions and the following disclaimer.
 *
 *  2. Redistributions in binary form must reproduce the above copyright notice, this list
 *      of conditions and the following disclaimer in the documentation and/or other materials
 *      provided with the distribution.
 *
 *  THIS SOFTWARE IS PROVIDED BY Rowe Technology Inc. ''AS IS'' AND ANY EXPRESS OR IMPLIED
 *  WARRANTIES, INCLUDING, BUT NOT LIMITED TO, THE IMPLIED WARRANTIES OF MERCHANTABILITY AND
 *  FITNESS FOR A PARTICULAR PURPOSE ARE DISCLAIMED. IN NO EVENT SHALL <COPYRIGHT HOLDER> OR
 *  CONTRIBUTORS BE LIABLE FOR ANY DIRECT, INDIRECT, INCIDENTAL, SPECIAL, EXEMPLARY, OR
 *  CONSEQUENTIAL DAMAGES (INCLUDING, BUT NOT LIMITED TO, PROCUREMENT OF SUBSTITUTE GOODS OR
 *  SERVICES; LOSS OF USE, DATA, OR PROFITS; OR BUSINESS INTERRUPTION) HOWEVER CAUSED AND ON
 *  ANY THEORY OF LIABILITY, WHETHER IN CONTRACT, STRICT LIABILITY, OR TORT (INCLUDING
 *  NEGLIGENCE OR OTHERWISE) ARISING IN ANY WAY OUT OF THE USE OF THIS SOFTWARE, EVEN IF
 *  ADVISED OF THE POSSIBILITY OF SUCH DAMAGE.
 *
 * The views and conclusions contained in the software and documentation are those of the
 * authors and should not be interpreted as representing official policies, either expressed
 * or implied, of Rowe Technology Inc.
 *
 *
 * HISTORY
 * -----------------------------------------------------------------
 * Date            Initials    Version    Comments
 * -----------------------------------------------------------------
 * 08/21/2015      RC          1.0        Initial coding
 *
 *
 */

package main

import (
	"log"
	"strconv"
	"strings"
)

// echoHub Connections and broadcast to
// send messages to all connections.
// A connection is either a serial port
// or websocket.
type echoHub struct {
	websocketConn   map[*websocketConn]bool // Registered connections.
	wsBroadcast     chan []byte             // Websocket broadcast.  This is messages from serial port to websocket.
	serialBroadcast chan []byte             // Serial port broadcast.  This is messages from websocket to serial port.
	register        chan *websocketConn     // Register requests from the connections.
	unregister      chan *websocketConn     // Unregister requests from connections.
}

// echo initializes the values.
// This will hold all the registered websocket
// connections.  It will also hold the send and receive
// buffer from the websockets.
var echo = echoHub{
	wsBroadcast:     make(chan []byte, 1000),       // Broadcast data to the websocket
	serialBroadcast: make(chan []byte, 1000),       // Broadcast data to the serial port
	register:        make(chan *websocketConn),     // Register a websocket connections
	unregister:      make(chan *websocketConn),     // Unregister a websocket connection
	websocketConn:   make(map[*websocketConn]bool), // Websocket connection map
}

// init starts the ECHO process.
// This will monitor all connections.
// And pass data between connections.
func (echo *echoHub) init(port *string, baudInt int) {

	// Start the serial port
	go serialHub.run()

	// Start Echo
	go echo.run()

	// If a port was given, open the port
	if len(*port) > 0 {
		go openSerialPort(*port, baudInt)
	}

}

// run the Echo process
// This will monitor websockets
// and serial ports for connections
// and disconnects.
func (echo *echoHub) run() {
	log.Print("Echo Hub running")
	for {
		select {

		// Register websocket
		case c := <-echo.register:
			// Register the websocket to the map
			echo.websocketConn[c] = true
			// send supported commands
			c.send <- []byte("{\"Version\" : \"" + version + "\"} ")
			c.send <- []byte("{\"Commands\" : [\"list\", \"open [portName] [baud]\", \"send [portName] [cmd]\",  \"close [portName]\", \"baudrates\", \"restart\", \"exit\", \"hostname\", \"version\"]} ")

			// Send the serial port list
			serialPortList()

			log.Println("Registering websocket")

		// Unregister websocket
		case c := <-echo.unregister:
			if _, ok := echo.websocketConn[c]; ok {

				log.Println("UnRegistering websocket")

				// Close the websocket send channel
				close(c.send)
				// Unregister the websocket from the map
				delete(echo.websocketConn, c)
			}

		// Data received from websocket
		case m := <-echo.serialBroadcast:
			log.Print("Got a serial broadcast " + string(m))
			if len(m) > 0 {
				// Check the command given
				checkCmd(m)
			}

		// Data received from the serial port
		case m := <-echo.wsBroadcast:
			//log.Print("Got a websocket broadcast" + string(m))

			// Send the data from broadcast to all websocket connections
			for c := range echo.websocketConn {
				select {
				case c.send <- m:
				default:
					log.Print("Close websocket send")
					close(c.send)
					delete(echo.websocketConn, c)
				}
			}

			// Send the data to all recorders

		}
		//log.Print("Echo Hub loop")
	}
}

// checkCmd will check which command was sent.
// It will then run the command based off the command given.
func checkCmd(cmd []byte) {
	log.Print("Inside checkCmd")
	s := string(cmd[:])
	log.Print(s)

	sl := strings.ToLower(s)

	if strings.HasPrefix(sl, "open") {
		openPort(s)
	} else if strings.HasPrefix(sl, "close") {
		closePort(s)
	} else if strings.HasPrefix(sl, "send") {
		// Write the data to the serial port
		spWrite(s)
	} else if strings.HasPrefix(sl, "list") {
		serialPortList()
	} else if strings.HasPrefix(sl, "record") {
		serialPortRecord(s)
	} else {

	}
	log.Print("leaving checkCmd")
}

// openPort will open the serial port.
// Cmd: OPEN COM6 115200
// Give the serial port and baud rate.
func openPort(cmd string) {
	// Trim the command
	cmd = strings.TrimPrefix(cmd, " ")

	// Split the command in to the 3 parameters
	cmds := strings.SplitN(cmd, " ", 3)
	if len(cmds) != 3 {
		errstr := "Could not parse open command: " + cmd
		log.Println(errstr)
		return
	}

	// Get the port name
	portname := strings.TrimSpace(cmds[1])

	// See if we have this port open
	_, isFound := findPortByName(portname)

	if isFound {
		//We found the serial port so it is already open
		log.Println("Serial port " + portname + " is already open.")

		// Close the serial port and reconnect
		closeSerialPort(portname)
	}

	// Convert the baud rate to int
	baudInt, err := strconv.Atoi(strings.TrimSpace(cmds[2]))
	if err != nil {
		log.Println("Baud rate give is bad", err)
		return
	}

	log.Printf("Opening Port %s at baud %d", portname, baudInt)

	// Open the serial port
	// This will also register the serial port
	go openSerialPort(portname, baudInt)
}

// closePort will close the serial port.
// Cmd: CLOSE COM6
// Give the serial port and baud rate.
func closePort(cmd string) {
	log.Println(cmd)
	// Trim the command
	cmd = strings.TrimPrefix(cmd, " ")

	// Split the command in to the 2 parameters
	cmds := strings.SplitN(cmd, " ", 2)
	if len(cmds) != 2 {
		errstr := "Could not parse close command: " + cmd
		log.Println(errstr)
		return
	}

	// Get the port name
	portname := strings.TrimSpace(cmds[1])

	log.Println(portname)

	// Close the given serial port
	closeSerialPort(portname)
}

// serialPortRecord will start or stop recording on the serial port.
// Cmd: RECORD COM6 START
// Give the serial port and baud rate.
func serialPortRecord(cmd string) {
	log.Println(cmd)
	// Trim the command
	cmd = strings.TrimPrefix(cmd, " ")

	// Split the command in to the 2 parameters
	cmds := strings.SplitN(cmd, " ", 3)
	if len(cmds) != 3 {
		errstr := "Could not parse close command: " + cmd
		log.Println(errstr)
		return
	}

	// Get the port name
	portname := strings.TrimSpace(cmds[1])

	log.Println(portname)

	if (strings.ToUpper(strings.TrimSpace(cmds[2])) == "START") {
		log.Printf("Start recording command recevied: %s", cmds[2])
		// Close the given serial port
		startRecording(portname)
	} else {
		log.Printf("Stop recording command received: %s", cmds[2])
		stopRecording(portname)
	}
}
