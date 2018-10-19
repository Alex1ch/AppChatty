package main

import (
	"bytes"
	"encoding/binary"
	"fmt"
	"net"
	"time"
)

const (
	//ADDRESS Address of the Login Server
	ADDRESS = "10.1.1.167"
	//PORT Listen port for the Login Server
	PORT       = "1203"
	BUFFERSIZE = 2048
)

var users map[string]net.Conn

func handlePacket(client net.Conn, buffer []byte, totalLen int) {
	pkgLen := binary.LittleEndian.Uint32(buffer[0:4])
	opCode := binary.LittleEndian.Uint16(buffer[4:6])
	// Check
	fmt.Println(pkgLen, opCode)

	recLen, buffer := ReadPacket(client)
	if recLen == 0 {
		return
	}

	handlePacket(client, buffer, recLen)
}

func handleSession(client net.Conn) {
	fmt.Println("Login session started for " + client.RemoteAddr().String())

	recLen, buffer := ReadPacket(client)
	if recLen == 0 {
		return
	}

	var offset uint16 = 4
	opCode := binary.LittleEndian.Uint16(buffer[offset : offset+2])
	offset += 2
	if opCode != 4 {
		SendPacket(client, 401, []byte{})
		client.Close()
	}
	nLen := binary.LittleEndian.Uint16(buffer[offset : offset+2])
	offset += 2
	username := string(binary.LittleEndian.Uint16(buffer[offset : offset+nLen]))
	offset += nLen
	//passLen := binary.LittleEndian.Uint16(buffer[offset : offset+2])
	offset += 2
	//password := binary.LittleEndian.Uint16(buffer[offset : offset+passLen])

	users[username] = client

	handlePacket(client, buffer, recLen)
}

func listenClient(IP string, PORT string) int {
	socket, error := net.Listen("tcp", fmt.Sprintf("%s:%s", ADDRESS, PORT))
	if error != nil {
		fmt.Println("Error while server startup:\n" + error.Error())
		return 1
	}
	fmt.Printf("Chat server started at %s:%s", ADDRESS, PORT)
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
	listenClient(ADDRESS, PORT)
}

func ReadPacket(client net.Conn) (lenght int, buffer []byte) {
	//Optimization?
	buffer = make([]byte, BUFFERSIZE)
	client.SetDeadline(time.Now().Add(time.Duration(30000)))
	recLen, error := client.Read(buffer)
	if error != nil {
		fmt.Println("Error in message receiving:\n" + error.Error())
		client.Close()
		lenght = 0
		return
	}
	if recLen == 0 {
		fmt.Println("Recieved empty message")
		client.Close()
		lenght = 0
		return
	}
	return recLen, buffer
}

func SendPacket(client net.Conn, opCode uint16, data []byte) {
	var buffer bytes.Buffer
	opCodeB := make([]byte, 2)
	lenB := make([]byte, 4)
	binary.LittleEndian.PutUint32(lenB, uint32(len(data)))
	binary.LittleEndian.PutUint16(opCodeB, opCode)

	buffer.Write(lenB)
	buffer.Write(opCodeB)
	buffer.Write(data)
}
