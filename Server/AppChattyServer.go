package main

import (
	"bytes"
	"encoding/binary"
	"fmt"
	"net"
	"time"
)

const (
	//ADDRESS Address of the Server
	ADDRESS = "127.0.0.1"
	//PORT Listen port for the Server
	PORT = "1237"
	//BUFFERSIZE Size of the tcp buffer
	BUFFERSIZE = 1024
)

var users map[string]net.Conn

func handleNextPacket(client net.Conn) {
	// Check

	exitCode, recLen, opCode, buffer := readPacket(client, 0)
	fmt.Println(exitCode, recLen, opCode, buffer)
	if recLen == 0 {
		return
	}

	handleNextPacket(client)
}

func handleSession(client net.Conn) {
	fmt.Println("Session started for " + client.RemoteAddr().String())

	exitCode, _, opCode, buffer := readPacket(client, 0)
	if exitCode != 0 {
		return
	}

	var offset uint16 = 0
	if !(opCode == 4 || opCode == 5) {
		fmt.Println("Session error, anauthorized")
		sendPacket(client, 401, []byte{})
		client.Close()
		return
	}
	nLen := binary.LittleEndian.Uint16(buffer[offset : offset+2])
	offset += 2
	username := string(buffer[offset : offset+nLen])
	offset += nLen
	//passLen := binary.LittleEndian.Uint16(buffer[offset : offset+2])
	offset += 2
	//password := binary.LittleEndian.Uint16(buffer[offset : offset+passLen])

	users[username] = client
	fmt.Println("Received register from", username)

	sendPacket(client, 200, []byte{})

	handleNextPacket(client)
}

func listenClient(IP string, PORT string) int {
	socket, error := net.Listen("tcp", fmt.Sprintf("%s:%s", ADDRESS, PORT))
	if error != nil {
		fmt.Println("Error while server startup: " + error.Error())
		return 1
	}
	fmt.Printf("Chat server started at %s:%s\n", ADDRESS, PORT)
	for {
		connection, error := socket.Accept()
		if error != nil {
			fmt.Println("Error in connection establishment:\n" + error.Error())
			connection.Close()
			continue
		}

		go handleSession(connection)
	}
}

func main() {
	users = make(map[string]net.Conn)
	listenClient(ADDRESS, PORT)
}

func readPacket(client net.Conn, timeout int) (exitCode int, dataLen uint32, opCode uint16, buffer []byte) {
	//defer readRecover(client, &exitCode)
	dataLenB := make([]byte, 4)
	_, err := client.Read(dataLenB)
	if err != nil {
		fmt.Println("Error in message receiving(len): " + err.Error())
		client.Close()
		exitCode = 1
		return
	}
	dataLen = binary.LittleEndian.Uint32(dataLenB)
	opCodeB := make([]byte, 2)
	_, err = client.Read(opCodeB)
	if err != nil {
		fmt.Println("Error in message receiving(opCode): " + err.Error())
		client.Close()
		exitCode = 1
		return
	}
	opCode = binary.LittleEndian.Uint16(opCodeB)
	if dataLen != 0 {
		buffer = make([]byte, dataLen)
		if timeout != 0 {
			client.SetDeadline(time.Now().Add(time.Duration(timeout)))
		}
		_, err = client.Read(buffer)
		if err != nil {
			fmt.Println("Error in message receiving(data): " + err.Error())
			client.Close()
			exitCode = 1
			return
		}
	} else {
		buffer = nil
		exitCode = 0
		return
	}
	exitCode = 0
	return
}

//func readRecover(client net.Conn, exitCode *int) {
//	if r := recover(); r != nil {
//		fmt.Println("Recovered from ", r)
//		*exitCode = 1
//		client.Close()
//	}
//}

func sendPacket(client net.Conn, opCode uint16, data []byte) error {
	var buffer bytes.Buffer
	opCodeB := make([]byte, 2)
	if data != nil {
		lenB := make([]byte, 4)
		lenght := len(data)
		binary.LittleEndian.PutUint32(lenB, uint32(lenght))
		binary.LittleEndian.PutUint16(opCodeB, opCode)

		buffer.Write(lenB)
		buffer.Write(opCodeB)
		buffer.Write(data)
		_, err := client.Write(buffer.Bytes())
		if err != nil {
			fmt.Println("Error in message sending: " + err.Error())
			return err
		}
		return nil
	} else {
		lenB := []byte{0, 0, 0, 0}
		binary.LittleEndian.PutUint16(opCodeB, opCode)

		buffer.Write(lenB)
		buffer.Write(opCodeB)
		_, err := client.Write(buffer.Bytes())
		if err != nil {
			fmt.Println("Error in message sending: " + err.Error())
			return err
		}
		return nil
	}
}
