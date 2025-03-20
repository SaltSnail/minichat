package main

import (
	"context"
	"encoding/json"
	"log"
	"net/http"
	"sync"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"github.com/gorilla/websocket"
	"github.com/redis/go-redis/v9"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
)

var upgrader = websocket.Upgrader{}
var jwtSecret = []byte("your-secret-key")
var mongoClient *mongo.Client
var redisClient *redis.Client

// Структура для сообщения
type Message struct {
	Type       string `json:"type"`
	Token      string `json:"token,omitempty"`
	ReceiverID string `json:"receiver_id,omitempty" bson:"receiver_id"`
	Text       string `json:"text" bson:"text"`
	SenderID   string `json:"-" bson:"sender_id"` // Не отправляем клиенту
	Timestamp  string `json:"timestamp" bson:"timestamp"`
}

// Структура клиента
type Client struct {
	UserID string
	Conn   *websocket.Conn
}

var clients = struct {
	sync.Mutex
	m map[string]*Client
}{m: make(map[string]*Client)}

func validateToken(tokenString string) (string, error) {
	token, err := jwt.Parse(tokenString, func(token *jwt.Token) (interface{}, error) {
		return jwtSecret, nil
	})
	if err != nil || !token.Valid {
		return "", err
	}
	claims, ok := token.Claims.(jwt.MapClaims)
	if !ok {
		return "", err
	}
	userID, ok := claims["user_id"].(string)
	if !ok {
		return "", err
	}
	return userID, nil
}

func handleWebSocket(w http.ResponseWriter, r *http.Request) {
	conn, err := upgrader.Upgrade(w, r, nil)
	if err != nil {
		log.Println("Upgrade error:", err)
		return
	}
	defer conn.Close()

	// Авторизация
	_, msg, err := conn.ReadMessage()
	if err != nil {
		log.Println("Read error:", err)
		return
	}
	var authMsg Message
	if err := json.Unmarshal(msg, &authMsg); err != nil || authMsg.Type != "auth" {
		log.Println("Invalid auth message")
		return
	}
	userID, err := validateToken(authMsg.Token)
	if err != nil {
		log.Println("Invalid token:", err)
		return
	}

	// Регистрация клиента
	client := &Client{UserID: userID, Conn: conn}
	clients.Lock()
	clients.m[userID] = client
	clients.Unlock()
	log.Printf("User %s connected", userID)
	defer func() {
		clients.Lock()
		delete(clients.m, userID)
		clients.Unlock()
		log.Printf("User %s disconnected", userID)
	}()

	// Отправка истории из Redis или MongoDB
	ctx := context.Background()
	history, err := redisClient.LRange(ctx, "chat:"+userID, 0, 9).Result() // Последние 10 сообщений
	if err != nil || len(history) == 0 {
		cursor, err := mongoClient.Database("chat").Collection("messages").Find(ctx, bson.M{
			"$or": []bson.M{
				{"sender_id": userID},
				{"receiver_id": userID},
			},
		})
		if err != nil {
			log.Println("MongoDB find error:", err)
		} else {
			var messages []Message
			cursor.All(ctx, &messages)
			for _, m := range messages {
				msgBytes, _ := json.Marshal(m)
				conn.WriteMessage(websocket.TextMessage, msgBytes)
				redisClient.LPush(ctx, "chat:"+userID, msgBytes) // Кэшируем в Redis
			}
			redisClient.LTrim(ctx, "chat:"+userID, 0, 9) // Ограничиваем до 10
		}
	} else {
		for _, m := range history {
			conn.WriteMessage(websocket.TextMessage, []byte(m))
		}
	}

	// Основной цикл
	for {
		_, msg, err := conn.ReadMessage()
		if err != nil {
			log.Println("Read error:", err)
			return
		}

		var message Message
		if err := json.Unmarshal(msg, &message); err != nil || message.Type != "message" {
			log.Println("Invalid message format")
			continue
		}

		// Заполняем сообщение
		message.SenderID = userID
		message.Timestamp = time.Now().UTC().Format(time.RFC3339)
		// Сохранение в MongoDB
		_, err = mongoClient.Database("chat").Collection("messages").InsertOne(ctx, message)
		if err != nil {
			log.Println("MongoDB insert error:", err)
		}

		// Кэширование в Redis
		msgBytes, _ := json.Marshal(message)
		redisClient.LPush(ctx, "chat:"+message.ReceiverID, msgBytes)
		redisClient.LTrim(ctx, "chat:"+message.ReceiverID, 0, 9)

		// Публикация в Redis Pub/Sub
		err = redisClient.Publish(ctx, "new_message", msgBytes).Err()
		if err != nil {
			log.Println("Redis publish error:", err)
		}

		// Отправка получателю
		clients.Lock()
		receiver, exists := clients.m[message.ReceiverID]
		clients.Unlock()
		if exists {
			err = receiver.Conn.WriteMessage(websocket.TextMessage, msgBytes)
			if err != nil {
				log.Printf("Error sending to %s: %v", message.ReceiverID, err)
			} else {
				log.Printf("Sent to %s: %s", message.ReceiverID, message.Text)
			}
		} else {
			log.Printf("User %s is offline", message.ReceiverID)
		}
	}
}

func main() {
	// Подключение к MongoDB
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	var err error
	mongoClient, err = mongo.Connect(ctx, options.Client().ApplyURI("mongodb://localhost:27017"))
	if err != nil {
		log.Fatal("MongoDB connection error:", err)
	}
	defer mongoClient.Disconnect(ctx)
	if err = mongoClient.Ping(ctx, nil); err != nil {
		log.Fatal("MongoDB ping error:", err)
	}
	log.Println("Connected to MongoDB")

	// Подключение к Redis
	redisClient = redis.NewClient(&redis.Options{
		Addr: "localhost:6379",
	})
	if _, err = redisClient.Ping(ctx).Result(); err != nil {
		log.Fatal("Redis connection error:", err)
	}
	log.Println("Connected to Redis")

	http.HandleFunc("/chat", handleWebSocket)
	log.Println("Message Service running on :8080")
	log.Fatal(http.ListenAndServe(":8080", nil))
}
