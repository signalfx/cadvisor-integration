package main

import (
	"fmt"
	"os"
	"reflect"
	"regexp"
	//"sort"
	"errors"
	"strings"
	"sync"
	"time"

	"net/url"

	"flag"
	"github.com/golang/glog"

	"encoding/json"
	"runtime"

	"golang.org/x/net/context"

	"github.com/signalfx/golib/datapoint"
	"github.com/signalfx/metricproxy/protocol/signalfx"

	"github.com/codegangsta/cli"
	"github.com/goinggo/workpool"
	kubeAPI "k8s.io/kubernetes/pkg/api"
	kube "k8s.io/kubernetes/pkg/client/unversioned"
	kubeFields "k8s.io/kubernetes/pkg/fields"
	kubeLabels "k8s.io/kubernetes/pkg/labels"

	"github.com/google/cadvisor/client"
	info "github.com/google/cadvisor/info/v1"
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
	CadvisorPort           int
	KubernetesURL          string
	KubernetesUsername     string
	KubernetesPassword     string
}

type prometheusScraper struct {
	forwarder *signalfx.Forwarder
	cfg       *Config
}

// workProxy will call work.DoWork and then callback
/*type workProxy struct {
	work     workpool.PoolWorker
	callback func()
}

func (wp *workProxy) DoWork(workRoutine int) {
	wp.work.DoWork(workRoutine)
	wp.callback()
}*/

type scrapWork2 struct {
	serverURL  string
	collector  *CadvisorCollector
	chRecvOnly chan datapoint.Datapoint
}

func (scrapWork *scrapWork2) DoWork(workRoutine int) {
	scrapWork.collector.Collect(scrapWork.chRecvOnly)
}

/*type sortableDatapoint []*datapoint.Datapoint

func (sd sortableDatapoint) Len() int {
	return len(sd)
}

func (sd sortableDatapoint) Swap(i, j int) {
	sd[i], sd[j] = sd[j], sd[i]
}

func (sd sortableDatapoint) Less(i, j int) bool {
	return sd[i].Timestamp.Unix() < sd[j].Timestamp.Unix()
}*/

type cadvisorInfoProvider struct {
	cc         *client.Client
	lastUpdate time.Time
}

func (cip *cadvisorInfoProvider) SubcontainersInfo(containerName string) ([]info.ContainerInfo, error) {
	curTime := time.Now()
	info, err := cip.cc.AllDockerContainers(&info.ContainerInfoRequest{Start: cip.lastUpdate, End: curTime})
	if len(info) > 0 {
		cip.lastUpdate = curTime
	}
	return info, err
}

func (cip *cadvisorInfoProvider) GetMachineInfo() (*info.MachineInfo, error) {
	return cip.cc.MachineInfo()
}

func newCadvisorInfoProvider(cadvisorClient *client.Client) *cadvisorInfoProvider {
	return &cadvisorInfoProvider{
		cc:         cadvisorClient,
		lastUpdate: time.Now(),
	}
}

const autoFlushTimerDuration = 500 * time.Millisecond
const maxDatapoints = 50

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
	"3m":  3 * time.Minute,
	"5m":  5 * time.Minute,
	"10m": 10 * time.Minute,
	"15m": 15 * time.Minute,
	"20m": 20 * time.Minute,
}

func printVersion() {
	glog.Infof("git build commit: %v\n", toolVersion)
}

