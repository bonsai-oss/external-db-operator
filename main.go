package main

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"k8s.io/client-go/rest"
	"os"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/dynamic"
	"k8s.io/client-go/tools/clientcmd"
)

type Database struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec DatabaseSpec `json:"spec,omitempty"`
}

type DatabaseSpec struct {
	Database string `json:"database,omitempty"`
}

func main() {
	var clientConfig *rest.Config
	var clientConfigError error
	if os.Getenv("KUBECONFIG") == "" {
		clientConfig, clientConfigError = rest.InClusterConfig()
	} else {
		clientConfig, clientConfigError = clientcmd.BuildConfigFromFlags("", os.Getenv("KUBECONFIG"))
	}
	if clientConfigError != nil {
		panic(clientConfigError.Error())
	}

	client, err := dynamic.NewForConfig(clientConfig)
	if err != nil {
		panic(err.Error())
	}

	watcher, watchInitError := client.Resource(schema.GroupVersionResource(metav1.GroupVersionResource{
		Group:    "fsrv.cloud",
		Version:  "v1",
		Resource: "databases",
	})).Namespace("").Watch(context.Background(), metav1.ListOptions{
		Watch: true,
	})

	if watchInitError != nil {
		panic(watchInitError)
	}

	for {
		select {
		case event := <-watcher.ResultChan():
			fmt.Printf("Event: %#v\n", event.Type)

			buf := bytes.NewBuffer(nil)
			database := &Database{}
			json.NewEncoder(buf).Encode(event.Object)
			json.NewDecoder(buf).Decode(database)

			// Now you can access the spec of the database
			fmt.Print(database.Namespace)
			fmt.Print(" ")
			fmt.Print(database.Name)
			fmt.Print(" ")
			fmt.Printf("Database Spec: %#v\n", database.Spec)
		}
	}
}
