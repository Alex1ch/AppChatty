package main

import (
	"log"

	"github.com/gotk3/gotk3/gtk"
)

func main() {
	// Инициализируем GTK.
	gtk.Init(nil)

	// Создаём окно верхнего уровня, устанавливаем заголовок
	// И соединяем с сигналом "destroy" чтобы можно было закрыть
	// приложение при закрытии окна
	win, err := gtk.WindowNew(gtk.WINDOW_TOPLEVEL)
	if err != nil {
		log.Fatal("Can't create window:", err)
	}
	win.SetTitle("AppChatty")
	win.Connect("destroy", func() {
		gtk.MainQuit()
	})

	// Создаём новую метку чтобы показать её в окне
	l, err := gtk.LabelNew("Hello world!")
	if err != nil {
		log.Fatal("Can't create label:", err)
	}

	// Добавляем метку в окно
	win.Add(l)

	// Устанавливаем размер окна по умолчанию
	win.SetDefaultSize(400, 1000)

	// Отображаем все виджеты в окне
	win.ShowAll()

	// Выполняем главный цикл GTK (для отрисовки). Он остановится когда
	// выполнится gtk.MainQuit()
	gtk.Main()
}
