package main

import (
	"log"
	"net"
	"time"
)

const (
	masterAddr = "127.0.0.1:9090"
	interval   = 5 * time.Second
)

func main() {
	for {
		log.Println("Подключение к mock-master:", masterAddr)

		conn, err := net.Dial("tcp", masterAddr)
		if err != nil {
			log.Println("Ошибка подключения:", err)
			time.Sleep(5 * time.Second)
			continue
		}

		log.Println("Подключен к mock-master")

		for {
			_, err := conn.Write([]byte("SLAVER_STATUS ISALIVE\n"))
			if err != nil {
				log.Println("Соединение потеряно, переподключение")
				conn.Close()
				break
			}

			log.Println("Отправлено: SLAVER_STATUS ISALIVE")
			time.Sleep(interval)
		}
	}
}
