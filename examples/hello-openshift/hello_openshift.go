package main

import (
	"fmt"
	"net/http"
	"os"
)

type greeter struct {
	message string
}

func newGreeter(message string) greeter {
	if message == "" {
		message = "Hello OpenShift!"
	}

	return greeter{message}
}

func (g greeter) helloHandler(w http.ResponseWriter, r *http.Request) {
	fmt.Fprintln(w, g.message)
}

func main() {
	greeter := newGreeter(os.Getenv("HELLO_OPENSHIFT_MESSAGE"))
	http.HandleFunc("/", greeter.helloHandler)

	fmt.Println("Started, serving at 8080")
	err := http.ListenAndServe(":8080", nil)
	if err != nil {
		panic("ListenAndServe: " + err.Error())
	}
}
