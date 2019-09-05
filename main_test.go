package main

import (
	"bytes"
	"encoding/json"
	"github.com/gorilla/mux"
	"github.com/stretchr/testify/assert"
	"io/ioutil"
	"log"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

var newId string
var db *dbServer

func init() {
	log.Println("TEST INIT: ")
	dbCfg = dbConfig{
		DatabaseName:   "testDb",
		CollectionName: "cards",
		Host:           "mongodb://localhost:27017/",
		WaitTime:       10 * time.Second,
	}
}

func Router() *mux.Router {
	dbServer, _ := connectDb(dbCfg)
	rt, _ := getRouter(dbServer.Database)
	return rt
}

// This turns off log.printLn() in the app for testing verbosity
//func TestMain(m *testing.M) { log.SetOutput(ioutil.Discard); os.Exit(m.Run()) }

func TestGetLocalTest(t *testing.T) {
	request, _ := http.NewRequest("GET", "/test", nil)
	response := httptest.NewRecorder()
	Router().ServeHTTP(response, request)
	router := mux.NewRouter()
	router.HandleFunc("/test", getLocalTest).Methods("GET")
	assert.Equal(t, 70, response.Body.Len(), "70 len expected")
	assert.Equal(t, 200, response.Code, "200 response expected")
	decodedBody, _ := ioutil.ReadAll(response.Body) // after this the response.Body.Len()  turns to zero
	var c []Card
	_ = json.Unmarshal(decodedBody, &c)
	assert.Equal(t, "111px", c[0].Width, "body length is 81")
}
func TestIsUsingTestDatabase(t *testing.T) {
	dbServer, _ := connectDb(dbCfg)
	assert.Equal(t, "testDb", dbServer.DatabaseName, "Database is testDb")
}
func TestGetAllCardsEmpty(t *testing.T) {
	request, _ := http.NewRequest("GET", "/cards", nil)
	response := httptest.NewRecorder()
	Router().ServeHTTP(response, request)
	router := mux.NewRouter()
	router.HandleFunc("/cards", func(w http.ResponseWriter, r *http.Request) { getAllCards(w, r, db.Database) }).Methods("GET")
	assert.Equal(t, http.StatusNotFound, response.Code, "http.StatusOK response expected")
}
func TestAddCardIncompleteDataSent(t *testing.T) {
	var jsonStr = []byte(`{"Height":"333px"}`)
	request, _ := http.NewRequest("POST", "/cards", bytes.NewBuffer(jsonStr))
	response := httptest.NewRecorder()
	Router().ServeHTTP(response, request)
	router := mux.NewRouter()
	router.HandleFunc("/cards", func(w http.ResponseWriter, r *http.Request) { addCard(w, r, db.Database) }).Methods("POST")
	assert.Equal(t, http.StatusBadRequest, response.Code, "http.StatusBadRequest response expected")
	decodedBody, _ := ioutil.ReadAll(response.Body) // after this the response.Body.Len()  turns to zero
	type InsertResult struct{ InsertedID string }
	var result = InsertResult{}
	_ = json.Unmarshal(decodedBody, &result)
	assert.Equal(t, 0, len(result.InsertedID), "Return ObjectId must be of length 0")
}
func TestAddCardGoodDataSent(t *testing.T) {
	var jsonStr = []byte(`{"Name":"Test card name goes here","Width":"111px","Height":"222px"}`)
	request, _ := http.NewRequest("POST", "/cards", bytes.NewBuffer(jsonStr))
	response := httptest.NewRecorder()
	Router().ServeHTTP(response, request)
	router := mux.NewRouter()
	router.HandleFunc("/cards", func(w http.ResponseWriter, r *http.Request) { addCard(w, r, db.Database) }).Methods("POST")
	assert.Equal(t, http.StatusOK, response.Code, "http.StatusOK response expected")
	decodedBody, _ := ioutil.ReadAll(response.Body) // after this the response.Body.Len()  turns to zero
	type InsertResult struct{ InsertedID string }
	var result = InsertResult{}
	_ = json.Unmarshal(decodedBody, &result)
	newId = result.InsertedID
	log.Println("TEST: newId:", newId)
	assert.Equal(t, 24, len(newId), "Return ObjectId must be of length 24")
}
func TestUpdateCardValues(t *testing.T) {
	var jsonStr = []byte(`{"Width":"777px","Height":"888px"}`)
	log.Println("TEST: url:", "/cards/"+newId)
	request, _ := http.NewRequest("PUT", "/cards/"+newId, bytes.NewBuffer(jsonStr))
	response := httptest.NewRecorder()
	Router().ServeHTTP(response, request)
	router := mux.NewRouter()
	router.HandleFunc("/cards/{id}", func(w http.ResponseWriter, r *http.Request) { updateCard(w, r, db.Database) }).Methods("PUT")
	assert.Equal(t, http.StatusOK, response.Code, "http.StatusOK response expected")
	decodedBody, _ := ioutil.ReadAll(response.Body) // after this the response.Body.Len()  turns to zero
	type InsertResult struct{ InsertedID string }
	var result = InsertResult{}
	_ = json.Unmarshal(decodedBody, &result)
}
func TestGetOneCardOne(t *testing.T) {
	request, _ := http.NewRequest("GET", "/cards/"+newId, nil)
	response := httptest.NewRecorder()
	Router().ServeHTTP(response, request)
	router := mux.NewRouter()
	router.HandleFunc("/cards/{id}", func(w http.ResponseWriter, r *http.Request) { getOneCard(w, r, db.Database) }).Methods("GET")
	assert.Equal(t, http.StatusOK, response.Code, "http.StatusOK response expected")
	assert.Greater(t, response.Body.Len(), 50, "Response must be at least 50 bytes")
}
func TestGetAllCardsOne(t *testing.T) {
	request, _ := http.NewRequest("GET", "/cards", nil)
	response := httptest.NewRecorder()
	Router().ServeHTTP(response, request)
	router := mux.NewRouter()
	router.HandleFunc("/cards", func(w http.ResponseWriter, r *http.Request) { getAllCards(w, r, db.Database) }).Methods("GET")
	assert.Equal(t, http.StatusOK, response.Code, "http.StatusOK response expected")
	decodedBody, _ := ioutil.ReadAll(response.Body) // after this the response.Body.Len()  turns to zero
	var c []Card
	_ = json.Unmarshal(decodedBody, &c) // only works if JSON is wrapped in []
	assert.Equal(t, 1, len(c), "1 row in response expected")
}

func TestGetIndex(t *testing.T) {
	request, _ := http.NewRequest("GET", "/template/"+newId, nil)
	response := httptest.NewRecorder()
	Router().ServeHTTP(response, request)
	router := mux.NewRouter()
	router.HandleFunc("/template/{id}", getLocalTest).Methods("GET")
	assert.Equal(t, 200, response.Code, "200 response expected")
}

func TestDeleteCardFailBadId(t *testing.T) {
	var uri = "/cards/123123123"
	log.Println("TEST: uri:", uri)
	request, _ := http.NewRequest("DELETE", uri, nil)
	response := httptest.NewRecorder()
	Router().ServeHTTP(response, request)
	router := mux.NewRouter()
	router.HandleFunc("/cards/{id}", func(w http.ResponseWriter, r *http.Request) { deleteCard(w, r, db.Database) }).Methods("DELETE")
	assert.Equal(t, http.StatusBadRequest, response.Code, "http.StatusBadRequest response expected")
}
func TestDeleteCardSuccessfully(t *testing.T) {
	var uri = "/cards/" + newId
	log.Println("TEST: uri:", uri)
	request, _ := http.NewRequest("DELETE", uri, nil)
	response := httptest.NewRecorder()
	Router().ServeHTTP(response, request)
	router := mux.NewRouter()
	router.HandleFunc("/cards/{id}", func(w http.ResponseWriter, r *http.Request) { deleteCard(w, r, db.Database) }).Methods("DELETE")
	assert.Equal(t, http.StatusOK, response.Code, "http.StatusOK response expected")
}
func TestGetOneCardNone(t *testing.T) {
	request, _ := http.NewRequest("GET", "/cards/"+newId, nil)
	response := httptest.NewRecorder()
	Router().ServeHTTP(response, request)
	router := mux.NewRouter()
	router.HandleFunc("/cards/{id}", func(w http.ResponseWriter, r *http.Request) { getOneCard(w, r, db.Database) }).Methods("GET")
	assert.Equal(t, http.StatusNotFound, response.Code, "http.StatusNotFound response expected")
	assert.Greater(t, response.Body.Len(), 20, "Response must be at least 20 bytes")
}
func TestGetAllCardsNone(t *testing.T) {
	request, _ := http.NewRequest("GET", "/cards", nil)
	response := httptest.NewRecorder()
	Router().ServeHTTP(response, request)
	router := mux.NewRouter()
	router.HandleFunc("/cards", func(w http.ResponseWriter, r *http.Request) { getAllCards(w, r, db.Database) }).Methods("GET")
	assert.Equal(t, http.StatusNotFound, response.Code, "http.StatusNotFound response expected")
	assert.Greater(t, response.Body.Len(), 20, "Response must be at least 20 bytes")
}
