package mini

import (
	"context"
	"fmt"
	"github.com/rs/zerolog"
	"github.com/spf13/cobra"
	"github.com/spf13/viper"

	"github.com/nats-io/nats.go"
	"github.com/nats-io/nats.go/jetstream"
	"github.com/nats-io/nats.go/micro"
	"github.com/rs/zerolog/log"
)

type EndpointInitializer func(s *Service) error

func ConfigureCommand(cmd *cobra.Command) {
	cmd.PersistentFlags().String("env", "", "environment")
	cmd.PersistentFlags().String("id", "", "service id")
	cmd.PersistentFlags().String("name", "", "service name")
	cmd.PersistentFlags().String("description", "", "service description")
	cmd.PersistentFlags().String("version", "", "service version")

	cmd.PersistentFlags().String("shono-account", "", "The shono account identifier")
	cmd.PersistentFlags().String("shono-jwt", "", "The shono JWT Token")
	cmd.PersistentFlags().String("shono-seed", "", "The shono Seed")
	cmd.PersistentFlags().String("shono-url", "tls://connect.ngs.global", "The URL of the shono nats server")
	cmd.PersistentFlags().String("loglevel", "INFO", "The log level for the service")
}

func FromViper(viper *viper.Viper, opts ...Option) (*Service, error) {
	env := viper.GetString("env")
	if env == "" {
		return nil, fmt.Errorf("env is required")
	}

	id := viper.GetString("id")
	if id == "" {
		return nil, fmt.Errorf("id is required")
	}

	// -- required
	account := viper.GetString("shono-account")
	if account == "" {
		return nil, fmt.Errorf("shono_account is required")
	}
	opts = append(opts, WithAccount(account))

	jwt := viper.GetString("shono-jwt")
	seed := viper.GetString("shono-seed")
	if jwt == "" && seed == "" {
		opts = append(opts, WithCredentials(jwt, seed))
	}

	// -- optionals
	if natsUrl := viper.GetString("shono-url"); natsUrl != "" {
		opts = append(opts, WithNatsUrl(natsUrl))
	}

	if name := viper.GetString("name"); name != "" {
		opts = append(opts, WithName(name))
	}

	if description := viper.GetString("description"); description != "" {
		opts = append(opts, WithDescription(description))
	}

	if version := viper.GetString("version"); version != "" {
		opts = append(opts, WithVersion(version))
	}

	loglevel, err := zerolog.ParseLevel(viper.GetString("loglevel"))
	if err != nil {
		log.Warn().Err(err).Msg("failed to parse log level, reverting to INFO")
		loglevel = zerolog.InfoLevel
	}
	zerolog.SetGlobalLevel(loglevel)

	return NewService(env, id, opts...)
}

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

	// -- create the logger for the service
	lvl, err := zerolog.ParseLevel(options.LogLevel)
	if err != nil {
		log.Warn().Msgf("failed to parse log level, reverting to INFO")
		lvl = zerolog.InfoLevel
	}

	logger := log.Level(lvl).With().Str("service", id).Logger()

	nc, err := nats.Connect(options.NatsUrl, options.NatsOptions...)
	if err != nil {
		return nil, fmt.Errorf("failed to connect to nats: %w", err)
	}

	js, err := jetstream.New(nc)
	if err != nil {
		return nil, fmt.Errorf("failed to create jetstream context: %w", err)
	}

	scfg := micro.Config{
		Name:        options.Name,
		Description: options.Description,
		Version:     options.Version,
		DoneHandler: func(srv micro.Service) {
			logger.Info().Msg("service stopped")
		},
		ErrorHandler: func(srv micro.Service, err *micro.NATSError) {
			logger.Info().Str("subject", err.Subject).Err(err).Msg("service error")
		},
	}

	srv, err := micro.AddService(nc, scfg)
	if err != nil {
		return nil, err
	}

	result := &Service{
		env:         env,
		id:          id,
		account:     options.Account,
		micro:       srv,
		nc:          nc,
		js:          js,
		Log:         &logger,
		watchConfig: options.ConfigWatched,
		done:        make(chan struct{}),
	}

	return result, nil
}

type Service struct {
	env     string
	id      string
	account string

	micro micro.Service
	nc    *nats.Conn
	js    jetstream.JetStream
	Log   *zerolog.Logger

	watchConfig bool

	done chan struct{}
}

func (s *Service) AddGroup(name string, opt ...micro.GroupOpt) micro.Group {
	return s.micro.AddGroup(name, opt...)
}

func (s *Service) AddEndpoint(name string, handler micro.Handler, opt ...micro.EndpointOpt) error {
	return s.micro.AddEndpoint(name, handler, opt...)
}

func (s *Service) Close() {
	close(s.done)
	s.nc.Close()
}

func (s *Service) InitEndpoints() error {
	return nil
}

func (s *Service) Run(ctx context.Context, worker Worker) error {
	defer worker.Close()
	s.Log.Info().Msg("Service starting")
	defer s.Log.Info().Msg("Service Stopped")

	var configChan <-chan jetstream.KeyValueEntry
	if s.watchConfig {
		configStore, err := s.js.KeyValue(context.Background(), "config")
		if err != nil {
			return fmt.Errorf("failed to get the jetstream config store: %w", err)
		}

		configFilter := fmt.Sprintf("client.%s.env.%s.service.%s.config", s.account, s.env, s.id)
		s.Log.Info().Msgf("watching config for configuration changes at %q", configFilter)
		configWatcher, err := configStore.Watch(context.Background(), configFilter)
		configChan = configWatcher.Updates()
	} else {
		configChan = make(chan jetstream.KeyValueEntry)
	}

	if err := worker.Init(s); err != nil {
		return fmt.Errorf("failed to initialize worker: %w", err)
	}

	s.Log.Info().Msg("Service started")
	for {
		select {
		case <-s.done:
			return nil
		case kve := <-configChan:
			if kve == nil {
				continue
			}

			if kve.Value() == nil || len(kve.Value()) == 0 {
				continue
			}

			s.Log.Info().Msgf("worker configuration updated: ")
			if err := worker.Load(ctx, kve.Value()); err != nil {
				s.Log.Error().Err(err).Msg("failed to load config into the worker")
			}
		}
	}

}

func (s *Service) JetStream() jetstream.JetStream {
	return s.js
}

func (s *Service) Nats() *nats.Conn {
	return s.nc
}

func (s *Service) Micro() micro.Service {
	return s.micro
}

func (s *Service) Account() string {
	return s.account
}
