package mini

import "context"

type Worker interface {
	Init(service *Service) error
	Load(ctx context.Context, configBytes []byte) error
	Close()
}

func NewIdleWorker() Worker {
	return idleWorker{}
}

type idleWorker struct{}

func (i idleWorker) Init(service *Service) error                        { return nil }
func (i idleWorker) Load(ctx context.Context, configBytes []byte) error { return nil }
func (i idleWorker) Close()                                             {}
