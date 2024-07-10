package main

import (
	"context"
	"encoding/json"
	"log"
	"net/http"

	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/bson/primitive"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
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
	// Redis initialization
	rdb = redis.NewClient(&redis.Options{
		Addr:     "redis_cache:6379",
		Password: "",
		DB:       0,
	})

	// MongoDB initialization
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

	// Insert user into MongoDB
	_, err := userCollection.InsertOne(ctx, user)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	// Cache user in Redis
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

	// Check Redis cache
	cachedUser, err := rdb.Get(ctx, user.Username).Result()
	if err == redis.Nil {
		// If user is not in Redis, check MongoDB
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

	// Validate password
	if user.Password != r.FormValue("password") {
		http.Error(w, "Invalid password", http.StatusUnauthorized)
		return
	}

	w.WriteHeader(http.StatusOK)
}

func deleteHandler(w http.ResponseWriter, r *http.Request) {
	var user User
	json.NewDecoder(r.Body).Decode(&user)

	// Delete user from MongoDB
	_, err := userCollection.DeleteOne(ctx, bson.M{"username": user.Username})
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	// Delete user from Redis
	err = rdb.Del(ctx, user.Username).Err()
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	w.WriteHeader(http.StatusOK)
}

func main() {
	defer mongoClient.Disconnect(ctx)

	http.HandleFunc("/api/v1/register", registerHandler)
	http.HandleFunc("/api/v1/authorization", authorizationHandler)
	http.HandleFunc("/api/v1/delete", deleteHandler)

	log.Fatal(http.ListenAndServe(":8045", nil))
}
