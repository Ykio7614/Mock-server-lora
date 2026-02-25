package main

import (
	"bufio"
	"fmt"
	"log"
	"math/rand"
	"net"
	"sort"
	"strings"
	"sync"
	"time"
)

type Session struct {
	ID        string
	Name      string
	StartDate string
	LastDate  string
	Points    string
	Settings  string
}

type uiClientState struct {
	pendingSettings string
}

var (
	sessionsMu sync.RWMutex
	sessions   = map[string]*Session{
		"Session1ID": {ID: "Session1ID", Name: "Сессия 1", StartDate: "2025-09-01", LastDate: "2025-09-10", Points: "12", Settings: "-"},
		"Session2ID": {ID: "Session2ID", Name: "Сессия 2", StartDate: "2025-09-05", LastDate: "2025-09-15", Points: "8", Settings: "SF=7, TX=14, BW=125"},
		"Session3ID": {ID: "Session3ID", Name: "Сессия 3", StartDate: "2025-09-08", LastDate: "2025-09-20", Points: "20", Settings: "-"},
		"Session4ID": {ID: "Session4ID", Name: "Сессия 4", StartDate: "2025-09-08", LastDate: "2025-09-20", Points: "20", Settings: "-"},
	}
)

var (
	stateMu          sync.Mutex
	uiClients        = make(map[net.Conn]*uiClientState)
	masterConnected  bool
	slaverConnected  bool
	lastSlaverSeen   time.Time
	activeMasterConn net.Conn
)

func writeLine(conn net.Conn, msg string) error {
	_, err := conn.Write([]byte(msg + "\n"))
	return err
}

func sanitizeField(s string) string {
	s = strings.ReplaceAll(s, "\n", " ")
	s = strings.ReplaceAll(s, "\r", " ")
	s = strings.ReplaceAll(s, "[", "(")
	s = strings.ReplaceAll(s, "]", ")")
	return strings.TrimSpace(s)
}

func formatSessions() string {
	sessionsMu.RLock()
	list := make([]*Session, 0, len(sessions))
	for _, s := range sessions {
		copySession := *s
		list = append(list, &copySession)
	}
	sessionsMu.RUnlock()

	sort.Slice(list, func(i, j int) bool {
		return list[i].ID < list[j].ID
	})

	var b strings.Builder
	b.WriteString("SESSIONS:")
	for _, s := range list {
		settings := s.Settings
		if strings.TrimSpace(settings) == "" {
			settings = "-"
		}
		b.WriteString(" [")
		b.WriteString(strings.Join([]string{
			sanitizeField(s.ID),
			sanitizeField(s.Name),
			sanitizeField(s.StartDate),
			sanitizeField(s.LastDate),
			sanitizeField(s.Points),
			sanitizeField(settings),
		}, ", "))
		b.WriteString("];")
	}
	b.WriteString("\n")
	return b.String()
}

func broadcastToUI(msg string) {
	stateMu.Lock()
	clients := make([]net.Conn, 0, len(uiClients))
	for conn := range uiClients {
		clients = append(clients, conn)
	}
	stateMu.Unlock()

	for _, conn := range clients {
		if err := writeLine(conn, msg); err != nil {
			stateMu.Lock()
			delete(uiClients, conn)
			stateMu.Unlock()
			_ = conn.Close()
		}
	}
}

func registerUIClient(conn net.Conn) {
	stateMu.Lock()
	uiClients[conn] = &uiClientState{}
	master := masterConnected
	slaver := slaverConnected
	stateMu.Unlock()

	if master {
		_ = writeLine(conn, "MASTER_STATUS CONNECTED")
	}
	if slaver {
		_ = writeLine(conn, "SLAVER_STATUS CONNECTED")
	}
}

func removeUIClient(conn net.Conn) {
	stateMu.Lock()
	delete(uiClients, conn)
	stateMu.Unlock()
}

func setPendingSettings(conn net.Conn, settings string) {
	stateMu.Lock()
	if st, ok := uiClients[conn]; ok {
		st.pendingSettings = settings
	}
	stateMu.Unlock()
}

