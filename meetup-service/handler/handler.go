// Copyright 2016, Ales Stimec.

package handler

import (
	"encoding/json"
	"fmt"
	"net/http"
	"time"

	"github.com/gorilla/mux"
	"github.com/juju/errors"
	"gopkg.in/macaroon-bakery.v1/bakery"
	"gopkg.in/macaroon-bakery.v1/bakery/checkers"
	"gopkg.in/macaroon-bakery.v1/httpbakery"
)

const (
	usernamePath      = "username"
	usernameCaveat    = "username"
	studentIDCaveat   = "student-id"
	professorIDCaveat = "professor-id"
)

// HandlerConfig contains configuration for the handler.
type HandlerConfig struct {
	// Bakery is the macaroon bakery to be used by the handler.
	Bakery *bakery.Service
	// UserServiceLocation is the location of the user service.
	UserServiceLocation string
	// UniversityServiceLocation is the location of the university service.
	UniversityServiceLocation string
}

// NewHandler returns a new handler struct using the provided condig.
func NewHandler(config HandlerConfig) *handler {
	return &handler{config: config}
}

type handler struct {
	config HandlerConfig
}

// RegisterHandlers registers all endpoints served by the handler.
func (h *handler) RegisterHandlers(r *mux.Router) {
	r.HandleFunc("/{username}", h.helloUser).Methods("GET")
	r.HandleFunc("/professor/{username}", h.helloProfessor).Methods("GET")
}

func (h *handler) helloUser(w http.ResponseWriter, req *http.Request) {
	username, err := h.checkStudent(w, req)
	if err != nil {
		return
	}

	response := struct {
		Message string `json:"message"`
	}{
		Message: fmt.Sprintf("Hello %v", username),
	}
	writeResponse(w, http.StatusOK, response)
}

func (h *handler) helloProfessor(w http.ResponseWriter, req *http.Request) {
	username, err := h.checkProfessor(w, req)
	if err != nil {
		return
	}

	response := struct {
		Message string `json:"message"`
	}{
		Message: fmt.Sprintf("Hello %v", username),
	}
	writeResponse(w, http.StatusOK, response)
}

func (h *handler) checkStudent(w http.ResponseWriter, req *http.Request) (string, error) {
	fail := ""
	// we check the presented macaroons
	// attrs is a map of all facts declared in presented macaroons
	attrs, verr := httpbakery.CheckRequest(h.config.Bakery, req, nil, checkers.New(checkers.TimeBefore, checkers.OperationChecker("student")))
	if verr == nil {
		// get the username from the url path.
		pathVars := mux.Vars(req)
		username, ok := pathVars[usernamePath]
		if !ok {
			http.Error(w, "internal server error", http.StatusInternalServerError)
			return fail, errors.New("internal server error")
		}
		// compare the "path" username and the username declared in the macaroon.
		if username != attrs[usernameCaveat] {
			writeResponse(w, http.StatusForbidden, "forbidden")
			return fail, errors.New("username mismatch")
		}
		return username, nil
	}
	// if the macaroon fails validation return an error.
	if _, ok := errors.Cause(verr).(*bakery.VerificationError); !ok {
		writeResponse(w, http.StatusForbidden, "forbidden")
		return fail, errors.Trace(verr)
	}
	// mint a new macaroon
	m, err := h.config.Bakery.NewMacaroon("", nil, []checkers.Caveat{
		checkers.AllowCaveat("student"),
		checkers.DenyCaveat("professor"),
		checkers.NeedDeclaredCaveat(checkers.Caveat{h.config.UserServiceLocation, "is-user"}, usernameCaveat),
		checkers.NeedDeclaredCaveat(checkers.Caveat{h.config.UniversityServiceLocation, "is-student"}, studentIDCaveat),
		checkers.TimeBeforeCaveat(time.Now().Add(5 * time.Minute)),
	})
	if err != nil {
		writeResponse(w, http.StatusInternalServerError, err)
		return fail, errors.Annotate(err, "cannot mint a new macaroon")
	}
	// write the discharge required error in response.
	httpbakery.WriteDischargeRequiredErrorForRequest(w, m, "/", verr, req)
	return fail, errors.Trace(verr)
}

func (h *handler) checkProfessor(w http.ResponseWriter, req *http.Request) (string, error) {
	fail := ""
	// we check the presented macaroons
	// attrs is a map of all facts declared in presented macaroons
	attrs, verr := httpbakery.CheckRequest(h.config.Bakery, req, nil, checkers.New(checkers.TimeBefore, checkers.OperationChecker("professor")))
	if verr == nil {
		// get the username from the url path.
		pathVars := mux.Vars(req)
		username, ok := pathVars[usernamePath]
		if !ok {
			http.Error(w, "internal server error", http.StatusInternalServerError)
			return fail, errors.New("internal server error")
		}
		// compare the "path" username and the username declared in the macaroon.
		if username != attrs[usernameCaveat] {
			writeResponse(w, http.StatusForbidden, "forbidden")
			return fail, errors.New("username mismatch")
		}
		return username, nil
	}
	// if the macaroon fails validation return an error.
	if _, ok := errors.Cause(verr).(*bakery.VerificationError); !ok {
		writeResponse(w, http.StatusForbidden, "forbidden")
		return fail, errors.Trace(verr)
	}
	// mint a new macaroon
	m, err := h.config.Bakery.NewMacaroon("", nil, []checkers.Caveat{
		checkers.AllowCaveat("student", "professor"),
		checkers.NeedDeclaredCaveat(checkers.Caveat{h.config.UserServiceLocation, "is-user"}, usernameCaveat),
		checkers.NeedDeclaredCaveat(checkers.Caveat{h.config.UniversityServiceLocation, "is-professor"}, professorIDCaveat),
		checkers.TimeBeforeCaveat(time.Now().Add(5 * time.Minute)),
	})
	if err != nil {
		writeResponse(w, http.StatusInternalServerError, err)
		return fail, errors.Annotate(err, "cannot mint a new macaroon")
	}
	// write the discharge required error in response.
	httpbakery.WriteDischargeRequiredErrorForRequest(w, m, "/", verr, req)
	return fail, errors.Trace(verr)
}

func writeResponse(w http.ResponseWriter, code int, object interface{}) {
	data, err := json.Marshal(object)
	if err != nil {
		panic(err)
	}
	w.Header().Set("content-type", "application/json")
	w.WriteHeader(code)
	w.Write(data)
}
