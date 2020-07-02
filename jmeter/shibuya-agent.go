package main

import (
	"fmt"
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

	"github.com/hpcloud/tail"
)

const (
	RESULT_ROOT      = "/test-result"
	TEST_DATA_FOLDER = "/test-data"
	PROPERTY_FILE    = "/test-conf/shibuya.properties"
	JMETER_BIN_FOLER = "/apache-jmeter-3.3/bin"
	JMETER_BIN       = "jmeter"
	STDERR           = "/dev/stderr"
)

var (
	JMETER_EXECUTABLE = path.Join(JMETER_BIN_FOLER, JMETER_BIN)
	JMETER_SHUTDOWN   = path.Join(JMETER_BIN_FOLER, "stoptest.sh")
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
	}

	// Set it running - listening and broadcasting events
	go sw.listen()
	return
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

func (sw *ShibuyaWrapper) runCommand(w http.ResponseWriter, planName string) int {
	log.Printf("shibuya-agent: Start to run plan %s", planName)
	logFile := sw.makeLogFile()
	cmd := exec.Command(JMETER_EXECUTABLE, "-n", "-t", planName, "-l", logFile,
		"-q", PROPERTY_FILE, "-G", PROPERTY_FILE, "-j", STDERR)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	err := cmd.Start()
	if err != nil {
		log.Panic(err)
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

func (sw *ShibuyaWrapper) startHandler(w http.ResponseWriter, r *http.Request) {
	sw.handlerLock.Lock()
	defer sw.handlerLock.Unlock()
	if r.Method == "POST" {
		if sw.getPid() != 0 {
			w.WriteHeader(http.StatusConflict)
			return
		}
		file, _, err := r.FormFile("test-data")
		if err != nil {
			log.Fatal(err)
		}
		defer file.Close()
		fileContents, err := ioutil.ReadAll(file)
		if err != nil {
			log.Fatal(err)
		}
		zipFileName := filepath.Join(TEST_DATA_FOLDER, "all.zip")
		if err := cleanTestData(); err != nil {
			log.Println(err)
			return
		}
		if err := ioutil.WriteFile(zipFileName, fileContents, 0777); err != nil {
			log.Println(err)
			return
		}
		cmd := exec.Command("unzip", "-o", zipFileName, "-d", "/test-data")
		cmd.Run()
		plan := r.Form.Get("plan")
		pid := sw.runCommand(w, plan)

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

func main() {
	sw := NewServer()
	http.HandleFunc("/start", sw.startHandler)
	http.HandleFunc("/stop", sw.stopHandler)
	http.HandleFunc("/stream", sw.streamHandler)
	http.HandleFunc("/progress", sw.progressHandler)
	log.Fatal(http.ListenAndServe(":8080", nil))
}
