// Copyright 2016, Ales Stimec.

package handler

import (
	"encoding/json"
	"fmt"
	"io/ioutil"
	"net/http"
	"net/url"

	"github.com/gorilla/mux"
	"gopkg.in/macaroon-bakery.v1/bakery"
	"gopkg.in/macaroon-bakery.v1/bakery/checkers"
	"gopkg.in/macaroon.v1"
	"gopkg.in/yaml.v2"
)

const (
	usernameCaveat = "username"
	usernameFile   = "user.yaml"
)

// HandlerConfig contains configuration for the handler.
type HandlerConfig struct {
	// Bakery is the macaroon bakery to be used by the handler.
	Bakery *bakery.Service
}

// NewHandler returns a new handler struct using the provided config.
func NewHandler(config HandlerConfig) *handler {
	return &handler{config: config}
}

type handler struct {
	config HandlerConfig
}

// RegisterHandlers registers all endpoints served by the handler.
func (h *handler) RegisterHandlers(r *mux.Router) {
	r.HandleFunc("/publickey", h.publicKey).Methods("GET")
	r.HandleFunc("/discharge", h.discharge).Methods("POST")
}

// publicKey returns the bakery service public key.
func (h *handler) publicKey(w http.ResponseWriter, req *http.Request) {
	response := struct {
		PublicKey *bakery.PublicKey
	}{
		PublicKey: h.config.Bakery.PublicKey(),
	}

	writeResponse(w, http.StatusOK, response)
}

// discharge discharges "is-user" third party caveats addressed to this service.
func (h *handler) discharge(w http.ResponseWriter, req *http.Request) {
	data, err := ioutil.ReadAll(req.Body)
	if err != nil {
		writeResponse(w, http.StatusBadRequest, "bad request")
		return
	}
	values, err := url.ParseQuery(string(data))
	if err != nil {
		writeResponse(w, http.StatusBadRequest, "bad request")
		return
	}
	id := values.Get("id")
	if id == "" {
		writeResponse(w, http.StatusBadRequest, "bad request")
		return
	}

	m, err := h.config.Bakery.Discharge(
		bakery.ThirdPartyCheckerFunc(
			func(cavId, cav string) ([]checkers.Caveat, error) {
				return h.checkThirdPartyCaveat(req, cavId, cav)
			}),
		id)
	if err != nil {
		writeResponse(w, http.StatusInternalServerError, "internal server error")
		return
	}

	response := struct {
		Macaroon *macaroon.Macaroon
	}{
		Macaroon: m,
	}
	writeResponse(w, http.StatusOK, response)
}

// checkThirdPartyCaveat checks the the third party caveat and returns a declared caveat
// declaring the username.
func (h *handler) checkThirdPartyCaveat(req *http.Request, cavId, cav string) ([]checkers.Caveat, error) {
	cond, _, err := checkers.ParseCaveat(cav)
	if err != nil {
		return nil, err
	}
	// this service knows how to discharge 3rd party "is-user" caveats
	// addressed to it.
	switch cond {
	case "is-user":
		username := readUsername()
		// we are returning a declared caveat, which means that the "username" will
		// be returned to the target service when calling the httpbakery.CheckRequest method.
		return []checkers.Caveat{checkers.DeclaredCaveat(usernameCaveat, username)}, nil
	default:
		return nil, checkers.ErrCaveatNotRecognized
	}
}

// writeResponse writes the http response.
func writeResponse(w http.ResponseWriter, code int, object interface{}) {
	fmt.Println("writing response:", code, object)
	data, err := json.Marshal(object)
	if err != nil {
		panic(err)
	}
	w.Header().Set("content-type", "application/json")
	w.WriteHeader(code)
	w.Write(data)
}

// readUsername reds the username from user.yaml.
func readUsername() string {
	username := "bob"

	data, err := ioutil.ReadFile(usernameFile)
	if err != nil {
		return username
	}

	var userData struct {
		Username string `yaml:"username"`
	}
	err = yaml.Unmarshal(data, &userData)
	if err != nil {
		return username
	}
	return userData.Username
}
