package main

import (
	"bytes"
	"encoding/binary"
	"errors"
	"fmt"
	"io"
	"io/ioutil"
	"log"
	"net"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/eidolon/wordwrap"
	"github.com/gotk3/gotk3/pango"

	"github.com/gotk3/gotk3/gdk"
	"github.com/gotk3/gotk3/gtk"
)

type message struct {
	senderID uint64
	name     string
	text     string
	row      *gtk.ListBoxRow
}

type chat struct {
	group    bool
	verbose  string
	id       uint64
	messages []message
}

var (
	connection   net.Conn
	subscribtion net.Conn
	buffersize   int

	gtkAlive bool

	wrapper wordwrap.WrapperFunc

	mainWindow    gtk.Window
	builder       *gtk.Builder
	authWin       *gtk.Window
	settingsWin   *gtk.Window
	addGroupWin   *gtk.Window
	messageText   *gtk.TextBuffer
	messageOutput *gtk.ListBox
	сontactsList  *gtk.ListBox
	messageScroll *gtk.ScrolledWindow
	stickerPop    *gtk.Popover
	stickerList   *gtk.ListBox

	usernames  map[uint64]string
	userids    map[string]uint64
	chats      map[uint64]chat
	settings   map[string]string
	stickerMap map[string]string
	online     bool
	activeChat uint64

	clUsername string
	clID       uint64
)

func main() {
	chats = make(map[uint64]chat)
	settings = make(map[string]string)
	usernames = make(map[uint64]string)
	userids = make(map[string]uint64)
	stickerMap = make(map[string]string)

	if parseSettings() != 0 {
		return
	}

	if initGtk() != 0 {
		return
	}
	gtkAlive = true

	if initWindows() != 0 {
		return
	}
	wrapper = wordwrap.Wrapper(40, false)

	scanStickers()

	connectToServer()
	go gtk.Main()

	listenMessages()
}

