package main

import (
	"flag"
	"fmt"
	"os"
	"time"

	"github.com/codegangsta/cli"
	"github.com/golang/glog"
	"github.com/signalfx/cadvisor-integration/poller"
)

const ingestURL = "ingestURL"
const apiToken = "apiToken"
const dataSendRate = "sendRate"
const nodeServiceDiscoveryRate = "nodeServiceDiscoveryRate"
const clusterName = "clusterName"
const cadvisorPort = "cadvisorPort"
const kubernetesURL = "kubernetesURL"
const kubernetesUsername = "kubernetesUsername"
const kubernetesPassword = "kubernetesPassword"

var dataSendRates = map[string]time.Duration{
	"5s":  5 * time.Second,
	"10s": 10 * time.Second,
	"30s": 30 * time.Second,
	"1m":  time.Minute,
	"5m":  5 * time.Minute,
}

var nodeServiceDiscoveryRates = map[string]time.Duration{
	"1m":  1 * time.Minute,
	"3m":  3 * time.Minute,
	"5m":  5 * time.Minute,
	"10m": 10 * time.Minute,
	"15m": 15 * time.Minute,
	"20m": 20 * time.Minute,
}

func getMapKeys(m map[string]time.Duration) (keys []string) {
	for k := range m {
		keys = append(keys, k)
	}
	return keys
}

func main() {

	flag.Parse()
	app := cli.NewApp()
	app.Name = "cadvisor-signalfx"
	app.Usage = "scraps metrics from kubernetes cadvisor and forwards them to SignalFx."
	app.Version = "git commit: " + poller.ToolVersion

	app.Flags = []cli.Flag{

		cli.StringFlag{
			Name:   ingestURL,
			Value:  "https://ingest.signalfx.com",
			EnvVar: "SFX_SCRAPPER_INGEST_URL",
			Usage:  "SignalFx ingest URL.",
		},
		cli.StringFlag{
			Name:   apiToken,
			Usage:  "API token.",
			EnvVar: "SFX_SCRAPPER_API_TOKEN",
		},
		cli.StringFlag{
			Name:   clusterName,
			Usage:  "Cluster name will appear as dimension.",
			EnvVar: "SFX_SCRAPPER_CLUSTER_NAME",
		},
		cli.StringFlag{
			Name:   dataSendRate,
			Value:  "5s",
			EnvVar: "SFX_SCRAPPER_SEND_RATE",
			Usage:  fmt.Sprintf("Rate at which data is queried from cAdvisor and send to SignalFx. Possible values: %v", getMapKeys(dataSendRates)),
		},
		cli.IntFlag{
			Name:   cadvisorPort,
			Value:  4194,
			EnvVar: "SFX_SCRAPPER_CADVISOR_PORT",
			Usage:  fmt.Sprintf("Port on which cAdvisor listens."),
		},
		cli.StringFlag{
			Name:   nodeServiceDiscoveryRate,
			Value:  "5m",
			EnvVar: "SFX_SCRAPPER_NODE_SERVICE_DISCOVERY_RATE",
			Usage:  fmt.Sprintf("Rate at which nodes and services will be rediscovered. Possible values: %v", getMapKeys(nodeServiceDiscoveryRates)),
		},
		cli.StringFlag{
			Name:   kubernetesURL,
			Usage:  "kubernetes URL.",
			EnvVar: "SFX_SCRAPPER_KUBERNETES_URL",
		},
		cli.StringFlag{
			Name:   kubernetesUsername,
			Usage:  "kubernetes username.",
			EnvVar: "SFX_SCRAPPER_KUBERNETES_USERNAME",
		},
		cli.StringFlag{
			Name:   kubernetesPassword,
			Usage:  "kubernetes username.",
			EnvVar: "SFX_SCRAPPER_KUBERNETES_PASSWORD",
		},
	}

	app.Action = func(c *cli.Context) {

		paramAPIToken := c.String(apiToken)
		if paramAPIToken == "" {
			glog.Errorf("\nERROR: apiToken must be set.\n\n")
			cli.ShowAppHelp(c)
			os.Exit(1)
		}

		paramDataSendRate, ok := dataSendRates[c.String(dataSendRate)]
		if !ok {
			glog.Errorf("\nERROR: dataSendRate must be one of: %v.\n\n", getMapKeys(dataSendRates))
			cli.ShowAppHelp(c)
			os.Exit(1)
		}

		paramNodeServiceDiscoveryRate, ok := nodeServiceDiscoveryRates[c.String(nodeServiceDiscoveryRate)]
		if !ok {
			glog.Errorf("\nERROR: nodeServiceDiscoveryRate must be one of: %v.\n\n", getMapKeys(nodeServiceDiscoveryRates))
			cli.ShowAppHelp(c)
			os.Exit(1)
		}

		paramClusterName := c.String(clusterName)
		if paramClusterName == "" {
			glog.Errorf("\nERROR: clusterName must be set.\n\n")
			cli.ShowAppHelp(c)
			os.Exit(1)
		}

		paramIngestURL := c.String(ingestURL)
		if paramIngestURL == "" {
			glog.Errorf("\nERROR: ingestUrl must be set.\n\n")
			cli.ShowAppHelp(c)
			os.Exit(1)
		}

		var instance = poller.PrometheusScraper{
			Forwarder: poller.NewSfxClient(paramIngestURL, paramAPIToken),
			Cfg: &poller.Config{
				IngestURL:              paramIngestURL,
				APIToken:               paramAPIToken,
				DataSendRate:           c.String(dataSendRate),
				ClusterName:            paramClusterName,
				NodeServiceRefreshRate: c.String(nodeServiceDiscoveryRate),
				CadvisorPort:           c.Int(cadvisorPort),
				KubernetesURL:          c.String(kubernetesURL),
				KubernetesUsername:     c.String(kubernetesUsername),
				KubernetesPassword:     c.String(kubernetesPassword),
			},
		}

		if err := instance.Main(paramDataSendRate, paramNodeServiceDiscoveryRate); err != nil {
			glog.Errorf("%s\n", err.Error())
			os.Exit(1)
		}
	}

	app.Run(os.Args)
}
