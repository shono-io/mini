package mini

import "context"

type Worker interface {
	Init(service *Service) error
	Load(ctx context.Context, configBytes []byte) error
	Close()
}
