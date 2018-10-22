package main

import (
	"bytes"
	"crypto/sha256"
	"encoding/binary"
	"fmt"
	"log"
	"net"
	"reflect"
	"time"

	"github.com/jinzhu/gorm"
	_ "github.com/jinzhu/gorm/dialects/sqlite"
)

var (
	users map[string]net.Conn
	appDB *gorm.DB
)

const (
	//ADDRESS Address of the Server
	ADDRESS = "127.0.0.1"
	//PORT Listen port for the Server
	PORT = "1237"
	//BUFFERSIZE Size of the tcp buffer
	BUFFERSIZE = 1024
)

//DB Structures
type userStruct struct {
	gorm.Model
	Username string
	Hash     []byte
}

func main() {
	//Initialization
	var err error
	appDB, err = gorm.Open("sqlite3", "AppChattyServer.db")
	if err != nil {
		panic("failed to connect database")
	}
	defer appDB.Close()

	appDB.AutoMigrate(&userStruct{})
	users = make(map[string]net.Conn)

	//ListenStart
	listenClient(ADDRESS, PORT)
}

func handleNextPacket(client net.Conn) {
	var (
		err    error
		recLen uint32
		opCode uint16
		buffer []byte
	)

	for {
		err, recLen, opCode, buffer = readPacket(client, 0)
		if err != nil {
			log.Println(err.Error())
			return
		}
		fmt.Println(recLen, opCode, buffer)
		switch opCode {
		case 1:

		default:
		}
	}
	handleNextPacket(client)
}

func handleSession(client net.Conn) {
	fmt.Println("Session started for " + client.RemoteAddr().String())

	var (
		err      error
		opCode   uint16
		buffer   []byte
		offset   uint16
		nLen     byte
		username string
		passLen  byte
		password []byte
		hash     [32]byte
	)

	for {
		err, _, opCode, buffer = readPacket(client, 0)
		if err != nil {
			log.Println(err.Error())
			return
		}

		offset = 0
		if !(opCode == 4 || opCode == 5) {
			fmt.Println("Session error, anauthorized")
			sendPacket(client, 401, nil)
			client.Close()
			return
		}
		nLen = buffer[offset]
		offset += 1
		username = string(buffer[offset : offset+uint16(nLen)])
		offset += uint16(nLen)
		passLen = buffer[offset]
		offset += 1
		password = buffer[offset : offset+uint16(passLen)]

		hash = sha256.Sum256(password)
		if opCode == 5 {
			var user userStruct
			appDB.First(&user, "username = ?", username)
			if reflect.DeepEqual(user, userStruct{}) {
				sendPacket(client, 404, nil)
				fmt.Println("Received NX auth from", username)
			} else if bytes.Equal(hash[:], user.Hash[:]) {
				sendPacket(client, 200, nil)
				fmt.Println("Received auth from", username)
				break
			} else {
				sendPacket(client, 423, nil)
				fmt.Println("Received wrong password from", username)
			}
		} else if opCode == 4 {
			fmt.Println("Received register from", username)
			appDB.Create(&userStruct{Username: username, Hash: hash[:]})
			sendPacket(client, 200, nil)
			break
		}
	}

	users[username] = client

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

func readPacket(client net.Conn, timeout int) (out_err error, dataLen uint32, opCode uint16, buffer []byte) {
	//defer readRecover(client, &exitCode)
	dataLenB := make([]byte, 4)
	_, err := client.Read(dataLenB)
	if err != nil {
		fmt.Println("Error in message receiving(len): " + err.Error())
		client.Close()
		out_err = err
		return
	}
	dataLen = binary.LittleEndian.Uint32(dataLenB)
	opCodeB := make([]byte, 2)
	_, err = client.Read(opCodeB)
	if err != nil {
		fmt.Println("Error in message receiving(opCode): " + err.Error())
		client.Close()
		out_err = err
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
			out_err = err
			return
		}
	} else {
		buffer = nil
		return
	}
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
