import socket
import threading
import time
import random

sessions = {
    "Session1ID": ["Session1ID", "Сессия 1", "2025-09-01", "2025-09-10", "12"],
    "Session2ID": ["Session2ID", "Сессия 2", "2025-09-05", "2025-09-15", "8"],
    "Session3ID": ["Session3ID", "Сессия 3", "2025-09-08", "2025-09-20", "20"],
    "Session4ID": ["Session4ID", "Сессия 4", "2025-09-08", "2025-09-20", "20"],
    "Session5ID": ["Session5ID", "Сессия 5", "2025-09-08", "2025-09-20", "20"],
    "Session6ID": ["Session6ID", "Сессия 6", "2025-09-05", "2025-09-15", "8"],
    "Session7ID": ["Session7ID", "Сессия 7", "2025-09-08", "2025-09-20", "20"],
    "Session8ID": ["Session8ID", "Сессия 8", "2025-09-08", "2025-09-20", "20"],
    "Session9ID": ["Session9ID", "Сессия 9", "2025-09-08", "2025-09-20", "20"]
    
}


def format_sessions():
    """Формирует строку сессий в нужном формате"""
    return "SESSIONS:" + "; ".join([f"[{', '.join(v)}]" for v in sessions.values()]) + "\n"


def handle_client(conn, addr):
    print("Подключен:", addr)
    try:
        while True:
            data = conn.recv(1024)
            if not data:
                break

            message = data.decode().strip()
            print("Получено:", message)

            if message.startswith("GET_MEASUREMENT_SESSIONS"):
                response = format_sessions()
                print("Отправлено:", response)
                conn.sendall(response.encode())

            elif message.startswith("START_MEASUREMENT"):
                parts = message.split(":")
                if len(parts) > 1:
                    session_id = parts[1].strip()
                    response = f"MEASUREMENT_STARTED: {session_id}\n"
                    print("Отправлено:", response)
                    conn.sendall(response.encode())

                    for i in range(10):
                        measurement = f"MEASUREMENT: [2025-09-21 11:59:{50+i}, -75, 6, 1, {59.930051 + random.randint(0,10)/10000000}, {30.29451 + random.randint(0,10)/10000000}]\n"
                        print("Отправлено (истор.):", measurement.strip())
                        conn.sendall(measurement.encode())
                        time.sleep(0.2)

                    def send_measurements():
                        for i in range(15):
                            time.sleep(1)
                            measurement = f"MEASUREMENT: [2025-09-22 12:00:{10+i}, -70, 7, 0, {59.930051 + random.randint(0,10)/100000000}, {30.29451 + random.randint(0,10)/10000000}]\n"
                            print("Отправлено (новое):", measurement.strip())
                            conn.sendall(measurement.encode())

                    threading.Thread(target=send_measurements, daemon=True).start()

            elif message.startswith("ADD_SESSION"):
                try:
                    content = message.split(":", 1)[1].strip()
                    content = content.strip("[]")
                    fields = [f.strip() for f in content.split(",")]
                    if len(fields) == 5:
                        sessions[fields[0]] = fields
                        print("Добавлена сессия:", fields)
                        conn.sendall(b"SESSION_ADDED\n")
                except Exception as e:
                    print("Ошибка при добавлении сессии:", e)
                    conn.sendall(b"ERROR: ADD_SESSION\n")

            elif message.startswith("REMOVE_SESSION"):
                parts = message.split(":")
                if len(parts) > 1:
                    session_id = parts[1].strip()
                    if session_id in sessions:
                        del sessions[session_id]
                        print("Удалена сессия:", session_id)
                        conn.sendall(b"SESSION_REMOVED\n")
                    else:
                        conn.sendall(b"ERROR: SESSION_NOT_FOUND\n")

            elif message.startswith("SET_SETTINGS"):
                print("Применены настройки:", message)
                conn.sendall(b"SETTINGS_APPLIED\n")

            else:
                conn.sendall(b"ERROR: UNKNOWN_COMMAND\n")

    except ConnectionResetError:
        print("Клиент отключился:", addr)
    finally:
        conn.close()


def start_server():
    server = socket.socket(socket.AF_INET, socket.SOCK_STREAM)
    server.bind(("127.0.0.1", 8082))
    server.listen(1)
    print("Сервер запущен на 127.0.0.1:8082 ...")

    while True:
        conn, addr = server.accept()
        threading.Thread(target=handle_client, args=(conn, addr), daemon=True).start()


if __name__ == "__main__":
    start_server()
