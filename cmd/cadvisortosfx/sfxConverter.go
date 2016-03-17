package main

import (
	"strconv"
	"time"

	"github.com/signalfx/golib/datapoint"

	"github.com/golang/glog"
	info "github.com/google/cadvisor/info/v1"
)

// This will usually be manager.Manager, but can be swapped out for testing.
type infoProvider interface {
	// Get information about all subcontainers of the specified container (includes self).
	SubcontainersInfo(containerName string) ([]info.ContainerInfo, error)
	// Get information about the machine.
	GetMachineInfo() (*info.MachineInfo, error)
}

// metricValue describes a single metric value for a given set of label values
// within a parent containerMetric.
type metricValue struct {
	value  datapoint.Value
	labels []string
}

type metricValues []metricValue

type containerMetric struct {
	name        string
	help        string
	valueType   datapoint.MetricType
	extraLabels []string
	getValues   func(s *info.ContainerStats) metricValues
}

// ContainerNameToLabelsFunc converter function
type ContainerNameToLabelsFunc func(containerName string) map[string]string

// CadvisorCollector metric collector and converter
type CadvisorCollector struct {
	infoProvider          infoProvider
	containerMetrics      []containerMetric
	containerNameToLabels ContainerNameToLabelsFunc
}

// fsValues is a helper method for assembling per-filesystem stats.
func fsValues(fsStats []info.FsStats, valueFn func(*info.FsStats) datapoint.Value) metricValues {
	values := make(metricValues, 0, len(fsStats))
	for _, stat := range fsStats {
		values = append(values, metricValue{
			value:  valueFn(&stat),
			labels: []string{stat.Device},
		})
	}
	return values
}

func networkValues(net []info.InterfaceStats, valueFn func(*info.InterfaceStats) datapoint.Value) metricValues {
	values := make(metricValues, 0, len(net))
	for _, value := range net {
		values = append(values, metricValue{
			value:  valueFn(&value),
			labels: []string{value.Name},
		})
	}
	return values
}

