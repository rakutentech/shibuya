package config

import (
	"database/sql"
	"encoding/json"
	"io/ioutil"
	"net/http"
	"net/url"
	"os"
	"path"

	log "github.com/sirupsen/logrus"
	apiv1 "k8s.io/api/core/v1"
)

type LdapConfig struct {
	BaseDN         string `json:"base_dn"`
	SystemUser     string `json:"system_user"`
	SystemPassword string `json:"system_password"`
	LdapServer     string `json:"ldap_server"`
	LdapPort       string `json:"ldap_port"`
}

type AuthConfig struct {
	AdminUsers []string `json:"admin_users"`
	NoAuth     bool     `json:"no_auth"`
	SessionKey string   `json:"session_key"`
	*LdapConfig
}

type ClusterConfig struct {
	Project     string  `json:"project"`
	Zone        string  `json:"zone"`
	ClusterID   string  `json:"cluster_id"`
	Kind        string  `json:"kind"`
	APIEndpoint string  `json:"api_endpoint"`
	NodeCPUSpec int     `json:"node_cpu_spec"`
	OnDemand    bool    `json:"on_demand"`
	GCDuration  float64 `json:"gc_duration"` // in minutes
}

type HostAlias struct {
	Hostname string `json:"hostname"`
	IP       string `json:"IP"`
}

type ExecutorConfig struct {
	InCluster       bool                `json:"in_cluster"`
	Namespace       string              `json:"namespace"`
	Cluster         *ClusterConfig      `json:"cluster"`
	ImagePullSecret string              `json:"pull_secret"`
	ImagePullPolicy apiv1.PullPolicy    `json:"pull_policy"`
	JmeterContainer *JmeterContainer    `json:"jmeter"`
	HostAliases     []*HostAlias        `json:"host_aliases,omitempty"`
	NodeAffinity    []map[string]string `json:"node_affinity"`
}

type ExecutorContainer struct {
	Image string `json:"image"`
	CPU   string `json:"cpu"`
	Mem   string `json:"mem"`
}

type JmeterContainer struct {
	*ExecutorContainer
}

type DashboardConfig struct {
	Url              string `json:"url"`
	RunDashboard     string `json:"run_dashboard"`
	EnginesDashboard string `json:"engine_dashboard"`
}

type HttpConfig struct {
	Proxy string `json:"proxy"`
}

type ObjectStorage struct {
	Provider     string `json:"provider"`
	Url          string `json:"url"`
	User         string `json:"user"`
	Password     string `json:"password"`
	Bucket       string `json:"bucket"`
	RequireProxy bool   `json:"require_proxy"`
}

type LogFormat struct {
	Json     bool   `json:"json"`
	JsonPath string `json:"path"`
}

type IngressConfig struct {
	Image string `json:"image"`
}

var defaultIngressConfig = IngressConfig{
	Image: "quay.io/kubernetes-ingress-controller/nginx-ingress-controller:0.20.0",
}

type ShibuyaConfig struct {
	ProjectHome      string           `json:"project_home"`
	UploadFileHelp   string           `json:"upload_file_help"`
	DBConf           *MySQLConfig     `json:"db"`
	ExecutorConfig   *ExecutorConfig  `json:"executors"`
	DashboardConfig  *DashboardConfig `json:"dashboard"`
	HttpConfig       *HttpConfig      `json:"http_config"`
	AuthConfig       *AuthConfig      `json:"auth_config"`
	ObjectStorage    *ObjectStorage   `json:"object_storage"`
	LogFormat        *LogFormat       `json:"log_format"`
	BackgroundColour string           `json:"bg_color"`
	IngressConfig    *IngressConfig   `json:"ingress"`

	// below are configs generated from above values
	DevMode         bool
	Context         string
	HTTPClient      *http.Client
	HTTPProxyClient *http.Client
	DBC             *sql.DB
	DBEndpoint      string
}

func loadContext() string {
	return os.Getenv("env")
}

func (sc *ShibuyaConfig) makeHTTPClients() {
	sc.HTTPClient = &http.Client{}
	if sc.HttpConfig.Proxy == "" {
		return
	}
	proxyUrl, err := url.Parse(sc.HttpConfig.Proxy)
	if err != nil {
		log.Fatal(err)
	}
	rt := &http.Transport{
		Proxy: http.ProxyURL(proxyUrl),
	}
	sc.HTTPProxyClient = &http.Client{Transport: rt}
}

func applyJsonLogging() {
	log.SetFormatter(&log.JSONFormatter{})
	err := os.MkdirAll(SC.LogFormat.JsonPath, os.ModePerm)
	if err != nil {
		log.Fatal(err)
	}
	file, err := os.OpenFile(path.Join(SC.LogFormat.JsonPath, "shibuya.json"),
		os.O_CREATE|os.O_WRONLY, 0666)
	if err != nil {
		log.Fatalf("Failed to log to file. %v", err)
	}
	log.SetOutput(file)
}

func setupLogging() {
	log.SetOutput(os.Stdout)
	log.SetReportCaller(true)
	if SC.LogFormat.Json {
		applyJsonLogging()
	}
}

func loadConfig() *ShibuyaConfig {
	sc := new(ShibuyaConfig)
	sc.IngressConfig = &defaultIngressConfig
	f, err := os.Open("/config.json")
	if err != nil {
		log.Fatal("Cannot find config file")
	}
	raw, err := ioutil.ReadAll(f)
	if err != nil {
		log.Fatalf("Cannot read json file %v", err)
	}
	if err := json.Unmarshal(raw, sc); err != nil {
		log.Fatalf("Cannot unmarshal json %v", err)
	}
	sc.Context = loadContext()
	sc.DevMode = sc.Context == "local"
	sc.makeHTTPClients()
	if sc.ExecutorConfig != nil && sc.ExecutorConfig.Cluster.GCDuration == 0 {
		sc.ExecutorConfig.Cluster.GCDuration = 15
	}
	return sc
}

var SC *ShibuyaConfig

func init() {
	sc := loadConfig()
	SC = sc
	setupLogging()
	if sc.DBConf != nil {
		sc.DBC = createMySQLClient(sc.DBConf)
		sc.DBEndpoint = sc.DBConf.Endpoint
	}
}
