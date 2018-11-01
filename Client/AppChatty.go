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
	"strconv"
	"strings"
	"time"

	"github.com/gotk3/gotk3/glib"

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
	online   bool
}

var (
	connection   net.Conn
	subscribtion net.Conn
	buffersize   int

	gtkAlive bool

	mainWindow     gtk.Window
	builder        *gtk.Builder
	authWin        *gtk.Window
	settingsWin    *gtk.Window
	addGroupWin    *gtk.Window
	messageText    *gtk.TextBuffer
	messageOutput  *gtk.ListBox
	сontactsList   *gtk.ListBox
	messageScroll  *gtk.ScrolledWindow
	stickerPop     *gtk.Popover
	stickerList    *gtk.ListBox
	stickerScroll  *gtk.ScrolledWindow
	groupNameEntry *gtk.Entry
	ipEntry        *gtk.Entry
	portEntry      *gtk.Entry
	usernameLabel  *gtk.Label

	stickerScrollAdj float64
	stickerScrollUpp float64

	usernames    map[uint64]string
	userids      map[string]uint64
	groupnames   map[uint64]string
	chats        map[uint64]*chat // [user_id]chat struct
	newMCounters map[uint64]int
	settings     map[string]string      // [key]value
	stickerBuf   map[string]*gdk.Pixbuf // [filename]pixbuf

	chatCount  uint64
	online     bool
	activeChat uint64
	clUsername string
	clID       uint64
)

