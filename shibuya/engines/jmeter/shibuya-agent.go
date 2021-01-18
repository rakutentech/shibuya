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
	"sync"
	"time"

	etree "github.com/beevik/etree"

	sos "github.com/rakutentech/shibuya/shibuya/object_storage"

	controllerModel "github.com/rakutentech/shibuya/shibuya/controller/model"
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
	reader io.ReadCloser
	writer io.Writer
	buffer []byte
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
		panic(err)
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

func (sw *ShibuyaWrapper) runCommand(w http.ResponseWriter) int {
	log.Printf("shibuya-agent: Start to run plan")
	logFile := sw.makeLogFile()
	cmd := exec.Command(JMETER_EXECUTABLE, "-n", "-t", JMX_FILEPATH, "-l", logFile,
		"-q", PROPERTY_FILE, "-G", PROPERTY_FILE, "-j", STDERR)
	cmd.Stderr = sw.writer
	err := cmd.Start()
	if err != nil {
		log.Panic(err)
	}
	pid := cmd.Process.Pid
	sw.setPid(pid)
	go func() {
		cmd.Wait()
		log.Printf("shibuya-agent: Shutdown is f finished, resetting pid to zero")
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

func (sw *ShibuyaWrapper) prepareTestData(edc controllerModel.EngineDataConfig) error {
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
			log.Panicln(err)
			w.WriteHeader(http.StatusBadRequest)
			return
		}
		defer r.Body.Close()
		var edc controllerModel.EngineDataConfig
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
		pid := sw.runCommand(w)

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

func main() {
	sw := NewServer()
	http.HandleFunc("/start", sw.startHandler)
	http.HandleFunc("/stop", sw.stopHandler)
	http.HandleFunc("/stream", sw.streamHandler)
	http.HandleFunc("/progress", sw.progressHandler)
	http.HandleFunc("/output", sw.stdoutHandler)
	log.Fatal(http.ListenAndServe(":8080", nil))
}
