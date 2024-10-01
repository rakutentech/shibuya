package main

import (
	"bufio"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/ioutil"
	"log"
	"net/http"
	"os"
	"os/exec"
	"path"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"

	etree "github.com/beevik/etree"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	_ "go.uber.org/automaxprocs"

	"github.com/rakutentech/shibuya/shibuya/config"
	sos "github.com/rakutentech/shibuya/shibuya/object_storage"

	"github.com/rakutentech/shibuya/shibuya/engines/containerstats"
	enginesModel "github.com/rakutentech/shibuya/shibuya/engines/model"
	"github.com/rakutentech/shibuya/shibuya/model"
	"github.com/rakutentech/shibuya/shibuya/utils"

	"github.com/hpcloud/tail"
)

const (
	RESULT_ROOT      = "/test-result"
	TEST_DATA_FOLDER = "/test-data"
	PROPERTY_FILE    = "/test-conf/shibuya.properties"
	JMETER_BIN_FOLER = "/apache-jmeter-3.3/bin"
	JMETER_BIN       = "jmeter"
	STDERR           = "/dev/stderr"
	JMX_FILENAME     = "modified.jmx"
)

var (
	JMETER_EXECUTABLE = path.Join(JMETER_BIN_FOLER, JMETER_BIN)
	JMETER_SHUTDOWN   = path.Join(JMETER_BIN_FOLER, "stoptest.sh")
	JMX_FILEPATH      = path.Join(TEST_DATA_FOLDER, JMX_FILENAME)
)

type ShibuyaWrapper struct {
	newClients     chan chan string
	closingClients chan chan string
	clients        map[chan string]bool
	closeSignal    chan int
	Bus            chan string
	logCounter     int
	httpClient     *http.Client
	wg             sync.WaitGroup
	pidLock        sync.RWMutex
	handlerLock    sync.RWMutex
	currentPid     int
	storageClient  sos.StorageInterface
	//stderr         io.ReadCloser
	reader       io.ReadCloser
	writer       io.Writer
	buffer       []byte
	runID        int
	collectionID string
	planID       string
	engineID     int
}

func findCollectionIDPlanID() (string, string) {
	return os.Getenv("collection_id"), os.Getenv("plan_id")
}

func NewServer() (sw *ShibuyaWrapper) {
	// Instantiate a broker
	sw = &ShibuyaWrapper{
		newClients:     make(chan chan string),
		closingClients: make(chan chan string),
		clients:        make(map[chan string]bool),
		closeSignal:    make(chan int),
		logCounter:     0,
		Bus:            make(chan string),
		httpClient:     &http.Client{},
		storageClient:  sos.Client.Storage,
	}
	sw.collectionID, sw.planID = findCollectionIDPlanID()
	reader, writer, _ := os.Pipe()
	mw := io.MultiWriter(writer, os.Stderr)
	sw.reader = reader
	sw.writer = mw
	log.SetOutput(mw)
	// Set it running - listening and broadcasting events
	go sw.listen()
	go sw.readOutput()
	return
}

func (sw *ShibuyaWrapper) readOutput() {
	rd := bufio.NewReader(sw.reader)
	for {
		line, _, err := rd.ReadLine()
		if err != nil {
			continue
		}
		line = append(line, '\n')
		sw.buffer = append(sw.buffer, line...)
	}
}

func parseRawMetrics(rawLine string) (enginesModel.ShibuyaMetric, error) {
	line := strings.Split(rawLine, "|")
	// We use char "|" as the separator in jmeter jtl file. If some users somehow put another | in their label name
	// we could end up a broken split. For those requests, we simply ignore otherwise the process will crash.
	// With current jmeter setup, we are expecting 12 items to be presented in the JTL file after split.
	// The column in the JTL files are:
	// timeStamp|elapsed|label|responseCode|responseMessage|threadName|success|bytes|grpThreads|allThreads|Latency|Connect
	if len(line) < 12 {
		log.Printf("line length was less than required. Raw line is %s", rawLine)
		return enginesModel.ShibuyaMetric{}, fmt.Errorf("line length was less than required. Raw line is %s", rawLine)
	}
	label := line[2]
	status := line[3]
	threads, _ := strconv.ParseFloat(line[9], 64)
	latency, err := strconv.ParseFloat(line[10], 64)
	if err != nil {
		return enginesModel.ShibuyaMetric{}, err
	}
	return enginesModel.ShibuyaMetric{
		Threads: threads,
		Label:   label,
		Status:  status,
		Latency: latency,
		Raw:     rawLine,
	}, nil
}

