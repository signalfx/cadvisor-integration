package converter

import (
	"strconv"
	"testing"
	"time"

	info "github.com/google/cadvisor/info/v1"
	"github.com/signalfx/golib/datapoint"
)

func Test_getContainerSpecMemMetrics(t *testing.T) {
	type args struct {
		excludedMetrics map[string]bool
	}
	tests := []struct {
		name string
		args args
		want []containerSpecMetric
	}{
		{
			"exclude all elements",
			args{
				map[string]bool{
					"container_spec_memory_limit_bytes":      true,
					"container_spec_memory_swap_limit_bytes": true,
				},
			},
			[]containerSpecMetric{},
		},
		{
			"include all elements",
			args{
				map[string]bool{},
			},
			[]containerSpecMetric{
				{
					containerMetric: containerMetric{
						name:        "container_spec_memory_limit_bytes",
						help:        "",
						valueType:   datapoint.Gauge,
						extraLabels: []string{},
					},
					getValues: func(container *info.ContainerInfo) metricValues {
						return metricValues{{value: datapoint.NewIntValue(int64(container.Spec.Memory.Limit))}}
					},
				},
				{
					containerMetric: containerMetric{
						name:        "container_spec_memory_swap_limit_bytes",
						help:        "",
						valueType:   datapoint.Gauge,
						extraLabels: []string{},
					},
					getValues: func(container *info.ContainerInfo) metricValues {
						return metricValues{{value: datapoint.NewIntValue(int64(container.Spec.Memory.SwapLimit))}}
					},
				},
			},
		},
		{
			"remove 1 element",
			args{
				map[string]bool{
					"container_spec_memory_swap_limit_bytes": true,
				},
			},
			[]containerSpecMetric{
				{
					containerMetric: containerMetric{
						name:        "container_spec_memory_limit_bytes",
						help:        "",
						valueType:   datapoint.Gauge,
						extraLabels: []string{},
					},
					getValues: func(container *info.ContainerInfo) metricValues {
						return metricValues{{value: datapoint.NewIntValue(int64(container.Spec.Memory.Limit))}}
					},
				},
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var metrics = getContainerSpecMemMetrics(tt.args.excludedMetrics)
			if len(metrics) == len(tt.want) {
				for _, metric := range metrics {
					isIn := false
					for _, expected := range tt.want {
						if metric.name == expected.name {
							isIn = true
						}
					}
					if !isIn {
						t.Errorf("getContainerSpecMemMetrics() = %v, want %v", metrics, tt.want)
					}
				}
			} else {
				t.Errorf("getContainerSpecMemMetrics() = %v, want %v", metrics, tt.want)
			}
		})
	}
}

func Test_getMachineInfoMetrics(t *testing.T) {
	type args struct {
		excludedMetrics map[string]bool
	}
	tests := []struct {
		name string
		args args
		want []machineInfoMetric
	}{
		{
			"exclude all elements",
			args{
				map[string]bool{
					"machine_cpu_frequency_khz": true,
					"machine_cpu_cores":         true,
					"machine_memory_bytes":      true,
				},
			},
			[]machineInfoMetric{},
		},
		{
			"remove 1 element",
			args{
				map[string]bool{
					"machine_cpu_frequency_khz": true,
				},
			},
			[]machineInfoMetric{
				{
					containerMetric: containerMetric{
						name:        "machine_cpu_cores",
						help:        "",
						valueType:   datapoint.Gauge,
						extraLabels: []string{},
					},
					getValues: func(machineInfo *info.MachineInfo) metricValues {
						return metricValues{{value: datapoint.NewIntValue(int64(machineInfo.NumCores))}}
					},
				},
				{
					containerMetric: containerMetric{
						name:        "machine_memory_bytes",
						help:        "",
						valueType:   datapoint.Gauge,
						extraLabels: []string{},
					},
					getValues: func(machineInfo *info.MachineInfo) metricValues {
						return metricValues{{value: datapoint.NewIntValue(int64(machineInfo.MemoryCapacity))}}
					},
				},
			},
		},
		{
			"include all elements",
			args{
				map[string]bool{},
			},
			[]machineInfoMetric{
				{
					containerMetric: containerMetric{
						name:        "machine_cpu_frequency_khz",
						help:        "",
						valueType:   datapoint.Gauge,
						extraLabels: []string{},
					},
					getValues: func(machineInfo *info.MachineInfo) metricValues {
						return metricValues{{value: datapoint.NewIntValue(int64(machineInfo.CpuFrequency))}}
					},
				},
				{
					containerMetric: containerMetric{
						name:        "machine_cpu_cores",
						help:        "",
						valueType:   datapoint.Gauge,
						extraLabels: []string{},
					},
					getValues: func(machineInfo *info.MachineInfo) metricValues {
						return metricValues{{value: datapoint.NewIntValue(int64(machineInfo.NumCores))}}
					},
				},
				{
					containerMetric: containerMetric{
						name:        "machine_memory_bytes",
						help:        "",
						valueType:   datapoint.Gauge,
						extraLabels: []string{},
					},
					getValues: func(machineInfo *info.MachineInfo) metricValues {
						return metricValues{{value: datapoint.NewIntValue(int64(machineInfo.MemoryCapacity))}}
					},
				},
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var metrics = getMachineInfoMetrics(tt.args.excludedMetrics)
			if len(metrics) == len(tt.want) {
				for _, metric := range metrics {
					isIn := false
					for _, expected := range tt.want {
						if metric.name == expected.name {
							isIn = true
						}
					}
					if !isIn {
						t.Errorf("getMachineInfoMetrics() = %v, want %v", metrics, tt.want)
					}
				}
			} else {
				t.Errorf("getMachineInfoMetrics() = %v, want %v", metrics, tt.want)

			}
		})
	}
}

