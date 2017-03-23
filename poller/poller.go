package poller

import (
	"fmt"
	"reflect"
	"regexp"
	//"sort"
	"errors"
	"strings"
	"sync"
	"time"

	"net/url"

	"github.com/golang/glog"

	"encoding/json"
	"runtime"

	"golang.org/x/net/context"

	"github.com/signalfx/cadvisor-integration/converter"
	"github.com/signalfx/golib/datapoint"
	"github.com/signalfx/metricproxy/protocol/signalfx"

	"github.com/goinggo/workpool"
	kubeAPI "k8s.io/kubernetes/pkg/api"
	kube "k8s.io/kubernetes/pkg/client/unversioned"
	kubeFields "k8s.io/kubernetes/pkg/fields"
	kubeLabels "k8s.io/kubernetes/pkg/labels"

	"os"

	"github.com/google/cadvisor/client"
	info "github.com/google/cadvisor/info/v1"
)

func init() {
	re = regexp.MustCompile(`^k8s_(?P<kubernetes_container_name>[^_\.]+)[^_]+_(?P<kubernetes_pod_name>[^_]+)_(?P<kubernetes_namespace>[^_]+)`)
	reCaptureNames = re.SubexpNames()
}

var re *regexp.Regexp
var reCaptureNames []string

// Set by build system
var ToolVersion = "NOT SET"

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
	DefaultDimensions      map[string]string
}

type PrometheusScraper struct {
	Forwarder *signalfx.Forwarder
	Cfg       *Config
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
	collector  *converter.CadvisorCollector
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
const dataSourceType = "kubernetes"

func printVersion() {
	glog.Infof("git build commit: %v\n", ToolVersion)
}

func NewSfxClient(ingestURL, authToken string) (forwarder *signalfx.Forwarder) {
	forwarder = signalfx.NewSignalfxJSONForwarder(strings.Join([]string{ingestURL, "v2/datapoint"}, "/"), time.Second*10, authToken, 10, "", "", "") //http://lab-ingest.corp.signalfuse.com:8080
	forwarder.UserAgent(fmt.Sprintf("SignalFxScrapper/1.0 (gover %s)", runtime.Version()))
	return
}

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
		kubeConfig := &kube.Config{
			Host:     config.KubernetesURL,
			Username: config.KubernetesUsername,
			Password: config.KubernetesPassword,
			Insecure: true,
		}
		kubeClient, kubeErr = kube.New(kubeConfig)
	}

	if kubeErr != nil {
		glog.Errorf("Failed to create kubernetes client. Error: %v\n", kubeErr)
		kubeClient = nil
	}

	return
}

// MonitorNode collects metrics from a single node
func MonitorNode(cfg *Config, forwarder *signalfx.Forwarder, dataSendRate time.Duration) (stop chan bool, stopped chan bool, err error) {
	swc := newScrapWorkCache(cfg, forwarder)
	cadvisorClient, err := client.NewClient(cfg.CadvisorURL[0])
	if err != nil {
		return nil, nil, err
	}

	collector := converter.NewCadvisorCollector(newCadvisorInfoProvider(cadvisorClient), nameToLabel)

	// TODO: fill in if we want node dimensions but that requires contacting apiserver.
	// swc.hostIPtoNameMap[]

	sw2 := &scrapWork2{
		// I think only used for swc.HostIPToNameMap lookup
		serverURL:  "",
		collector:  collector,
		chRecvOnly: make(chan datapoint.Datapoint),
	}

	swc.addWork(sw2)

	ticker := time.NewTicker(dataSendRate)
	stop = make(chan bool, 1)
	stopped = make(chan bool, 1)

	go func() {
		for {
			select {
			case <-stop:
				glog.Info("stopping collection")
				ticker.Stop()
				close(sw2.chRecvOnly)
				return
			case <-ticker.C:
				collector.Collect(sw2.chRecvOnly)
			}
		}
	}()

	go func() {
		swc.waitAndForward()
		stopped <- true
		glog.Info("waitAndForward returned")
	}()

	return stop, stopped, nil
}

func (p *PrometheusScraper) Main(paramDataSendRate, paramNodeServiceDiscoveryRate time.Duration) (err error) {

	kubeClient, err := newKubeClient(p.Cfg)
	if err != nil {
		return err
	}

	podToServiceMap := updateServices(kubeClient)
	hostIPtoNameMap, nodeIPs := updateNodes(kubeClient, p.Cfg.CadvisorPort)
	p.Cfg.CadvisorURL = nodeIPs

	cadvisorServers := make([]*url.URL, len(p.Cfg.CadvisorURL))
	for i, serverURL := range p.Cfg.CadvisorURL {
		cadvisorServers[i], err = url.Parse(serverURL)
		if err != nil {
			return err
		}
	}

	printVersion()
	cfg, _ := json.MarshalIndent(p.Cfg, "", "  ")
	glog.Infof("Scrapper started with following params:\n%v\n", string(cfg))

	scrapWorkCache := newScrapWorkCache(p.Cfg, p.Forwarder)
	stop := make(chan error, 1)

	scrapWorkCache.setPodToServiceMap(podToServiceMap)
	scrapWorkCache.setHostIPtoNameMap(hostIPtoNameMap)

	scrapWorkCache.buildWorkList(p.Cfg.CadvisorURL)

	// Wait on channel input and forward datapoints to SignalFx
	go func() {
		scrapWorkCache.waitAndForward()                // Blocking call!
		stop <- errors.New("all channels were closed") // Stop all timers
	}()

	workPool := workpool.New(runtime.NumCPU(), int32(len(p.Cfg.CadvisorURL)+1))

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
			hostMap, _ := updateNodes(kubeClient, p.Cfg.CadvisorPort)

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
						collector:  converter.NewCadvisorCollector(newCadvisorInfoProvider(cadvisorClient), nameToLabel),
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
			collector:  converter.NewCadvisorCollector(newCadvisorInfoProvider(cadvisorClient), nameToLabel),
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
	} else {
		// This should only happen when doing MonitorNode().
		// TODO: Add rest of dimensions above.
		if hostname, err := os.Hostname(); err != nil {
			dims["node"] = hostname
		}
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
			matched, _ := regexp.MatchString("^pod_network_.*", dp.Metric)
			if !matched {
				continue
			}
			delete(dims, "kubernetes_container_name")
		}

		dims["metric_source"] = dataSourceType
		dims["cluster"] = swc.cfg.ClusterName

		swc.fillNodeDims(chosen, dims)

		for k, v := range swc.cfg.DefaultDimensions {
			dims[k] = v
		}

		// remove high cardinality dimensions
		delete(dims, "id")
		delete(dims, "name")

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
