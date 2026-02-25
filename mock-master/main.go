package main

import (
	"bufio"
	"log"
	"net"
)

const (
	mockServerAddr = "127.0.0.1:9091" // сервер из предыдущего шага
	masterListen   = "127.0.0.1:9090" // где слушает master
)

func main() {
	// Подключение к mock-server
	serverConn, err := net.Dial("tcp", mockServerAddr)
	if err != nil {
		log.Fatal("Не удалось подключиться к mock-server:", err)
	}
	defer serverConn.Close()

	log.Println("Подключен к mock-server")
	serverConn.Write([]byte("MASTER_STATUS CONNECTED\n"))

	// Запуск TCP-сервера для slaver
	ln, err := net.Listen("tcp", masterListen)
	if err != nil {
		log.Fatal("Ошибка запуска master-сервера:", err)
	}
	defer ln.Close()

	log.Println("mock-master слушает:", masterListen)

	for {
		slaverConn, err := ln.Accept()
		if err != nil {
			continue
		}
		log.Println("Подключился mock-slaver:", slaverConn.RemoteAddr())
		go handleSlaver(slaverConn, serverConn)
	}
}

func handleSlaver(slaverConn net.Conn, serverConn net.Conn) {
	defer slaverConn.Close()

	reader := bufio.NewReader(slaverConn)

	for {
		msg, err := reader.ReadString('\n')
		if err != nil {
			log.Println("mock-slaver отключился")
			return
		}

		log.Println("Получено от slaver:", msg[:len(msg)-1])

		// Пробрасываем сообщение в mock-server
		serverConn.Write([]byte(msg))
	}
}
