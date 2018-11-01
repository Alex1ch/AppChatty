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
	"strconv"
	"time"

	"github.com/jinzhu/gorm"
	_ "github.com/jinzhu/gorm/dialects/sqlite"
)

type msgStruct struct {
	client  net.Conn
	message string
	group   bool
	ID      uint64
	sender  uint64
}

var (
	users        map[uint64]net.Conn
	subscription map[uint64]net.Conn
	appDB        *gorm.DB
	none, none64 []byte
)

const (
	//ADDRESS Address of the Server
	ADDRESS = "0.0.0.0"
	//PORT Listen port for the Server
	PORT = "1237"
	//BUFFERSIZE Size of the tcp buffer
	BUFFERSIZE = 1024
)

//DB Structures
type userStruct struct {
	ID       uint64 `gorm:"primary_key"`
	Username string
	Hash     []byte
}

type groupStruct struct {
	ID      uint64 `gorm:"primary_key"`
	OwnerID uint64
	Verbose string
}

type groupMemberStruct struct {
	ID       uint64 `gorm:"primary_key"`
	UserID   uint64
	GroupID  uint64
	Username string
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
	appDB.AutoMigrate(&groupStruct{})
	appDB.AutoMigrate(&groupMemberStruct{})
	appDB.Create(&userStruct{Username: "System", Hash: []byte{0, 0, 0, 0}, ID: 1})
	subscription = make(map[uint64]net.Conn)
	users = make(map[uint64]net.Conn)

	//ListenStart
	listenClient(ADDRESS, PORT)
}