func main() {
	chats = make(map[uint64]*chat)
	newMCounters = make(map[uint64]int)
	settings = make(map[string]string)
	usernames = make(map[uint64]string)
	groupnames = make(map[uint64]string)
	userids = make(map[string]uint64)
	stickerBuf = make(map[string]*gdk.Pixbuf)

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

	scanStickers()

	connectToServer()

	go gtk.Main()
	go onlineChecker()
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
		if connection != nil {
			connection.Close()
		}
		if subscribtion != nil {
			subscribtion.Close()
		}
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
		ipEntry.SetText(settings["ip"])
		portEntry.SetText(settings["port"])
		settingsWin.ShowAll()
	})

	//
	//Username Label
	//
	obj, err = builder.GetObject("UsernameLabel")
	if err != nil {
		log.Fatal("Error:", err)
		return 2
	}
	usernameLabel = obj.(*gtk.Label)

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
			} else {
				glib.IdleAdd(clearText)
			}
		}
	})

	messageText, err = textView.GetBuffer()
	if err != nil {
		log.Fatal("Error in object getting:", err)
		return 2
	}

	//
	// IpEntry
	//
	obj, err = builder.GetObject("IP")
	if err != nil {
		log.Fatal("Error in object getting:", err)
		return 2
	}
	ipEntry = obj.(*gtk.Entry)

	//
	// Port Entry
	//
	obj, err = builder.GetObject("PORT")
	if err != nil {
		log.Fatal("Error in object getting:", err)
		return 2
	}
	portEntry = obj.(*gtk.Entry)

	//
	//Settings OK button
	//
	obj, err = builder.GetObject("Settings OK")
	if err != nil {
		log.Fatal("Error:", err)
		return 2
	}
	setOKBtn := obj.(*gtk.Button)
	setOKBtn.Connect("clicked", func() {
		ip, err := ipEntry.GetText()
		if err != nil {
			return
		}
		port, err := portEntry.GetText()
		if err != nil {
			return
		}
		if net.ParseIP(ip) != nil {
			settings["ip"] = ip
		} else {
			popupError("Wrong IP format", "Error")
			return
		}
		i, err := strconv.Atoi(port)
		if err == nil {
			if i > 0 && i < 65536 {
				settings["port"] = port
				settingsWin.Hide()
			} else {
				popupError("Wrong PORT range", "Error")
			}
		} else {
			popupError("Wrong PORT format", "Error")
		}

	})

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
		adj, _ := gtk.AdjustmentNew(stickerScrollAdj, 0, stickerScrollUpp, 0, 0, 0)
		stickerScroll.SetVAdjustment(adj)
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
		chatID, err := strconv.ParseUint(nameSplit[1], 10, 64)
		if err != nil {
			return
		}
		redrawChat(activeChat, chatID)
		activeChat = chatID
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
		str, err := addContactEntry.GetText()
		if str == "" {
			return
		}
		if connection == nil {
			popupError("Error: No connection", "Error")
			return
		}
		if err != nil {
			popupError("Error: "+err.Error(), "Error")
			return
		}
		err = addContact(0, str, 0)
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
		addContactEntry.SetText("")
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
	obj, err = builder.GetObject("CreateGroup")
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
	//GroupButton
	//
	obj, err = builder.GetObject("CreateGroupBtn")
	if err != nil {
		log.Fatal("Error in object getting:", err)
		return 2
	}

	createGroupBtn := obj.(*gtk.Button)
	createGroupBtn.Connect("clicked", func() {
		err := createGroup()
		if err != nil {
			popupError("Error: "+err.Error(), "Error")
			return
		}
		addGroupWin.Hide()
	})

	//
	//GroupButton
	//
	obj, err = builder.GetObject("GroupName")
	if err != nil {
		log.Fatal("Error in object getting:", err)
		return 2
	}
	groupNameEntry = obj.(*gtk.Entry)

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

	//
	//StikerScroll
	//
	obj, err = builder.GetObject("StickerScroll")
	if err != nil {
		log.Fatal("Error:", err)
		return 2
	}
	stickerScroll = obj.(*gtk.ScrolledWindow)
	stickerScrollAdj = stickerScroll.GetVAdjustment().GetValue()
	stickerScrollUpp = stickerScroll.GetVAdjustment().GetUpper()

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
	if client == nil {
		return errors.New("No subscription available"), 0, 0, nil
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
	var op uint16
	if auth {
		op = 5
	} else {
		op = 4
	}

	err := sendPacket(connection, op, buffer.Bytes())
	if err != nil {
		return err
	}

	err, _, opCode, _ := readPacket(connection, 2)
	if err != nil {
		return errors.New("Server not responding")
	}

	switch opCode {
	case 200:

	case 404:
		return errors.New("404: Not found. \nUser doesn't exists")
	case 406:
		return errors.New("406: Not acceptable. \nUser already exists")
	case 423:
		return errors.New("423: Locked. Wrong password")
	case 409:
		return errors.New("409: Conflict. User is already online")
	case 400:
		return errors.New("400: Bad request")
	default:
		return errors.New(fmt.Sprint("Unhandled server response - ", opCode))
	}

	err = sendPacket(subscribtion, 10, buffer.Bytes())
	if err != nil {
		return err
	}

	err, _, opCode, _ = readPacket(subscribtion, 5)
	if err != nil {
		return errors.New("Server not responding")
	}

	switch opCode {
	case 200:
		return nil
	case 404:
		return errors.New("404: Not found. \nUser doesn't exists")
	case 406:
		return errors.New("406: Not acceptable. \nUser already exists")
	case 423:
		return errors.New("423: Locked. Wrong password")
	case 409:
		return errors.New("409: Conflict. User is already online")
	case 400:
		return errors.New("400: Bad request")
	default:
		return errors.New(fmt.Sprint("Unhandled server response - ", opCode))
	}
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
	clUsername = username
	usernameLabel.SetText(username)
	clID, err = getUserID(clUsername)
	if err != nil {
		return err
	}
	setOnline(true)
	authWin.Hide()
	return nil
}