// NewCadvisorCollector creates new CadvisorCollector
func NewCadvisorCollector(infoProvider infoProvider, f ContainerNameToLabelsFunc) *CadvisorCollector {
	return &CadvisorCollector{
		infoProvider:          infoProvider,
		containerNameToLabels: f,
		containerMetrics: []containerMetric{
			{
				name:      "container_last_seen",
				help:      "Last time a container was seen by the exporter",
				valueType: datapoint.Timestamp,
				getValues: func(s *info.ContainerStats) metricValues {
					return metricValues{{value: datapoint.NewIntValue(time.Now().UnixNano())}}
				},
			},
			{
				name:      "container_cpu_user_seconds_total",
				help:      "Cumulative user cpu time consumed in nanoseconds.",
				valueType: datapoint.Counter,
				getValues: func(s *info.ContainerStats) metricValues {
					return metricValues{{value: datapoint.NewIntValue(int64(s.Cpu.Usage.User))}}
				},
			},
			{
				name:      "container_cpu_system_seconds_total",
				help:      "Cumulative system cpu time consumed in nanoseconds.",
				valueType: datapoint.Counter,
				getValues: func(s *info.ContainerStats) metricValues {
					return metricValues{{value: datapoint.NewIntValue(int64(s.Cpu.Usage.System))}}
				},
			},
			{
				name:      "container_cpu_usage_seconds_total",
				help:      "Cumulative cpu time consumed per cpu in nanoseconds.",
				valueType: datapoint.Counter,
				getValues: func(s *info.ContainerStats) metricValues {
					return metricValues{{value: datapoint.NewIntValue(int64(s.Cpu.Usage.Total))}}
				},
			},
			{
				name:      "container_cpu_utilization",
				help:      "Cumulative cpu utilization in percentages.",
				valueType: datapoint.Counter,
				getValues: func(s *info.ContainerStats) metricValues {
					return metricValues{{value: datapoint.NewIntValue(int64(s.Cpu.Usage.Total / 10000000))}}
				},
			},
			{
				name:        "container_cpu_utilization_per_core",
				help:        "Cumulative cpu utilization in percentages per core",
				valueType:   datapoint.Counter,
				extraLabels: []string{"cpu"},
				getValues: func(s *info.ContainerStats) metricValues {
					metricValues := make(metricValues, len(s.Cpu.Usage.PerCpu))
					for index, coreUsage := range s.Cpu.Usage.PerCpu {
						if coreUsage > 0 {
							metricValues[index] = metricValue{value: datapoint.NewIntValue(int64(coreUsage / 10000000)), labels: []string{"cpu" + strconv.Itoa(index)}}
						} else {
							metricValues[index] = metricValue{value: datapoint.NewIntValue(int64(0)), labels: []string{strconv.Itoa(index)}}
						}
					}
					return metricValues
				},
			},
			{
				name:      "container_memory_failcnt",
				help:      "Number of memory usage hits limits",
				valueType: datapoint.Counter,
				getValues: func(s *info.ContainerStats) metricValues {
					return metricValues{{value: datapoint.NewIntValue(int64(s.Memory.Failcnt))}}
				},
			},
			{
				name:      "container_memory_usage_bytes",
				help:      "Current memory usage in bytes.",
				valueType: datapoint.Gauge,
				getValues: func(s *info.ContainerStats) metricValues {
					return metricValues{{value: datapoint.NewIntValue(int64(s.Memory.Usage))}}
				},
			},
			{
				name:      "container_memory_working_set_bytes",
				help:      "Current working set in bytes.",
				valueType: datapoint.Gauge,
				getValues: func(s *info.ContainerStats) metricValues {
					return metricValues{{value: datapoint.NewIntValue(int64(s.Memory.WorkingSet))}}
				},
			},
			{
				name:        "container_memory_failures_total",
				help:        "Cumulative count of memory allocation failures.",
				valueType:   datapoint.Counter,
				extraLabels: []string{"type", "scope"},
				getValues: func(s *info.ContainerStats) metricValues {
					return metricValues{
						{
							value:  datapoint.NewIntValue(int64(s.Memory.ContainerData.Pgfault)),
							labels: []string{"pgfault", "container"},
						},
						{
							value:  datapoint.NewIntValue(int64(s.Memory.ContainerData.Pgmajfault)),
							labels: []string{"pgmajfault", "container"},
						},
						{
							value:  datapoint.NewIntValue(int64(s.Memory.HierarchicalData.Pgfault)),
							labels: []string{"pgfault", "hierarchy"},
						},
						{
							value:  datapoint.NewIntValue(int64(s.Memory.HierarchicalData.Pgmajfault)),
							labels: []string{"pgmajfault", "hierarchy"},
						},
					}
				},
			},
			{
				name:        "container_fs_limit_bytes",
				help:        "Number of bytes that can be consumed by the container on this filesystem.",
				valueType:   datapoint.Gauge,
				extraLabels: []string{"device"},
				getValues: func(s *info.ContainerStats) metricValues {
					return fsValues(s.Filesystem, func(fs *info.FsStats) datapoint.Value {
						return datapoint.NewIntValue(int64(fs.Limit))
					})
				},
			},
			{
				name:        "container_fs_usage_bytes",
				help:        "Number of bytes that are consumed by the container on this filesystem.",
				valueType:   datapoint.Gauge,
				extraLabels: []string{"device"},
				getValues: func(s *info.ContainerStats) metricValues {
					return fsValues(s.Filesystem, func(fs *info.FsStats) datapoint.Value {
						return datapoint.NewIntValue(int64(fs.Usage))
					})
				},
			},
			{
				name:        "container_fs_reads_total",
				help:        "Cumulative count of reads completed",
				valueType:   datapoint.Gauge,
				extraLabels: []string{"device"},
				getValues: func(s *info.ContainerStats) metricValues {
					return fsValues(s.Filesystem, func(fs *info.FsStats) datapoint.Value {
						return datapoint.NewIntValue(int64(fs.ReadsCompleted))
					})
				},
			},
			{
				name:        "container_fs_sector_reads_total",
				help:        "Cumulative count of sector reads completed",
				valueType:   datapoint.Counter,
				extraLabels: []string{"device"},
				getValues: func(s *info.ContainerStats) metricValues {
					return fsValues(s.Filesystem, func(fs *info.FsStats) datapoint.Value {
						return datapoint.NewIntValue(int64(fs.SectorsRead))
					})
				},
			},
			{
				name:        "container_fs_reads_merged_total",
				help:        "Cumulative count of reads merged",
				valueType:   datapoint.Counter,
				extraLabels: []string{"device"},
				getValues: func(s *info.ContainerStats) metricValues {
					return fsValues(s.Filesystem, func(fs *info.FsStats) datapoint.Value {
						return datapoint.NewIntValue(int64(fs.ReadsMerged))
					})
				},
			},
			{
				name:        "container_fs_read_seconds_total",
				help:        "Cumulative count of seconds spent reading",
				valueType:   datapoint.Counter,
				extraLabels: []string{"device"},
				getValues: func(s *info.ContainerStats) metricValues {
					return fsValues(s.Filesystem, func(fs *info.FsStats) datapoint.Value {
						return datapoint.NewIntValue(int64(fs.ReadTime / uint64(time.Second)))
					})
				},
			},
			{
				name:        "container_fs_writes_total",
				help:        "Cumulative count of writes completed",
				valueType:   datapoint.Counter,
				extraLabels: []string{"device"},
				getValues: func(s *info.ContainerStats) metricValues {
					return fsValues(s.Filesystem, func(fs *info.FsStats) datapoint.Value {
						return datapoint.NewIntValue(int64(fs.WritesCompleted))
					})
				},
			},
			{
				name:        "container_fs_sector_writes_total",
				help:        "Cumulative count of sector writes completed",
				valueType:   datapoint.Counter,
				extraLabels: []string{"device"},
				getValues: func(s *info.ContainerStats) metricValues {
					return fsValues(s.Filesystem, func(fs *info.FsStats) datapoint.Value {
						return datapoint.NewIntValue(int64(fs.SectorsWritten))
					})
				},
			},
			{
				name:        "container_fs_writes_merged_total",
				help:        "Cumulative count of writes merged",
				valueType:   datapoint.Counter,
				extraLabels: []string{"device"},
				getValues: func(s *info.ContainerStats) metricValues {
					return fsValues(s.Filesystem, func(fs *info.FsStats) datapoint.Value {
						return datapoint.NewIntValue(int64(fs.WritesMerged))
					})
				},
			},
			{
				name:        "container_fs_write_seconds_total",
				help:        "Cumulative count of seconds spent writing",
				valueType:   datapoint.Counter,
				extraLabels: []string{"device"},
				getValues: func(s *info.ContainerStats) metricValues {
					return fsValues(s.Filesystem, func(fs *info.FsStats) datapoint.Value {
						return datapoint.NewIntValue(int64(fs.WriteTime / uint64(time.Second)))
					})
				},
			},
			{
				name:        "container_fs_io_current",
				help:        "Number of I/Os currently in progress",
				valueType:   datapoint.Gauge,
				extraLabels: []string{"device"},
				getValues: func(s *info.ContainerStats) metricValues {
					return fsValues(s.Filesystem, func(fs *info.FsStats) datapoint.Value {
						return datapoint.NewIntValue(int64(fs.IoInProgress))
					})
				},
			},
			{
				name:        "container_fs_io_time_seconds_total",
				help:        "Cumulative count of seconds spent doing I/Os",
				valueType:   datapoint.Counter,
				extraLabels: []string{"device"},
				getValues: func(s *info.ContainerStats) metricValues {
					return fsValues(s.Filesystem, func(fs *info.FsStats) datapoint.Value {
						return datapoint.NewIntValue(int64(fs.IoTime / uint64(time.Second)))
					})
				},
			},
			{
				name:        "container_fs_io_time_weighted_seconds_total",
				help:        "Cumulative weighted I/O time in seconds",
				valueType:   datapoint.Counter,
				extraLabels: []string{"device"},
				getValues: func(s *info.ContainerStats) metricValues {
					return fsValues(s.Filesystem, func(fs *info.FsStats) datapoint.Value {
						return datapoint.NewIntValue(int64(fs.WeightedIoTime / uint64(time.Second)))
					})
				},
			},
			{
				name:        "pod_network_receive_bytes_total",
				help:        "Cumulative count of bytes received",
				valueType:   datapoint.Counter,
				extraLabels: []string{"interface"},
				getValues: func(s *info.ContainerStats) metricValues {
					return networkValues(s.Network.Interfaces, func(is *info.InterfaceStats) datapoint.Value {
						return datapoint.NewIntValue(int64(is.RxBytes))
					})
				},
			},
			{
				name:        "pod_network_receive_packets_total",
				help:        "Cumulative count of packets received",
				valueType:   datapoint.Counter,
				extraLabels: []string{"interface"},
				getValues: func(s *info.ContainerStats) metricValues {
					return networkValues(s.Network.Interfaces, func(is *info.InterfaceStats) datapoint.Value {
						return datapoint.NewIntValue(int64(is.RxPackets))
					})
				},
			},
			{
				name:        "pod_network_receive_packets_dropped_total",
				help:        "Cumulative count of packets dropped while receiving",
				valueType:   datapoint.Counter,
				extraLabels: []string{"interface"},
				getValues: func(s *info.ContainerStats) metricValues {
					return networkValues(s.Network.Interfaces, func(is *info.InterfaceStats) datapoint.Value {
						return datapoint.NewIntValue(int64(is.RxDropped))
					})
				},
			},
			{
				name:        "pod_network_receive_errors_total",
				help:        "Cumulative count of errors encountered while receiving",
				valueType:   datapoint.Counter,
				extraLabels: []string{"interface"},
				getValues: func(s *info.ContainerStats) metricValues {
					return networkValues(s.Network.Interfaces, func(is *info.InterfaceStats) datapoint.Value {
						return datapoint.NewIntValue(int64(is.RxErrors))
					})
				},
			},
			{
				name:        "pod_network_transmit_bytes_total",
				help:        "Cumulative count of bytes transmitted",
				valueType:   datapoint.Counter,
				extraLabels: []string{"interface"},
				getValues: func(s *info.ContainerStats) metricValues {
					return networkValues(s.Network.Interfaces, func(is *info.InterfaceStats) datapoint.Value {
						return datapoint.NewIntValue(int64(is.TxBytes))
					})
				},
			},
			{
				name:        "pod_network_transmit_packets_total",
				help:        "Cumulative count of packets transmitted",
				valueType:   datapoint.Counter,
				extraLabels: []string{"interface"},
				getValues: func(s *info.ContainerStats) metricValues {
					return networkValues(s.Network.Interfaces, func(is *info.InterfaceStats) datapoint.Value {
						return datapoint.NewIntValue(int64(is.TxPackets))
					})
				},
			},
			{
				name:        "pod_network_transmit_packets_dropped_total",
				help:        "Cumulative count of packets dropped while transmitting",
				valueType:   datapoint.Counter,
				extraLabels: []string{"interface"},
				getValues: func(s *info.ContainerStats) metricValues {
					return networkValues(s.Network.Interfaces, func(is *info.InterfaceStats) datapoint.Value {
						return datapoint.NewIntValue(int64(is.TxDropped))
					})
				},
			},
			{
				name:        "pod_network_transmit_errors_total",
				help:        "Cumulative count of errors encountered while transmitting",
				valueType:   datapoint.Counter,
				extraLabels: []string{"interface"},
				getValues: func(s *info.ContainerStats) metricValues {
					return networkValues(s.Network.Interfaces, func(is *info.InterfaceStats) datapoint.Value {
						return datapoint.NewIntValue(int64(is.TxErrors))
					})
				},
			},
			{
				name:        "container_tasks_state",
				help:        "Number of tasks in given state",
				extraLabels: []string{"state"},
				valueType:   datapoint.Gauge,
				getValues: func(s *info.ContainerStats) metricValues {
					return metricValues{
						{
							value:  datapoint.NewIntValue(int64(s.TaskStats.NrSleeping)),
							labels: []string{"sleeping"},
						},
						{
							value:  datapoint.NewIntValue(int64(s.TaskStats.NrRunning)),
							labels: []string{"running"},
						},
						{
							value:  datapoint.NewIntValue(int64(s.TaskStats.NrStopped)),
							labels: []string{"stopped"},
						},
						{
							value:  datapoint.NewIntValue(int64(s.TaskStats.NrUninterruptible)),
							labels: []string{"uninterruptible"},
						},
						{
							value:  datapoint.NewIntValue(int64(s.TaskStats.NrIoWait)),
							labels: []string{"iowaiting"},
						},
					}
				},
			},
		},
	}
}

