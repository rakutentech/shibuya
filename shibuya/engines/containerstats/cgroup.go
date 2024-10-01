package containerstats

import (
	"bufio"
	"fmt"
	"os"
	"strconv"
	"strings"
)

const (
	cgroupV2Path = "/sys/fs/cgroup/cgroup.controllers"
)

var (
	cgroupVersion      = ""
	noContentError     = "No content in the file %s"
	cpuByCgroupVersion = map[string]StatsParser{
		"v2": {
			path:   "/sys/fs/cgroup/cpu.stat",
			parser: readCPUStatFileV2,
		},
		"v1": {
			path:   "/sys/fs/cgroup/cpu,cpuacct/cpuacct.usage",
			parser: readCPUStatFileV1,
		},
	}
	memByCgroupVersion = map[string]StatsParser{
		"v2": {
			path:   "/sys/fs/cgroup/memory.current",
			parser: readMemoryStatFile,
		},
		"v1": {
			path:   "/sys/fs/cgroup/memory/memory.usage_in_bytes",
			parser: readMemoryStatFile,
		},
	}
)

type StatsParser struct {
	path   string
	parser func(path string) (uint64, error)
}

func (sp StatsParser) readUsage() (uint64, error) {
	return sp.parser(sp.path)
}

func detectCgroupVersion() string {
	if cgroupVersion != "" {
		return cgroupVersion
	}
	if _, err := os.Stat(cgroupV2Path); err == nil {
		cgroupVersion = "v2"
		return cgroupVersion
	}
	cgroupVersion = "v1"
	return cgroupVersion
}

func head(path string, nol int) ([]string, error) {
	file, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	scanner := bufio.NewScanner(file)
	scanned := 0
	r := []string{}
	for scanner.Scan() {
		r = append(r, scanner.Text())
		scanned += 1
		if scanned == nol {
			break
		}
	}
	return r, nil
}

func readCPUStatFileV1(path string) (uint64, error) {
	t, err := head(path, 1)
	if err != nil {
		return 0, err
	}
	if len(t) == 0 {
		return 0, fmt.Errorf(noContentError, path)
	}
	return strconv.ParseUint(t[0], 10, 64)
}

func readCPUStatFileV2(path string) (uint64, error) {
	t, err := head(path, 1)
	if err != nil {
		return 0, err
	}
	if len(t) == 0 {
		return 0, fmt.Errorf(noContentError, path)
	}
	firstLine := t[0]
	return strconv.ParseUint(strings.Split(firstLine, " ")[1], 10, 64)
}

func readMemoryStatFile(path string) (uint64, error) {
	t, err := head(path, 1)
	if err != nil {
		return 0, err
	}
	if len(t) == 0 {
		return 0, fmt.Errorf(noContentError, path)
	}
	return strconv.ParseUint(t[0], 10, 64)
}

func readUsage(m map[string]StatsParser) (uint64, error) {
	cgroupVersion := detectCgroupVersion()
	sp := m[cgroupVersion]
	return sp.readUsage()
}

// Return the raw usage in microseconds
func ReadCPUUsage() (uint64, error) {
	return readUsage(cpuByCgroupVersion)
}

// Return the RSS memory in bytes
func ReadMemoryUsage() (uint64, error) {
	return readUsage(memByCgroupVersion)
}
