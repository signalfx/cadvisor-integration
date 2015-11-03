package main

import (
	//"bytes"
	"fmt"
	"os"
	//"reflect"
	"sort"
	"strconv"
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

	"github.com/google/cadvisor/client"
	info "github.com/google/cadvisor/info/v1"
	//kube "github.com/kubernetes/kubernetes/pkg/client/unversioned"
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

type scrapWork2 struct {
	ctx           context.Context
	serverURL     string
	containerName string
	lastTimestamp time.Time
	clusterName   string
	stop          chan error
	forwarder     *signalfx.Forwarder
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

func (scrapWork *scrapWork2) DoWork(workRoutine int) {
	cadvisorClient, err := client.NewClient(scrapWork.serverURL)
	if err != nil {
		fmt.Printf("Failed connect to server: %v\n", err)
		return
	}

	containerInfoRequest := info.ContainerInfoRequest{
		NumStats: 60,
		Start:    scrapWork.lastTimestamp,
	}
	contInfo, err := cadvisorClient.ContainerInfo(scrapWork.containerName, &containerInfoRequest)
	if err != nil {
		fmt.Printf("Failed get docker containers: %v\n", err)
		return
	}

	sfxDatapoints := make([]*datapoint.Datapoint, 0, 100)
	sfxDatapointsPtr := &sfxDatapoints

	scrapWork.lastTimestamp = addMetric(contInfo, sfxDatapointsPtr, scrapWork.clusterName).Add(time.Duration(1) * time.Millisecond)
	fmt.Printf("%v%v\nDatapoints: %v\n", scrapWork.serverURL, scrapWork.containerName, len(*sfxDatapointsPtr))

	sort.Sort(sortableDatapoint(*sfxDatapointsPtr))
	scrapWork.forwarder.AddDatapoints(scrapWork.ctx, *sfxDatapointsPtr)
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

	scrapWorkCache = make([]scrapWork2, 0, int32(len(p.cfg.CadvisorURL)+1))

	stop := make(chan error, 1)

	// Build list of work
	idx := 0
	for _, serverURL := range p.cfg.CadvisorURL {
		cadvisorClient, localERR := client.NewClient(serverURL)
		if localERR != nil {
			fmt.Printf("Failed connect to server: %v\n", localERR)
			continue
		}
		containerInfoRequest := info.DefaultContainerInfoRequest()
		containersInfo, localERR := cadvisorClient.AllDockerContainers(&containerInfoRequest)
		if localERR != nil {
			fmt.Printf("Failed get docker containers: %v\n", localERR)
			continue
		}
		for _, contInfo := range containersInfo {
			_, localERR := cadvisorClient.ContainerInfo(contInfo.Name, &containerInfoRequest)
			if localERR != nil {
				fmt.Printf("Error: %v\n", localERR)
				continue
			}
			fmt.Printf("Added container to monitor: %v\n", contInfo.Name)
			scrapWorkCache = append(scrapWorkCache, scrapWork2{
				ctx:           ctx,
				serverURL:     serverURL,
				containerName: contInfo.Name,
				lastTimestamp: time.Unix(0, 0),
				clusterName:   p.cfg.ClusterName,
				stop:          stop,
				forwarder:     p.forwarder,
			})
			idx = idx + 1
		}
	}

	fmt.Printf("Work size: %v\n", len(scrapWorkCache))
	if len(scrapWorkCache) == 0 {
		fmt.Printf("Nothing to do. Exiting.\n")
		return
	}

	workPool := workpool.New(runtime.NumCPU(), int32(len(scrapWorkCache)+1))

	ticker := time.NewTicker(paramDataSendRate)
	go func() {
		for t := range ticker.C {
			fmt.Printf("--------[ %v ]---------\n", t)
			for idx := range scrapWorkCache {
				workPool.PostWork("", &scrapWorkCache[idx])
			}
		}
	}()
	err = <-stop

	ticker.Stop()

	return err
}

func addMetric(ci *info.ContainerInfo, dst *[]*datapoint.Datapoint, clusterName string) (maxTimestamp time.Time) {
	dimsMap := make(map[string]string)
	dimsMap["name"] = ci.Name //Spec.Labels["io.kubernetes.pod.name"]
	dimsMap["namespace"] = ci.Namespace
	dimsMap["clusterName"] = clusterName

	for labelName, label := range ci.Spec.Labels {
		dimsMap[labelName] = label
	}

	maxTimestamp = time.Unix(0, 0)
	for _, cs := range ci.Stats {
		tt := cs.Timestamp

		if maxTimestamp.Unix() < tt.Unix() {
			maxTimestamp = tt
		}

		//Cpu
		addDp(dst, createGauge("Cpu_Usage_Total", dimsMap, datapoint.NewIntValue(int64(cs.Cpu.Usage.Total)), tt))
		fmt.Printf("Cpu_Usage_Total: %v tt: %v\n", cs.Cpu.Usage.Total, tt)
		addDp(dst, createGauge("Cpu_Usage_User", dimsMap, datapoint.NewIntValue(int64(cs.Cpu.Usage.User)), tt))
		addDp(dst, createGauge("Cpu_Usage_System", dimsMap, datapoint.NewIntValue(int64(cs.Cpu.Usage.System)), tt))
		addDp(dst, createGauge("Cpu_LoadAverage", dimsMap, datapoint.NewIntValue(int64(cs.Cpu.LoadAverage)), tt))
		for i, v := range cs.Cpu.Usage.PerCpu {
			newDims := make(map[string]string, len(dimsMap)+1)
			for k, v := range dimsMap {
				newDims[k] = v
			}
			newDims["cpu_id"] = strconv.Itoa(i)
			addDp(dst, createGauge("Cpu_Usage_PerCpu", newDims, datapoint.NewIntValue(int64(v)), tt))
		}

		//Memory
		addDp(dst, createGauge("Memory_Usage", dimsMap, datapoint.NewIntValue(int64(cs.Memory.Usage)), tt))
		addDp(dst, createGauge("Memory_WorkingSet", dimsMap, datapoint.NewIntValue(int64(cs.Memory.WorkingSet)), tt))
		addDp(dst, createGauge("Memory_Failcnt", dimsMap, datapoint.NewIntValue(int64(cs.Memory.Failcnt)), tt))
		addDp(dst, createGauge("Memory_ContainerData_Pgfault", dimsMap, datapoint.NewIntValue(int64(cs.Memory.ContainerData.Pgfault)), tt))
		addDp(dst, createGauge("Memory_ContainerData_Pgmajfault", dimsMap, datapoint.NewIntValue(int64(cs.Memory.ContainerData.Pgmajfault)), tt))
		addDp(dst, createGauge("Memory_HierarchicalData_Pgfault", dimsMap, datapoint.NewIntValue(int64(cs.Memory.HierarchicalData.Pgfault)), tt))
		addDp(dst, createGauge("Memory_HierarchicalData_Pgmajfault", dimsMap, datapoint.NewIntValue(int64(cs.Memory.HierarchicalData.Pgmajfault)), tt))

		//Task load stats
		addDp(dst, createCount("TaskStats_NrStopped", dimsMap, datapoint.NewIntValue(int64(cs.TaskStats.NrStopped)), tt))
		addDp(dst, createCount("TaskStats_NrUninterruptible", dimsMap, datapoint.NewIntValue(int64(cs.TaskStats.NrUninterruptible)), tt))
		addDp(dst, createCount("TaskStats_NrIoWait", dimsMap, datapoint.NewIntValue(int64(cs.TaskStats.NrIoWait)), tt))
		addDp(dst, createCount("TaskStats_NrSleeping", dimsMap, datapoint.NewIntValue(int64(cs.TaskStats.NrSleeping)), tt))
		addDp(dst, createCount("TaskStats_NrRunning", dimsMap, datapoint.NewIntValue(int64(cs.TaskStats.NrRunning)), tt))

		//Network
		newDims := make(map[string]string, len(dimsMap)+1)
		for k, v := range dimsMap {
			newDims[k] = v
		}
		newDims["interface"] = cs.Network.InterfaceStats.Name
		addDp(dst, createGauge("Network_InterfaceStats_RxBytes", newDims, datapoint.NewIntValue(int64(cs.Network.InterfaceStats.RxBytes)), tt))
		addDp(dst, createGauge("Network_InterfaceStats_RxPackets", newDims, datapoint.NewIntValue(int64(cs.Network.InterfaceStats.RxPackets)), tt))
		addDp(dst, createGauge("Network_InterfaceStats_RxErrors", newDims, datapoint.NewIntValue(int64(cs.Network.InterfaceStats.RxErrors)), tt))
		addDp(dst, createGauge("Network_InterfaceStats_RxDropped", newDims, datapoint.NewIntValue(int64(cs.Network.InterfaceStats.RxDropped)), tt))
		addDp(dst, createGauge("Network_InterfaceStats_TxBytes", newDims, datapoint.NewIntValue(int64(cs.Network.InterfaceStats.TxBytes)), tt))
		addDp(dst, createGauge("Network_InterfaceStats_TxPackets", newDims, datapoint.NewIntValue(int64(cs.Network.InterfaceStats.TxPackets)), tt))
		addDp(dst, createGauge("Network_InterfaceStats_TxErrors", newDims, datapoint.NewIntValue(int64(cs.Network.InterfaceStats.TxErrors)), tt))
		addDp(dst, createGauge("Network_InterfaceStats_TxDropped", newDims, datapoint.NewIntValue(int64(cs.Network.InterfaceStats.TxDropped)), tt))

		//Tcp
		makeTCPStatDatapoints(&cs.Network.Tcp, dst, newDims, tt)

		//Tcp6
		makeTCPStatDatapoints(&cs.Network.Tcp6, dst, newDims, tt)

		if cs.Network.Interfaces != nil {
			for _, val := range cs.Network.Interfaces {
				newDims = make(map[string]string, len(dimsMap)+1)
				for k, v := range dimsMap {
					newDims[k] = v
				}
				newDims["interface"] = val.Name
				makeInterfaceStatDatapoints(&val, dst, newDims, tt)
			}
		}

		//Filesystem
		for _, fsStat := range cs.Filesystem {
			newDims := make(map[string]string, len(dimsMap)+1)
			for k, v := range dimsMap {
				newDims[k] = v
			}
			newDims["device"] = fsStat.Device

			addDp(dst, createGauge("Filesystem_Limit", newDims, datapoint.NewIntValue(int64(fsStat.Limit)), tt))
			addDp(dst, createGauge("Filesystem_Usage", newDims, datapoint.NewIntValue(int64(fsStat.Usage)), tt))
			addDp(dst, createGauge("Filesystem_Available", newDims, datapoint.NewIntValue(int64(fsStat.Available)), tt))
			addDp(dst, createCount("Filesystem_ReadsCompleted", newDims, datapoint.NewIntValue(int64(fsStat.ReadsCompleted)), tt))
			addDp(dst, createCount("Filesystem_ReadsMerged", newDims, datapoint.NewIntValue(int64(fsStat.ReadsMerged)), tt))
			addDp(dst, createCount("Filesystem_SectorsRead", newDims, datapoint.NewIntValue(int64(fsStat.SectorsRead)), tt))
			addDp(dst, createCount("Filesystem_ReadTime", newDims, datapoint.NewIntValue(int64(fsStat.ReadTime)), tt))
			addDp(dst, createCount("Filesystem_WritesCompleted", newDims, datapoint.NewIntValue(int64(fsStat.WritesCompleted)), tt))
			addDp(dst, createCount("Filesystem_WritesMerged", newDims, datapoint.NewIntValue(int64(fsStat.WritesMerged)), tt))
			addDp(dst, createCount("Filesystem_SectorsWritten", newDims, datapoint.NewIntValue(int64(fsStat.SectorsWritten)), tt))
			addDp(dst, createCount("Filesystem_WriteTime", newDims, datapoint.NewIntValue(int64(fsStat.WriteTime)), tt))
			addDp(dst, createGauge("Filesystem_IoInProgress", newDims, datapoint.NewIntValue(int64(fsStat.IoInProgress)), tt))
			addDp(dst, createGauge("Filesystem_IoTime", newDims, datapoint.NewIntValue(int64(fsStat.IoTime)), tt))
			addDp(dst, createGauge("Filesystem_WeightedIoTime", newDims, datapoint.NewIntValue(int64(fsStat.WeightedIoTime)), tt))
		}

		//CustomMetrics
		/*if customMetricSpec != nil {
			for name, cm := range cs.CustomMetrics {
				//TODO: Maybe lookup should be done by cm.Label
				if metricSpec, ok := customMetricSpec[name]; ok {
					var value datapoint.Value
					if metricSpec.Format == info.IntType {
						value = datapoint.NewIntValue(int64(cm.IntValue))
					} else {
						value = datapoint.NewFloatValue(int64(cm.FloatValue))
					}
					metricName := fmt.Sprintf("CustomMetrics_%v", cm.Label)

					switch metricSpec.Type {
					case info.MetricGauge:
						addDp(dst, createGauge(metricName, dimsMap, value, cm.Timestamp))
					case info.MetricCumulative:
						addDp(dst, createCount(metricName, dimsMap, value, cm.Timestamp))
					case info.MetricDelta:
						addDp(dst, createRate(metricName, dimsMap, value, cm.Timestamp))
					}
				}
			}
		}*/

	}

	return maxTimestamp
}

func makeInterfaceStatDatapoints(stat *info.InterfaceStats, dst *[]*datapoint.Datapoint, dimsMap map[string]string, tt time.Time) {
	addDp(dst, createGauge("Network_InterfaceStats_RxBytes", dimsMap, datapoint.NewIntValue(int64(stat.RxBytes)), tt))
	addDp(dst, createGauge("Network_InterfaceStats_RxPackets", dimsMap, datapoint.NewIntValue(int64(stat.RxPackets)), tt))
	addDp(dst, createGauge("Network_InterfaceStats_RxErrors", dimsMap, datapoint.NewIntValue(int64(stat.RxErrors)), tt))
	addDp(dst, createGauge("Network_InterfaceStats_RxDropped", dimsMap, datapoint.NewIntValue(int64(stat.RxDropped)), tt))
	addDp(dst, createGauge("Network_InterfaceStats_TxBytes", dimsMap, datapoint.NewIntValue(int64(stat.TxBytes)), tt))
	addDp(dst, createGauge("Network_InterfaceStats_TxPackets", dimsMap, datapoint.NewIntValue(int64(stat.TxPackets)), tt))
	addDp(dst, createGauge("Network_InterfaceStats_TxErrors", dimsMap, datapoint.NewIntValue(int64(stat.TxErrors)), tt))
	addDp(dst, createGauge("Network_InterfaceStats_TxDropped", dimsMap, datapoint.NewIntValue(int64(stat.TxDropped)), tt))
}

func makeTCPStatDatapoints(stat *info.TcpStat, dst *[]*datapoint.Datapoint, dimsMap map[string]string, tt time.Time) {
	addDp(dst, createCount("Network_Tcp_FinWait2", dimsMap, datapoint.NewIntValue(int64(stat.FinWait2)), tt))
	addDp(dst, createCount("Network_Tcp_TimeWait", dimsMap, datapoint.NewIntValue(int64(stat.TimeWait)), tt))
	addDp(dst, createCount("Network_Tcp_Established", dimsMap, datapoint.NewIntValue(int64(stat.Established)), tt))
	addDp(dst, createCount("Network_Tcp_SynSent", dimsMap, datapoint.NewIntValue(int64(stat.SynSent)), tt))
	addDp(dst, createCount("Network_Tcp_Close", dimsMap, datapoint.NewIntValue(int64(stat.Close)), tt))
	addDp(dst, createCount("Network_Tcp_CloseWait", dimsMap, datapoint.NewIntValue(int64(stat.CloseWait)), tt))
	addDp(dst, createCount("Network_Tcp_LastAck", dimsMap, datapoint.NewIntValue(int64(stat.LastAck)), tt))
	addDp(dst, createCount("Network_Tcp_Listen", dimsMap, datapoint.NewIntValue(int64(stat.Listen)), tt))
	addDp(dst, createCount("Network_Tcp_Closing", dimsMap, datapoint.NewIntValue(int64(stat.Closing)), tt))
	addDp(dst, createCount("Network_Tcp_SynRecv", dimsMap, datapoint.NewIntValue(int64(stat.SynRecv)), tt))
	addDp(dst, createCount("Network_Tcp_FinWait1", dimsMap, datapoint.NewIntValue(int64(stat.FinWait1)), tt))
}

func addDp(dst *[]*datapoint.Datapoint, dp *datapoint.Datapoint) {
	*dst = append(*dst, dp)
}

func createGauge(metric string, dimensions map[string]string, value datapoint.Value, timestamp time.Time) *datapoint.Datapoint {
	return datapoint.New(metric, dimensions, value, datapoint.Gauge, timestamp)
}

/*func createCounter(metric string, dimensions map[string]string, value datapoint.Value, timestamp time.Time) *datapoint.Datapoint {
	return datapoint.New(metric, dimensions, value, datapoint.Counter, timestamp)
}*/

/*func createRate(metric string, dimensions map[string]string, value datapoint.Value, timestamp time.Time) *datapoint.Datapoint {
	return datapoint.New(metric, dimensions, value, datapoint.Rate, timestamp)
}*/

/*func createEnum(metric string, dimensions map[string]string, value datapoint.Value, timestamp time.Time) *datapoint.Datapoint {
	return datapoint.New(metric, dimensions, value, datapoint.Enum, timestamp)
}*/

func createCount(metric string, dimensions map[string]string, value datapoint.Value, timestamp time.Time) *datapoint.Datapoint {
	return datapoint.New(metric, dimensions, value, datapoint.Count, timestamp)
}
