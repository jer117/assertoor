package coordinator

import (
	"context"
	"fmt"
	"net/http"
	"time"

	"github.com/ethpandaops/minccino/pkg/coordinator/clients"
	"github.com/ethpandaops/minccino/pkg/coordinator/test"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	"github.com/sirupsen/logrus"
)

type Coordinator struct {
	// Config is the coordinator configuration.
	Config          *Config
	log             logrus.FieldLogger
	metricsPort     int
	lameDuckSeconds int
}

func NewCoordinator(config *Config, log logrus.FieldLogger, metricsPort, lameDuckSeconds int) *Coordinator {
	return &Coordinator{
		log:             log,
		Config:          config,
		metricsPort:     metricsPort,
		lameDuckSeconds: lameDuckSeconds,
	}
}

// Run executes the coordinator until completion.
func (c *Coordinator) Run(ctx context.Context) error {
	c.log.
		WithField("metrics_port", c.metricsPort).
		WithField("lame_duck_seconds", c.lameDuckSeconds).
		Info("starting coordinator")

	// init client pool
	clientPool, err := clients.NewClientPool()
	if err != nil {
		return err
	}
	for idx := range c.Config.Endpoints {
		err = clientPool.AddClient(&c.Config.Endpoints[idx])
		if err != nil {
			return err
		}
	}

	// run test
	testToRun, err := test.CreateRunnable(ctx, c.log, clientPool, c.Config.Test)
	if err != nil {
		return err
	}

	if err := testToRun.Validate(); err != nil {
		return err
	}

	c.log.Info(fmt.Sprintf("starting test '%s'", testToRun.Name()))

	//nolint:errcheck // ignore
	go c.startMetrics()

	if err := testToRun.Run(ctx); err != nil {
		return err
	}

	c.log.WithField("test", c.Config.Test).Info("test completed!")

	c.log.WithField("seconds", c.lameDuckSeconds).Info("Initiating lame duck")

	time.Sleep(time.Duration(c.lameDuckSeconds) * time.Second)

	c.log.Info("lame duck complete")
	c.log.Info("Shutting down..")

	return nil
}

func (c *Coordinator) startMetrics() error {
	c.log.
		Info(fmt.Sprintf("Starting metrics server on :%v", c.metricsPort))

	http.Handle("/metrics", promhttp.Handler())

	err := http.ListenAndServe(fmt.Sprintf(":%v", c.metricsPort), nil)

	return err
}
