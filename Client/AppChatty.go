package main

import (
	"bytes"
	"encoding/binary"
	"errors"
	"fmt"
	"io/ioutil"
	"log"
	"net"
	"strconv"
	"strings"
	"time"

	"github.com/gotk3/gotk3/gtk"
)

var (
	connection net.Conn
	buffersize int

	mainWindow gtk.Window
	builder    *gtk.Builder

	settings map[string]string
	online   bool
)

var authWin *gtk.Window

func main() {
	settings = make(map[string]string)

	if parseSettings() != 0 {
		return
	}

	if initGtk() != 0 {
		return
	}

	if drawMain() != 0 {
		return
	}

	connectToServer()
	gtk.Main()

}

func drawMain() int {

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
	//Send button
	//
	obj, err = builder.GetObject("CloseEvt")
	if err != nil {
		log.Fatal("Error:", err)
		return 2
	}
	closeBtn := obj.(*gtk.EventBox)
	closeBtn.Connect("button-release-event", func() {
		gtk.MainQuit()
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
	//Close Button
	//
	obj, err = builder.GetObject("CloseAuth")
	if err != nil {
		log.Fatal("Error:", err)
		return 3
	}
	CloseBtn := obj.(*gtk.Button)
	CloseBtn.Connect("clicked", func() {
		authWin.Hide()
	})

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
			popupError(err.Error(), "Error")
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
			popupError(err.Error(), "Error")
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
		}
		connection, err = net.Dial("tcp", settings["ip"]+":"+settings["port"])
		if err != nil {
			popupError("Can't connect to the server\nException: "+err.Error(), "Error")
			return 1
		}

		authWin.SetPosition(gtk.WIN_POS_CENTER_ON_PARENT)

		authWin.Show()

	} else {
		popupError("The server IP is not defined in settings(file)", "Error")
		return 2
	}
	return 0
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
	popup.SetTitle("Error")
	popup.ShowAll()
}

func popupInfo(content, title string) {
	popup := gtk.MessageDialogNew(nil, 0, gtk.MESSAGE_INFO, gtk.BUTTONS_NONE, content)
	popup.SetTitle("Error")
	popup.ShowAll()
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

func sendRegisterOrAuth(connetcion net.Conn, username, password string, auth bool) error {
	var (
		buffer bytes.Buffer
	)
	nLen := make([]byte, 2)
	pLen := make([]byte, 2)
	usernameB := []byte(username)
	passwordB := []byte(password)
	binary.LittleEndian.PutUint16(nLen, uint16(len(usernameB)))
	binary.LittleEndian.PutUint16(pLen, uint16(len(passwordB)))

	buffer.Write(nLen)
	buffer.Write(usernameB)
	buffer.Write(pLen)
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

func setOffline() {
	online = false
	obj, err := builder.GetObject("OnlineIcon")
	if err != nil {
		log.Fatal("Can't change online icon, error in object getting:", err)
	}
	icon := obj.(*gtk.Image)
	icon.SetFromIconName("network-offline", 4)

	obj, err = builder.GetObject("ReconnectIcon")
	if err != nil {
		log.Fatal("Can't change online icon, error in object getting:", err)
	}
	icon = obj.(*gtk.Image)
	icon.SetVisible(true)

	obj, err = builder.GetObject("ReconnectEvt")
	if err != nil {
		log.Fatal("Can't change online icon, error in object getting:", err)
	}
	evt := obj.(*gtk.EventBox)
	evt.SetVisible(true)
}

func setOnline() {
	online = true
	obj, err := builder.GetObject("OnlineIcon")
	if err != nil {
		log.Fatal("Can't change online icon, error in object getting:", err)
	}
	icon := obj.(*gtk.Image)
	icon.SetFromIconName("network-wired", 4)

	obj, err = builder.GetObject("ReconnectIcon")
	if err != nil {
		log.Fatal("Can't change online icon, error in object getting:", err)
	}
	icon = obj.(*gtk.Image)
	icon.SetVisible(false)

	obj, err = builder.GetObject("ReconnectEvt")
	if err != nil {
		log.Fatal("Can't change online icon, error in object getting:", err)
	}
	evt := obj.(*gtk.EventBox)
	evt.SetVisible(false)
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
	err = sendRegisterOrAuth(connection, username, password, auth)
	if err != nil {
		connection.Close()
		connection = nil
		return err
	}
	err, _, opCode, _ := readPacket(connection, 0)
	if err != nil {
		return err
	}
	if opCode == 200 {
		setOnline()
		authWin.Hide()
		return nil
	}
	return nil
}
