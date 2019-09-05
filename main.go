package main

import (
	"context"
	"encoding/json"
	"errors"
	"github.com/gorilla/mux"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/bson/primitive"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
	"html/template"
	"log"
	"net/http"
	"os"
	"os/signal"
	"reflect"
	"sync"
	"time"
)

type Card struct {
	ID     primitive.ObjectID `json:"_id,omitempty" bson:"_id,omitempty"`
	Name   string             `json:"name,omitempty" bson:"name,omitempty"`
	Width  string             `json:"width,omitempty" bson:"width,omitempty"`
	Height string             `json:"height,omitempty" bson:"height,omitempty"`
}

type errorString struct{ s string }

func New(text string) error          { return &errorString{text} }
func (e *errorString) Error() string { return e.s }

type dbConfig struct {
	CollectionName string
	DatabaseName   string
	Host           string
	WaitTime       time.Duration
}
type dbServer struct {
	CollectionName string
	DatabaseName   string
	Database       *mongo.Database
	Client         *mongo.Client
	Collection     *mongo.Collection
	WaitTime       time.Duration
}

type httpConfig struct {
	Host         string
	ReadTimeout  time.Duration
	WriteTimeout time.Duration
}
type httpServer struct {
	server *http.Server
	wg     sync.WaitGroup
	Host         string
	ReadTimeout  time.Duration
	WriteTimeout time.Duration
}

var dbCfg dbConfig
var httpCfg httpConfig

func main() {
	// TODO: put back in code to defer closing mongo connection on exit

	dbCfg = dbConfig{
		DatabaseName:   "devDb",
		CollectionName: "cards",
		Host:           "mongodb://localhost:27017/",
		WaitTime:       10 * time.Second,
	}
	dbServer, err := connectDb(dbCfg)
	if err != nil {
		log.Println("ERROR: Database connect failed:", err)
		os.Exit(5)
	}

	httpCfg = httpConfig{
		Host:         "localhost:8080",
		ReadTimeout:  5 * time.Second,
		WriteTimeout: 5 * time.Second,
	}
	router, _ := getRouter(dbServer.Database)
	httpServer := startHttp(httpCfg, router)

	defer func() {
		err = httpServer.stopHttp()
		if err != nil {
			log.Println("stopHttp error:", err)
		}
	}()
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, os.Interrupt)
	<-sigChan

}
func connectDb(cfg dbConfig) (*dbServer, error) {
	ctx, _ := context.WithTimeout(context.Background(), cfg.WaitTime)
	client, err := mongo.Connect(ctx, options.Client().ApplyURI(cfg.Host))
	if err != nil {
		err := errors.New(`{"error":"Connect to db failed"}`)
		return nil, err
	}
	err = client.Ping(ctx, nil)
	if err != nil {
		err := errors.New(`{"error":"Connect to db failed"}`)
		return nil, err
	}
	log.Println("MongoDB | Uri:", cfg.Host, " | Database:", cfg.DatabaseName)
	db := dbServer{
		Database:       client.Database(cfg.DatabaseName),
		DatabaseName:       cfg.DatabaseName,
		Client:         client,
		CollectionName: cfg.CollectionName,
	}
	return &db, err
}
func getRouter(db *mongo.Database) (*mux.Router, error) {
	r := mux.NewRouter()
	r.HandleFunc("/test", getLocalTest).Methods("GET")
	r.HandleFunc("/template/{id}", func(w http.ResponseWriter, r *http.Request) { getIndex(w, r, db) }).Methods("GET")
	r.HandleFunc("/cards", func(w http.ResponseWriter, r *http.Request) { getAllCards(w, r, db) }).Methods("GET")
	r.HandleFunc("/cards", func(w http.ResponseWriter, r *http.Request) { addCard(w, r, db) }).Methods("POST")
	r.HandleFunc("/cards/{id}", func(w http.ResponseWriter, r *http.Request) { getOneCard(w, r, db) }).Methods("GET")
	r.HandleFunc("/cards/{id}", func(w http.ResponseWriter, r *http.Request) { deleteCard(w, r, db) }).Methods("DELETE")
	r.HandleFunc("/cards/{id}", func(w http.ResponseWriter, r *http.Request) { updateCard(w, r, db) }).Methods("PUT")
	err := errors.New(`{"error":"getRouter"}`)
	return r, err
}
func startHttp(cfg httpConfig, router *mux.Router) *httpServer {
	_, cancel := context.WithCancel(context.Background())
	defer cancel()
	httpServer := httpServer{
		server: &http.Server{
			Addr:           cfg.Host,
			Handler:        router,
			ReadTimeout:    cfg.ReadTimeout,
			WriteTimeout:   cfg.WriteTimeout,
			MaxHeaderBytes: 1 << 20,
		},
	}
	httpServer.wg.Add(1)
	go func() {
		log.Println("httpServer | Host:", cfg.Host)
		err := httpServer.server.ListenAndServe()
		if err != nil {
			log.Println("httpServer Status:", err)
			os.Exit(6)
		}
		httpServer.wg.Done()
	}()
	return &httpServer
}
func (httpServer *httpServer) stopHttp() error {
	ctx, cancel := context.WithTimeout(context.Background(), httpServer.server.WriteTimeout)
	defer cancel()
	log.Println("httpServer | Service stopping")
	if err := httpServer.server.Shutdown(ctx); err != nil {
		if err := httpServer.server.Close(); err != nil {
			log.Println("httpServer | ERROR:", err)
			return err
		}
	}
	httpServer.wg.Wait()
	log.Println("httpServer5 | Stopped")
	return nil
}

