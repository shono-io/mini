package mini

import (
	"context"
	"fmt"

	"github.com/nats-io/nats.go"
	"github.com/nats-io/nats.go/jetstream"
	"github.com/nats-io/nats.go/micro"
	"github.com/rs/zerolog/log"
)

type EndpointInitializer func(s *Service) error

func NewService(env string, id string, opts ...Option) (*Service, error) {
	options := &Options{
		NatsUrl:     "tls://connect.ngs.global",
		Name:        id,
		Description: fmt.Sprintf("service %q", id),
		Version:     "0.0.0",
	}

	for _, o := range opts {
		if err := o(options); err != nil {
			return nil, err
		}
	}

	nc, err := nats.Connect(options.NatsUrl, options.NatsOptions...)
	if err != nil {
		return nil, fmt.Errorf("failed to connect to nats: %w", err)
	}

	js, err := jetstream.New(nc)
	if err != nil {
		return nil, fmt.Errorf("failed to create jetstream context: %w", err)
	}

	configStore, err := js.KeyValue(context.Background(), "config")
	if err != nil {
		return nil, fmt.Errorf("failed to get the jetstream config store: %w", err)
	}

	configFilter := fmt.Sprintf("client.%s.env.%s.service.%s.config", options.Account, env, id)
	log.Info().Str("service", id).Msgf("watching config for configuration changes at %q", configFilter)
	configWatcher, err := configStore.Watch(context.Background(), configFilter)

	scfg := micro.Config{
		Name:        options.Name,
		Description: options.Description,
		Version:     options.Version,
		DoneHandler: func(srv micro.Service) {
			info := srv.Info()
			fmt.Printf("stopped service %q with ID %q\n", info.Name, info.ID)
		},
		ErrorHandler: func(srv micro.Service, err *micro.NATSError) {
			info := srv.Info()
			fmt.Printf("Service %q returned an error on subject %q: %s", info.Name, err.Subject, err.Description)
		},
	}

	srv, err := micro.AddService(nc, scfg)
	if err != nil {
		return nil, err
	}

	result := &Service{
		Micro:     srv,
		Nats:      nc,
		JetStream: js,
		cs:        configStore,
		cw:        configWatcher,
	}

	return result, nil
}

type Service struct {
	Micro     micro.Service
	Nats      *nats.Conn
	JetStream jetstream.JetStream

	cs jetstream.KeyValue
	cw jetstream.KeyWatcher
}

func (s *Service) Close() {
	s.Nats.Close()
}

func (s *Service) InitEndpoints() error {
	return nil
}

func (s *Service) Run(ctx context.Context, worker Worker) error {
	defer worker.Close()

	if err := worker.Init(s); err != nil {
		return fmt.Errorf("failed to initialize worker: %w", err)
	}

	for {
		select {
		case kve := <-s.cw.Updates():
			if kve == nil {
				continue
			}

			if kve.Value() == nil || len(kve.Value()) == 0 {
				continue
			}

			log.Info().Msgf("worker configuration updated: ")
			if err := worker.Load(ctx, kve.Value()); err != nil {
				log.Error().Err(err).Msg("failed to load config into the worker")
			}
		}
	}
}
