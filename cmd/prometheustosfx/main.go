package main

import (
	//"bytes"
	"fmt"
	"os"
	//"reflect"
	"regexp"
	"sort"
	//"strconv"
	"strings"
	"time"

	//	"net/http"
	"net/url"

	//	"log"

	"encoding/json"
	"runtime"

	"golang.org/x/net/context"

	"github.com/signalfx/golib/datapoint"
	"github.com/signalfx/metricproxy/protocol/signalfx"
	"github.com/signalfx/prometheustosignalfx/scrapper"

	"github.com/codegangsta/cli"
	"github.com/goinggo/workpool"

	//"github.com/fatih/structs"
	"github.com/google/cadvisor/client"
	info "github.com/google/cadvisor/info/v1"
	"github.com/google/cadvisor/metrics"
	"github.com/prometheus/client_golang/prometheus"
	dto "github.com/prometheus/client_model/go"
)

// Set by build system
var toolVersion = "NOT SET"

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

type scrapWork2 struct {
	ctx         context.Context
	serverURL   string
	clusterName string
	stop        chan error
	forwarder   *signalfx.Forwarder
	collector   *metrics.PrometheusCollector
	chRecvOnly  chan prometheus.Metric
}

var scrapWorkCache []scrapWork2

type sortableDatapoint []*datapoint.Datapoint

func (sd sortableDatapoint) Len() int {
	return len(sd)
}

func (sd sortableDatapoint) Swap(i, j int) {
	sd[i], sd[j] = sd[j], sd[i]
}

func (sd sortableDatapoint) Less(i, j int) bool {
	return sd[i].Timestamp.Unix() < sd[j].Timestamp.Unix()
}

type cadvisorInfoProvider struct {
	cc *client.Client
}

func (cip *cadvisorInfoProvider) SubcontainersInfo(containerName string, query *info.ContainerInfoRequest) ([]info.ContainerInfo, error) {
	return cip.cc.AllDockerContainers(query) //&info.ContainerInfoRequest{NumStats: 10, Start: time.Unix(0, time.Now().UnixNano()-10*time.Second.Nanoseconds())})
}

func (cip *cadvisorInfoProvider) GetVersionInfo() (*info.VersionInfo, error) {
	//TODO: remove fake info
	return &info.VersionInfo{
		KernelVersion:      "4.1.6-200.fc22.x86_64",
		ContainerOsVersion: "Fedora 22 (Twenty Two)",
		DockerVersion:      "1.8.1",
		CadvisorVersion:    "0.16.0",
		CadvisorRevision:   "abcdef",
	}, nil
}

func (cip *cadvisorInfoProvider) GetMachineInfo() (*info.MachineInfo, error) {
	return cip.cc.MachineInfo()
}

//Consume metrics from channel and forward them to SignalFx
func (scrapWork *scrapWork2) SafeForwardMetrics() {
	defer func() {
		if r := recover(); r != nil {
			fmt.Println("Recovered in f", r)
		}
	}()
	scrapWork.forwardMetrics()
}

func (scrapWork *scrapWork2) forwardMetrics() {
	const maxDatapoints = 100
	ret := make([]*datapoint.Datapoint, 0, maxDatapoints)
	for m := range scrapWork.chRecvOnly {
		pMetric := dto.Metric{}
		m.Write(&pMetric)
		tsMs := pMetric.GetTimestampMs()
		dims := make(map[string]string, len(pMetric.GetLabel()))
		for _, l := range pMetric.GetLabel() {
			key := l.GetName()
			value := l.GetValue()
			if key != "" && value != "" {
				dims[key] = value
			}
		}
		dims["cluster_name"] = scrapWork.clusterName
		//dims["node_url"] = scrapWork.serverURL

		metricName := m.Desc().MetricName()
		timestamp := time.Unix(0, tsMs*time.Millisecond.Nanoseconds())

		for _, conv := range scrapper.ConvertMeric(&pMetric) {
			dp := datapoint.New(metricName+conv.MetricNameSuffix, scrapper.AppendDims(dims, conv.ExtraDims), conv.Value, conv.MType, timestamp)
			//dpJSON, _ := json.MarshalIndent(dp, "", "  ")
			//fmt.Printf("%v\n", string(dpJSON))
			ret = append(ret, dp)
			if len(ret) == maxDatapoints {
				sort.Sort(sortableDatapoint(ret))

				scrapWork.forwarder.AddDatapoints(scrapWork.ctx, ret)
				ret = make([]*datapoint.Datapoint, 0, maxDatapoints)
			}
		}
	}
}

