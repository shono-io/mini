# Mini
A very thin layer around NATS micro to facilitate writing micro services.

## Features
- [x] Read service configuration from a nats KV store
- [x] Expose a service with endpoints
- [x] Run a worker in the background

## How it works
A service manages a worker which can perform background tasks. The service itself can have endpoints associated 
with it and exposes itself through the NATS service.

Once a service is created it can be ran by calling the `run` method. This will make the service listen for configuration
changes and update itself accordingly. In fact, when a configuration change is detected, the worker will be notified at 
which point the worker can update itself. If no background tasks are to be executed, an IdleWorker can be used.

## Getting Started
To get started, create a service by at least providing a path to where the service configuration is stored. This path
is the key which will be watched for changes.

```go
package main

import (
  "context"
  "github.com/nats-io/nats.go/micro"
  "github.com/shono-io/mini"
)

func main() {
  service, err := mini.NewService(mini.WithPath("path/to/config"))
  if err != nil {
    panic(err)
  }

  grp := service.AddGroup("my_group")
  err = grp.AddEndpoint("hello",
    micro.HandlerFunc(func(req micro.Request) {
      req.Respond([]byte("Hello World"))
    }),
    micro.WithEndpointMetadata(map[string]string{
      "description": "A simple hello world endpoint",
      "format":      "text/plain",
    }))
  if err != nil {
    panic(err)
  }

  if err := service.Run(context.Background(), mini.NewIdleWorker()); err != nil {
    panic(err)
  }
}
```