func consumePendingSettings(conn net.Conn) string {
	stateMu.Lock()
	defer stateMu.Unlock()
	st, ok := uiClients[conn]
	if !ok {
		return ""
	}
	settings := strings.TrimSpace(st.pendingSettings)
	st.pendingSettings = ""
	return settings
}

func applySettingsToSession(sessionID, settings string) {
	if settings == "" {
		return
	}
	sessionsMu.Lock()
	defer sessionsMu.Unlock()
	if s, ok := sessions[sessionID]; ok {
		s.Settings = settings
	}
}

func handleClient(conn net.Conn) {
	defer conn.Close()
	addr := conn.RemoteAddr()
	log.Println("[UI] Подключен:", addr)

	registerUIClient(conn)
	defer removeUIClient(conn)

	reader := bufio.NewReader(conn)

	for {
		data, err := reader.ReadString('\n')
		if err != nil {
			log.Println("[UI] Клиент отключился:", addr)
			return
		}

		message := strings.TrimSpace(data)
		if message == "" {
			continue
		}
		log.Println("[UI] Получено:", message)

		switch {
		case strings.HasPrefix(message, "GET_MEASUREMENT_SESSIONS"):
			if _, err := conn.Write([]byte(formatSessions())); err != nil {
				return
			}

		case strings.HasPrefix(message, "START_MEASUREMENT"):
			parts := strings.SplitN(message, ":", 2)
			if len(parts) != 2 {
				_ = writeLine(conn, "ERROR: START_MEASUREMENT")
				continue
			}

			sessionID := strings.TrimSpace(parts[1])
			if sessionID == "" {
				_ = writeLine(conn, "ERROR: START_MEASUREMENT")
				continue
			}

			applySettingsToSession(sessionID, consumePendingSettings(conn))
			_ = writeLine(conn, "MEASUREMENT_STARTED: "+sessionID)
			go streamMeasurement(conn, sessionID)

		case strings.HasPrefix(message, "ADD_SESSION"):
			parts := strings.SplitN(message, ":", 2)
			if len(parts) != 2 {
				_ = writeLine(conn, "ERROR: ADD_SESSION")
				continue
			}

			content := strings.TrimSpace(parts[1])
			content = strings.TrimPrefix(content, "[")
			content = strings.TrimSuffix(content, "]")
			fields := strings.Split(content, ",")
			if len(fields) != 5 {
				_ = writeLine(conn, "ERROR: ADD_SESSION")
				continue
			}

			for i := range fields {
				fields[i] = strings.TrimSpace(fields[i])
			}

			s := &Session{
				ID:        fields[0],
				Name:      fields[1],
				StartDate: fields[2],
				LastDate:  fields[3],
				Points:    fields[4],
				Settings:  "-",
			}

			sessionsMu.Lock()
			sessions[s.ID] = s
			sessionsMu.Unlock()
			_ = writeLine(conn, "SESSION_ADDED")

		case strings.HasPrefix(message, "REMOVE_SESSION"):
			parts := strings.SplitN(message, ":", 2)
			if len(parts) != 2 {
				_ = writeLine(conn, "ERROR: REMOVE_SESSION")
				continue
			}
			id := strings.TrimSpace(parts[1])

			sessionsMu.Lock()
			_, ok := sessions[id]
			if ok {
				delete(sessions, id)
			}
			sessionsMu.Unlock()

			if ok {
				_ = writeLine(conn, "SESSION_REMOVED")
			} else {
				_ = writeLine(conn, "ERROR: SESSION_NOT_FOUND")
			}

		case strings.HasPrefix(message, "SET_SETTINGS"):
			parts := strings.SplitN(message, ":", 2)
			if len(parts) != 2 {
				_ = writeLine(conn, "ERROR: SET_SETTINGS")
				continue
			}
			settings := strings.TrimSpace(parts[1])
			setPendingSettings(conn, settings)
			log.Println("[UI] Применены настройки (ожидают привязки к сессии):", settings)
			_ = writeLine(conn, "SETTINGS_APPLIED")

		default:
			_ = writeLine(conn, "ERROR: UNKNOWN_COMMAND")
		}
	}
}