func (sw *ShibuyaWrapper) makePromMetrics(line string) {
	metric, err := parseRawMetrics(line)
	// we need to pass the engine meta(project, collection, plan), especially run id
	// Run id is generated at controller side
	if err != nil {
		return
	}
	collectionID := sw.collectionID
	planID := sw.planID
	engineID := fmt.Sprintf("%d", sw.engineID)
	runID := fmt.Sprintf("%d", sw.runID)

	label := metric.Label
	status := metric.Status
	latency := metric.Latency
	threads := metric.Threads

	config.StatusCounter.WithLabelValues(sw.collectionID, planID, runID, engineID, label, status).Inc()
	config.CollectionLatencySummary.WithLabelValues(collectionID, runID).Observe(latency)
	config.PlanLatencySummary.WithLabelValues(collectionID, planID, runID).Observe(latency)
	config.LabelLatencySummary.WithLabelValues(collectionID, label, runID).Observe(latency)
	config.ThreadsGauge.WithLabelValues(collectionID, planID, runID, engineID).Set(threads)

}

func (sw *ShibuyaWrapper) listen() {
	for {
		select {
		case s := <-sw.newClients:
			// A new client has connected.
			// Register their message channel
			sw.clients[s] = true
			log.Printf("shibuya-agent: Metric subscriber added. %d registered subscribers", len(sw.clients))
		case s := <-sw.closingClients:
			// A client has dettached and we want to
			// stop sending them messages.
			delete(sw.clients, s)
			close(s)
			log.Printf("shibuya-agent: Metric subscriber removed. %d registered subscribers", len(sw.clients))
		case event := <-sw.Bus:
			// We got a new event from the outside!
			// Send event to all connected clients
			sw.makePromMetrics(event)
			for clientMessageChan, _ := range sw.clients {
				clientMessageChan <- event
			}
		}
	}
}

func (sw *ShibuyaWrapper) makeLogFile() string {
	filename := fmt.Sprintf("kpi-%d.jtl", sw.logCounter)
	return path.Join(RESULT_ROOT, filename)
}

func (sw *ShibuyaWrapper) tailJemeter() {
	var t *tail.Tail
	var err error
	logFile := sw.makeLogFile()
	for {
		t, err = tail.TailFile(logFile, tail.Config{MustExist: true, Follow: true, Poll: true})
		if err != nil {
			time.Sleep(time.Second)
			continue
		}
		break
	}
	// It's not thread safe. But we should be ok since we don't perform tests in parallel.
	sw.logCounter += 1
	log.Printf("shibuya-agent: Start tailing JTL file %s", logFile)
	for {
		select {
		case <-sw.closeSignal:
			t.Stop()
			return
		case line := <-t.Lines:
			sw.Bus <- line.Text
		}
	}
}

func (sw *ShibuyaWrapper) streamHandler(w http.ResponseWriter, r *http.Request) {
	messageChan := make(chan string)
	flusher, ok := w.(http.Flusher)
	if !ok {
		http.Error(w, "Streaming unsupported!", http.StatusInternalServerError)
		return

	}
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")
	w.Header().Set("Access-Control-Allow-Origin", "*")

	// Signal the sw that we have a new connection
	sw.newClients <- messageChan
	// Listen to connection close and un-register messageChan
	notify := w.(http.CloseNotifier).CloseNotify()

	go func() {
		<-notify
		sw.closingClients <- messageChan
	}()

	for message := range messageChan {
		if message == "" {
			continue
		}
		fmt.Fprintf(w, "data: %s\n\n", message)
		flusher.Flush()
	}
}

func (sw *ShibuyaWrapper) stopHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		return
	}
	err := r.ParseForm()
	if err != nil {
		log.Println(err)
		return
	}
	pid := sw.getPid()
	if pid == 0 {
		return
	}
	log.Printf("shibuya-agent: Shutting down Jmeter process %d", sw.getPid())
	cmd := exec.Command(JMETER_SHUTDOWN)
	cmd.Run()
	for {
		if sw.getPid() == 0 {
			break
		}
		time.Sleep(time.Second * 2)
	}
	sw.closeSignal <- 1
}

func (sw *ShibuyaWrapper) setPid(pid int) {
	sw.pidLock.Lock()
	defer sw.pidLock.Unlock()

	sw.currentPid = pid
}