// Collect fetches the stats from all containers and delivers them as
// Prometheus metrics. It implements prometheus.PrometheusCollector.
func (c *CadvisorCollector) Collect(ch chan<- datapoint.Datapoint) {
	c.collectMachineInfo(ch)
	c.collectVersionInfo(ch)
	c.collectContainersInfo(ch)
	//c.errors.Collect(ch)
}

func copyDims(dims map[string]string) map[string]string {
	newMap := make(map[string]string)
	for k, v := range dims {
		newMap[k] = v
	}
	return newMap
}

func (c *CadvisorCollector) collectContainersInfo(ch chan<- datapoint.Datapoint) {
	containers, err := c.infoProvider.SubcontainersInfo("/")
	if err != nil {
		//c.errors.Set(1)
		glog.Warningf("Couldn't get containers: %s", err)
		return
	}
	for _, container := range containers {
		dims := make(map[string]string)
		id := container.Name
		dims["id"] = id

		name := id
		if len(container.Aliases) > 0 {
			name = container.Aliases[0]
			dims["name"] = name
		}

		image := container.Spec.Image
		if len(image) > 0 {
			dims["image"] = image
		}

		if c.containerNameToLabels != nil {
			newLabels := c.containerNameToLabels(name)
			for k, v := range newLabels {
				dims[k] = v
			}
		}

		tt := time.Now()
		// Container spec
		// Start time of the container since unix epoch in seconds.
		ch <- *datapoint.New("container_start_time_seconds", copyDims(dims), datapoint.NewIntValue(container.Spec.CreationTime.Unix()), datapoint.Gauge, tt)

		if container.Spec.HasCpu {
			// CPU share of the container.
			ch <- *datapoint.New("container_spec_cpu_shares", copyDims(dims), datapoint.NewIntValue(int64(container.Spec.Cpu.Limit)), datapoint.Gauge, tt)
		}

		if container.Spec.HasMemory {
			// Memory limit for the container.
			ch <- *datapoint.New("container_spec_memory_limit_bytes", copyDims(dims), datapoint.NewIntValue(int64(container.Spec.Memory.Limit)), datapoint.Gauge, tt)
			// Memory swap limit for the container.
			ch <- *datapoint.New("container_spec_memory_swap_limit_bytes", copyDims(dims), datapoint.NewIntValue(int64(container.Spec.Memory.SwapLimit)), datapoint.Gauge, tt)
		}

		// Now for the actual metrics
		for _, stats := range container.Stats {
			for _, cm := range c.containerMetrics {
				for _, metricValue := range cm.getValues(stats) {
					newDims := copyDims(dims)

					// Add extra dimensions
					for i, label := range cm.extraLabels {
						newDims[label] = metricValue.labels[i]
					}

					ch <- *datapoint.New(cm.name, newDims, metricValue.value, cm.valueType, stats.Timestamp)
				}
			}
		}

	}
}

func (c *CadvisorCollector) collectVersionInfo(ch chan<- datapoint.Datapoint) {}

func (c *CadvisorCollector) collectMachineInfo(ch chan<- datapoint.Datapoint) {
	machineInfo, err := c.infoProvider.GetMachineInfo()
	if err != nil {
		//c.errors.Set(1)
		glog.Warningf("Couldn't get machine info: %s", err)
		return
	}
	tt := time.Now()

	// CPU frequency.
	ch <- *datapoint.New("machine_cpu_frequency_khz", make(map[string]string), datapoint.NewIntValue(int64(machineInfo.CpuFrequency)), datapoint.Gauge, tt)

	// Number of CPU cores on the machine.
	ch <- *datapoint.New("machine_cpu_cores", make(map[string]string), datapoint.NewIntValue(int64(machineInfo.NumCores)), datapoint.Gauge, tt)

	// Amount of memory installed on the machine.
	ch <- *datapoint.New("machine_memory_bytes", make(map[string]string), datapoint.NewIntValue(int64(machineInfo.MemoryCapacity)), datapoint.Gauge, tt)
}
