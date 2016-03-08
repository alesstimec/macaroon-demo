// Copyright 2016, Ales Stimec.

package main

import (
	"fmt"
	"net/http"
	"os"

	"github.com/gorilla/mux"
	"gopkg.in/macaroon-bakery.v1/bakery"
	"gopkg.in/macaroon-bakery.v1/httpbakery"

	"github.com/alesstimec/macaroon-demo/target-service/handler"
)

const (
	userServiceLocation = "http://localhost:9080"
)

func returnError(err error) {
	fmt.Fprintf(os.Stderr, "%v", err)
	os.Exit(1)
}

func main() {
	keypair, err := bakery.GenerateKey()
	if err != nil {
		returnError(err)
	}
	fmt.Println("keypair created")

	userServicePublicKey, err := httpbakery.PublicKeyForLocation(&http.Client{}, userServiceLocation)
	if err != nil {
		returnError(err)
	}

	keyring := bakery.NewPublicKeyRing()
	err = keyring.AddPublicKeyForLocation(userServiceLocation, true, userServicePublicKey)
	if err != nil {
		returnError(err)
	}
	fmt.Println("keyring created")

	svc, err := bakery.NewService(bakery.NewServiceParams{
		Location: "target-service",
		Key:      keypair,
		Locator:  keyring,
	})
	if err != nil {
		returnError(err)
	}
	fmt.Println("bakery created")

	h := handler.NewHandler(handler.HandlerConfig{
		Bakery:              svc,
		UserServiceLocation: userServiceLocation,
	})
	fmt.Println("handler created")

	r := mux.NewRouter()
	h.RegisterHandlers(r)

	fmt.Println("listening...")
	http.ListenAndServe(":8080", r)
}
