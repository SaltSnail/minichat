package main

import (
	"bufio"
	"encoding/json"
	"log"
	"os"

	"github.com/gorilla/websocket"
)

type Message struct {
	Type       string `json:"type"`
	Token      string `json:"token"`
	ReceiverID string `json:"receiver_id"`
	Text       string `json:"text"`
}

func main() {
	conn, _, err := websocket.DefaultDialer.Dial("ws://localhost:8080/chat", nil)
	if err != nil {
		log.Fatal("Dial error:", err)
	}
	defer conn.Close()

	// Отправка токена
	//"eyJhbGciOiJIUzI1NiIsInR5cCI6IkpXVCJ9.eyJlbWFpbCI6InRlc3QyQGV4YW1wbGUuY29tIiwiZXhwIjoxNzQyNDYyNzg1LCJ1c2VyX2lkIjoidXNlcl8yIn0.gNAt7B1ZN7r3Du6CEmlJFjj_DW47NGK17tsTMzdLpyI" 2
	//"eyJhbGciOiJIUzI1NiIsInR5cCI6IkpXVCJ9.eyJlbWFpbCI6InRlc3RAZXhhbXBsZS5jb20iLCJleHAiOjE3NDI0NjI5MjEsInVzZXJfaWQiOiJ1c2VyXzEifQ.AFBMXguPxI2I3qLSAtAxrRCnkRELVBP45NzSAQodb7Q" 1
	authMsg := Message{Type: "auth", Token: "eyJhbGciOiJIUzI1NiIsInR5cCI6IkpXVCJ9.eyJlbWFpbCI6InRlc3QyQGV4YW1wbGUuY29tIiwiZXhwIjoxNzQyNDYyNzg1LCJ1c2VyX2lkIjoidXNlcl8yIn0.gNAt7B1ZN7r3Du6CEmlJFjj_DW47NGK17tsTMzdLpyI"}
	authBytes, _ := json.Marshal(authMsg)
	err = conn.WriteMessage(websocket.TextMessage, authBytes)
	if err != nil {
		log.Fatal("Auth error:", err)
	}
	log.Println("Connected to WebSocket")

	// Горoutine для чтения сообщений
	go func() {
		for {
			_, msg, err := conn.ReadMessage()
			if err != nil {
				log.Println("Read error:", err)
				return
			}
			log.Printf("Received: %s", msg)
		}
	}()

	// Чтение ввода из консоли и отправка сообщений
	scanner := bufio.NewScanner(os.Stdin)
	for scanner.Scan() {
		input := scanner.Text()
		msg := Message{
			Type:       "message",
			ReceiverID: "user_1",
			Text:       input,
		}
		msgBytes, _ := json.Marshal(msg)
		err = conn.WriteMessage(websocket.TextMessage, msgBytes)
		if err != nil {
			log.Println("Write error:", err)
			return
		}
	}
}
