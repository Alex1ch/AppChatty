package main

import (
	"fmt"
	"io/ioutil"
	"log"
	"net"

	"github.com/gotk3/gotk3/gtk"
)

var connection net.Conn

var mainWindow gtk.Window

var settings map[string]string

func main() {
	if parseSettings() != 0 {
		return
	}
	if drawMain() != 0 {
		return
	}
	gtk.Main()

}

func drawMain() int {
	//Init
	gtk.Init(nil)
	fmt.Println("GTK initialized")

	//Builder init
	b, err := gtk.BuilderNew()
	if err != nil {
		log.Fatal("Error:", err)
		return 1
	}
	err = b.AddFromFile("Layout/Layout.glade")
	if err != nil {
		log.Fatal("Error:", err)
		return 1
	}
	fmt.Println("Layout was loaded")

	//Getting objects and defining events
	//Main window
	obj, err := b.GetObject("Main_window")
	if err != nil {
		log.Fatal("Error:", err)
		return 2
	}
	mainWindow := obj.(*gtk.Window)
	mainWindow.Connect("destroy", func() {
		gtk.MainQuit()
	})

	//Send button
	obj, err = b.GetObject("CloseEvt")
	if err != nil {
		log.Fatal("Error:", err)
		return 3
	}
	closeBtn := obj.(*gtk.EventBox)
	closeBtn.Connect("button-release-event", func() {
		gtk.MainQuit()
	})

	//Sticker button
	obj, err = b.GetObject("StickersEvt")
	if err != nil {
		log.Fatal("Error:", err)
		return 3
	}
	stickersBtn := obj.(*gtk.EventBox)
	stickersBtn.Connect("button-release-event", func() {
		gtk.MainQuit()
	})

	mainWindow.ShowAll()
	return 0
}

func connectToServer() {

}

func parseSettings() int {
	b, err := ioutil.ReadFile("settings")
	if err != nil {
		log.Fatal("Error in setting parsing: ", err)
		return 1
	}
	str := string(b)
	//string.s
	return 0
}
