package main

import (
	"fmt"
	"os"
	"strings"
	"time"

	"net/http"
	"net/url"

	"log"

	"encoding/json"
	"runtime"

	"golang.org/x/net/context"

	"github.com/signalfx/metricproxy/protocol/signalfx"
	"github.com/signalfx/prometheustosignalfx/scrapper"

	"github.com/codegangsta/cli"
	"github.com/goinggo/workpool"
)

// Config for prometheusScraper
type Config struct {
	IngestURL    string
	CadvisorURL  []string
	APIToken     string
	DataSendRate string
	ClusterName  string
}

type prometheusScraper struct {
	forwarder *signalfx.Forwarder
	cfg       *Config
}

type scrapWork struct {
	ctx         context.Context
	scrapperSFX *scrapper.Scrapper
	serverURL   *url.URL
	clusterName string
	stop        chan error
	forwarder   *signalfx.Forwarder
}

func (scrapWork *scrapWork) DoWork(workRoutine int) {
	points, err := scrapWork.scrapperSFX.Fetch(scrapWork.ctx, scrapWork.serverURL, scrapWork.clusterName)
	if err != nil {
		scrapWork.stop <- err
		return
	}

	scrapWork.forwarder.AddDatapoints(scrapWork.ctx, points)
}

const ingestURL = "ingestURL"
const apiToken = "apiToken"
const cadvisorURL = "cadvisorURL"
const dataSendRate = "sendRate"
const clusterName = "clusterName"

var dataSendRates = map[string]time.Duration{
	"1s":  time.Second,
	"5s":  5 * time.Second,
	"10s": 10 * time.Second,
	"30s": 30 * time.Second,
	"1m":  time.Minute,
	"5m":  5 * time.Minute,
	"1h":  time.Hour,
}

func main() {
	runtime.GOMAXPROCS(runtime.NumCPU())

	app := cli.NewApp()
	app.Name = "prometheustosfx"
	app.Usage = "scraps metrics from cAdvisor and forwards them to SignalFx."

	app.Flags = []cli.Flag{
		cli.StringFlag{
			Name:  ingestURL,
			Value: "https://ingest.signalfx.com",
			Usage: "SignalFx ingest URL.",
		},
		cli.StringFlag{
			Name:   apiToken,
			Usage:  "API token.",
			EnvVar: "SFX_SCRAPPER_API_TOKEN",
		},
		cli.StringSliceFlag{
			Name:   cadvisorURL,
			Usage:  "cAdvisor URLs. Env. Var. example: SFX_SCRAPPER_CADVISOR_URL=<addr#1>,<arrd#2>,...",
			EnvVar: "SFX_SCRAPPER_CADVISOR_URL",
		},
		cli.StringFlag{
			Name:   clusterName,
			Usage:  "Cluster name will appear as dimension.",
			EnvVar: "SFX_SCRAPPER_CLUSTER_NAME",
		},
		cli.StringFlag{
			Name:  dataSendRate,
			Value: "1s",
			Usage: fmt.Sprintf("Rate at which data is queried from cAdvisor and send to SignalFx. Possible values: %v", getMapKeys(dataSendRates)),
		},
	}

	app.Action = func(c *cli.Context) {

		var paramAPIToken = c.String(apiToken)
		if paramAPIToken == "" {
			fmt.Fprintf(os.Stderr, "\nERROR: apiToken must be set.\n\n")
			cli.ShowAppHelp(c)
			os.Exit(1)
		}

		paramDataSendRate, ok := dataSendRates[c.String(dataSendRate)]
		if !ok {
			fmt.Fprintf(os.Stderr, "\nERROR: dataSendRate must be one of: %v.\n\n", getMapKeys(dataSendRates))
			cli.ShowAppHelp(c)
			os.Exit(1)
		}

		var paramCadvisorURL = c.StringSlice(cadvisorURL)
		if paramCadvisorURL == nil || len(paramCadvisorURL) == 0 {
			fmt.Fprintf(os.Stderr, "\nERROR: cadvisorURL must be set.\n\n")
			cli.ShowAppHelp(c)
			os.Exit(1)
		}

		var paramClusterName = c.String(clusterName)
		if paramClusterName == "" {
			fmt.Fprintf(os.Stderr, "\nERROR: clusterName must be set.\n\n")
			cli.ShowAppHelp(c)
			os.Exit(1)
		}

		var paramIngestURL = c.String(ingestURL)
		if paramIngestURL == "" {
			fmt.Fprintf(os.Stderr, "\nERROR: ingestUrl must be set.\n\n")
			cli.ShowAppHelp(c)
			os.Exit(1)
		}

		var instance = prometheusScraper{
			forwarder: newSfxClient(paramIngestURL, paramAPIToken), //"PjzqXDrnlfCn2h1ClAvVig"
			cfg: &Config{
				IngestURL:    paramIngestURL,
				CadvisorURL:  paramCadvisorURL,
				APIToken:     paramAPIToken,
				DataSendRate: c.String(dataSendRate),
				ClusterName:  paramClusterName,
			},
		}

		if err := instance.main(paramDataSendRate); err != nil {
			fmt.Fprintf(os.Stderr, "%s\n", err.Error())
			os.Exit(1)
		}
	}

	app.Run(os.Args)
}

func getMapKeys(m map[string]time.Duration) (keys []string) {
	for k := range m {
		keys = append(keys, k)
	}
	return keys
}

func newSfxClient(ingestURL, authToken string) (forwarder *signalfx.Forwarder) {
	forwarder = signalfx.NewSignalfxJSONForwarder(strings.Join([]string{ingestURL, "v2/datapoint"}, "/"), time.Second*10, authToken, 10, "", "", "") //http://lab-ingest.corp.signalfuse.com:8080
	forwarder.UserAgent(fmt.Sprintf("SignalFxScrapper/1.0 (gover %s)", runtime.Version()))
	return
}

func (p *prometheusScraper) main(paramDataSendRate time.Duration) (err error) {

	scrapperSFX := scrapper.Scrapper{
		Client: http.DefaultClient,
		L:      log.New(os.Stdout, "", 0),
	}

	ctx := context.Background()
	cadvisorServers := make([]*url.URL, len(p.cfg.CadvisorURL))
	for i, serverURL := range p.cfg.CadvisorURL {
		cadvisorServers[i], err = url.Parse(serverURL)
		if err != nil {
			return err
		}
	}

	cfg, _ := json.MarshalIndent(p.cfg, "", "  ")
	fmt.Printf("Scrapper started with following params:\n%v\n", string(cfg))

	workPool := workpool.New(runtime.NumCPU(), int32(len(p.cfg.CadvisorURL)+1))

	stop := make(chan error, 1)
	ticker := time.NewTicker(paramDataSendRate)
	go func() {
		for range ticker.C {
			for _, serverURL := range cadvisorServers {
				workPool.PostWork("", &scrapWork{
					ctx:         ctx,
					scrapperSFX: &scrapperSFX,
					serverURL:   serverURL,
					clusterName: p.cfg.ClusterName,
					stop:        stop,
					forwarder:   p.forwarder,
				})
			}
		}
	}()
	err = <-stop

	ticker.Stop()

	return err
}
