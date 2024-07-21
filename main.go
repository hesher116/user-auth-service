package main

import (
	"context"
	"encoding/json"
	"fmt"
	"github.com/gin-gonic/gin"
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

type Module struct {
	redisCli       *redis.Client
	userCollection *mongo.Collection
}

func (m *Module) registerHandler(c *gin.Context) {
	ctx := c.Request.Context()
	var user User
	if err := c.ShouldBindJSON(&user); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	// Вставка користувача в MongoDB
	_, err := m.userCollection.InsertOne(ctx, user)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	// Кешування користувача в Redis
	userJson, err := json.Marshal(user)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"Failed to marshal user data": err.Error()})
	}
	err = m.redisCli.Set(ctx, user.Username, userJson, 0).Err()
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusCreated, user)
}

func (m *Module) authorizationHandler(c *gin.Context) {
	ctx := c.Request.Context()
	var user User
	if err := c.ShouldBindJSON(&user); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	// Перевірка в Redis
	cachedUser, err := m.redisCli.Get(ctx, user.Username).Result()
	switch err {
	case redis.Nil:
		// Якщо користувача немає в Redis, перевіряємо в MongoDB
		err := m.userCollection.FindOne(ctx, bson.M{"username": user.Username}).Decode(&user)
		if err != nil {
			c.JSON(http.StatusNotFound, gin.H{"error": "User not found"})
			return
		}
	case nil:
		json.Unmarshal([]byte(cachedUser), &user)
	default:
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	// Перевірка пароля
	if user.Password != c.PostForm("password") {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "Invalid password"})
		return
	}

	c.JSON(http.StatusOK, gin.H{"message": "Authorization successful"})
}

func (m *Module) deleteHandler(c *gin.Context) {
	ctx := c.Request.Context()
	var user User
	if err := c.ShouldBindJSON(&user); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	// Видалення користувача з MongoDB
	_, err := m.userCollection.DeleteOne(ctx, bson.M{"username": user.Username})
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	// Видалення користувача з Redis
	err = m.redisCli.Del(ctx, user.Username).Err()
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{"message": "User deleted"})
}

func main() {
	err := godotenv.Load()

	fmt.Println("redisPort", os.Getenv("REDIS_PORT"))

	redisPort := os.Getenv("REDIS_PORT")
	redisHost := os.Getenv("REDIS_HOST")
	redisUrl := fmt.Sprintf("%s:%s", redisHost, redisPort)
	// Ініціалізація Redis
	rdb := redis.NewClient(&redis.Options{
		Addr: redisUrl, // Ім'я контейнера замість localhost
		DB:   0,        // Використання стандартної бази даних
	})

	// Перевірка підключення до Redis
	ctx := context.Background()
	pong, err := rdb.Ping(ctx).Result()
	if err != nil {
		log.Fatalf("Помилка підключення:", err)
		return
	}
	fmt.Println("Підключено до Redis:", pong)

	// Ініціалізація MongoDB
	mongoPort := os.Getenv("MONGO_PORT")
	mongoHost := os.Getenv("MONGO_HOST")
	mongoUrl := fmt.Sprintf("mongodb://%s:%s", mongoHost, mongoPort)
	clientOptions := options.Client().ApplyURI(mongoUrl)
	client, err := mongo.Connect(ctx, clientOptions)
	if err != nil {
		log.Fatal(err)
	}

	//mongoClient = client
	userCollection := client.Database("test").Collection("users")

	module := &Module{
		redisCli:       rdb,
		userCollection: userCollection,
	}

	router := gin.Default()

	// Маршрутизація
	router.POST("/api/v1/register", module.registerHandler)
	router.POST("/api/v1/authorization", module.authorizationHandler)
	fmt.Println()
	router.DELETE("/api/v1/delete", module.deleteHandler)

	fmt.Println("Працює")
	// Запуск сервера
	log.Fatal(router.Run(":8045"))
}
