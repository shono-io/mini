package mini

import (
	"context"
	"encoding/json"
	"fmt"
	"github.com/nats-io/nats.go"
	"github.com/nats-io/nats.go/jetstream"
	"github.com/nats-io/nats.go/micro"
	"github.com/rs/zerolog"
	"github.com/rs/zerolog/log"
	"github.com/spf13/cobra"
	"github.com/spf13/viper"
	"net/http"
	"strings"
)

type EndpointInitializer func(s *Service) error

func ConfigureCommand(cmd *cobra.Command) {
	cmd.PersistentFlags().String("path", "", "service config path")

	cmd.PersistentFlags().String("shono-jwt", "", "The shono JWT Token")
	cmd.PersistentFlags().String("shono-seed", "", "The shono Seed")
	cmd.PersistentFlags().String("shono-creds-file", "", "The shono credentials file")
	cmd.PersistentFlags().String("shono-url", "tls://connect.ngs.global", "The URL of the shono nats server")
	cmd.PersistentFlags().String("loglevel", "INFO", "The log level for the service")
}

func FromViper(viper *viper.Viper, opts ...Option) (*Service, error) {
	path := viper.GetString("path")
	if path == "" {
		return nil, fmt.Errorf("path is required")
	}
	opts = append(opts, WithPath(path))

	// -- required
	jwt := viper.GetString("shono-jwt")
	seed := viper.GetString("shono-seed")
	credsFile := viper.GetString("shono-creds-file")
	if credsFile != "" {
		opts = append(opts, WithCredentialsFile(credsFile))
	} else if jwt != "" && seed != "" {
		opts = append(opts, WithCredentials(jwt, seed))
	}

	// -- optionals
	if natsUrl := viper.GetString("shono-url"); natsUrl != "" {
		opts = append(opts, WithNatsUrl(natsUrl))
	}

	opts = append(opts, WithLogLevel(viper.GetString("loglevel")))

	return NewService(opts...)
}

func NewService(opts ...Option) (*Service, error) {
	options := &Options{
		NatsUrl: "tls://connect.ngs.global",
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
	zerolog.SetGlobalLevel(lvl)

	nc, err := nats.Connect(options.NatsUrl, options.NatsOptions...)
	if err != nil {
		return nil, fmt.Errorf("failed to connect to nats: %w", err)
	}

	js, err := jetstream.New(nc)
	if err != nil {
		return nil, fmt.Errorf("failed to create jetstream context: %w", err)
	}

	meta, err := js.KeyValue(context.Background(), "meta")
	if err != nil {
		return nil, fmt.Errorf("failed to get the jetstream meta store: %w", err)
	}

	configKv, err := meta.Get(context.Background(), options.Path)
	if err != nil {
		return nil, fmt.Errorf("failed to get the service configuration: %w", err)
	}

	// -- parse the configuration
	var m Meta
	if err := json.Unmarshal(configKv.Value(), &m); err != nil {
		return nil, fmt.Errorf("failed to parse the service configuration: %w", err)
	}

	pp := strings.Split(options.Path, ".")
	id := pp[len(pp)-1]
	logger := log.With().Str("service", id).Logger()

	scfg := micro.Config{
		Name:        id,
		Description: m.Description,
		Version:     m.Version,
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
		Meta:        m,
		micro:       srv,
		nc:          nc,
		js:          js,
		kv:          meta,
		Path:        options.Path,
		Log:         &logger,
		watchConfig: options.ConfigWatched,
		done:        make(chan struct{}),
	}

	return result, nil
}

type Service struct {
	Meta
	Path string

	micro micro.Service
	nc    *nats.Conn
	js    jetstream.JetStream
	kv    jetstream.KeyValue
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
		s.Log.Info().Msgf("watching config for configuration changes at %q", s.Path)
		configWatcher, err := s.kv.Watch(context.Background(), s.Path)
		if err != nil {
			return fmt.Errorf("failed to watch for configuration changes: %w", err)
		}

		configChan = configWatcher.Updates()
	} else {
		configChan = make(chan jetstream.KeyValueEntry)
	}

	wcc := make(chan []byte)
	if err := worker.Init(s, wcc); err != nil {
		return fmt.Errorf("failed to initialize worker: %w", err)
	}

	go func() {
		if err := http.ListenAndServe(":8080", http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if r.Method != http.MethodGet {
				w.WriteHeader(http.StatusMethodNotAllowed)
				return
			}

			info := s.micro.Info()
			b, err := json.Marshal(info)
			if err != nil {
				w.WriteHeader(http.StatusInternalServerError)
				return
			}

			w.WriteHeader(http.StatusOK)
			w.Write(b)
		})); err != nil {
			s.Log.Error().Err(err).Msg("failed to start http server")
		}
	}()

	go func() {
		if err := worker.Run(ctx); err != nil {
			s.Log.Error().Err(err).Msg("failed to load config into the worker")
		}
	}()

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

			var newMeta Meta
			if err := json.Unmarshal(kve.Value(), &newMeta); err != nil {
				s.Log.Error().Err(err).Msg("failed to parse configuration; not updating worker")
				continue
			}

			s.Log.Info().Msgf("worker configuration updated")
			wcc <- []byte(newMeta.Config)
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