func Test_getContainerSpecMetrics(t *testing.T) {
	type args struct {
		excludedMetrics map[string]bool
	}
	tests := []struct {
		name string
		args args
		want []containerSpecMetric
	}{
		{
			"exclude all elements",
			args{
				map[string]bool{
					"container_start_time_seconds": true,
				},
			},
			[]containerSpecMetric{},
		},
		{
			"include all elements",
			args{
				map[string]bool{},
			},
			[]containerSpecMetric{
				{
					containerMetric: containerMetric{
						name:        "container_start_time_seconds",
						help:        "",
						valueType:   datapoint.Gauge,
						extraLabels: []string{},
					},
					getValues: func(container *info.ContainerInfo) metricValues {
						return metricValues{{value: datapoint.NewIntValue(container.Spec.CreationTime.Unix())}}
					},
				},
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var metrics = getContainerSpecMetrics(tt.args.excludedMetrics)
			if len(metrics) == len(tt.want) {
				for _, metric := range metrics {
					isIn := false
					for _, expected := range tt.want {
						if metric.name == expected.name {
							isIn = true
						}
					}
					if !isIn {
						t.Errorf("getContainerSpecMetrics() = %v, want %v", metrics, tt.want)
					}
				}
			} else {
				t.Errorf("getContainerSpecMetrics() = %v, want %v", metrics, tt.want)
			}
		})
	}
}

func Test_getContainerSpecCPUMetrics(t *testing.T) {
	type args struct {
		excludedMetrics map[string]bool
	}
	tests := []struct {
		name string
		args args
		want []containerSpecMetric
	}{
		{
			"exclude all elements",
			args{
				map[string]bool{
					"container_spec_cpu_shares": true,
				},
			},
			[]containerSpecMetric{},
		},
		{
			"exclude 1 elements",
			args{
				map[string]bool{
					"container_spec_cpu_shares": true,
				},
			},
			[]containerSpecMetric{},
		},
		{
			"include all elements",
			args{
				map[string]bool{},
			},
			[]containerSpecMetric{
				{
					containerMetric: containerMetric{
						name:        "container_spec_cpu_shares",
						help:        "",
						valueType:   datapoint.Gauge,
						extraLabels: []string{},
					},
					getValues: func(container *info.ContainerInfo) metricValues {
						return metricValues{{value: datapoint.NewIntValue(container.Spec.CreationTime.Unix())}}
					},
				},
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var metrics = getContainerSpecCPUMetrics(tt.args.excludedMetrics)
			if len(metrics) == len(tt.want) {
				for _, metric := range metrics {
					isIn := false
					for _, expected := range tt.want {
						if metric.name == expected.name {
							isIn = true
						}
					}
					if !isIn {
						t.Errorf("getContainerSpecCPUMetrics() = %v, want %v", metrics, tt.want)
					}
				}
			} else {
				t.Errorf("getContainerSpecCPUMetrics() = %v, want %v", metrics, tt.want)
			}
		})
	}
}

func Test_getContainerMetrics(t *testing.T) {
	type args struct {
		excludedMetrics map[string]bool
	}
	tests := []struct {
		name string
		args args
		want []containerMetric
	}{
		{
			"exclude all elements",
			args{
				map[string]bool{
					"container_last_seen":                         true,
					"container_cpu_user_seconds_total":            true,
					"container_cpu_system_seconds_total":          true,
					"container_cpu_usage_seconds_total":           true,
					"container_cpu_utilization":                   true,
					"container_cpu_utilization_per_core":          true,
					"container_memory_failcnt":                    true,
					"container_memory_usage_bytes":                true,
					"container_memory_working_set_bytes":          true,
					"container_memory_failures_total":             true,
					"container_fs_limit_bytes":                    true,
					"container_fs_usage_bytes":                    true,
					"container_fs_reads_total":                    true,
					"container_fs_sector_reads_total":             true,
					"container_fs_reads_merged_total":             true,
					"container_fs_read_seconds_total":             true,
					"container_fs_writes_total":                   true,
					"container_fs_sector_writes_total":            true,
					"container_fs_writes_merged_total":            true,
					"container_fs_write_seconds_total":            true,
					"container_fs_io_current":                     true,
					"container_fs_io_time_seconds_total":          true,
					"container_fs_io_time_weighted_seconds_total": true,
					"pod_network_receive_bytes_total":             true,
					"pod_network_receive_packets_total":           true,
					"pod_network_receive_packets_dropped_total":   true,
					"pod_network_receive_errors_total":            true,
					"pod_network_transmit_bytes_total":            true,
					"pod_network_transmit_packets_total":          true,
					"pod_network_transmit_packets_dropped_total":  true,
					"pod_network_transmit_errors_total":           true,
					"container_tasks_state":                       true,
				},
			},
			[]containerMetric{},
		},
		{
			"exclude 1 element",
			args{
				map[string]bool{
					"container_fs_limit_bytes": true,
				},
			},
			[]containerMetric{
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
		},
		{
			"include all elements",
			args{
				map[string]bool{},
			},
			[]containerMetric{
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
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var metrics = getContainerMetrics(tt.args.excludedMetrics)
			if len(metrics) == len(tt.want) {
				for _, metric := range metrics {
					isIn := false
					for _, expected := range tt.want {
						if metric.name == expected.name {
							isIn = true
						}
					}
					if !isIn {
						t.Errorf("getContainerMetrics() = %v, want %v", metrics, tt.want)
					}
				}
			} else {
				t.Errorf("getContainerMetrics() = %v, want %v", metrics, tt.want)
			}
		})
	}
}
