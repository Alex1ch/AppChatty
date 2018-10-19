package main

import (
	"bytes"
	"encoding/binary"
	"fmt"
	"io/ioutil"
	"log"
	"net"
	"strconv"
	"strings"

	"github.com/gotk3/gotk3/gtk"
)

var (
	connection net.Conn
	buffersize int

	mainWindow gtk.Window
	builder    *gtk.Builder

	settings map[string]string
)

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

	//Send button
	obj, err = builder.GetObject("CloseEvt")
	if err != nil {
		log.Fatal("Error:", err)
		return 2
	}
	closeBtn := obj.(*gtk.EventBox)
	closeBtn.Connect("button-release-event", func() {
		gtk.MainQuit()
	})

	//Sticker button
	obj, err = builder.GetObject("StickersEvt")
	if err != nil {
		log.Fatal("Error:", err)
		return 2
	}
	stickersBtn := obj.(*gtk.EventBox)
	stickersBtn.Connect("button-release-event", func() {
		gtk.MainQuit()
	})

	mainWindow.ShowAll()
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
		connection, err = net.Dial("tcp", settings["ip"]+":"+settings["port"])
		if err != nil {
			popupError("Can't connect to the server\nException: "+err.Error(), "Error")
			return 1
		}

		//Showing the auth window
		obj, err := builder.GetObject("Auth")
		if err != nil {
			log.Fatal("Error in object getting:", err)
			return 2
		}
		authWin := obj.(*gtk.Window)
		authWin.SetPosition(gtk.WIN_POS_CENTER_ON_PARENT)
		authWin.ShowAll()

		obj, err = builder.GetObject("SignIn")
		if err != nil {
			log.Fatal("Error:", err)
			return 3
		}
		SignInBtn := obj.(*gtk.Button)
		SignInBtn.Connect("clicked", func() {
			sendPacket(connection, 3, []byte{1, 2, 3, 4, 5})
		})

		obj, err = builder.GetObject("SignIn")
		if err != nil {
			log.Fatal("Error:", err)
			return 3
		}
		SignUpBtn := obj.(*gtk.Button)
		SignUpBtn.Connect("button-release-event", func() {
			sendPacket(connection, 3, []byte{1, 2, 3, 4, 5})
		})

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

func readPacket(client net.Conn) (exitCode int, dataLen uint32, opCode uint16, buffer []byte) {
	//defer readRecover(client, &exitCode)
	dataLenB := make([]byte, 4)
	recLen, err := client.Read(dataLenB)
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
	buffer = make([]byte, dataLen)
	//client.SetDeadline(time.Now().Add(time.Duration(30000)))
	_, err = client.Read(buffer)
	if err != nil {
		fmt.Println("Error in message receiving(data): " + err.Error())
		client.Close()
		exitCode = 1
		return
	}
	if recLen == 0 {
		fmt.Println("Recieved empty message")
		client.Close()
		exitCode = 2
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

func sendPacket(client net.Conn, opCode uint16, data []byte) int {
	var buffer bytes.Buffer
	opCodeB := make([]byte, 2)
	lenB := make([]byte, 4)
	lenght := len(data)
	binary.LittleEndian.PutUint32(lenB, uint32(lenght))
	binary.LittleEndian.PutUint16(opCodeB, opCode)

	buffer.Write(lenB)
	buffer.Write(opCodeB)
	buffer.Write(data)
	sendData := make([]byte, lenght+6)
	_, err := buffer.Read(sendData)
	if err != nil {
		fmt.Println("Error in message forming: " + err.Error())
		return 1
	}
	_, err = client.Write(sendData)
	if err != nil {
		fmt.Println("Error in message sending: " + err.Error())
		return 2
	}
	return 0
}
