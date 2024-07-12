package main

import (
	"context"
	"encoding/json"
	"fmt"
	"github.com/go-redis/redis/v8"
	"github.com/joho/godotenv"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/bson/primitive"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
	"log"
	"net/http"
	"os"
)

type User struct {
	ID       primitive.ObjectID `bson:"_id,omitempty"`
	Username string             `json:"username" bson:"username"`
	Password string             `json:"password" bson:"password"`
}

var (
	ctx            = context.Background()
	rdb            *redis.Client
	mongoClient    *mongo.Client
	userCollection *mongo.Collection
)

func init() {
	// Завантаження змінних середовища з файлу .env
	err := godotenv.Load()
	if err != nil {
		log.Fatalf("Error loading .env file")
	}

	// Ініціалізація Redis
	rdb = redis.NewClient(&redis.Options{
		Addr:     "redis_cache:6379", // Ім'я контейнера замість localhost
		Password: os.Getenv("REDIS_PASSWORD"),
		DB:       0, // Використання стандартної бази даних
	})
	fmt.Println("AAAAA")
	err = rdb.Set(ctx, "key", "value", 0).Err()
	if err != nil {
		panic(err)
	}

	val, err := rdb.Get(ctx, "key").Result()
	rdb.Set(ctx, "key", "value", 0)
	if err != nil {
		panic(err)
	}
	fmt.Println("key", val)

	// Ініціалізація MongoDB
	clientOptions := options.Client().ApplyURI("mongodb://mongo_db:27017")
	client, err := mongo.Connect(ctx, clientOptions)
	if err != nil {
		log.Fatal(err)
	}

	mongoClient = client
	userCollection = client.Database("test").Collection("users")
}

func registerHandler(w http.ResponseWriter, r *http.Request) {
	var user User
	json.NewDecoder(r.Body).Decode(&user)

	// Вставка користувача в MongoDB
	_, err := userCollection.InsertOne(ctx, user)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	// Кешування користувача в Redis
	userJson, _ := json.Marshal(user)
	err = rdb.Set(ctx, user.Username, userJson, 0).Err()
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	w.WriteHeader(http.StatusCreated)
}

func authorizationHandler(w http.ResponseWriter, r *http.Request) {
	var user User
	json.NewDecoder(r.Body).Decode(&user)

	// Перевірка в Redis
	cachedUser, err := rdb.Get(ctx, user.Username).Result()
	if err == redis.Nil {
		// Якщо користувача немає в Redis, перевіряємо в MongoDB
		err := userCollection.FindOne(ctx, bson.M{"username": user.Username}).Decode(&user)
		if err != nil {
			http.Error(w, "User not found", http.StatusNotFound)
			return
		}
	} else if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	} else {
		json.Unmarshal([]byte(cachedUser), &user)
	}

	// Перевірка пароля
	if user.Password != r.FormValue("password") {
		http.Error(w, "Invalid password", http.StatusUnauthorized)
		return
	}

	w.WriteHeader(http.StatusOK)
}

func deleteHandler(w http.ResponseWriter, r *http.Request) {
	var user User
	json.NewDecoder(r.Body).Decode(&user)

	// Видалення користувача з MongoDB
	_, err := userCollection.DeleteOne(ctx, bson.M{"username": user.Username})
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	// Видалення користувача з Redis
	err = rdb.Del(ctx, user.Username).Err()
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	w.WriteHeader(http.StatusOK)
}

func main() {
	// Перевірка підключення до Redis
	pong, err := rdb.Ping(ctx).Result()
	if err != nil {
		fmt.Println("Помилка підключення:", err)
		return
	}
	fmt.Println("Підключено до Redis:", pong)

	http.HandleFunc("/api/v1/register", registerHandler)
	http.HandleFunc("/api/v1/authorization", authorizationHandler)
	//http.HandleFunc("/api/v1/delete", deleteHandler)
	s, _ := rdb.Get(ctx, "key").Result()
	fmt.Println(s)

	fmt.Println("Працює")

	log.Fatal(http.ListenAndServe(":8045", nil))
}
