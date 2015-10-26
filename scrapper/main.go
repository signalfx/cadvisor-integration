package main

import (
	"fmt"
	"os"
	"time"
	"strings"

	"net/http"
	"net/url"

	"log"
	
	"runtime"
	"encoding/json"

	"golang.org/x/net/context"

	"prometheustosignalfx/scrapper_lib"
	"github.com/signalfx/metricproxy/protocol/signalfx"
	
	"github.com/codegangsta/cli"
)

type Config struct {
	IngestUrl string
	CadvisorUrl string
	ApiToken string
	DataSendRate string
	ClusterName string
}

type prometheusScraper struct {
	forwarder *signalfx.Forwarder
	cfg *Config
}

const ingestUrl = "ingestURL"
const apiToken = "apiToken"
const cadvisorURL = "cadvisorURL"
const dataSendRate = "sendRate"
const clusterName = "clusterName"

var dataSendRates = map[string]time.Duration {
	"1s": time.Second,
	"5s": 5 * time.Second,
	"10s": 10 * time.Second,
	"30s": 30 * time.Second,
	"1m": time.Minute,
	"5m": 5 * time.Minute,
	"1h": time.Hour,
}

func main() {
	app := cli.NewApp()
	app.Name = "scrapper"
  	app.Usage = "scraps metrics from cAdvisor and forwards them to SignalFx."
	
	app.Flags = []cli.Flag {
		cli.StringFlag{
		    Name: ingestUrl,
		    Value: "https://ingest.signalfx.com",
			Usage: "SignalFx ingest URL.",
		},
		cli.StringFlag{
		    Name: apiToken,
		    Usage: "API token.",
			EnvVar: "SFX_SCRAPPER_API_TOKEN",
		},
		cli.StringFlag{
		    Name: cadvisorURL,
		    Usage: "cAdvisor URL.",
			EnvVar: "SFX_SCRAPPER_CADVISOR_URL",
		},
		cli.StringFlag{
		    Name: clusterName,
		    Usage: "Cluster name will appear as dimension.",
			EnvVar: "SFX_SCRAPPER_CLUSTER_NAME",
		},
		cli.StringFlag{
		    Name: dataSendRate,
			Value: "1s",
		    Usage: fmt.Sprintf("Rate at which data is queried from cAdvisor and send to SignalFx. Possible values: %v", getMapKeys(dataSendRates)),
		},
	}
	
	app.Action = func(c *cli.Context) {
				
		var paramApiToken = c.String(apiToken)
		if paramApiToken == "" {
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
		
		var paramCadvisorURL = c.String(cadvisorURL)
		if paramCadvisorURL == "" {
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
		
		var paramIngestUrl = c.String(ingestUrl)
		if paramIngestUrl == "" {
			fmt.Fprintf(os.Stderr, "\nERROR: ingestUrl must be set.\n\n")
			cli.ShowAppHelp(c)
			os.Exit(1)
		}
		
		var instance = prometheusScraper{
			forwarder: newSfxClient(paramIngestUrl, paramApiToken),//"PjzqXDrnlfCn2h1ClAvVig"
			cfg: &Config{
				IngestUrl: paramIngestUrl,
				CadvisorUrl: paramCadvisorURL,
				ApiToken: paramApiToken,
				DataSendRate: c.String(dataSendRate),
				ClusterName: paramClusterName,
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

func newSfxClient(ingestUrl, authToken string) (forwarder *signalfx.Forwarder) {
	forwarder = signalfx.NewSignalfxJSONForwarder(strings.Join([]string{ingestUrl, "v2/datapoint"}, "/"), time.Second*10, authToken, 10, "", "", "")//http://lab-ingest.corp.signalfuse.com:8080
	forwarder.UserAgent(fmt.Sprintf("SignalFxScrapper/1.0 (gover %s)", runtime.Version()))
	return 
}

func (p *prometheusScraper) main(paramDataSendRate time.Duration) error {

	scrapper := prometheustosignalfx.Scrapper{
		Client: http.DefaultClient,
		L:      log.New(os.Stdout, "", 0),
	}

	ctx := context.Background()
	serverUrl, err := url.Parse(p.cfg.CadvisorUrl)//"http://192.168.99.100:8080/metrics"
	if err != nil {
		return err
	}
	
	cfg, _ := json.MarshalIndent(p.cfg, "", "  ")
	fmt.Printf("Scrapper started with following params:\n%v\n", string(cfg))
	
	stop := make(chan error, 1)
    ticker := time.NewTicker(paramDataSendRate)
    go func() {
        for _ = range ticker.C {
            points, err := scrapper.Fetch(ctx, serverUrl, p.cfg.ClusterName)
			if err != nil {
				stop <- err
				return
			}
			
			p.forwarder.AddDatapoints(ctx, points)			
        }
    }()
    err = <-stop
	 
    ticker.Stop()
	
	/*for {
		points, err := scrapper.Fetch(ctx, serverUrl)
		if err != nil {
			return err
		}
		
		p.forwarder.AddDatapoints(ctx, points)
	}*/
	
	/*for _, p := range points {
		b, _ := json.Marshal(p)
		fmt.Printf("%s\n", b)
	}*/
	return err
}
