// Copyright 2016, Ales Stimec.

package main

import (
	"fmt"
	"net/http"
	"os"

	"github.com/gorilla/mux"
	"gopkg.in/macaroon-bakery.v1/bakery"

	"github.com/alesstimec/macaroon-demo/user-service/handler"
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

	svc, err := bakery.NewService(bakery.NewServiceParams{
		Location: "user-service",
		Key:      keypair,
	})
	if err != nil {
		returnError(err)
	}
	fmt.Println("bakery created")

	h := handler.NewHandler(handler.HandlerConfig{
		Bakery: svc,
	})
	fmt.Println("handler created")

	r := mux.NewRouter()
	h.RegisterHandlers(r)

	fmt.Println("listening...")
	http.ListenAndServe(":9080", r)
}