func main() {

	flag.Parse()
	app := cli.NewApp()
	app.Name = "prometheustosfx"
	app.Usage = "scraps metrics from cAdvisor and forwards them to SignalFx."
	app.Version = "git commit: " + toolVersion

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

		var instance = prometheusScraper{
			forwarder: newSfxClient(paramIngestURL, paramAPIToken),
			cfg: &Config{
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

		if err := instance.main(paramDataSendRate, paramNodeServiceDiscoveryRate); err != nil {
			glog.Errorf("%s\n", err.Error())
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

func updateNodes(kubeClient *kube.Client, cPort int) (hostIPtoNodeMap map[string]kubeAPI.Node, nodeIPs []string) {

	hostIPtoNodeMap = make(map[string]kubeAPI.Node, 2)
	nodeIPs = make([]string, 0, 2)
	nodeList, apiErr := kubeClient.Nodes().List(kubeLabels.Everything(), kubeFields.Everything())
	if apiErr != nil {
		glog.Errorf("Failed to list kubernetes nodes. Error: %v\n", apiErr)
	} else {
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
				hostIP = fmt.Sprintf("http://%v:%v", hostIP, cPort)
				nodeIPs = append(nodeIPs, hostIP)
				hostIPtoNodeMap[hostIP] = node
			}
		}
	}

	return hostIPtoNodeMap, nodeIPs
}

func updateServices(kubeClient *kube.Client) (podToServiceMap map[string]string) {

	serviceList, apiErr := kubeClient.Services("").List(kubeLabels.Everything(), kubeFields.Everything())
	if apiErr != nil {
		glog.Errorf("Failed to list kubernetes services. Error: %v\n", apiErr)
		return nil
	}

	podToServiceMap = make(map[string]string, 2)
	for _, service := range serviceList.Items {
		podList, apiErr := kubeClient.Pods("").List(kubeLabels.SelectorFromSet(service.Spec.Selector), kubeFields.Everything())
		if apiErr != nil {
			glog.Errorf("Failed to list kubernetes pods. Error: %v\n", apiErr)
		} else {
			for _, pod := range podList.Items {
				//fmt.Printf("%v -> %v\n", pod.ObjectMeta.Name, service.ObjectMeta.Name)
				podToServiceMap[pod.ObjectMeta.Name] = service.ObjectMeta.Name
			}
		}
	}
	return podToServiceMap
}

func newKubeClient(config *Config) (kubeClient *kube.Client, kubeErr error) {

	if config.KubernetesURL == "" {
		kubeClient, kubeErr = kube.NewInCluster()
	} else {
		kubeConfig := new(kube.Config)
		kubeConfig.Host = config.KubernetesURL
		kubeConfig.Username = config.KubernetesUsername
		kubeConfig.Password = config.KubernetesPassword
		kubeConfig.Insecure = true
		kubeClient, kubeErr = kube.New(kubeConfig)
	}

	if kubeErr != nil {
		glog.Errorf("Failed to create kubernetes client. Error: %v\n", kubeErr)
		kubeClient = nil
	}

	return
}

func (p *prometheusScraper) main(paramDataSendRate, paramNodeServiceDiscoveryRate time.Duration) (err error) {

	kubeClient, err := newKubeClient(p.cfg)
	if err != nil {
		return err
	}

	podToServiceMap := updateServices(kubeClient)
	hostIPtoNameMap, nodeIPs := updateNodes(kubeClient, p.cfg.CadvisorPort)
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
	glog.Infof("Scrapper started with following params:\n%v\n", string(cfg))

	scrapWorkCache := newScrapWorkCache(p.cfg, p.forwarder)
	stop := make(chan error, 1)

	scrapWorkCache.setPodToServiceMap(podToServiceMap)
	scrapWorkCache.setHostIPtoNameMap(hostIPtoNameMap)

	scrapWorkCache.buildWorkList(p.cfg.CadvisorURL)

	// Wait on channel input and forward datapoints to SignalFx
	go func() {
		scrapWorkCache.waitAndForward()                // Blocking call!
		stop <- errors.New("all channels were closed") // Stop all timers
	}()

	workPool := workpool.New(runtime.NumCPU(), int32(len(p.cfg.CadvisorURL)+1))

	// Collect data from nodes
	scrapWorkTicker := time.NewTicker(paramDataSendRate)
	go func() {
		for range scrapWorkTicker.C {

			scrapWorkCache.foreachWork(func(i int, w *scrapWork2) bool {
				workPool.PostWork("CollectDataWork", w)
				return true
			})
		}
	}()

	// New nodes and services discovery
	updateNodeAndPodTimer := time.NewTicker(paramNodeServiceDiscoveryRate)
	go func() {

		for range updateNodeAndPodTimer.C {

			podMap := updateServices(kubeClient)
			hostMap, _ := updateNodes(kubeClient, p.cfg.CadvisorPort)

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
						glog.Errorf("Failed connect to server: %v\n", localERR)
						continue
					}

					scrapWorkCache.addWork(&scrapWork2{
						serverURL:  serverURL,
						collector:  NewCadvisorCollector(newCadvisorInfoProvider(cadvisorClient), nameToLabel),
						chRecvOnly: make(chan datapoint.Datapoint),
					})
				}
			}

			scrapWorkCache.setPodToServiceMap(podMap)
		}
	}()

	err = <-stop // Block here till stopped

	updateNodeAndPodTimer.Stop()
	scrapWorkTicker.Stop()

	return
}

type responseChannel *chan bool

type scrapWorkCache struct {
	workCache       []*scrapWork2
	cases           []reflect.SelectCase
	flushChan       chan responseChannel
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
		flushChan: make(chan responseChannel, 1),
		forwarder: forwarder,
		cfg:       cfg,
		mutex:     &sync.Mutex{},
	}
}

func (swc *scrapWorkCache) addWork(work *scrapWork2) {
	swc.mutex.Lock()
	defer swc.mutex.Unlock()

	swc.workCache = append(swc.workCache, work)
	c := reflect.SelectCase{Dir: reflect.SelectRecv, Chan: reflect.ValueOf(work.chRecvOnly)}
	swc.cases = append(swc.cases, c)
}

// Build list of work
func (swc *scrapWorkCache) buildWorkList(URLList []string) {
	for _, serverURL := range URLList {
		cadvisorClient, localERR := client.NewClient(serverURL)
		if localERR != nil {
			glog.Errorf("Failed connect to server: %v\n", localERR)
			continue
		}

		swc.addWork(&scrapWork2{
			serverURL:  serverURL,
			collector:  NewCadvisorCollector(newCadvisorInfoProvider(cadvisorClient), nameToLabel),
			chRecvOnly: make(chan datapoint.Datapoint),
		})
	}
}