func (sw *ShibuyaWrapper) getPid() int {
	sw.pidLock.RLock()
	defer sw.pidLock.RUnlock()

	return sw.currentPid
}

func (sw *ShibuyaWrapper) runCommand() int {
	log.Printf("shibuya-agent: Start to run plan")
	logFile := sw.makeLogFile()
	cmd := exec.Command(JMETER_EXECUTABLE, "-n", "-t", JMX_FILEPATH, "-l", logFile,
		"-q", PROPERTY_FILE, "-G", PROPERTY_FILE, "-j", STDERR)
	cmd.Stderr = sw.writer
	err := cmd.Start()
	if err != nil {
		log.Println(err)
		return 0
	}
	pid := cmd.Process.Pid
	sw.setPid(pid)
	go func() {
		cmd.Wait()
		log.Printf("shibuya-agent: Shutdown is finished, resetting pid to zero")
		sw.setPid(0)
	}()
	return pid
}

func cleanTestData() error {
	if err := os.RemoveAll(TEST_DATA_FOLDER); err != nil {
		return err
	}
	if err := os.MkdirAll(TEST_DATA_FOLDER, os.ModePerm); err != nil {
		return err
	}
	return nil
}

func saveToDisk(filename string, file []byte) error {
	filePath := filepath.Join(TEST_DATA_FOLDER, filepath.Base(filename))
	log.Println(filePath)
	if err := ioutil.WriteFile(filePath, file, 0777); err != nil {
		return err
	}
	return nil
}

func GetThreadGroups(planDoc *etree.Document) ([]*etree.Element, error) {
	jtp := planDoc.SelectElement("jmeterTestPlan")
	if jtp == nil {
		return nil, errors.New("Missing Jmeter Test plan in jmx")
	}
	ht := jtp.SelectElement("hashTree")
	if ht == nil {
		return nil, errors.New("Missing hash tree inside Jmeter test plan in jmx")
	}
	ht = ht.SelectElement("hashTree")
	if ht == nil {
		return nil, errors.New("Missing hash tree inside hash tree in jmx")
	}
	tgs := ht.SelectElements("ThreadGroup")
	stgs := ht.SelectElements("SetupThreadGroup")
	tgs = append(tgs, stgs...)
	return tgs, nil
}

func parseTestPlan(file []byte) (*etree.Document, error) {
	doc := etree.NewDocument()
	if err := doc.ReadFromBytes(file); err != nil {
		return nil, err
	}
	return doc, nil
}

func modifyJMX(file []byte, threads, duration, rampTime string) ([]byte, error) {
	planDoc, err := parseTestPlan(file)
	if err != nil {
		return nil, err
	}
	durationInt, err := strconv.Atoi(duration)
	if err != nil {
		return nil, err
	}
	// it includes threadgroups and setupthreadgroups
	threadGroups, err := GetThreadGroups(planDoc)
	if err != nil {
		return nil, err
	}
	for _, tg := range threadGroups {
		children := tg.ChildElements()
		for _, child := range children {
			attrName := child.SelectAttrValue("name", "")
			switch attrName {
			case "ThreadGroup.duration":
				child.SetText(strconv.Itoa(durationInt * 60))
			case "ThreadGroup.scheduler":
				child.SetText("true")
			case "ThreadGroup.num_threads":
				child.SetText(threads)
			case "ThreadGroup.ramp_time":
				child.SetText(rampTime)
			}
		}
	}
	return planDoc.WriteToBytes()
}

func (sw *ShibuyaWrapper) prepareJMX(sf *model.ShibuyaFile, threads, duration, rampTime string) error {
	file, err := sw.storageClient.Download(sf.Filepath)
	if err != nil {
		log.Println(err)
		return err
	}
	modified, err := modifyJMX(file, threads, duration, rampTime)
	if err != nil {
		return err
	}
	return saveToDisk(JMX_FILENAME, modified)
}

func (sw *ShibuyaWrapper) prepareCSV(sf *model.ShibuyaFile) error {
	file, err := sw.storageClient.Download(sf.Filepath)
	if err != nil {
		return err
	}
	splittedCSV, err := utils.SplitCSV(file, sf.TotalSplits, sf.CurrentSplit)
	if err != nil {
		return err
	}
	return saveToDisk(sf.Filename, splittedCSV)
}

