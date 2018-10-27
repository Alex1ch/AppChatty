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
	connection net.Conn
	buffersize int

	wrapper wordwrap.WrapperFunc

	mainWindow    gtk.Window
	builder       *gtk.Builder
	authWin       *gtk.Window
	settingsWin   *gtk.Window
	messageText   *gtk.TextBuffer
	messageOutput *gtk.ListBox
	сontactsList  *gtk.ListBox
	messageScroll *gtk.ScrolledWindow

	chats      map[uint64]chat
	settings   map[string]string
	online     bool
	activeChat uint64

	clUsername string
	clID       uint64

	none, none64 []byte
)

func main() {
	chats = make(map[uint64]chat)
	settings = make(map[string]string)
	none = []byte{0xff, 0xff, 0xff, 0xff}
	none64 = []byte{0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff}

	if parseSettings() != 0 {
		return
	}

	if initGtk() != 0 {
		return
	}

	if initWindows() != 0 {
		return
	}
	wrapper = wordwrap.Wrapper(40, false)

	connectToServer()
	gtk.Main()

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
		gtk.MainQuit()
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
			sendMessage()
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
		sendMessage()
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
		gtk.MainQuit()
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
				setOnline(false)
			}
			popupError("Error: "+err.Error(), "Error")
			return
		}
	})

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
			setOnline(false)
		}
		connection, err = net.Dial("tcp", settings["ip"]+":"+settings["port"])
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
//Online parts
//

func sendRegisterOrAuth(username, password string, auth bool) error {
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
	err = sendRegisterOrAuth(username, password, auth)
	if err != nil {
		connection.Close()
		connection = nil
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

func sendMessage() {
	str, err := messageText.GetText(messageText.GetIterAtOffset(0), messageText.GetEndIter(), true)
	if err != nil {
		popupError("Error: "+err.Error(), "Error")
	}
	if activeChat != 0 {
		row, _ := gtk.ListBoxRowNew()

		chatEntry := chats[activeChat]
		chatEntry.messages = append(chatEntry.messages, message{clID, clUsername, str, row})
		chats[activeChat] = chatEntry
		var label *gtk.Label
		if chats[activeChat].group {
			label, _ = gtk.LabelNew(clUsername + ": " + str)
		} else {
			label, _ = gtk.LabelNew(str)
		}

		//label.SetEllipsize(pango.ELLIPSIZE_MIDDLE)
		label.SetLineWrap(true)
		label.SetLineWrapMode(pango.WRAP_WORD_CHAR)
		label.SetXAlign(0)
		label.SetMarginStart(10)
		label.SetMarginTop(12)
		label.SetMarginBottom(12)

		row.Add(label)
		row.SetMarginStart(100)

		messageOutput.Add(row)
		messageOutput.ShowAll()

		adj, _ := gtk.AdjustmentNew(0xffffffff, 0, 0xffffffff, 0, 0, 0)
		messageScroll.SetVAdjustment(adj)

		go clearText()
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

	err, len, opCode, recieved := readPacket(connection, 5)

	switch opCode {
	case 200:
		if len != 8 {
			return 0, errors.New("Bad response")
		}
		userID := binary.LittleEndian.Uint64(recieved)

		return userID, nil
	case 404:
		return 0, errors.New("404: Not found. \nUser doesn't exists")
	case 400:
		return 0, errors.New("400: Bad request")
	default:
		return 0, errors.New(fmt.Sprint("Unhandled server response - ", opCode))
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

	//chatNext := chats[next]
	chatNextMsg := chats[next].messages

	for i := range chatNextMsg {

		row, _ := gtk.ListBoxRowNew()
		var label *gtk.Label
		if chats[activeChat].group {
			label, _ = gtk.LabelNew(clUsername + ": " + chatNextMsg[i].text)
		} else {
			label, _ = gtk.LabelNew(chatNextMsg[i].text)
		}

		label.SetLineWrap(true)
		label.SetLineWrapMode(pango.WRAP_WORD_CHAR)
		label.SetXAlign(0)
		label.SetMarginStart(10)
		label.SetMarginTop(12)
		label.SetMarginBottom(12)

		row.Add(label)
		if chatNextMsg[i].senderID == clID {
			row.SetMarginStart(100)
		} else {
			row.SetMarginEnd(100)
		}

		messageOutput.Add(row)
		messageOutput.ShowAll()

		chatNextMsg[i].row = row
	}
	//chatNext.messages = chatNextMsg
}

func createRow() {

}

//
// Parser
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

func (obj *parserStruct) String(len uint32) (string, error) {
	if obj.offset+len > obj.length {
		return "", errors.New("Offset is out of range")
	}
	defer incrementOffset(len, obj)
	return string(obj.data[obj.offset : obj.offset+len]), nil
}

func (obj *parserStruct) Chunk(len uint32) ([]byte, error) {
	if obj.offset+len > obj.length {
		return nil, errors.New("Offset is out of range")
	}
	defer incrementOffset(len, obj)
	return obj.data[obj.offset : obj.offset+len], nil
}

func incrementOffset(count uint32, obj *parserStruct) {
	obj.offset += count
}

//
// Serializer
//

type serializerStruct struct {
	buffer bytes.Buffer
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