func (swc *scrapWorkCache) setPodToServiceMap(m map[string]string) {
	swc.mutex.Lock()
	defer swc.mutex.Unlock()

	swc.podToServiceMap = m
}

func (swc *scrapWorkCache) setHostIPtoNameMap(m map[string]kubeAPI.Node) {
	swc.mutex.Lock()
	defer swc.mutex.Unlock()

	swc.hostIPtoNameMap = m
}

type eachWorkFunc func(int, *scrapWork2) bool

// foreachWork iterates over scrapWorkCache.workCache and calls eachWorkFunc on every element
// foreachWork will operate on copy of scrapWorkCache.workCache
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

// This function will block
func (swc *scrapWorkCache) flush() {
	respChan := make(chan bool, 1)
	swc.flushChan <- &respChan
	<-respChan
}

func (swc *scrapWorkCache) fillNodeDims(chosen int, dims map[string]string) {

	node, ok := func() (n kubeAPI.Node, b bool) {
		swc.mutex.Lock()
		defer func() {
			swc.mutex.Unlock()
			if r := recover(); r != nil {
				glog.Warningln("Recovered in fillNodeDims: ", r)
			}
		}()

		n, b = swc.hostIPtoNameMap[swc.workCache[chosen].serverURL]
		return
	}()

	if ok {
		dims["node"] = node.ObjectMeta.Name
		dims["node_container_runtime_version"] = node.Status.NodeInfo.ContainerRuntimeVersion
		dims["node_kernel_version"] = node.Status.NodeInfo.KernelVersion
		dims["node_kubelet_version"] = node.Status.NodeInfo.KubeletVersion
		dims["node_os_image"] = node.Status.NodeInfo.OsImage
		dims["node_kubeproxy_version"] = node.Status.NodeInfo.KubeProxyVersion
	}
}

// Wait on channel input and forward datapoints to SignalFx.
// This function will block.
func (swc *scrapWorkCache) waitAndForward() {
	swc.mutex.Lock()
	remaining := len(swc.cases)
	swc.mutex.Unlock()

	ctx := context.Background()

	// localMutex used to sync i access
	localMutex := &sync.Mutex{}
	i := 0

	// ret is buffer that accumulates datapoints to be send to SignalFx
	ret := make([]*datapoint.Datapoint, maxDatapoints)

	autoFlushTimer := time.NewTimer(autoFlushTimerDuration)
	stopFlusher := make(chan bool, 1)
	flushFunc := func(respChan responseChannel) {
		func() {
			localMutex.Lock()
			defer localMutex.Unlock()

			if i > 0 {
				swc.forwarder.AddDatapoints(ctx, ret)
				i = 0
			}
		}()

		if respChan != nil {
			*respChan <- true
		}
	}

	// This thread will flush ret buffer if requested
	// Also it will auto flush it in 500 milliseconds
	go func() {
		for true {
			select {
			case respChan := <-swc.flushChan:
				flushFunc(respChan)
			case <-autoFlushTimer.C:
				flushFunc(nil)
				autoFlushTimer.Reset(autoFlushTimerDuration)
			case <-stopFlusher:
				return
			}
		}
	}()

	for remaining > 0 {
		autoFlushTimer.Reset(autoFlushTimerDuration)
		chosen, value, ok := reflect.Select(swc.cases)
		autoFlushTimer.Stop()
		if !ok {
			// The chosen channel has been closed, so remove the case and work
			swc.mutex.Lock()
			swc.cases[chosen].Chan = reflect.ValueOf(nil)
			swc.cases = append(swc.cases[:chosen], swc.cases[chosen+1:]...)
			swc.workCache = append(swc.workCache[:chosen], swc.workCache[chosen+1:]...)
			remaining = len(swc.cases)
			swc.mutex.Unlock()
			continue
		}

		dp := value.Interface().(datapoint.Datapoint)
		dims := dp.Dimensions

		// filter POD level metrics
		if dims["kubernetes_container_name"] == "POD" {
			matched, _ := regexp.MatchString("^container_network_.*", dp.Metric)
			if !matched {
				continue
			}
		}

		dims["cluster"] = swc.cfg.ClusterName

		swc.fillNodeDims(chosen, dims)

		podName, ok := dims["kubernetes_pod_name"]
		if ok {
			swc.mutex.Lock()
			serviceName, ok := swc.podToServiceMap[podName]
			swc.mutex.Unlock()
			if ok {
				dims["hostHasService"] = serviceName
			}
		}

		func() {
			localMutex.Lock()
			defer localMutex.Unlock()

			ret[i] = &dp
			i++
			if i == maxDatapoints {
				//sort.Sort(sortableDatapoint(ret))

				func() {
					localMutex.Unlock()
					defer localMutex.Lock()

					swc.flush()
				}()
			}
		}()
	}
	stopFlusher <- true
}
