package mini

import "context"

type Worker interface {
	Init(service *Service, configChan <-chan []byte) error
	Run(ctx context.Context) error
	Close()
}

func NewIdleWorker() Worker {
	return idleWorker{}
}

type idleWorker struct{}

func (i idleWorker) Init(service *Service, configChan <-chan []byte) error { return nil }
func (i idleWorker) Run(ctx context.Context) error                         { return nil }
func (i idleWorker) Close()                                                {}
