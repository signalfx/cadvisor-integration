package main

import (
	//"bytes"
	"fmt"
	"os"
	"reflect"
	"regexp"
	"sort"
	//"strconv"
	"errors"
	"strings"
	"sync"
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
	kubeAPI "k8s.io/kubernetes/pkg/api"
	kube "k8s.io/kubernetes/pkg/client/unversioned"
	kubeFields "k8s.io/kubernetes/pkg/fields"
	kubeLabels "k8s.io/kubernetes/pkg/labels"

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
	IngestURL              string
	CadvisorURL            []string
	APIToken               string
	DataSendRate           string
	ClusterName            string
	NodeServiceRefreshRate string
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
	serverURL  string
	stop       chan error
	collector  *metrics.PrometheusCollector
	chRecvOnly chan prometheus.Metric
}

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
	return cip.cc.AllDockerContainers(&info.ContainerInfoRequest{NumStats: 3}) //&info.ContainerInfoRequest{NumStats: 10, Start: time.Unix(0, time.Now().UnixNano()-10*time.Second.Nanoseconds())})
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

const maxDatapoints = 50

func (scrapWork *scrapWork2) DoWork(workRoutine int) {
	scrapWork.collector.Collect(scrapWork.chRecvOnly)
}

const ingestURL = "ingestURL"
const apiToken = "apiToken"
const dataSendRate = "sendRate"
const nodeServiceDiscoveryRate = "nodeServiceDiscoveryRate"
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

var nodeServiceDiscoveryRates = map[string]time.Duration{
	"3m":  3 * time.Minute,
	"5m":  5 * time.Minute,
	"10m": 10 * time.Minute,
	"15m": 15 * time.Minute,
	"20m": 20 * time.Minute,
}

func printVersion() {
	fmt.Printf("git build commit: %v\n", toolVersion)
}