func handlePacket(clID uint64, client net.Conn) {
	var (
		err     error
		dataLen uint16
		opCode  uint16
		buffer  []byte
	)

	for {
		client.SetReadDeadline(time.Time{})
		err, dataLen, opCode, buffer = readPacket(client, 0) // For some reason, timeout doesn't work
		if err != nil {
			log.Println(err.Error())
			client.Close()
			if subscription[clID] != nil {
				subscription[clID].Close()
				subscription[clID] = nil
			}

			if users[clID] != nil {
				users[clID].Close()
				users[clID] = nil
			}
			return
		}
		switch opCode {
		case 1:
			parser := parserStruct{buffer, dataLen, 0}
			senderID, err := parser.UInt64()
			if err != nil {
				sendPacket(client, 400, nil)
				continue
			}
			userID, err := parser.UInt64()
			if err != nil {
				sendPacket(client, 400, nil)
				continue
			}
			groupID, err := parser.UInt64()
			if err != nil {
				sendPacket(client, 400, nil)
				continue
			}
			msgLen, err := parser.UInt16()
			if err != nil {
				sendPacket(client, 400, nil)
				continue
			}
			msg, err := parser.String(msgLen)
			if err != nil {
				sendPacket(client, 400, nil)
				continue
			}

			var msgObj msgStruct
			var groupMem groupMemberStruct
			if userID != 0 {
				var user userStruct
				appDB.First(&user, "id = ?", userID)
				if reflect.DeepEqual(user, userStruct{}) {
					sendPacket(client, 404, nil)
					continue
				}
				sendPacket(client, 200, nil)
				msgObj = msgStruct{users[userID], msg, false, userID, senderID}
			} else {
				var group groupStruct
				appDB.First(&group, "id = ?", groupID)
				if reflect.DeepEqual(group, groupStruct{}) {
					sendPacket(client, 404, nil)
					continue
				}
				sendPacket(client, 200, nil)

				msgObj = msgStruct{nil, msg, true, groupID, senderID}

				if len(msg) > 5 {
					if msg[:6] == "/leave" {
						if clID == group.OwnerID {
							go sendSystemMessageToUserInGroup(&msgStruct{nil, "You can't leave group without owner, use /grant and then /leave", true, groupID, 1}, clID)
							continue
						} else {
							appDB.First(&groupMem, "group_id = ? AND user_id = ?", groupID, clID)
							if groupMem.ID != 0 {
								appDB.Delete(groupMemberStruct{}, "group_id = ? AND user_id = ?", groupID, clID)
								msgObj = msgStruct{nil, groupMem.Username + " left the group", true, groupID, 1}
							} else {
								go sendSystemMessageToUserInGroup(&msgStruct{nil, "You are not in the group", true, groupID, 1}, clID)
								continue
							}
						}
					}
					if msg[:5] == "/add " {
						username := msg[5:]

						if group.OwnerID != clID {
							go sendSystemMessageToUserInGroup(&msgStruct{nil, "You aren't owner of this group", true, groupID, 1}, clID)
							continue
						}

						addID, err := getUserIDbyName([]byte(username))
						if err != nil {
							go sendSystemMessageToUserInGroup(&msgStruct{nil, username + " is doesn't exists", true, groupID, 1}, clID)
							continue
						} else {
							appDB.First(&groupMem, "group_id = ? AND user_id = ?", groupID, addID)
							if groupMem.ID == 0 {
								appDB.Create(&groupMemberStruct{UserID: addID, GroupID: group.ID, Username: username})
								msgObj = msgStruct{nil, username + " was added to the group", true, groupID, 1}
							} else {
								go sendSystemMessageToUserInGroup(&msgStruct{nil, username + " already in group", true, groupID, 1}, clID)
								continue
							}
						}
					}
				}

				if len(msg) > 4 {
					if msg[:5] == "/list" {
						var iterId uint64
						var iterUsername string
						list := "List of users in group"
						counter := 1
						rows, err := appDB.Raw("SELECT user_id, username FROM group_member_structs WHERE group_id = " + strconv.FormatUint(groupID, 10)).Rows()

						if err != nil {
							fmt.Println(err.Error())
							return
						}
						for rows.Next() {
							err = rows.Scan(&iterId, &iterUsername)
							list += "\n"
							list += strconv.Itoa(counter) + ". "
							if isOnline(iterId) {
								list += " 🔵 "
							} else {
								list += " 🌑 "
							}
							list += iterUsername
							if group.OwnerID == iterId {
								list += " 👑"
							}
							counter++
						}
						go sendSystemMessageToUserInGroup(&msgStruct{nil, list, true, groupID, 1}, clID)
						continue
					}
				}

				if len(msg) > 6 {

					if msg[:6] == "/kick " {
						username := msg[6:]

						if group.OwnerID != clID {
							go sendSystemMessageToUserInGroup(&msgStruct{nil, "You aren't owner of this group", true, groupID, 1}, clID)
							continue
						}

						kickID, err := getUserIDbyName([]byte(username))
						if err != nil {
							go sendSystemMessageToUserInGroup(&msgStruct{nil, username + " is doesn't exists", true, groupID, 1}, clID)
							continue
						} else {
							if kickID == clID {
								go sendSystemMessageToUserInGroup(&msgStruct{nil, "You can't delete yourself, use /grant and then /leave", true, groupID, 1}, clID)
								continue
							}
							appDB.First(&groupMem, "group_id = ? AND user_id = ?", groupID, kickID)
							if groupMem.ID != 0 {
								msgObj = msgStruct{nil, username + " was deleted from the group", true, groupID, 1}
								go sendMessage(&msgObj)
								appDB.Delete(groupMemberStruct{}, "group_id = ? AND user_id = ?", groupID, kickID)
								continue
							} else {
								go sendSystemMessageToUserInGroup(&msgStruct{nil, username + " not in group", true, groupID, 1}, clID)
								continue
							}
						}
					}
				}
				if len(msg) > 7 {
					if msg[:7] == "/grant " {
						username := msg[7:]

						if group.OwnerID != clID {
							go sendSystemMessageToUserInGroup(&msgStruct{nil, "You aren't owner of this group", true, groupID, 1}, clID)
							continue
						}
						grantID, err := getUserIDbyName([]byte(username))
						if err != nil {
							go sendSystemMessageToUserInGroup(&msgStruct{nil, username + " is doesn't exists", true, groupID, 1}, clID)
							continue
						} else {
							if grantID == clID {
								go sendSystemMessageToUserInGroup(&msgStruct{nil, "You can't /grant to yourself", true, groupID, 1}, clID)
								continue
							} else {
								appDB.First(&groupMem, "group_id = ? AND user_id = ?", groupID, grantID)
								if groupMem.ID != 0 {
									appDB.Model(&group).Update("owner_id", grantID)
									msgObj = msgStruct{nil, username + " is now owner of the group", true, groupID, 1}
								} else {
									go sendSystemMessageToUserInGroup(&msgStruct{nil, username + " not in group", true, groupID, 1}, clID)
									continue
								}
							}
						}
					}
				}

			}

			go sendMessage(&msgObj)

		case 2:
			groupID, err := createGroup(clID, buffer, dataLen)
			if err != nil {
				if err.Error() == "409" {
					sendPacket(client, 409, nil)
					continue
				} else {
					sendPacket(client, 400, nil)
					continue
				}
			}
			if groupID == 0 {
				sendPacket(client, 500, nil)
				continue
			} else {
				serial := createSerializer()
				err := serial.UInt64(groupID)
				if err != nil {
					sendPacket(client, 500, nil)
					continue
				}
				sendPacket(client, 200, serial.buffer.Bytes())
			}
		case 3:
			if len(buffer) != 8 {
				sendPacket(client, 400, nil)
			}
			groupname, err := getGroupNamebyID(binary.LittleEndian.Uint64(buffer))
			if err != nil {
				log.Println(err.Error())
				sendPacket(client, 404, nil)
			}
			serial := createSerializer()
			serial.String(groupname, 1)
			sendPacket(client, 200, serial.buffer.Bytes())
		case 6:
			id, err := getUserIDbyName(buffer[1:])
			if err != nil {
				log.Println(err.Error())
				if err.Error() == "Bad format" {
					sendPacket(client, 400, nil)
				}
				sendPacket(client, 404, nil)
			}
			idB := make([]byte, 8)
			binary.LittleEndian.PutUint64(idB, id)
			sendPacket(client, 200, idB)
		case 7:
			if len(buffer) != 8 {
				sendPacket(client, 400, nil)
			}
			username, err := getNamebyUserID(binary.LittleEndian.Uint64(buffer))
			if err != nil {
				log.Println(err.Error())
				sendPacket(client, 404, nil)
			}
			serial := createSerializer()
			serial.String(username, 1)
			sendPacket(client, 200, serial.buffer.Bytes())
		case 8:
			parser := parserStruct{buffer, dataLen, 0}
			idCount, err := parser.UInt16()
			if err != nil {
				continue
			}
			ids := make([]uint64, idCount)
			var i uint16
			for i = 0; i < idCount; i++ {
				ids[i], err = parser.UInt64()
				if err != nil {
					continue
				}
			}
			serial := createSerializer()
			serial.UInt16(idCount)
			for i = 0; i < idCount; i++ {
				serial.UInt64(ids[i])
				if err != nil {
					continue
				}
				if isOnline(ids[i]) {
					serial.Byte(1)
				} else {
					serial.Byte(0)
				}
			}

			sendPacketToSubscriber(clID, 8, serial.buffer.Bytes())

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
		id       uint64
	)

	for {
		err, _, opCode, buffer = readPacket(client, 0)
		if err != nil {
			log.Println(err.Error())
			return
		}

		parser := parserStruct{buffer, uint16(len(buffer)), 0}

		if !(opCode == 4 || opCode == 5 || opCode == 10) {
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
		username, err = parser.String(uint16(nLen))
		if err != nil {
			fmt.Println("Session error, bad request")
			sendPacket(client, 400, nil)
			return
		}
		passLen, err = parser.Byte()
		if err != nil {
			fmt.Println("Session error, bad request")
			sendPacket(client, 400, nil)
			return
		}
		if passLen == 0 {
			fmt.Println("Session error, bad request")
			sendPacket(client, 400, nil)
			client.Close()
			return
		}
		password, err = parser.Chunk(uint16(passLen))
		if err != nil {
			fmt.Println("Session error, bad request")
			sendPacket(client, 400, nil)
			return
		}

		hash = sha256.Sum256(password)
		if opCode == 5 {
			var user userStruct
			appDB.First(&user, "username = ?", username)
			if isOnline(uint64(user.ID)) {
				sendPacket(client, 409, nil)
				return
			} else if reflect.DeepEqual(user, userStruct{}) {
				sendPacket(client, 404, nil)
				fmt.Println("Received NX auth from", username)
			} else if bytes.Equal(hash[:], user.Hash[:]) {
				users[uint64(user.ID)] = client
				sendPacket(client, 200, nil)
				fmt.Println("Received auth from", username)
				id = uint64(user.ID)
				break
			} else {
				sendPacket(client, 423, nil)
				fmt.Println("Received wrong password from", username)
			}
		} else if opCode == 4 {
			var user userStruct
			appDB.First(&user, "username = ?", username)
			if isOnline(uint64(user.ID)) {
				sendPacket(client, 409, nil)
				return
			} else if reflect.DeepEqual(user, userStruct{}) {
				fmt.Println("Received register from", username)
				appDB.Create(&userStruct{Username: username, Hash: hash[:]})
				sendPacket(client, 200, nil)
				users[uint64(user.ID)] = client
				id = uint64(user.ID)
				break
			} else {
				sendPacket(client, 406, nil)
				fmt.Println("User already exists", username)
			}
		} else if opCode == 10 {
			var user userStruct
			appDB.First(&user, "username = ?", username)
			if isOnline(uint64(user.ID)) {
				sendPacket(client, 409, nil)
				return
			} else if reflect.DeepEqual(user, userStruct{}) {
				sendPacket(client, 404, nil)
				fmt.Println("Received NX auth from", username)
			} else if bytes.Equal(hash[:], user.Hash[:]) {
				sendPacket(client, 200, nil)
				id = uint64(user.ID)
				subscription[id] = client
				fmt.Println("Received subscribe from", username)
				go sendHelloFromGroup(id, 0)
				break
			} else {
				sendPacket(client, 423, nil)
				fmt.Println("Received wrong password from", username)
			}
		}
	}

	handlePacket(id, client)
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

//
// Packet reading
//

func readPacket(client net.Conn, timeout int64) (out_err error, dataLen uint16, opCode uint16, buffer []byte) {
	//defer readRecover(client, &exitCode)
	if timeout != 0 {
		client.SetReadDeadline(time.Now().Add(time.Duration(timeout * int64(time.Second))))
	} else {
		client.SetReadDeadline(time.Time{})
	}
	dataLenB := make([]byte, 2)
	_, err := client.Read(dataLenB)
	if err != nil {
		fmt.Println("Error in message receiving(len): " + err.Error())
		client.Close()
		out_err = err
		return
	}
	dataLen = binary.LittleEndian.Uint16(dataLenB)
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

func readPacketFromSubscriber(id uint64, timeout int64) (out_err error, dataLen uint16, opCode uint16, buffer []byte) {
	defer recover()
	client, ok := subscription[id]
	if !ok {
		return errors.New("No subscription available"), 0, 0, nil
	}
	if client == nil {
		return errors.New("No subscription available"), 0, 0, nil
	}
	if timeout != 0 {
		client.SetReadDeadline(time.Now().Add(time.Duration(timeout * int64(time.Second))))
	} else {
		client.SetReadDeadline(time.Time{})
	}
	dataLenB := make([]byte, 2)
	_, err := client.Read(dataLenB)
	if err != nil {
		fmt.Println("Error in message receiving(len): " + err.Error())
		client.Close()
		out_err = err
		return
	}
	dataLen = binary.LittleEndian.Uint16(dataLenB)
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

func sendPacket(client net.Conn, opCode uint16, data []byte) error {
	var buffer bytes.Buffer
	opCodeB := make([]byte, 2)
	if data != nil {
		lenB := make([]byte, 2)
		length := len(data)
		if len(data) > 65535 {
			return errors.New("Data is to big, packet split is not implemented")
		}
		binary.LittleEndian.PutUint16(lenB, uint16(length))
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
		lenB := []byte{0, 0}
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
func sendPacketToSubscriber(id uint64, opCode uint16, data []byte) error {
	client, ok := subscription[id]
	if !ok || client == nil {
		return errors.New("No subscription available")
	}
	var buffer bytes.Buffer
	opCodeB := make([]byte, 2)
	if data != nil {
		lenB := make([]byte, 2)
		length := len(data)
		if len(data) > 65535 {
			return errors.New("Data is to big, packet split is not implemented")
		}
		binary.LittleEndian.PutUint16(lenB, uint16(length))
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
		lenB := []byte{0, 0}
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

func addUserToGroup() {

}

//0 - all groups
func sendHelloFromGroup(id uint64, currentGroup uint64) {
	if currentGroup == 0 {
		var groupID uint64
		var iterId uint64
		var iterUsername string
		rows, err := appDB.Raw("SELECT group_id FROM group_member_structs WHERE user_id = " + strconv.FormatUint(id, 10)).Rows()

		if err != nil {
			fmt.Println(err.Error())
			return
		}
		for rows.Next() {
			err = rows.Scan(&groupID)
			if err != nil {
				fmt.Print(err.Error())
			}
			counter := 1
			list := "Admin commands: /add /kick /grant\nCommon commands: /leave /list\nList of users in group"
			rows1, err := appDB.Raw("SELECT user_id, username FROM group_member_structs WHERE group_id = " + strconv.FormatUint(groupID, 10)).Rows()

			if err != nil {
				fmt.Println(err.Error())
				return
			}
			for rows1.Next() {
				err = rows1.Scan(&iterId, &iterUsername)
				list += "\n"
				list += strconv.Itoa(counter) + ". "
				if isOnline(iterId) {
					list += " 🔵 "
				} else {
					list += " 🌑 "
				}
				list += iterUsername
				counter++
			}

			go sendSystemMessageToUserInGroup(&msgStruct{nil, list, true, groupID, 1}, id)
		}
	} else {
		list := "Admin commands: /add /kick /grant\nCommon commands: /leave /list"

		go sendSystemMessageToUserInGroup(&msgStruct{nil, list, true, currentGroup, 1}, id)
	}
}

//
//
//packet Handle Functions
//
//

func createGroup(clID uint64, buffer []byte, dataLen uint16) (uint64, error) { //returns groupID
	parser := parserStruct{buffer, dataLen, 0}
	nLen, err := parser.Byte()
	if err != nil {
		return 0, errors.New("400")
	}
	groupName, err := parser.String(uint16(nLen))
	if err != nil {
		return 0, errors.New("400")
	}
	var group groupStruct
	appDB.First(&group, "verbose = ?", groupName)
	username, err := getNamebyUserID(clID)
	if err != nil {
		return 0, errors.New("500")
	}
	if reflect.DeepEqual(group, groupStruct{}) {
		appDB.Create(&groupStruct{OwnerID: clID, Verbose: groupName})
		appDB.First(&group, "verbose = ?", groupName)
		appDB.Create(&groupMemberStruct{UserID: clID, GroupID: group.ID, Username: username})
		sendHelloFromGroup(clID, group.ID)
		return uint64(group.ID), nil
	} else {
		return 0, errors.New("409")
	}
}

func getUserIDbyName(buffer []byte) (uint64, error) {
	var (
		user   userStruct
		userID uint64
	)
	name := string(buffer)
	appDB.First(&user, "username = ?", name)
	if reflect.DeepEqual(user, userStruct{}) {
		return userID, errors.New("User doesn't exists")
	} else {
		userID = uint64(user.ID)
		return userID, nil
	}
}

func getNamebyUserID(id uint64) (string, error) {
	var (
		user     userStruct
		username string
	)
	appDB.First(&user, "id = ?", id)
	if reflect.DeepEqual(user, userStruct{}) {
		return "", errors.New("User doesn't exists")
	} else {
		username = user.Username
		return username, nil
	}
}

func sendMessage(msg *msgStruct) {
	serial := createSerializer()
	serial.UInt64(msg.sender)
	if msg.group == false {
		serial.UInt64(msg.ID)
		serial.UInt64(0)
		serial.String(msg.message, 2)
		err := sendPacketToSubscriber(msg.ID, 1, serial.buffer.Bytes())

		err, _, opCode, _ := readPacketFromSubscriber(msg.ID, 0)
		if err != nil {
			fmt.Println(err.Error())
			return
		}

		switch opCode {
		case 200:
			return
		case 400:
			fmt.Println("Bad syntax???")
			return
		case 404:
			fmt.Println("Wrong receipient")
			return
		default:
			fmt.Println("Unknown response")
			return
		}

	} else {
		var userID uint64
		rows, err := appDB.Raw("SELECT user_id FROM group_member_structs WHERE group_id = " + strconv.FormatUint(msg.ID, 10)).Rows()

		var usersToSend []uint64
		if err != nil {
			fmt.Println(err.Error())
			return
		}
		for rows.Next() {
			err = rows.Scan(&userID)
			if err != nil {
				fmt.Print(err.Error())
			}
			usersToSend = append(usersToSend, userID)
		}

		notInGroup := true
		for i := range usersToSend {
			if usersToSend[i] == msg.sender {
				notInGroup = false
				break
			}
		}
		if msg.sender == 1 {
			notInGroup = false
		}

		if notInGroup {
			serial := createSerializer()
			serial.UInt64(1)
			serial.UInt64(0)
			serial.UInt64(msg.ID)
			serial.String("You are not the member of the group", 2)

			sendPacketToSubscriber(msg.sender, 1, serial.buffer.Bytes())
			return
		}

		for i := range usersToSend {
			serial := createSerializer()
			serial.UInt64(msg.sender)
			serial.UInt64(0)
			serial.UInt64(msg.ID)
			serial.String(msg.message, 2)

			sendPacketToSubscriber(usersToSend[i], 1, serial.buffer.Bytes())
		}
		return
	}
}

func sendSystemMessageToUserInGroup(msg *msgStruct, userID uint64) {
	serial := createSerializer()
	serial.UInt64(msg.sender)
	serial.UInt64(0)
	serial.UInt64(msg.ID)
	serial.String(msg.message, 2)

	err := sendPacketToSubscriber(userID, 1, serial.buffer.Bytes())

	err, _, opCode, _ := readPacketFromSubscriber(msg.ID, 0)
	if err != nil {
		fmt.Println(err.Error())
	}

	switch opCode {
	case 200:
		return
	case 400:
		fmt.Println("Bad syntax???")
	case 404:
		fmt.Println("Wrong receipient")
	default:
		fmt.Println("Unknown response")
	}
}

func getGroupNamebyID(id uint64) (string, error) {
	var (
		group groupStruct
		name  string
	)
	appDB.First(&group, "id = ?", id)
	if reflect.DeepEqual(group, groupStruct{}) {
		return "", errors.New("User doesn't exists")
	} else {
		name = group.Verbose
		return name, nil
	}
}

//
//
// etc
//
//

func isOnline(user uint64) bool {
	if subscription[user] != nil {
		return true
	}
	return false
}

//
//
// Parser
//
//
type parserStruct struct {
	data   []byte
	length uint16
	offset uint16
}

func (obj *parserStruct) Byte() (byte, error) {
	if obj.offset+1 > obj.length {
		return 0, errors.New("Offset is out of range")
	}
	defer incrementOffset(1, obj)
	return byte(obj.data[obj.offset]), nil
}

func (obj *parserStruct) UInt16() (uint16, error) {
	if obj.offset+2 > obj.length {
		return 0, errors.New("Offset is out of range")
	}
	defer incrementOffset(2, obj)
	return binary.LittleEndian.Uint16(obj.data[obj.offset : obj.offset+2]), nil
}

func (obj *parserStruct) UInt32() (uint32, error) {
	if obj.offset+4 > obj.length {
		return 0, errors.New("Offset is out of range")
	}
	defer incrementOffset(4, obj)
	return binary.LittleEndian.Uint32(obj.data[obj.offset : obj.offset+4]), nil
}

func (obj *parserStruct) UInt64() (uint64, error) {
	if obj.offset+8 > obj.length {
		return 0, errors.New("Offset is out of range")
	}
	defer incrementOffset(8, obj)
	return binary.LittleEndian.Uint64(obj.data[obj.offset : obj.offset+8]), nil
}

func (obj *parserStruct) String(len uint16) (string, error) {
	if obj.offset+len > obj.length {
		return "", errors.New("Offset is out of range")
	}
	defer incrementOffset(len, obj)
	return string(obj.data[obj.offset : obj.offset+len]), nil
}

func (obj *parserStruct) Chunk(len uint16) ([]byte, error) {
	if obj.offset+len > obj.length {
		return nil, errors.New("Offset is out of range")
	}
	defer incrementOffset(len, obj)
	return obj.data[obj.offset : obj.offset+len], nil
}

func incrementOffset(count uint16, obj *parserStruct) {
	obj.offset += count
}

//
//
// Serializer
//
//
func createSerializer() serializerStruct {
	var buffer bytes.Buffer
	obj := serializerStruct{buffer}
	return obj
}

type serializerStruct struct {
	buffer bytes.Buffer
}

func (obj *serializerStruct) Byte(input byte) error {
	err := obj.buffer.WriteByte(input)
	if err != nil {
		return err
	}
	return nil
}

func (obj *serializerStruct) UInt16(input uint16) error {
	temp := make([]byte, 2)
	binary.LittleEndian.PutUint16(temp, input)
	_, err := obj.buffer.Write(temp)
	if err != nil {
		return err
	}
	return nil
}

func (obj *serializerStruct) UInt32(input uint32) error {
	temp := make([]byte, 4)
	binary.LittleEndian.PutUint32(temp, input)
	_, err := obj.buffer.Write(temp)
	if err != nil {
		return err
	}
	return nil
}

func (obj *serializerStruct) UInt64(input uint64) error {
	temp := make([]byte, 8)
	binary.LittleEndian.PutUint64(temp, input)
	_, err := obj.buffer.Write(temp)
	if err != nil {
		return err
	}
	return nil
}

func (obj *serializerStruct) String(input string, lenLen int) error {
	inputB := []byte(input)
	len := len(inputB)
	if lenLen == 1 {
		err := obj.buffer.WriteByte(byte(len))
		if err != nil {
			return err
		}
	} else if lenLen == 2 {
		err := obj.UInt16(uint16(len))
		if err != nil {
			return err
		}
	} else {
		err := obj.UInt32(uint32(len))
		if err != nil {
			return err
		}
	}
	_, err := obj.buffer.Write(inputB)
	if err != nil {
		return err
	}
	return nil
}

func (obj *serializerStruct) Chunk(input []byte, lenLen int) error {
	len := len(input)
	if lenLen == 1 {
		err := obj.buffer.WriteByte(byte(len))
		if err != nil {
			return err
		}
	} else if lenLen == 2 {
		err := obj.UInt16(uint16(len))
		if err != nil {
			return err
		}
	} else {
		err := obj.UInt32(uint32(len))
		if err != nil {
			return err
		}
	}
	_, err := obj.buffer.Write(input)
	if err != nil {
		return err
	}
	return nil
}
