// Copyright 2016, Ales Stimec.

package handler

import (
	"encoding/json"
	"fmt"
	"io/ioutil"
	"net/http"
	"net/url"
	"time"

	"github.com/gorilla/mux"
	"github.com/juju/errors"
	"github.com/juju/utils"
	"gopkg.in/macaroon-bakery.v1/bakery"
	"gopkg.in/macaroon-bakery.v1/bakery/checkers"
	"gopkg.in/macaroon-bakery.v1/httpbakery"
	"gopkg.in/macaroon.v1"
	"gopkg.in/yaml.v2"
)

const (
	usernamePath   = "username"
	usernameCaveat = "username"
	groupsFile     = "groups.yaml"
)

// HandlerConfig contains configuration for the handler.
type HandlerConfig struct {
	// Bakery is the macaroon bakery to be used by the handler.
	Bakery *bakery.Service
	// UserServiceLocation is the location of the user service.
	UserServiceLocation string
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

// discharge discharges third party caveats addressed to this service.
func (h *handler) discharge(w http.ResponseWriter, req *http.Request) {
	username, err := h.checkUser(w, req)
	if err != nil {
		return
	}

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
				return h.checkThirdPartyCaveat(username, cavId, cav)
			}),
		id)
	if err != nil {
		e := struct {
			Message string
			Code    string
		}{
			Message: err.Error(),
			Code:    "unauthorized",
		}
		writeResponse(w,
			http.StatusUnauthorized,
			e,
		)
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
func (h *handler) checkThirdPartyCaveat(username, cavId, cav string) ([]checkers.Caveat, error) {
	cond, _, err := checkers.ParseCaveat(cav)
	if err != nil {
		return nil, errors.Trace(err)
	}

	groups, err := readGroups()
	if err != nil {
		return nil, errors.Trace(err)
	}

	switch cond {
	case "is-student":
		students, ok := groups["student"]
		if !ok {
			return nil, errors.New("student group not found")
		}
		for _, student := range students {
			if student == username {
				return []checkers.Caveat{
					checkers.DeclaredCaveat("student-id", utils.MustNewUUID().String()),
					checkers.TimeBeforeCaveat(time.Now().Add(5 * time.Minute)),
				}, nil
			}
		}
		return nil, errors.New("not a student")
	case "is-professor":
		students, ok := groups["professor"]
		if !ok {
			return nil, errors.New("professor group not found")
		}
		for _, student := range students {
			if student == username {
				return []checkers.Caveat{
					checkers.DeclaredCaveat("professor-id", utils.MustNewUUID().String()),
					checkers.TimeBeforeCaveat(time.Now().Add(5 * time.Minute)),
				}, nil
			}
		}
		return nil, errors.New("not a professor")
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

// readGroups reads groups from groups.yaml.
func readGroups() (map[string][]string, error) {
	data, err := ioutil.ReadFile(groupsFile)
	if err != nil {
		return nil, err
	}

	var groupData struct {
		Groups map[string][]string `yaml:"groups"`
	}
	err = yaml.Unmarshal(data, &groupData)
	if err != nil {
		return nil, err
	}
	return groupData.Groups, nil
}

func (h *handler) checkUser(w http.ResponseWriter, req *http.Request) (string, error) {
	fail := ""
	// we check the presented macaroons
	// attrs is a map of all facts declared in presented macaroons
	attrs, verr := httpbakery.CheckRequest(h.config.Bakery, req, nil, checkers.TimeBefore)
	if verr == nil {
		username, ok := attrs[usernameCaveat]
		if !ok {
			writeResponse(w, http.StatusForbidden, "forbidden")
			return fail, errors.New("username not declared")
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
		checkers.NeedDeclaredCaveat(checkers.Caveat{h.config.UserServiceLocation, "is-user"}, usernameCaveat),
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