func sendMessage(str string, clear bool) {
	if activeChat != 0 {

		serial := createSerializer()
		serial.UInt64(clID)
		if chats[activeChat].group == false {
			serial.UInt64(chats[activeChat].id)
			serial.UInt64(0)
		} else {
			serial.UInt64(0)
			serial.UInt64(chats[activeChat].id)
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

		err, _, opCode, _ := readPacket(connection, 5)
		if err != nil {
			popupError("Server is not responding", "Error")
			return
		}

		switch opCode {
		case 200:
			chatEntry := chats[activeChat]
			row := createRow(clID, clUsername, str, false)
			chatEntry.messages = append(chatEntry.messages, message{clID, clUsername, str, row})
			chats[activeChat] = chatEntry

			messageOutput.Add(row)
			messageOutput.ShowAll()

			scrollDown()

			if clear {
				glib.IdleAdd(clearText)
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

func addContact(isGroup int, contactName string, id uint64) error {
	if isGroup == 0 {
		if id == 0 {
			var err error
			id, err = getUserID(contactName)
			if err != nil {
				return err
			}
		}
		fmt.Println(id)
		if id == clID {
			return errors.New("It's you!")
		}
		_, oldChat := getChatByID(id, false)
		if oldChat != nil {
			return errors.New("User already in contacts")
		}
		chatCount++
		addToContactLists(isGroup, chatCount, id, contactName)
	} else {
		_, oldChat := getChatByID(id, true)
		if oldChat != nil {
			return errors.New("Group chat already in contacts")
		}

		chatCount++
		addToContactLists(isGroup, chatCount, id, contactName)
	}
	return nil
}

func createGroup() error {
	serial := createSerializer()
	text, _ := groupNameEntry.GetText()

	if text == "" {
		return errors.New("Empty line")
	}

	err := serial.String(text, 1)
	if err != nil {
		return err
	}

	err = sendPacket(connection, 2, serial.buffer.Bytes())
	if err != nil {
		return err
	}

	err, dataLen, opCode, buf := readPacket(connection, 5)
	if err != nil {
		return err
	}
	switch opCode {
	case 200:
		parser := parserStruct{buf, dataLen, 0}
		id, err := parser.UInt64()
		if err != nil {
			return errors.New("Parse error: " + err.Error())
		}
		groupname, err := getGroupname(id)
		if err != nil {
			return errors.New("Get error: " + err.Error())
		}
		addContact(1, groupname, id)
	case 400:
		return errors.New("400: Bad syntax")
	case 500:
		return errors.New("500: Server error")
	case 409:
		return errors.New("409: Conflict. Group with this name already exists")
	}

	return nil
}

func getGroupname(id uint64) (string, error) {
	if v, ok := groupnames[id]; ok {
		return v, nil
	}

	serial := createSerializer()
	serial.UInt64(id)

	err := sendPacket(connection, 3, serial.buffer.Bytes())
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
		groupnames[id] = contactName

		return contactName, nil
	case 404:
		return "", errors.New("404: Not found. \nUser doesn't exists")
	case 400:
		return "", errors.New("400: Bad request")
	default:
		return "", errors.New(fmt.Sprint("Unhandled server response - ", opCode))
	}
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
			time.Sleep(1 * time.Second)
			continue
		}
		if subscribtion == nil {
			time.Sleep(1 * time.Second)
			continue
		}
		err, dataLen, opCode, data = readPacket(subscribtion, 0)
		if err != nil {
			log.Println("Error: Subscription fail")
			setOnline(false)
			continue
		}
		switch opCode {
		case 1:
			parser := parserStruct{data, dataLen, 0}
			senderID, err := parser.UInt64()
			if err != nil {
				fmt.Printf("Error: " + err.Error())
				break
			}
			userID, err := parser.UInt64()
			if err != nil {
				fmt.Printf("Error: " + err.Error())
				break
			}
			groupID, err := parser.UInt64() //groupID
			if err != nil {
				fmt.Printf("Error: " + err.Error())
				break
			}
			mLen, err := parser.UInt16()
			if err != nil {
				fmt.Printf("Error: " + err.Error())
				break
			}
			msg, err := parser.String(mLen)
			if err != nil {
				fmt.Printf("Error: " + err.Error())
				break
			}
			if userID != 0 {
				username, err := getUsername(senderID)
				if err != nil {
					fmt.Printf("Error: " + err.Error())
					break
				}
				row := createRow(senderID, username, msg, false)
				_, destChat := getChatByID(senderID, false)
				if destChat == nil {
					chatCount++
					addToContactLists(0, chatCount, senderID, username)
				}
				key, destChat := getChatByID(senderID, false)
				destChat.messages = append(destChat.messages, message{senderID, username, msg, row})
				if key == activeChat {
					messageOutput.Add(row)
					messageOutput.ShowAll()

					glib.IdleAdd(scrollDown, nil)
				} else {
					newMCounters[key]++
					glib.IdleAdd(setContactText, chat{destChat.group, destChat.verbose, key, make([]message, 0), destChat.online})
				}
				chats[key] = destChat
			} else {
				if senderID == clID {
					break
				}
				username, err := getUsername(senderID)
				if err != nil {
					fmt.Printf("Error: " + err.Error())
					break
				}
				groupname, err := getGroupname(groupID)
				if err != nil {
					fmt.Printf("Error: " + err.Error())
					break
				}
				_, destChat := getChatByID(groupID, true)
				if destChat == nil {
					chatCount++
					addToContactLists(1, chatCount, groupID, groupname)
				}
				key, destChat := getChatByID(groupID, true)
				var row *gtk.ListBoxRow
				chatLen := len(destChat.messages)
				if chatLen != 0 {
					if destChat.messages[chatLen-1].senderID == senderID {
						row = createRow(senderID, username, msg, false)
					} else {
						row = createRow(senderID, username, msg, true)
					}
				} else {
					row = createRow(senderID, username, msg, true)
				}
				destChat.messages = append(destChat.messages, message{senderID, username, msg, row})
				if key == activeChat {
					messageOutput.Add(row)
					messageOutput.ShowAll()

					glib.IdleAdd(scrollDown, nil)
				} else {
					newMCounters[key]++
					glib.IdleAdd(setContactText, chat{destChat.group, destChat.verbose, key, make([]message, 0), destChat.online})
				}
				chats[key] = destChat
			}
		case 8:
			parser := parserStruct{data, dataLen, 0}
			idCount, err := parser.UInt16()
			if err != nil {
				continue
			}
			var id uint64
			var i uint16
			var online byte
			for i = 0; i < idCount; i++ {
				id, err = parser.UInt64()
				if err != nil {
					continue
				}

				online, err = parser.Byte()
				if err != nil {
					continue
				}

				key, destChat := getChatByID(id, false)
				if destChat == nil {
					continue
				}
				if online == 1 {
					destChat.online = true
				} else {
					destChat.online = false
				}
				glib.IdleAdd(setContactText, chat{destChat.group, destChat.verbose, key, make([]message, 0), destChat.online})
			}
		}
	}
}

//
// etc
//

func onlineChecker() {
	for {
		if !online {
			if !gtkAlive {
				return
			}
			time.Sleep(1 * time.Second)
			continue
		}
		time.Sleep(5 * time.Second)
		ids := make([]uint64, 0)
		for _, value := range chats {
			if value.group == false {
				ids = append(ids, value.id)
			}
		}
		glib.IdleAdd(checkOnline, ids)
	}
}

func checkOnline(ids []uint64) {
	serial := createSerializer()
	serial.UInt16(uint16(len(ids)))
	var i uint16
	for i = 0; i < uint16(len(ids)); i++ {
		serial.UInt64(ids[i])
	}
	if connection == nil {
		return
	}
	sendPacket(connection, 8, serial.buffer.Bytes())
}

func setContactText(crutch chat) {
	fmt.Print(crutch.id, crutch.verbose)
	contact, _ := сontactsList.GetRowAtIndex(int(chatCount - crutch.id)).GetChild()
	label := gtk.Label{*contact}
	str := crutch.verbose
	if !crutch.group {
		if crutch.online {
			str += " 🔵"
		} else {
			str += " 🌑"
		}
	}
	if newMCounters[crutch.id] != 0 {
		str += " (" + strconv.Itoa(newMCounters[crutch.id]) + ")"
	}
	label.SetText(str)
}

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

func addToContactLists(isGroup int, chatID, ID uint64, verbose string) {
	row, _ := gtk.ListBoxRowNew()

	if isGroup == 0 {
		chats[chatID] = &chat{false, verbose, ID, make([]message, 0), false}
	} else {
		chats[chatID] = &chat{true, verbose, ID, make([]message, 0), false}
	}

	label, _ := gtk.LabelNew(verbose)
	label.SetXAlign(0)
	label.SetMarginStart(20)
	label.SetSizeRequest(0, 66)
	row.Add(label)
	row.SetName(strconv.Itoa(isGroup) + " " + strconv.FormatUint(chatID, 10))

	сontactsList.Insert(row, 0)
	glib.IdleAdd(setContactText, chat{chats[chatID].group, chats[chatID].verbose, chatID, make([]message, 0), chats[chatID].online})
	сontactsList.ShowAll()
}

func redrawChat(prev, next uint64) { //, isPreviousChatGroup bool) {
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

	newMCounters[next] = 0

	glib.IdleAdd(setContactText, chat{chats[next].group, chats[next].verbose, next, make([]message, 0), chats[next].online})

	chatNextMsg := chats[next].messages

	for i := range chatNextMsg {
		row := chatNextMsg[i].row

		messageOutput.Add(row)
		messageOutput.ShowAll()

		chatNextMsg[i].row = row
	}
	scrollDown()
}

func scrollDown() {
	adj, _ := gtk.AdjustmentNew(0xffffffff, 0, 0xffffffff, 0, 0, 0)
	messageScroll.SetVAdjustment(adj)
}

func createRow(sender uint64, name string, str string, includeName bool) *gtk.ListBoxRow {
	row, _ := gtk.ListBoxRowNew()
	box, _ := gtk.BoxNew(gtk.ORIENTATION_VERTICAL, 0)

	if includeName {
		label, _ := gtk.LabelNew("")
		label.SetXAlign(0)
		label.SetMaxWidthChars(1)
		label.SetLineWrap(true)
		label.SetLineWrapMode(pango.WRAP_WORD_CHAR)
		label.SetMarginTop(10)
		label.SetMarginBottom(10)
		label.SetMarginStart(20)
		label.SetSelectable(true)
		label.SetUseMarkup(true)
		label.SetMarkup("<i><b>" + name + "</b></i>")
		box.PackStart(label, true, true, 0)

		label.SetXAlign(0)
		row.SetMarginEnd(250)
	}

	if len(str) > 10 {
		if str[0:9] == "/sticker:" {
			var image *gtk.Image

			if v, ok := stickerBuf[str[9:]]; ok {
				image, _ = gtk.ImageNewFromPixbuf(v)

				image.SetMarginTop(6)
				image.SetMarginBottom(6)

				if sender == clID {
					image.SetMarginEnd(10)
					image.SetHAlign(gtk.ALIGN_END)
					row.SetMarginStart(250)
				} else {
					image.SetMarginStart(10)
					image.SetHAlign(gtk.ALIGN_START)
					row.SetMarginEnd(250)
				}
				box.PackStart(image, true, true, 0)
				row.Add(box)
				return row
			}
		}
	}

	label, _ := gtk.LabelNew(str)

	label.SetMaxWidthChars(1)
	label.SetLineWrap(true)
	label.SetLineWrapMode(pango.WRAP_WORD_CHAR)
	label.SetMarginTop(10)
	label.SetMarginBottom(10)
	label.SetMarginStart(20)
	label.SetMarginEnd(20)
	label.SetSelectable(true)

	if sender != clID {
		label.SetXAlign(0)
		row.SetMarginEnd(250)
	} else {
		label.SetXAlign(10000)
		row.SetMarginStart(250)
	}

	box.PackStart(label, true, true, 0)
	row.Add(box)
	return row
}

func scanStickers() {
	dirs, err := ioutil.ReadDir("./Stickers")
	if err != nil {
		popupError("Error while loading stickers(folders)", "Error")
		return
	}
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
				box        *gtk.Box
				pxbuf      *gdk.Pixbuf
				pxbufSmall *gdk.Pixbuf
				image      *gtk.Image
				eBox       *gtk.EventBox
				pos        int
			)

			for j := range files {
				if pos%4 == 0 {
					box, _ = gtk.BoxNew(gtk.ORIENTATION_HORIZONTAL, 0)
				}
				pxbuf, err = gdk.PixbufNewFromFile("./Stickers/" + dirs[i].Name() + "/" + files[j].Name())
				if err != nil {
					log.Println("error in sticker loading")
					continue
				}

				stickerBuf[dirs[i].Name()+"/"+files[j].Name()] = pxbuf

				pxbufSmall, _ = pxbuf.ScaleSimple(64, 64, gdk.INTERP_BILINEAR)
				image, err = gtk.ImageNewFromPixbuf(pxbufSmall)

				eBox, _ = gtk.EventBoxNew()
				eBox.Add(image)

				eBox.SetName(dirs[i].Name() + "/" + files[j].Name())

				eBox.Connect("button-release-event", func(obj *gtk.EventBox) {
					name, err := obj.GetName()
					if err != nil {
						popupError("Error in sticker sending (Can't get id through EventBox name)", "Error")
					}
					sendMessage("/sticker:"+name, false)
					stickerScrollAdj = stickerScroll.GetVAdjustment().GetValue()
					stickerScrollUpp = stickerScroll.GetVAdjustment().GetUpper()
					stickerPop.Hide()
				})

				box.PackStart(eBox, true, true, 0)

				if (pos+1)%4 == 0 || j == len(files)-1 {
					row, _ := gtk.ListBoxRowNew()
					row.Add(box)
					stickerList.Add(row)
				}
				pos++
			}
		}
	}
}

func getChatByID(id uint64, isGroup bool) (uint64, *chat) {
	for k, v := range chats {
		if v.id == id && v.group == isGroup {
			return k, v
		}
	}
	return 0, nil
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
		if len > 255 {
			return errors.New("String is too long")
		}
		err := obj.buffer.WriteByte(byte(len))
		if err != nil {
			return err
		}
	} else if lenLen == 2 {
		if len > 65535 {
			return errors.New("String is too long")
		}
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
