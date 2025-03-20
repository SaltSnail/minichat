package main

import (
	"context"
	"encoding/json"
	"log"
	"net/http"
	"net/smtp"

	"github.com/redis/go-redis/v9"
)

var redisClient *redis.Client

// Структура сообщения из Redis
type Message struct {
	SenderID   string `json:"sender_id"`
	ReceiverID string `json:"receiver_id"`
	Email      string `json:"email"`
	Text       string `json:"text"`
	Timestamp  string `json:"timestamp"`
}

func sendEmail(to, subject, body string) error {
	smtpHost := "smtp.gmail.com"
	smtpPort := "587"
	smtpUser := "denchikdreams@gmail.com"
	smtpPass := "your-app-password"

	auth := smtp.PlainAuth("", smtpUser, smtpPass, smtpHost)
	msg := []byte("To: " + to + "\r\n" +
		"Subject: " + subject + "\r\n" +
		"\r\n" +
		body + "\r\n")

	return smtp.SendMail(smtpHost+":"+smtpPort, auth, smtpUser, []string{to}, msg)
}

func subscribeToMessages() {
	ctx := context.Background()
	pubsub := redisClient.Subscribe(ctx, "new_message")
	defer pubsub.Close()

	log.Println("Subscribed to new_message channel")

	for msg := range pubsub.Channel() {
		var message Message
		if err := json.Unmarshal([]byte(msg.Payload), &message); err != nil {
			log.Println("Error unmarshaling message:", err)
			continue
		}

		// Отправка email
		subject := "New message from " + message.SenderID
		body := "You received a message from " + message.SenderID + " at " + message.Timestamp + ":\n" + message.Text
		err := sendEmail(message.Email, subject, body)
		if err != nil {
			log.Printf("Error sending email to %s: %v", message.Email, err)
		} else {
			log.Printf("Email sent to %s", message.Email)
		}
	}
}

func main() {
	// Подключение к Redis
	redisClient = redis.NewClient(&redis.Options{
		Addr: "localhost:6379",
	})
	ctx := context.Background()
	if _, err := redisClient.Ping(ctx).Result(); err != nil {
		log.Fatal("Redis connection error:", err)
	}
	log.Println("Connected to Redis")

	// Запуск подписки на сообщения
	go subscribeToMessages()

	// HTTP для проверки состояния
	http.HandleFunc("/health", func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte("Notification service is alive"))
	})
	log.Println("Notification Service running on :8082")
	log.Fatal(http.ListenAndServe(":8082", nil))
}