func streamMeasurement(conn net.Conn, sessionID string) {
	for i := 0; i < 10; i++ {
		msg := fmt.Sprintf(
			"MEASUREMENT: [2025-09-21 11:59:%02d, -75, 6, 1, %.8f, %.8f]",
			50+i,
			59.930051+float64(rand.Intn(10))/10000000,
			30.294510+float64(rand.Intn(10))/10000000,
		)
		if err := writeLine(conn, msg); err != nil {
			return
		}
		time.Sleep(200 * time.Millisecond)
	}

	for i := 0; i < 15; i++ {
		time.Sleep(1 * time.Second)
		msg := fmt.Sprintf(
			"MEASUREMENT: [2025-09-22 12:00:%02d, -70, 7, 0, %.9f, %.8f]",
			10+i,
			59.930051+float64(rand.Intn(10))/100000000,
			30.294510+float64(rand.Intn(10))/10000000,
		)
		if err := writeLine(conn, msg); err != nil {
			return
		}
	}

	// Обновим метаданные сессии после тестового потока.
	sessionsMu.Lock()
	if s, ok := sessions[sessionID]; ok {
		s.LastDate = time.Now().Format(time.RFC3339)
	}
	sessionsMu.Unlock()
}

func handleMaster(conn net.Conn) {
	defer conn.Close()
	log.Println("[MASTER] Подключен:", conn.RemoteAddr())

	stateMu.Lock()
	activeMasterConn = conn
	masterConnected = true
	stateMu.Unlock()
	broadcastToUI("MASTER_STATUS CONNECTED")

	reader := bufio.NewReader(conn)
	for {
		msg, err := reader.ReadString('\n')
		if err != nil {
			break
		}

		message := strings.TrimSpace(msg)
		if message == "" {
			continue
		}

		switch message {
		case "SLAVER_STATUS ISALIVE":
			stateMu.Lock()
			lastSlaverSeen = time.Now()
			shouldBroadcast := !slaverConnected
			slaverConnected = true
			stateMu.Unlock()

			if shouldBroadcast {
				broadcastToUI("SLAVER_STATUS CONNECTED")
			}
		case "MASTER_STATUS CONNECTED":
			// mock-master может отправить это сообщение после подключения; статус уже выставлен по факту соединения.
		default:
			log.Println("[MASTER] Сообщение:", message)
		}
	}

	stateMu.Lock()
	if activeMasterConn == conn {
		activeMasterConn = nil
		masterConnected = false
	}
	stateMu.Unlock()
	broadcastToUI("MASTER_STATUS DISCONNECTED")
	log.Println("[MASTER] Отключен")
}

func slaverWatcher() {
	ticker := time.NewTicker(1 * time.Second)
	defer ticker.Stop()

	for range ticker.C {
		stateMu.Lock()
		disconnect := slaverConnected && time.Since(lastSlaverSeen) > 10*time.Second
		if disconnect {
			slaverConnected = false
		}
		stateMu.Unlock()

		if disconnect {
			broadcastToUI("SLAVER_STATUS DISCONNECTED")
		}
	}
}

func startUIListener() {
	ln, err := net.Listen("tcp", "127.0.0.1:8082")
	if err != nil {
		log.Fatal(err)
	}
	log.Println("UI listener: 127.0.0.1:8082")

	for {
		conn, err := ln.Accept()
		if err != nil {
			log.Println("[UI] Accept error:", err)
			continue
		}
		go handleClient(conn)
	}
}

func startMasterListener() {
	ln, err := net.Listen("tcp", "127.0.0.1:9091")
	if err != nil {
		log.Fatal(err)
	}
	log.Println("MASTER listener: 127.0.0.1:9091")

	for {
		conn, err := ln.Accept()
		if err != nil {
			log.Println("[MASTER] Accept error:", err)
			continue
		}
		go handleMaster(conn)
	}
}

func main() {
	rand.Seed(time.Now().UnixNano())

	go startUIListener()
	go startMasterListener()
	go slaverWatcher()

	select {}
}