func main() {
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
		cli.StringFlag{
			Name:   nodeServiceDiscoveryRate,
			Value:  "5m",
			EnvVar: "SFX_SCRAPPER_NODE_SERVICE_DISCOVERY_RATE",
			Usage:  fmt.Sprintf("Rate at which nodes and services will be rediscovered. Possible values: %v", getMapKeys(nodeServiceDiscoveryRates)),
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

		paramNodeServiceDiscoveryRate, ok := nodeServiceDiscoveryRates[c.String(nodeServiceDiscoveryRate)]
		if !ok {
			fmt.Fprintf(os.Stderr, "\nERROR: nodeServiceDiscoveryRate must be one of: %v.\n\n", getMapKeys(nodeServiceDiscoveryRates))
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
				IngestURL:              paramIngestURL,
				APIToken:               paramAPIToken,
				DataSendRate:           c.String(dataSendRate),
				ClusterName:            paramClusterName,
				NodeServiceRefreshRate: c.String(nodeServiceDiscoveryRate),
			},
		}

		if err := instance.main(paramDataSendRate, paramNodeServiceDiscoveryRate); err != nil {
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

func updateNodes() (hostIPtoNodeMap map[string]kubeAPI.Node, nodeIPs []string) {
	fmt.Printf("Updating Nodes\n")
	kubeClient, kubeErr := kube.NewInCluster()
	if kubeErr != nil {
		fmt.Printf("kubeErr: %v\n", kubeErr)
		return nil, nil
	}

	fmt.Printf("kubeClient created.\n")
	hostIPtoNodeMap = make(map[string]kubeAPI.Node, 2)
	nodeIPs = make([]string, 0, 2)
	nodeList, apiErr := kubeClient.Nodes().List(kubeLabels.Everything(), kubeFields.Everything())
	if apiErr != nil {
		fmt.Printf("apiErr: %v\n", apiErr)
	} else {
		fmt.Printf("nodeList received.\n")
		for _, node := range nodeList.Items {
			var hostIP string
			for _, nodeAddress := range node.Status.Addresses {
				switch nodeAddress.Type {
				case kubeAPI.NodeInternalIP:
					hostIP = nodeAddress.Address
					break
				case kubeAPI.NodeLegacyHostIP:
					hostIP = nodeAddress.Address
				}
			}
			if hostIP != "" {
				hostIP = "http://" + hostIP + ":4194"
				nodeIPs = append(nodeIPs, hostIP)
				hostIPtoNodeMap[hostIP] = node
			}
		}
	}

	return hostIPtoNodeMap, nodeIPs
}

func updateServices() (podToServiceMap map[string]string) {
	fmt.Printf("Updating Services\n")

	kubeClient, kubeErr := kube.NewInCluster()
	if kubeErr != nil {
		fmt.Printf("kubeErr: %v\n", kubeErr)
		return nil
	}

	serviceList, apiErr := kubeClient.Services("").List(kubeLabels.Everything(), kubeFields.Everything())
	if apiErr != nil {
		fmt.Printf("apiErr: %v\n", apiErr)
		return nil
	}

	fmt.Printf("serviceList received.\n")
	podToServiceMap = make(map[string]string, 2)
	for _, service := range serviceList.Items {
		podList, apiErr := kubeClient.Pods("").List(kubeLabels.SelectorFromSet(service.Spec.Selector), kubeFields.Everything())
		if apiErr != nil {
			fmt.Printf("apiErr: %v\n", apiErr)
		} else {
			for _, pod := range podList.Items {
				//fmt.Printf("%v -> %v\n", pod.ObjectMeta.Name, service.ObjectMeta.Name)
				podToServiceMap[pod.ObjectMeta.Name] = service.ObjectMeta.Name
			}
		}
	}
	return podToServiceMap
}

func (p *prometheusScraper) main(paramDataSendRate, paramNodeServiceDiscoveryRate time.Duration) (err error) {
	podToServiceMap := updateServices()
	hostIPtoNameMap, nodeIPs := updateNodes()
	p.cfg.CadvisorURL = nodeIPs

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

	scrapWorkCache := newScrapWorkCache(p.cfg, p.forwarder)
	stop := make(chan error, 1)

	scrapWorkCache.setPodToServiceMap(podToServiceMap)
	scrapWorkCache.setHostIPtoNameMap(hostIPtoNameMap)

	// Build list of work
	for _, serverURL := range p.cfg.CadvisorURL {
		cadvisorClient, localERR := client.NewClient(serverURL)
		if localERR != nil {
			fmt.Printf("Failed connect to server: %v\n", localERR)
			continue
		}

		scrapWorkCache.addWork(&scrapWork2{
			serverURL: serverURL,
			stop:      stop,
			collector: metrics.NewPrometheusCollector(&cadvisorInfoProvider{
				cc: cadvisorClient,
			}, nameToLabel),
			chRecvOnly: make(chan prometheus.Metric),
		})
	}

	// Wait on channel input and forward datapoints to SignalFx
	go func() {
		scrapWorkCache.waitAndForward()
		stop <- errors.New("All channels were closed.")
	}()

	workPool := workpool.New(runtime.NumCPU(), int32(len(p.cfg.CadvisorURL)+1))

	scrapWorkTicker := time.NewTicker(paramDataSendRate)
	go func() {
		for range scrapWorkTicker.C {
			scrapWorkCache.foreachWork(func(i int, w *scrapWork2) bool {
				workPool.PostWork("", w)
				return true
			})
		}
	}()

	updateNodeAndPodTimer := time.NewTicker(paramNodeServiceDiscoveryRate)
	go func() {
		for range updateNodeAndPodTimer.C {

			podMap := updateServices()
			hostMap, _ := updateNodes()

			hostMapCopy := make(map[string]kubeAPI.Node)
			for k, v := range hostMap {
				hostMapCopy[k] = v
			}

			// Remove known nodes
			scrapWorkCache.foreachWork(func(i int, w *scrapWork2) bool {
				delete(hostMapCopy, w.serverURL)
				return true
			})

			if len(hostMapCopy) != 0 {
				scrapWorkCache.setHostIPtoNameMap(hostMap)

				// Add new(remaining) nodes to monitoring
				for serverURL := range hostMapCopy {
					cadvisorClient, localERR := client.NewClient(serverURL)
					if localERR != nil {
						fmt.Printf("Failed connect to server: %v\n", localERR)
						continue
					}

					scrapWorkCache.addWork(&scrapWork2{
						serverURL: serverURL,
						stop:      stop,
						collector: metrics.NewPrometheusCollector(&cadvisorInfoProvider{
							cc: cadvisorClient,
						}, nameToLabel),
						chRecvOnly: make(chan prometheus.Metric),
					})
				}
			} else {
				fmt.Printf("No new nodes appeared.\n")
			}

			scrapWorkCache.setPodToServiceMap(podMap)
		}
	}()

	err = <-stop

	updateNodeAndPodTimer.Stop()
	scrapWorkTicker.Stop()

	return err
}

type scrapWorkCache struct {
	workCache       []*scrapWork2
	cases           []reflect.SelectCase
	podToServiceMap map[string]string
	hostIPtoNameMap map[string]kubeAPI.Node
	forwarder       *signalfx.Forwarder
	cfg             *Config
	mutex           *sync.Mutex
}

func newScrapWorkCache(cfg *Config, forwarder *signalfx.Forwarder) *scrapWorkCache {
	return &scrapWorkCache{
		workCache: make([]*scrapWork2, 0, 1),
		cases:     make([]reflect.SelectCase, 0, 1),
		forwarder: forwarder,
		cfg:       cfg,
		mutex:     &sync.Mutex{},
	}
}

func (swc *scrapWorkCache) addWork(work *scrapWork2) {
	swc.mutex.Lock()
	swc.workCache = append(swc.workCache, work)
	c := reflect.SelectCase{Dir: reflect.SelectRecv, Chan: reflect.ValueOf(work.chRecvOnly)}
	swc.cases = append(swc.cases, c)
	swc.mutex.Unlock()
}

func (swc *scrapWorkCache) setPodToServiceMap(m map[string]string) {
	swc.mutex.Lock()
	swc.podToServiceMap = m
	swc.mutex.Unlock()
}

func (swc *scrapWorkCache) setHostIPtoNameMap(m map[string]kubeAPI.Node) {
	swc.mutex.Lock()
	swc.hostIPtoNameMap = m
	swc.mutex.Unlock()
}

type eachWorkFunc func(int, *scrapWork2) bool

func (swc *scrapWorkCache) foreachWork(f eachWorkFunc) {
	swc.mutex.Lock()
	workCacheCopy := make([]*scrapWork2, len(swc.workCache))
	copy(workCacheCopy, swc.workCache)
	swc.mutex.Unlock()

	for index, work := range workCacheCopy {
		if !f(index, work) {
			return
		}
	}
}

// Wait on channel input and forward datapoints to SignalFx
func (swc *scrapWorkCache) waitAndForward() {
	swc.mutex.Lock()
	remaining := len(swc.cases)
	swc.mutex.Unlock()

	ctx := context.Background()

	i := 0
	ret := make([]*datapoint.Datapoint, maxDatapoints)
	for remaining > 0 {
		chosen, value, ok := reflect.Select(swc.cases)
		if !ok {
			// The chosen channel has been closed, so zero out the channel to disable the case
			swc.mutex.Lock()
			swc.cases[chosen].Chan = reflect.ValueOf(nil)
			swc.cases = append(swc.cases[:chosen], swc.cases[chosen+1:]...)
			swc.workCache = append(swc.workCache[:chosen], swc.workCache[chosen+1:]...)
			remaining = len(swc.cases)
			swc.mutex.Unlock()
			continue
		}

		prometheusMetric := value.Interface().(prometheus.Metric)
		pMetric := dto.Metric{}
		prometheusMetric.Write(&pMetric)
		tsMs := pMetric.GetTimestampMs()
		dims := make(map[string]string, len(pMetric.GetLabel()))
		for _, l := range pMetric.GetLabel() {
			key := l.GetName()
			value := l.GetValue()
			if key != "" && value != "" {
				dims[key] = value
			}
		}
		dims["cluster"] = swc.cfg.ClusterName

		swc.mutex.Lock()
		node, ok := swc.hostIPtoNameMap[swc.workCache[chosen].serverURL]
		swc.mutex.Unlock()
		if ok {
			dims["node"] = node.ObjectMeta.Name
			dims["node_container_runtime_version"] = node.Status.NodeInfo.ContainerRuntimeVersion
			dims["node_kernel_version"] = node.Status.NodeInfo.KernelVersion
			dims["node_kubelet_version"] = node.Status.NodeInfo.KubeletVersion
			dims["node_os_image"] = node.Status.NodeInfo.OsImage
			dims["node_kubeproxy_version"] = node.Status.NodeInfo.KubeProxyVersion
		}

		podName, ok := dims["kubernetes_pod_name"]
		if ok {
			swc.mutex.Lock()
			serviceName, ok := swc.podToServiceMap[podName]
			swc.mutex.Unlock()
			if ok {
				dims["service"] = serviceName
			}
		}

		metricName := prometheusMetric.Desc().MetricName()
		timestamp := time.Unix(0, tsMs*time.Millisecond.Nanoseconds())

		for _, conv := range scrapper.ConvertMeric(&pMetric) {
			dp := datapoint.New(metricName+conv.MetricNameSuffix, scrapper.AppendDims(dims, conv.ExtraDims), conv.Value, conv.MType, timestamp)
			ret[i] = dp
			i++
			if i == maxDatapoints {
				sort.Sort(sortableDatapoint(ret))
				swc.forwarder.AddDatapoints(ctx, ret)
				i = 0
			}
		}
	}
}