func (scrapWork *scrapWork2) DoWork(workRoutine int) {
	scrapWork.collector.Collect(scrapWork.chRecvOnly)
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

func printVersion() {
	fmt.Printf("git build commit: %v\n", toolVersion)
}

func main() {
	runtime.GOMAXPROCS(runtime.NumCPU())

	app := cli.NewApp()
	app.Name = "prometheustosfx"
	app.Usage = "scraps metrics from cAdvisor and forwards them to SignalFx."
	app.Version = "git commit: " + toolVersion

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
			Name:   dataSendRate,
			Value:  "1s",
			EnvVar: "SFX_SCRAPPER_SEND_RATE",
			Usage:  fmt.Sprintf("Rate at which data is queried from cAdvisor and send to SignalFx. Possible values: %v", getMapKeys(dataSendRates)),
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

	re = regexp.MustCompile(`^k8s_(?P<kubernetes_container_name>[^_\.]+)[^_]+_(?P<kubernetes_pod_name>[^_]+)_(?P<kubernetes_namespace>[^_]+)`)
	reCaptureNames = re.SubexpNames()

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

var re *regexp.Regexp
var reCaptureNames []string

func nameToLabel(name string) map[string]string {
	extraLabels := map[string]string{}
	matches := re.FindStringSubmatch(name)
	for i, match := range matches {
		if len(reCaptureNames[i]) > 0 {
			extraLabels[re.SubexpNames()[i]] = match
		}
	}
	return extraLabels
}

func (p *prometheusScraper) main(paramDataSendRate time.Duration) (err error) {

	ctx := context.Background()
	cadvisorServers := make([]*url.URL, len(p.cfg.CadvisorURL))
	for i, serverURL := range p.cfg.CadvisorURL {
		cadvisorServers[i], err = url.Parse(serverURL)
		if err != nil {
			return err
		}
	}

	printVersion()
	cfg, _ := json.MarshalIndent(p.cfg, "", "  ")
	fmt.Printf("Scrapper started with following params:\n%v\n", string(cfg))

	scrapWorkCache = make([]scrapWork2, 0, int32(len(p.cfg.CadvisorURL)+1))

	stop := make(chan error, 1)

	// Build list of work
	for _, serverURL := range p.cfg.CadvisorURL {
		cadvisorClient, localERR := client.NewClient(serverURL)
		if localERR != nil {
			fmt.Printf("Failed connect to server: %v\n", localERR)
			continue
		}

		work := scrapWork2{
			ctx:         ctx,
			serverURL:   serverURL,
			clusterName: p.cfg.ClusterName,
			stop:        stop,
			forwarder:   p.forwarder,
			collector: metrics.NewPrometheusCollector(&cadvisorInfoProvider{
				cc: cadvisorClient,
			}, nameToLabel),
			chRecvOnly: make(chan prometheus.Metric),
		}
		scrapWorkCache = append(scrapWorkCache, work)

		go func() {
			for {
				work.SafeForwardMetrics()
			}
		}()
	}

	fmt.Printf("Work size: %v\n", len(scrapWorkCache))
	if len(scrapWorkCache) == 0 {
		fmt.Printf("Nothing to do. Exiting.\n")
		return
	}

	workPool := workpool.New(runtime.NumCPU(), int32(len(scrapWorkCache)+1))

	ticker := time.NewTicker(paramDataSendRate)
	go func() {
		for range ticker.C {
			//fmt.Printf("--------[ %v ]---------\n", t)
			for idx := range scrapWorkCache {
				workPool.PostWork("", &scrapWorkCache[idx])
			}
		}
	}()
	err = <-stop

	ticker.Stop()

	return err
}