func initWindows() int {

	//Getting objects and defining events
	//Main window
	obj, err := builder.GetObject("Main_window")
	if err != nil {
		log.Fatal("Error in object getting:", err)
		return 1
	}
	mainWindow := obj.(*gtk.Window)
	mainWindow.Connect("destroy", func() {
		gtkAlive = false
		gtk.MainQuit()
		connection.Close()
		subscribtion.Close()
	})
	mainWindow.ShowAll()

	//
	//Settings Window
	//
	obj, err = builder.GetObject("Settings")
	if err != nil {
		log.Fatal("Error in object getting:", err)
		return 2
	}

	settingsWin = obj.(*gtk.Window)

	settingsWin.Connect("delete-event", func() bool {
		settingsWin.Hide()
		return true
	})
	settingsWin.SetPosition(gtk.WIN_POS_CENTER_ON_PARENT)
	settingsWin.SetTitle("Settings")

	//
	//Settings button
	//
	obj, err = builder.GetObject("SettingsEvt")
	if err != nil {
		log.Fatal("Error:", err)
		return 2
	}
	settingsBtn := obj.(*gtk.EventBox)
	settingsBtn.Connect("button-release-event", func() {
		settingsWin.ShowAll()
	})

	//
	//MessageEntry
	//
	obj, err = builder.GetObject("MessageText")
	if err != nil {
		log.Fatal("Error in object getting:", err)
		return 2
	}
	textView := obj.(*gtk.TextView)
	textView.Connect("key-press-event", func(any interface{}, gdkEvent *gdk.Event) { //func() { //
		keyEvent := &gdk.EventKey{gdkEvent}
		if keyEvent.KeyVal() == 65293 && keyEvent.State()%2 != 1 {
			str, err := messageText.GetText(messageText.GetIterAtOffset(0), messageText.GetEndIter(), true)
			if err != nil {
				popupError("Error: "+err.Error(), "Error")
			}
			if str != "" {
				sendMessage(str, true)
			}
		}
	})

	messageText, err = textView.GetBuffer()
	if err != nil {
		log.Fatal("Error in object getting:", err)
		return 2
	}

	//
	//Send button
	//
	obj, err = builder.GetObject("SendEvt")
	if err != nil {
		log.Fatal("Error:", err)
		return 2
	}
	closeBtn := obj.(*gtk.EventBox)
	closeBtn.Connect("button-release-event", func() {
		str, err := messageText.GetText(messageText.GetIterAtOffset(0), messageText.GetEndIter(), true)
		if err != nil {
			popupError("Error: "+err.Error(), "Error")
		}
		if str != "" {
			sendMessage(str, true)
		}
	})

	//
	//Sticker button
	//
	obj, err = builder.GetObject("StickersEvt")
	if err != nil {
		log.Fatal("Error:", err)
		return 2
	}
	stickersBtn := obj.(*gtk.EventBox)
	stickersBtn.Connect("button-release-event", func() {
		stickerPop.ShowAll()
	})

	//
	//Reconnect button
	//
	obj, err = builder.GetObject("ReconnectEvt")
	if err != nil {
		log.Fatal("Error:", err)
		return 2
	}
	ReconnectBtn := obj.(*gtk.EventBox)
	ReconnectBtn.Connect("button-release-event", func() {
		connectToServer()
	})

	//
	//Auth window
	//
	obj, err = builder.GetObject("Auth")
	if err != nil {
		log.Fatal("Error in object getting:", err)
		return 2
	}

	authWin = obj.(*gtk.Window)
	authWin.Connect("delete-event", func() bool {
		authWin.Hide()
		return true
	})
	authWin.SetPosition(gtk.WIN_POS_CENTER_ON_PARENT)
	authWin.SetTitle("Authentication")

	//
	//Username Entry
	//
	obj, err = builder.GetObject("AuthUsername")
	if err != nil {
		log.Fatal("Error in object getting:", err)
		return 2
	}
	authUser := obj.(*gtk.Entry)

	//
	//Password Entry
	//
	obj, err = builder.GetObject("AuthPassword")
	if err != nil {
		log.Fatal("Error in object getting:", err)
		return 2
	}
	authPass := obj.(*gtk.Entry)

	//
	//SignIn Button
	//
	obj, err = builder.GetObject("SignIn")
	if err != nil {
		log.Fatal("Error:", err)
		return 3
	}
	SignInBtn := obj.(*gtk.Button)
	SignInBtn.Connect("clicked", func() {
		err := establishConnetcion(true, authPass, authUser)
		if err != nil {
			popupError("Error: "+err.Error(), "Error")
		}
	})

	//
	//SignUp Button
	//
	obj, err = builder.GetObject("SignUp")
	if err != nil {
		log.Fatal("Error:", err)
		return 3
	}
	SignUpBtn := obj.(*gtk.Button)
	SignUpBtn.Connect("clicked", func() {
		err := establishConnetcion(false, authPass, authUser)
		if err != nil {
			popupError("Error: "+err.Error(), "Error")
		}
	})

	//
	//MessageOutput
	//
	obj, err = builder.GetObject("MessageOutput")
	if err != nil {
		log.Fatal("Error:", err)
		return 4
	}
	messageOutput = obj.(*gtk.ListBox)

	//
	//MessageOutput
	//
	obj, err = builder.GetObject("MessageScroll")
	if err != nil {
		log.Fatal("Error:", err)
		return 4
	}
	messageScroll = obj.(*gtk.ScrolledWindow)

	//
	//ContactsList
	//
	obj, err = builder.GetObject("ContactsList")
	if err != nil {
		log.Fatal("Error:", err)
		return 5
	}
	сontactsList = obj.(*gtk.ListBox)
	сontactsList.Connect("row-activated", func(cList *gtk.ListBox, cListR *gtk.ListBoxRow) {
		name, err := cListR.GetName()
		if err != nil {
			return
		}
		nameSplit := strings.Split(name, " ")
		isGroup, err := strconv.Atoi(nameSplit[0])
		if err != nil {
			fmt.Print(isGroup)
			return
		}
		ID, err := strconv.ParseUint(nameSplit[1], 10, 64)
		if err != nil {
			return
		}

		redrawChat(activeChat, ID)
		activeChat = ID

	})

	//
	//AddContactEntry
	//
	obj, err = builder.GetObject("AddContactEntry")
	if err != nil {
		log.Fatal("Error:", err)
		return 5
	}
	addContactEntry := obj.(*gtk.Entry)

	//
	//AddButton
	//
	obj, err = builder.GetObject("AddButton")
	if err != nil {
		log.Fatal("Error:", err)
		return 6
	}
	addButton := obj.(*gtk.Button)
	addButton.Connect("clicked", func() {
		if connection == nil {
			popupError("Error: No connection", "Error")
			return
		}
		str, err := addContactEntry.GetText()
		if err != nil {
			popupError("Error: "+err.Error(), "Error")
			return
		}
		err = addContact(0, str)
		if err != nil {
			if err == io.EOF {
				connection.Close()
				connection = nil
				subscribtion.Close()
				subscribtion = nil
				setOnline(false)
			}
			popupError("Error: "+err.Error(), "Error")
			return
		}
	})

	//
	//Send button
	//
	obj, err = builder.GetObject("GroupEvt")
	if err != nil {
		log.Fatal("Error:", err)
		return 2
	}
	groupBtn := obj.(*gtk.EventBox)
	groupBtn.Connect("button-release-event", func() {
		addGroupWin.ShowAll()
	})

	//
	//GroupWindow
	//
	obj, err = builder.GetObject("AddGroup")
	if err != nil {
		log.Fatal("Error in object getting:", err)
		return 2
	}

	addGroupWin = obj.(*gtk.Window)
	addGroupWin.Connect("delete-event", func() bool {
		addGroupWin.Hide()
		return true
	})
	addGroupWin.SetPosition(gtk.WIN_POS_CENTER_ON_PARENT)

	//
	//Popover Stiker
	//
	obj, err = builder.GetObject("StickerPop")
	if err != nil {
		log.Fatal("Error:", err)
		return 2
	}
	stickerPop = obj.(*gtk.Popover)

	//
	//StikerList
	//
	obj, err = builder.GetObject("StickerList")
	if err != nil {
		log.Fatal("Error:", err)
		return 2
	}
	stickerList = obj.(*gtk.ListBox)

	return 0
}

