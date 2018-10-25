package main

import (
	"bytes"
	"crypto/sha256"
	"encoding/binary"
	"errors"
	"fmt"
	"log"
	"net"
	"reflect"
	"time"

	"github.com/jinzhu/gorm"
	_ "github.com/jinzhu/gorm/dialects/sqlite"
)

var (
	users        map[string]net.Conn
	appDB        *gorm.DB
	none, none64 []byte
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
	none = []byte{0xff, 0xff, 0xff, 0xff}
	none64 = []byte{0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff}

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

func handlePacket(client net.Conn) {
	var (
		err    error
		recLen uint32
		opCode uint16
		buffer []byte
	)

	for {
		err, recLen, opCode, buffer = readPacket(client, 0)
		if err != nil {
			sendPacket(client, 400, nil)
			log.Println(err.Error())
			return
		}
		fmt.Println(recLen, opCode, buffer)
		switch opCode {
		case 6:
			id, err := getUserIDbyName(buffer)
			if err != nil {
				log.Println(err.Error())
				sendPacket(client, 404, nil)
			}
			idB := make([]byte, 8)
			binary.LittleEndian.PutUint64(idB, id)
			sendPacket(client, 200, idB)
		default:
		}
	}
	//handleNextPacket(client)
}

func handleSession(client net.Conn) {
	fmt.Println("Session started for " + client.RemoteAddr().String())

	var (
		err      error
		opCode   uint16
		buffer   []byte
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

		parser := parserStruct{buffer, uint32(len(buffer)), 0}

		if !(opCode == 4 || opCode == 5) {
			fmt.Println("Session error, unauthorized")
			sendPacket(client, 401, nil)
			client.Close()
			return
		}
		nLen, err = parser.Byte()
		if err != nil {
			return
		}
		if nLen == 0 {
			fmt.Println("Session error, bad request")
			sendPacket(client, 400, nil)
			client.Close()
			return
		}
		username, err = parser.String(uint32(nLen))
		if err != nil {
			return
		}
		passLen, err = parser.Byte()
		if err != nil {
			return
		}
		if passLen == 0 {
			fmt.Println("Session error, bad request")
			sendPacket(client, 400, nil)
			client.Close()
			return
		}
		password, err = parser.Chunk(uint32(passLen))
		if err != nil {
			return
		}

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
			var user userStruct
			appDB.First(&user, "username = ?", username)
			if reflect.DeepEqual(user, userStruct{}) {
				fmt.Println("Received register from", username)
				appDB.Create(&userStruct{Username: username, Hash: hash[:]})
				sendPacket(client, 200, nil)
				break
			} else {
				sendPacket(client, 406, nil)
				fmt.Println("User already exists", username)
			}
		}
	}

	users[username] = client

	handlePacket(client)
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

func readPacket(client net.Conn, timeout int64) (out_err error, dataLen uint32, opCode uint16, buffer []byte) {
	//defer readRecover(client, &exitCode)
	if timeout != 0 {
		client.SetReadDeadline(time.Now().Add(time.Duration(timeout * int64(time.Second))))
	}
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
		length := len(data)
		binary.LittleEndian.PutUint32(lenB, uint32(length))
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

//
//
//packet Handle Functions
//
//

func getUserIDbyName(buffer []byte) (uint64, error) {
	var (
		user   userStruct
		userID uint64
	)
	nLen := buffer[0]
	name := string(buffer[1 : 1+nLen])
	appDB.First(&user, "username = ?", name)
	if reflect.DeepEqual(user, userStruct{}) {
		return userID, errors.New("User doesn't exists")
	} else {
		userID = uint64(user.ID)
		return userID, nil
	}
}

//
//
//Parser
//
//
type parserStruct struct {
	data   []byte
	length uint32
	offset uint32
}

func (obj *parserStruct) Byte() (byte, error) {
	if obj.offset+1 > obj.length {
		return 0, errors.New("Offset is out of range")
	}
	defer incrementOffset(1, obj)
	return byte(obj.data[obj.offset]), nil
}

func (obj *parserStruct) UInt16() (uint16, error) {
	if obj.offset+1 > obj.length {
		return 0, errors.New("Offset is out of range")
	}
	defer incrementOffset(2, obj)
	return binary.LittleEndian.Uint16(obj.data[obj.offset : obj.offset+2]), nil
}

func (obj *parserStruct) UInt32() (uint32, error) {
	if obj.offset+1 > obj.length {
		return 0, errors.New("Offset is out of range")
	}
	defer incrementOffset(4, obj)
	return binary.LittleEndian.Uint32(obj.data[obj.offset : obj.offset+2]), nil
}

func (obj *parserStruct) UInt64() (uint64, error) {
	if obj.offset+1 > obj.length {
		return 0, errors.New("Offset is out of range")
	}
	defer incrementOffset(8, obj)
	return binary.LittleEndian.Uint64(obj.data[obj.offset : obj.offset+2]), nil
}

func (obj *parserStruct) String(len uint32) (string, error) {
	if obj.offset+1 > obj.length {
		return "", errors.New("Offset is out of range")
	}
	defer incrementOffset(len, obj)
	return string(obj.data[obj.offset : obj.offset+len]), nil
}

func (obj *parserStruct) Chunk(len uint32) ([]byte, error) {
	if obj.offset+1 > obj.length {
		return nil, errors.New("Offset is out of range")
	}
	defer incrementOffset(len, obj)
	return obj.data[obj.offset : obj.offset+len], nil
}

func incrementOffset(count uint32, obj *parserStruct) {
	obj.offset += count
}