func (sw *ShibuyaWrapper) downloadAndSaveFile(sf *model.ShibuyaFile) error {
	file, err := sw.storageClient.Download(sf.Filepath)
	if err != nil {
		return err
	}
	return saveToDisk(sf.Filename, file)
}

func (sw *ShibuyaWrapper) prepareTestData(edc enginesModel.EngineDataConfig) error {
	for _, sf := range edc.EngineData {
		fileType := filepath.Ext(sf.Filename)
		switch fileType {
		case ".jmx":
			if err := sw.prepareJMX(sf, edc.Concurrency, edc.Duration, edc.Rampup); err != nil {
				return err
			}
		case ".csv":
			if err := sw.prepareCSV(sf); err != nil {
				return err
			}
		default:
			if err := sw.downloadAndSaveFile(sf); err != nil {
				return err
			}
		}
	}
	return nil
}

func (sw *ShibuyaWrapper) startHandler(w http.ResponseWriter, r *http.Request) {
	sw.handlerLock.Lock()
	defer sw.handlerLock.Unlock()

	if r.Method == "POST" {
		if sw.getPid() != 0 {
			w.WriteHeader(http.StatusConflict)
			return
		}
		file, err := ioutil.ReadAll(r.Body)
		if err != nil {
			log.Println(err)
			w.WriteHeader(http.StatusBadRequest)
			return
		}
		defer r.Body.Close()
		var edc enginesModel.EngineDataConfig
		if err := json.Unmarshal(file, &edc); err != nil {
			log.Println(err)
			w.WriteHeader(http.StatusBadRequest)
			return
		}
		if err := cleanTestData(); err != nil {
			log.Println(err)
			w.WriteHeader(http.StatusInternalServerError)
			return
		}
		if err := sw.prepareTestData(edc); err != nil {
			if errors.Is(err, sos.FileNotFoundError()) {
				w.WriteHeader(http.StatusNotFound)
				return
			}
			log.Println(err)
			w.WriteHeader(http.StatusInternalServerError)
			return
		}
		sw.runID = int(edc.RunID)
		sw.engineID = edc.EngineID
		pid := sw.runCommand()
		go sw.tailJemeter()
		log.Printf("shibuya-agent: Start running Jmeter process with pid: %d", pid)
		w.Write([]byte(strconv.Itoa(pid)))
		return
	}
	w.Write([]byte("hmm"))
}

func (sw *ShibuyaWrapper) progressHandler(w http.ResponseWriter, r *http.Request) {
	pid := sw.getPid()
	if pid == 0 {
		w.WriteHeader(http.StatusNotFound)
		return
	}
	w.WriteHeader(http.StatusOK)
}

func (sw *ShibuyaWrapper) stdoutHandler(w http.ResponseWriter, r *http.Request) {
	w.Write(sw.buffer)
}

// This func reports the cpu/memory usage of the engine
// It will run when the engine is started until it's finished.
func (sw *ShibuyaWrapper) reportOwnMetrics(interval time.Duration) error {
	prev := uint64(0)
	engineNumber := strconv.Itoa(sw.engineID)
	for {
		time.Sleep(interval)
		cpuUsage, err := containerstats.ReadCPUUsage()
		if err != nil {
			return err
		}
		if prev == 0 {
			prev = cpuUsage
			continue
		}
		used := (cpuUsage - prev) / uint64(interval.Seconds()) / 1000
		prev = cpuUsage
		memoryUsage, err := containerstats.ReadMemoryUsage()
		if err != nil {
			return err
		}
		config.CpuGauge.WithLabelValues(sw.collectionID,
			sw.planID, engineNumber).Set(float64(used))
		config.MemGauge.WithLabelValues(sw.collectionID,
			sw.planID, engineNumber).Set(float64(memoryUsage))
	}
}

func main() {
	sw := NewServer()
	go func() {
		if err := sw.reportOwnMetrics(5 * time.Second); err != nil {
			// if the engine is having issues with reading stats from cgroup
			// we should fast fail to detect the issue. It could be due to
			// kernel change
			log.Fatal(err)
		}
	}()
	http.HandleFunc("/start", sw.startHandler)
	http.HandleFunc("/stop", sw.stopHandler)
	http.HandleFunc("/stream", sw.streamHandler)
	http.HandleFunc("/progress", sw.progressHandler)
	http.HandleFunc("/output", sw.stdoutHandler)
	http.HandleFunc("/metrics", promhttp.Handler().ServeHTTP)
	log.Fatal(http.ListenAndServe(":8080", nil))
}