func initGtk() int {
	//Init
	gtk.Init(nil)
	fmt.Println("GTK initialized")

	//Builder init
	var err error
	builder, err = gtk.BuilderNew()
	if err != nil {
		log.Fatal("Error:", err)
		return 1
	}
	err = builder.AddFromFile("Layout/Layout.glade")
	if err != nil {
		log.Fatal("Error:", err)
		return 1
	}
	fmt.Println("Layout was loaded")
	return 0
}

func connectToServer() int { //connection net.Conn {
	_, ok := settings["ip"]
	if ok {
		//Connecting to server
		var err error
		if connection != nil {
			connection.Close()
			connection = nil
			subscribtion.Close()
			subscribtion = nil
			setOnline(false)
		}
		connection, err = net.Dial("tcp", settings["ip"]+":"+settings["port"])
		if err != nil {
			popupError("Can't connect to the server\nException: "+err.Error(), "Error")
			return 1
		}

		//Subscribe to server
		if subscribtion != nil {
			connection.Close()
			connection = nil
			subscribtion.Close()
			subscribtion = nil
			setOnline(false)
		}
		subscribtion, err = net.Dial("tcp", settings["ip"]+":"+settings["port"])
		if err != nil {
			popupError("Can't connect to the server\nException: "+err.Error(), "Error")
			return 1
		}

		authWin.SetPosition(gtk.WIN_POS_CENTER_ON_PARENT)

		authWin.ShowAll()

	} else {
		popupError("The server IP is not defined in settings(file)", "Error")
		return 2
	}
	return 0
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

//
//Online parts
//

func sendRegisterOrAuthAndSubscribe(username, password string, auth bool) error {
	var buffer bytes.Buffer

	usernameB := []byte(username)
	passwordB := []byte(password)
	nLen := len(usernameB)
	pLen := len(passwordB)

	if pLen > 255 || nLen < 0 {
		return errors.New("Password is too big")
	}
	if nLen > 255 || nLen < 0 {
		return errors.New("Username is too big")
	}

	buffer.WriteByte(byte(nLen))
	buffer.Write(usernameB)
	buffer.WriteByte(byte(pLen))
	buffer.Write(passwordB)
	if auth {
		err := sendPacket(connection, 5, buffer.Bytes())
		if err != nil {
			return err
		}
	} else {
		err := sendPacket(connection, 4, buffer.Bytes())
		if err != nil {
			return err
		}
	}
	err := sendPacket(subscribtion, 10, buffer.Bytes())
	if err != nil {
		return err
	}
	return nil
}

func setOnline(_online bool) {
	online = _online
	obj, err := builder.GetObject("OnlineIcon")
	if err != nil {
		log.Fatal("Can't change online icon, error in object getting:", err)
	}
	icon := obj.(*gtk.Image)
	if online {
		icon.SetFromIconName("network-wired", 4)
	} else {
		icon.SetFromIconName("network-offline", 4)
	}

	obj, err = builder.GetObject("ReconnectIcon")
	if err != nil {
		log.Fatal("Can't change online icon, error in object getting:", err)
	}
	icon = obj.(*gtk.Image)
	if online {
		icon.SetFromIconName("emblem-unreadable", 4)
	} else {
		icon.SetFromIconName("view-refresh", 4)
	}

	//icon.SetVisible(!online)
	//
	//obj, err = builder.GetObject("ReconnectEvt")
	//if err != nil {
	//	log.Fatal("Can't change online icon, error in object getting:", err)
	//}
	//evt := obj.(*gtk.EventBox)
	//evt.SetVisible(!online)
}

func establishConnetcion(auth bool, authPass, authUser *gtk.Entry) error {
	var err error
	if connection == nil {
		connection, err = net.Dial("tcp", settings["ip"]+":"+settings["port"])
		subscribtion, err = net.Dial("tcp", settings["ip"]+":"+settings["port"])
		if err != nil {
			return err
		}
	}
	password, err := authPass.GetText()
	if err != nil {
		return err
	}
	username, err := authUser.GetText()
	if err != nil {
		return err
	}
	if username == "" {
		return errors.New("Empty username")
	}
	if password == "" {
		return errors.New("Empty password")
	}
	err = sendRegisterOrAuthAndSubscribe(username, password, auth)
	if err != nil {
		connection.Close()
		connection = nil
		subscribtion.Close()
		subscribtion = nil
		return err
	}
	err, _, opCode, _ := readPacket(connection, 5)
	if err != nil {
		return errors.New("Server not responding")
	}
	switch opCode {
	case 200:
		clUsername = username
		clID, err = getUserID(clUsername)
		if err != nil {
			return err
		}
		setOnline(true)
		authWin.Hide()
		return nil
	case 404:
		return errors.New("404: Not found. \nUser doesn't exists")
	case 406:
		return errors.New("406: Not acceptable. \nUser already exists")
	case 423:
		return errors.New("423: Locked. Wrong password")
	case 400:
		return errors.New("400: Bad request")
	default:
		return errors.New(fmt.Sprint("Unhandled server response - ", opCode))
	}
}

func sendMessage(str string, clear bool) {
	if activeChat != 0 {

		serial := createSerializer()
		serial.UInt64(clID)
		if chats[activeChat].group == false {
			serial.UInt64(activeChat)
			serial.UInt64(0)
		} else {
			serial.UInt64(0)
			serial.UInt64(activeChat)
		}
		msgLen := len([]byte(str))
		if msgLen > 65535 {
			popupError("Message is too long", "Error")
			return
		}
		serial.String(str, 2)
		err := sendPacket(connection, 1, serial.buffer.Bytes())
		if err != nil {
			popupError("Error: "+err.Error(), "Error")
			return
		}

		err, _, opCode, _ := readPacket(connection, 0)

		switch opCode {
		case 200:
			row := createRow(clID, clUsername, str, chats[activeChat].group)
			chatEntry := chats[activeChat]
			chatEntry.messages = append(chatEntry.messages, message{clID, clUsername, str, row})
			chats[activeChat] = chatEntry

			messageOutput.Add(row)
			messageOutput.ShowAll()

			adj, _ := gtk.AdjustmentNew(0xffffffff, 0, 0xffffffff, 0, 0, 0)
			messageScroll.SetVAdjustment(adj)

			if clear {
				go clearText()
			}
			return
		case 400:
			popupError("400: Bad syntax", "Error")
			return
		case 404:
			popupError("404: User doesn't exist", "Error")
			return
		}

	}
}

func clearText() {
	time.Sleep(10000000)
	messageText.SetText("")
}

func addContact(isGroup int, contactName string) error {
	userID, err := getUserID(contactName)
	if err != nil {
		return err
	}
	if userID == clID {
		return errors.New("It's you!")
	}

	if _, ok := chats[userID]; ok {
		return errors.New("User already in contacts")
	}

	addToContactLists(isGroup, userID, contactName)
	return nil
}

func getUserID(contactName string) (uint64, error) {
	if v, ok := userids[contactName]; ok {
		return v, nil
	}

	var (
		buffer bytes.Buffer
	)

	contactNameB := []byte(contactName)
	length := len(contactNameB)

	if length > 255 || length < 0 {
		return 0, errors.New("Name is too big")
	}

	buffer.WriteByte(byte(length))
	buffer.Write(contactNameB)

	err := sendPacket(connection, 6, buffer.Bytes())
	if err != nil {
		return 0, err
	}

	err, len, opCode, recieved := readPacket(connection, 0)

	switch opCode {
	case 200:
		if len != 8 {
			return 0, errors.New("Bad response")
		}
		userID := binary.LittleEndian.Uint64(recieved)
		userids[contactName] = userID
		usernames[userID] = contactName

		return userID, nil
	case 404:
		return 0, errors.New("404: Not found. \nUser doesn't exists")
	case 400:
		return 0, errors.New("400: Bad request")
	default:
		return 0, errors.New(fmt.Sprint("Unhandled server response - ", opCode))
	}
}

func getUsername(id uint64) (string, error) {
	if v, ok := usernames[id]; ok {
		return v, nil
	}

	serial := createSerializer()
	serial.UInt64(id)

	err := sendPacket(connection, 7, serial.buffer.Bytes())
	if err != nil {
		return "", err
	}

	err, len, opCode, recieved := readPacket(connection, 0)

	switch opCode {
	case 200:
		parser := parserStruct{recieved, len, 0}
		nLen, err := parser.Byte()
		if err != nil {
			return "", err
		}
		contactName, err := parser.String(uint16(nLen))
		if err != nil {
			return "", err
		}
		userids[contactName] = id
		usernames[id] = contactName

		return contactName, nil
	case 404:
		return "", errors.New("404: Not found. \nUser doesn't exists")
	case 400:
		return "", errors.New("400: Bad request")
	default:
		return "", errors.New(fmt.Sprint("Unhandled server response - ", opCode))
	}
}

func listenMessages() {
	var (
		err     error
		dataLen uint16
		opCode  uint16
		data    []byte
	)
	for {
		if !online {
			if !gtkAlive {
				return
			}
			time.Sleep(3 * time.Second)
			continue
		}
		err, dataLen, opCode, data = readPacket(subscribtion, 0)
		if err != nil {
			popupError("Subscription fail", "Error")
			connection.Close()
			connection = nil
			subscribtion.Close()
			subscribtion = nil
			setOnline(false)
		}
		switch opCode {
		case 1:
			parser := parserStruct{data, dataLen, 0}
			senderID, err := parser.UInt64()
			if err != nil {
				fmt.Printf("Error: " + err.Error())
				sendPacket(subscribtion, 400, nil)
				break
			}
			userID, err := parser.UInt64()
			if err != nil {
				fmt.Printf("Error: " + err.Error())
				sendPacket(subscribtion, 400, nil)
				break
			}
			_, err = parser.UInt64() //groupID
			if err != nil {
				fmt.Printf("Error: " + err.Error())
				sendPacket(subscribtion, 400, nil)
				break
			}
			mLen, err := parser.UInt16()
			if err != nil {
				fmt.Printf("Error: " + err.Error())
				sendPacket(subscribtion, 400, nil)
				break
			}
			msg, err := parser.String(mLen)
			if err != nil {
				fmt.Printf("Error: " + err.Error())
				sendPacket(subscribtion, 400, nil)
				break
			}
			if userID != 0 {
				sendPacket(subscribtion, 200, nil)
				username, err := getUsername(senderID)
				if err != nil {
					fmt.Printf("Error: " + err.Error())
					break
				}
				row := createRow(senderID, username, msg, false)
				if _, ok := chats[senderID]; !ok {
					username, err = getUsername(senderID)
					if err != nil {
						fmt.Printf("Error: " + err.Error())

						break
					}
					addToContactLists(0, senderID, username)
				}
				chatEntry := chats[senderID]
				chatEntry.messages = append(chatEntry.messages, message{senderID, username, msg, row})
				chats[senderID] = chatEntry
				if senderID == activeChat {
					messageOutput.Add(row)
					messageOutput.ShowAll()

					adj, _ := gtk.AdjustmentNew(0xffffffff, 0, 0xffffffff, 0, 0, 0)
					messageScroll.SetVAdjustment(adj)
				}
			}
			sendPacket(subscribtion, 501, nil)
		}
	}
}

//
// etc
//

func parseSettings() int {
	settings["buffersize"] = "2048"
	settings["port"] = "1666"
	b, err := ioutil.ReadFile("settings")
	if err != nil {
		log.Fatal("Error in setting parsing: ", err)
		return 1
	}
	str := string(b)
	if len(str) == 0 {
		log.Fatal("Error: empty settings file")
		return 1
	}
	lines := strings.Split(str, "\n")
	var split []string
	for i := range lines {
		split = strings.Split(lines[i], "=")
		if len(split) < 2 {
			continue
		}
		settings[split[0]] = split[1]
	}

	buffersize, err = strconv.Atoi(settings["buffersize"])
	if err != nil {
		buffersize = 2048
		log.Println("Warning: wrong value for \"buffersize\" in settings")
	}

	return 0
}

func popupError(content, title string) {
	popup := gtk.MessageDialogNew(nil, 0, gtk.MESSAGE_ERROR, gtk.BUTTONS_NONE, content)
	popup.SetTitle(title)
	popup.ShowAll()
}

func popupInfo(content, title string) {
	popup := gtk.MessageDialogNew(nil, 0, gtk.MESSAGE_INFO, gtk.BUTTONS_NONE, content)
	popup.SetTitle(title)
	popup.ShowAll()
}

func addToContactLists(isGroup int, ID uint64, verbose string) {
	row, _ := gtk.ListBoxRowNew()

	chats[ID] = chat{false, verbose, ID, make([]message, 0)}

	label, _ := gtk.LabelNew(verbose)
	label.SetXAlign(0)
	label.SetMarginStart(20)
	label.SetSizeRequest(0, 66)
	row.Add(label)
	row.SetName(strconv.Itoa(isGroup) + " " + strconv.FormatUint(ID, 10))

	сontactsList.Insert(row, 0)
	сontactsList.ShowAll()

	//for i := 0; i < chats.Len(); i++ {
	//
	//	}
}

func redrawChat(prev, next uint64) {
	if prev == next {
		return
	}
	if prev != 0 {
		chatPrevMsg := chats[prev].messages

		for i := range chatPrevMsg {
			messageOutput.Remove(chatPrevMsg[i].row)
		}
		fmt.Println(chatPrevMsg)
	}
	fmt.Println(prev, next)

	chatNextMsg := chats[next].messages

	for i := range chatNextMsg {
		username, _ := getUsername(chatNextMsg[i].senderID)
		row := createRow(chatNextMsg[i].senderID, username, chatNextMsg[i].text, chats[activeChat].group)

		messageOutput.Add(row)
		messageOutput.ShowAll()

		chatNextMsg[i].row = row
	}
	adj, _ := gtk.AdjustmentNew(0xffffffff, 0, 0xffffffff, 0, 0, 0)
	messageScroll.SetVAdjustment(adj)
}

func createRow(sender uint64, name string, str string, group bool) *gtk.ListBoxRow {
	row, _ := gtk.ListBoxRowNew()
	if len(str) > 10 {
		if str[0:9] == "/sticker:" && !group {
			var image *gtk.Image

			if _, err := os.Stat("Stickers/" + str[9:]); os.IsNotExist(err) {

			} else {
				image, _ = gtk.ImageNew()

				image.SetFromFile("Stickers/" + str[9:])

				image.SetPixelSize(2)

				image.SetMarginTop(12)
				image.SetMarginBottom(12)

				if sender == clID {
					image.SetMarginEnd(10)
					image.SetHAlign(gtk.ALIGN_END)
					row.SetMarginStart(250)
				} else {
					image.SetMarginStart(10)
					image.SetHAlign(gtk.ALIGN_START)
					row.SetMarginEnd(250)
				}
				row.Add(image)

				return row
			}
		}
	}
	var label *gtk.Label
	if group {
		label, _ = gtk.LabelNew(name + ": " + str)
	} else {
		label, _ = gtk.LabelNew(str)
	}

	label.SetLineWrap(true)
	label.SetLineWrapMode(pango.WRAP_WORD_CHAR)
	label.SetMarginTop(12)
	label.SetMarginBottom(12)

	if sender != clID {
		label.SetMarginStart(10)
		label.SetXAlign(0)
		row.SetMarginEnd(250)
	} else {
		label.SetMarginEnd(10)
		label.SetXAlign(10000)
		row.SetMarginStart(250)
	}

	row.Add(label)
	return row
}

func scanStickers() {
	dirs, err := ioutil.ReadDir("./Stickers")
	if err != nil {
		popupError("Error while loading stickers(folders)", "Error")
		return
	}
	id := 0
	for i := range dirs {
		if dirs[i].IsDir() {
			files, err := ioutil.ReadDir("./Stickers/" + dirs[i].Name())
			if err != nil {
				popupError("Error while loading stickers(files)", "Error")
				return
			}

			row, _ := gtk.ListBoxRowNew()
			label, _ := gtk.LabelNew(dirs[i].Name())
			label.SetXAlign(0)
			label.SetMarginStart(10)
			label.SetMarginTop(12)
			label.SetMarginBottom(12)
			label.SetMaxWidthChars(1)
			label.SetEllipsize(pango.ELLIPSIZE_END)
			row.Add(label)
			stickerList.Add(row)

			var (
				box   *gtk.Box
				pxbuf *gdk.Pixbuf
				image *gtk.Image
				eBox  *gtk.EventBox
			)

			for j := range files {
				id++
				if j%4 == 0 {
					box, _ = gtk.BoxNew(gtk.ORIENTATION_HORIZONTAL, 0)
				}
				pxbuf, _ = gdk.PixbufNewFromFile("./Stickers/" + dirs[i].Name() + "/" + files[j].Name())
				pxbuf, _ = pxbuf.ScaleSimple(64, 64, gdk.INTERP_BILINEAR)

				image, _ = gtk.ImageNewFromPixbuf(pxbuf)

				eBox, _ = gtk.EventBoxNew()
				eBox.Add(image)

				eBox.SetName(strconv.Itoa(id))
				stickerMap[strconv.Itoa(id)] = dirs[i].Name() + "/" + files[j].Name()

				eBox.Connect("button-release-event", func(obj *gtk.EventBox) {
					name, err := obj.GetName()
					if err != nil {
						popupError("Error in sticker sending (Can't get id through EventBox name)", "Error")
					}
					sendMessage("/sticker:"+stickerMap[name], false)
				})

				box.PackStart(eBox, true, true, 0)

				if (j-3)%4 == 0 || j == len(files)-1 {
					row, _ := gtk.ListBoxRowNew()
					row.Add(box)
					stickerList.Add(row)
				}
			}
		}
	}
	//stickerList
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