func getLocalTest(w http.ResponseWriter, r *http.Request) {
	log.Println("API: getLocalTest()")
	var card []Card
	card = append(card, Card{
		ID:     primitive.ObjectID{000000000000000000000011},
		Width:  "111px",
		Height: "222px",
	})
	w.WriteHeader(200)
	w.Header().Set("content-type", "application/json")
	_ = json.NewEncoder(w).Encode(card)
}
func addCard(w http.ResponseWriter, r *http.Request, db *mongo.Database) {
	w.Header().Set("content-type", "application/json")
	var card Card
	err := json.NewDecoder(r.Body).Decode(&card)
	log.Println("API: addCard() card:", card)
	if err != nil {
		w.WriteHeader(http.StatusBadRequest)
		bytes, _ := w.Write([]byte(`{ "error": "` + err.Error() + `" }`))
		log.Println("API: addCard() ERROR1 | Err:", err.Error(), "  |  Bytes: ", bytes)
		return
	}
	err = isValidCard(card)
	if err != nil {
		w.WriteHeader(http.StatusBadRequest)
		bytes, _ := w.Write([]byte(`{ "error": "` + err.Error() + `" }`))
		log.Println("API: addCard() ERROR2 | Err:", err.Error(), "  |  Bytes: ", bytes)
		return
	}
	card.ID = primitive.NewObjectID()
	col := db.Collection("cards")
	ret, err := col.InsertOne(context.Background(), card)
	if err != nil {
		w.WriteHeader(http.StatusBadRequest)
		bytes, _ := w.Write([]byte(`{ "error": "` + err.Error() + `" }`))
		log.Println("API: addCard() ERROR3 | Err:", err.Error(), "  |  Bytes: ", bytes)
		return
	}
	w.WriteHeader(http.StatusOK)
	log.Println("API: addCard() | Added:", ret.InsertedID)
	_ = json.NewEncoder(w).Encode(ret)
}
func updateCard(w http.ResponseWriter, r *http.Request, db *mongo.Database) {
	w.Header().Set("content-type", "application/json")
	params := mux.Vars(r)
	id, err := primitive.ObjectIDFromHex(params["id"])
	if err != nil {
		w.WriteHeader(http.StatusBadRequest)
		bytes, _ := w.Write([]byte(`{ "error": "` + err.Error() + `" }`))
		log.Println("API: updateCard() ERROR | Id:", id, "  |   Err:", err.Error(), "  |  Bytes: ", bytes)
		return
	}
	var card Card
	err = json.NewDecoder(r.Body).Decode(&card)
	if err != nil {
		w.WriteHeader(http.StatusBadRequest)
		bytes, _ := w.Write([]byte(`{ "error": "` + err.Error() + `" }`))
		log.Println("API: updateCard() ERROR | Err:", err.Error(), "  |  Bytes: ", bytes)
		return
	}
	idDoc := bson.D{{"_id", id}}
	col := db.Collection("cards")
	upd := bson.D{{"$set",
		bson.D{
			{"width", card.Width},
			{"height", card.Height},
		}}}
	ret, err := col.UpdateOne(context.TODO(), idDoc, upd)
	if err != nil {
		w.WriteHeader(http.StatusBadRequest)
		bytes, _ := w.Write([]byte(`{ "error": "` + err.Error() + `" }`))
		log.Println("API: updateCard() ERROR | Err:", err.Error(), "  |  Bytes: ", bytes)
		return
	}
	w.WriteHeader(http.StatusOK)
	log.Println("API: updateCard() | Updated:", id)
	_ = json.NewEncoder(w).Encode(ret)
}
func isValidCard(card Card) error {
	var errCnt int = 0
	if len(card.Name) < 2 || reflect.TypeOf(card.Name).String() != "string" {
		errCnt = errCnt + 1
	}
	if len(card.Width) < 2 || reflect.TypeOf(card.Width).String() != "string" {
		errCnt = errCnt + 1
	}
	if len(card.Height) < 2 || reflect.TypeOf(card.Height).String() != "string" {
		errCnt = errCnt + 1
	}
	if errCnt != 0 {
		return errors.New("isValidCard() failed, missing fields in Insert()")
	}
	return nil
}
func getOneCard(w http.ResponseWriter, r *http.Request, db *mongo.Database) {
	w.Header().Set("content-type", "application/json")
	params := mux.Vars(r)
	id, err := primitive.ObjectIDFromHex(params["id"])
	if err != nil {
		w.WriteHeader(http.StatusBadRequest)
		bytes, _ := w.Write([]byte(`{ "error": "` + err.Error() + `" }`))
		log.Println("API: getOneCard() ERROR: ObjectId() failed | Id:", id, "  |   Err:", err.Error(), "  |  Bytes: ", bytes)
		return
	}
	col := db.Collection("cards")
	var ret Card
	filter := Card{ID: id}
	err = col.FindOne(context.Background(), filter).Decode(&ret)
	if err != nil {
		w.WriteHeader(http.StatusNotFound)
		bytes, _ := w.Write([]byte(`{ "error": "` + err.Error() + `" }`))
		log.Println("API: getOneCard() ERROR: FindOne() failed | Id:", id, "  |   Err:", err.Error(), " |  Bytes: ", bytes)
		return
	}
	w.WriteHeader(http.StatusOK)
	log.Println("API: getOneCard() | Found:", ret.ID)
	_ = json.NewEncoder(w).Encode(ret)
}
func getAllCards(w http.ResponseWriter, r *http.Request, db *mongo.Database) {
	cur, _ := db.Collection("cards").Find(context.TODO(), bson.M{})
	var ret []Card
	for cur.Next(context.Background()) {
		var card Card
		_ = cur.Decode(&card)
		ret = append(ret, card)
	}
	if len(ret) == 0 {
		w.WriteHeader(http.StatusNotFound)
		bytes, _ := w.Write([]byte(`{ "error": "No records found" }`))
		log.Println("API: getAllCards() ERROR: None found | bytes:", bytes)
		return
	}
	log.Println("API: getAllCards() | Found:", len(ret))
	w.Header().Set("content-type", "application/json")
	w.WriteHeader(http.StatusOK)
	_ = json.NewEncoder(w).Encode(ret)
}
func deleteCard(w http.ResponseWriter, r *http.Request, db *mongo.Database) {
	w.Header().Set("content-type", "application/json")
	params := mux.Vars(r)
	id, err := primitive.ObjectIDFromHex(params["id"])
	if err != nil {
		w.WriteHeader(http.StatusBadRequest)
		bytes, _ := w.Write([]byte(`{ "error": "` + err.Error() + `" }`))
		log.Println("API: deleteCard() ERROR | Id:", id, "  |   Err:", err.Error(), "  |  Bytes: ", bytes)
		return
	}
	col := db.Collection("cards")
	ret, err := col.DeleteOne(context.Background(), bson.M{"_id": id})
	if err != nil {
		w.WriteHeader(http.StatusBadRequest)
		bytes, _ := w.Write([]byte(`{ "error": "` + err.Error() + `" }`))
		log.Println("API: deleteCard() ERROR | Err:", err.Error(), "  |  Bytes: ", bytes)
		return
	}
	log.Println("API: deleteCard() | Deleted:", ret.DeletedCount)
	w.WriteHeader(http.StatusOK)
	_ = json.NewEncoder(w).Encode(ret)
}

func getIndex(w http.ResponseWriter, r *http.Request, db *mongo.Database) {

	params := mux.Vars(r)
	id, err := primitive.ObjectIDFromHex(params["id"])
	if err != nil {
		w.WriteHeader(http.StatusBadRequest)
		bytes, _ := w.Write([]byte(`{ "error": "` + err.Error() + `" }`))
		log.Println("API: getOneCard() ERROR: ObjectId() failed | Id:", id, "  |   Err:", err.Error(), "  |  Bytes: ", bytes)
		return
	}
	col := db.Collection("cards")
	var ret Card
	filter := Card{ID: id}
	err = col.FindOne(context.Background(), filter).Decode(&ret)
	if err != nil {
		w.WriteHeader(http.StatusNotFound)
		bytes, _ := w.Write([]byte(`{ "error": "` + err.Error() + `" }`))
		log.Println("API: getOneCard() ERROR: FindOne() failed | Id:", id, "  |   Err:", err.Error(), " |  Bytes: ", bytes)
		return
	}
	log.Println("API: getOneCard() | Found:", ret.ID)

	tmpl := template.Must(template.ParseFiles("card.html"))
	_ = tmpl.Execute(w, ret)
}
